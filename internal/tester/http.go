package tester

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"cc-switch-cli/internal/model"
)

// doMessages issues a POST /v1/messages request against the provider. It
// returns the response so the caller can stream it. Callers own resp.Body.
func (c *Client) doMessages(ctx context.Context, m *model.Model, body any, forceAuth AuthMode) (*http.Response, error) {
	base := m.BaseURL()
	if base == "" {
		return nil, errors.New("provider has no ANTHROPIC_BASE_URL configured")
	}
	// The Anthropic path is /v1/messages. Some proxies already include /v1 in
	// the base URL: only append the path components that are still missing.
	url := joinURL(base, "/v1/messages")
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to build request: %w", err)
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("accept", "text/event-stream")
	req.Header.Set("anthropic-version", "2023-06-01")
	req.Header.Set("user-agent", "cc-switch-cli/tester")

	token := m.Token()
	mode := forceAuth
	if mode == "" {
		mode = c.opts.Auth
	}
	if mode == AuthAuto {
		mode = AuthBearer
	}
	if token != "" {
		switch mode {
		case AuthAPIKey:
			req.Header.Set("x-api-key", token)
		default:
			req.Header.Set("authorization", "Bearer "+token)
		}
	}

	client := &http.Client{Timeout: c.opts.Timeout}
	return client.Do(req)
}

// callAndStream is a helper that runs a single stream over the SSE parser.
// It returns (ttft, status, error) and handles auth fallback on 401/403: when
// auto mode retries with x-api-key, the returned error describes only the last
// attempt.
func (c *Client) callAndStream(ctx context.Context, m *model.Model, body any, onEvent func(ev sseEvent) (done bool, err error)) (int, time.Duration, error) {
	start := time.Now()
	resp, err := c.doMessages(ctx, m, body, "")
	// Auth fallback: only if auto mode and no bytes have been consumed.
	if c.opts.Auth == AuthAuto && err == nil && (resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden) && m.Token() != "" {
		_ = resp.Body.Close()
		resp, err = c.doMessages(ctx, m, body, AuthAPIKey)
	}
	if err != nil {
		return 0, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		raw := readErrorBody(resp.Body)
		return resp.StatusCode, 0, &ProbeError{
			HTTPStatus: resp.StatusCode,
			APIType:    extractErrType(raw),
			Message:    fmt.Sprintf("http %d: %s", resp.StatusCode, extractErrMsg(raw)),
		}
	}

	first := time.Time{}
	err = readSSE(ctx, resp.Body, func(ev sseEvent) (bool, error) {
		if first.IsZero() {
			first = time.Now()
		}
		return onEvent(ev)
	})
	if err != nil {
		return resp.StatusCode, 0, err
	}
	if first.IsZero() {
		return resp.StatusCode, 0, errors.New("no events received")
	}
	return resp.StatusCode, first.Sub(start), nil
}

// joinURL appends path to base without introducing double slashes or
// duplicating /v1 when base already ends with it.
func joinURL(base, path string) string {
	base = strings.TrimRight(base, "/")
	if strings.HasSuffix(base, "/v1") && strings.HasPrefix(path, "/v1") {
		path = strings.TrimPrefix(path, "/v1")
	}
	return base + path
}

// extractErrMsg tries to peel a human-readable message out of a provider error
// body. It understands the plain Anthropic JSON shape, a top-level "message"
// field, and SSE bodies where the error is delivered as `event:error\ndata:{…}`
// (some Anthropic-compatible gateways return errors this way inside HTTP 400).
// Falls back to the (clipped) raw body when nothing parses.
func extractErrMsg(body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return "(empty body)"
	}
	if msg := jsonErrMsg(body); msg != "" {
		return clipMsg(msg)
	}
	for _, payload := range sseDataPayloads(body) {
		if msg := jsonErrMsg(payload); msg != "" {
			return clipMsg(msg)
		}
	}
	return clipMsg(body)
}

// extractErrType mirrors extractErrMsg but pulls the Anthropic error.type out
// of the same three shapes: {"error":{"type":…}}, a top-level {"type":…} inside
// an error event, and SSE-wrapped data lines. Returns "" when absent.
func extractErrType(body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return ""
	}
	if typ := jsonErrType(body); typ != "" {
		return typ
	}
	for _, payload := range sseDataPayloads(body) {
		if typ := jsonErrType(payload); typ != "" {
			return typ
		}
	}
	return ""
}

// sseDataPayloads returns the JSON payloads embedded in `data:` lines, if any.
func sseDataPayloads(body string) []string {
	if !strings.Contains(body, "data:") {
		return nil
	}
	var payloads []string
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload != "" && payload != "[DONE]" {
			payloads = append(payloads, payload)
		}
	}
	return payloads
}

// jsonErrMsg extracts a message from a single JSON object, accepting either
// {"error":{"message":…}} or a top-level {"message":…}.
func jsonErrMsg(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || s[0] != '{' {
		return ""
	}
	var wrap struct {
		Message string `json:"message"`
		Error   struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(s), &wrap); err != nil {
		return ""
	}
	if wrap.Error.Message != "" {
		return wrap.Error.Message
	}
	return wrap.Message
}

// jsonErrType extracts an error type from a single JSON object, accepting the
// wire shape {"type":"error","error":{"type":…}} or a bare {"type":…}.
func jsonErrType(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || s[0] != '{' {
		return ""
	}
	var wrap struct {
		Type  string `json:"type"`
		Error struct {
			Type string `json:"type"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(s), &wrap); err != nil {
		return ""
	}
	if wrap.Error.Type != "" {
		return wrap.Error.Type
	}
	// A top-level {"type":"error"} only marks an SSE envelope, not the cause;
	// keep looking for the nested value instead of reporting it as one.
	if wrap.Type == "error" {
		return ""
	}
	return wrap.Type
}

// clipMsg clips a provider error message to a fixed rune budget so it stays
// valid UTF-8 even when the source text is CJK or otherwise multi-byte.
func clipMsg(s string) string {
	r := []rune(s)
	if len(r) > 500 {
		return string(r[:500]) + "…"
	}
	return s
}

// readAllString reads r fully into a string. Kept next to extractErrMsg for
// symmetry; not currently used elsewhere.
func readAllString(r io.Reader) string {
	b, _ := io.ReadAll(r)
	return string(b)
}
