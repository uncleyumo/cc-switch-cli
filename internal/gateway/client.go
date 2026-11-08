// Package gateway talks to a new-api compatible endpoint: it lists models and
// reads pricing, and caches both on disk keyed by host AND token hash.
//
// Keying by token matters: the same new-api instance shows different models to
// different keys, so a host-only cache would silently hand one key's model list
// to another.
package gateway

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/uncleyumo/ccsc/internal/group"
	"github.com/uncleyumo/ccsc/internal/store"
)

// Model is one row of GET /v1/models.
type Model struct {
	ID            string   `json:"id"`
	Object        string   `json:"object,omitempty"`
	Created       int64    `json:"created,omitempty"`
	OwnedBy       string   `json:"owned_by,omitempty"`
	EndpointTypes []string `json:"supported_endpoint_types,omitempty"`
}

// SupportsAnthropic reports whether this model can be driven over the
// Anthropic wire format.
func (m Model) SupportsAnthropic() bool {
	for _, t := range m.EndpointTypes {
		if strings.EqualFold(t, "anthropic") {
			return true
		}
	}
	// An endpoint that does not advertise endpoint types at all: fall back to
	// the naming convention rather than hiding every model.
	if len(m.EndpointTypes) == 0 {
		return strings.HasPrefix(strings.ToLower(m.ID), "claude")
	}
	return false
}

// Pricing is one row of GET /api/pricing.
type Pricing struct {
	ModelName       string   `json:"model_name"`
	ModelRatio      float64  `json:"model_ratio"`
	CompletionRatio float64  `json:"completion_ratio"`
	CacheRatio      float64  `json:"cache_ratio"`
	EnableGroups    []string `json:"enable_groups,omitempty"`
}

// Snapshot is everything ccsc caches about one endpoint+token pair.
type Snapshot struct {
	BaseURL   string             `json:"base_url"`
	FetchedAt time.Time          `json:"fetched_at"`
	Models    []Model            `json:"models"`
	Pricing   map[string]Pricing `json:"pricing,omitempty"`
}

// AnthropicModels returns the model ids usable through the Anthropic format.
func (s *Snapshot) AnthropicModels() []Model {
	var out []Model
	for _, m := range s.Models {
		if m.SupportsAnthropic() {
			out = append(out, m)
		}
	}
	return out
}

// Client talks to one endpoint.
type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

// New builds a client. baseURL is normalised so a trailing slash never doubles.
func New(baseURL, token string) *Client {
	return &Client{
		BaseURL: group.NormalizeBaseURL(baseURL),
		Token:   token,
		HTTP:    &http.Client{Timeout: 20 * time.Second},
	}
}

// Fetch pulls the model list and, best effort, the pricing table.
func (c *Client) Fetch(ctx context.Context) (*Snapshot, error) {
	models, err := c.Models(ctx)
	if err != nil {
		return nil, err
	}
	snap := &Snapshot{BaseURL: c.BaseURL, FetchedAt: time.Now(), Models: models}
	if pricing, err := c.Pricing(ctx); err == nil {
		snap.Pricing = pricing
	}
	return snap, nil
}

// Models calls GET /v1/models.
func (c *Client) Models(ctx context.Context) ([]Model, error) {
	var out struct {
		Data []Model `json:"data"`
	}
	if err := c.getJSON(ctx, "/v1/models?limit=1000", true, &out); err != nil {
		return nil, err
	}
	if len(out.Data) == 0 {
		return nil, fmt.Errorf("%s returned no models", c.BaseURL)
	}
	sort.Slice(out.Data, func(i, j int) bool { return out.Data[i].ID < out.Data[j].ID })
	return out.Data, nil
}

// Pricing calls GET /api/pricing, which new-api serves without authentication.
func (c *Client) Pricing(ctx context.Context) (map[string]Pricing, error) {
	var out struct {
		Data []Pricing `json:"data"`
	}
	if err := c.getJSON(ctx, "/api/pricing", false, &out); err != nil {
		return nil, err
	}
	m := make(map[string]Pricing, len(out.Data))
	for _, p := range out.Data {
		m[p.ModelName] = p
	}
	return m, nil
}

func (c *Client) getJSON(ctx context.Context, path string, auth bool, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return fmt.Errorf("cannot build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if auth && c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("cannot reach %s: %w", c.BaseURL, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return fmt.Errorf("cannot read the response from %s: %w", c.BaseURL, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("%s%s returned HTTP %d: %s", c.BaseURL, path, resp.StatusCode, snippet(body))
	}
	if err := json.Unmarshal(body, dst); err != nil {
		return fmt.Errorf("%s%s did not return the expected JSON: %w", c.BaseURL, path, err)
	}
	return nil
}

func snippet(b []byte) string {
	s := strings.Join(strings.Fields(string(b)), " ")
	if len(s) > 200 {
		s = s[:200] + "..."
	}
	return s
}

// CacheKey is the file stem for a snapshot: host plus a short token hash.
func CacheKey(baseURL, token string) string {
	host := baseURL
	if u, err := url.Parse(baseURL); err == nil && u.Host != "" {
		host = u.Host
	}
	sum := sha256Sum(token)
	r := strings.NewReplacer(":", "_", "/", "_", "\\", "_")
	host = r.Replace(host)
	return host + "-" + hex.EncodeToString(sum)[:12]
}

// CachePath is where the snapshot for this endpoint+token lives.
func CachePath(dir, baseURL, token string) string {
	return filepath.Join(dir, CacheKey(baseURL, token)+".json")
}

// LoadCache returns the cached snapshot when it is younger than maxAge.
// maxAge <= 0 means "never expire".
func LoadCache(dir, baseURL, token string, maxAge time.Duration) (*Snapshot, error) {
	path := CachePath(dir, baseURL, token)
	data, err := store.ReadFileOrEmpty(path)
	if err != nil {
		return nil, err
	}
	if data == nil {
		return nil, os.ErrNotExist
	}
	var snap Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, fmt.Errorf("%s is corrupt: %w", path, err)
	}
	if maxAge > 0 && time.Since(snap.FetchedAt) > maxAge {
		return nil, os.ErrNotExist
	}
	return &snap, nil
}

// SaveCache writes the snapshot atomically.
func SaveCache(dir string, snap *Snapshot, token string) error {
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return store.AtomicWrite(CachePath(dir, snap.BaseURL, token), data, 0o600)
}
