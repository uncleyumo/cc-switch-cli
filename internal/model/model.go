// Package model defines the typed provider config used across cc-switch-cli.
//
// Provider files under ~/.cc-switch-cli/models/ are complete Claude Code
// settings.json documents. Model exposes the fields cc-switch-cli reads and
// writes as typed values, while preserving every other top-level key and
// unknown env variable as json.RawMessage so round-tripping is lossless.
package model

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Known Anthropic env keys handled by the typed Env struct.
const (
	EnvAuthToken          = "ANTHROPIC_AUTH_TOKEN"
	EnvAPIKey             = "ANTHROPIC_API_KEY"
	EnvBaseURL            = "ANTHROPIC_BASE_URL"
	EnvModel              = "ANTHROPIC_MODEL"
	EnvDefaultFableModel  = "ANTHROPIC_DEFAULT_FABLE_MODEL"
	EnvDefaultHaikuModel  = "ANTHROPIC_DEFAULT_HAIKU_MODEL"
	EnvDefaultOpusModel   = "ANTHROPIC_DEFAULT_OPUS_MODEL"
	EnvDefaultSonnetModel = "ANTHROPIC_DEFAULT_SONNET_MODEL"

	// Context-window env keys Claude Code reads for unrecognized / gateway
	// model IDs. MAX_CONTEXT declares the real window; AUTO_COMPACT_WINDOW
	// is the documented compact threshold; AUTO_COMPACT_WINDOW_TOKENS is
	// written alongside it because some Claude Code builds surface that name.
	EnvMaxContextTokens        = "CLAUDE_CODE_MAX_CONTEXT_TOKENS"
	EnvAutoCompactWindow       = "CLAUDE_CODE_AUTO_COMPACT_WINDOW"
	EnvAutoCompactWindowTokens = "CLAUDE_CODE_AUTO_COMPACT_WINDOW_TOKENS"
)

// ModelScopedEnvKeys are per-provider env values that must not leak across
// `ccsc local` merges. If the incoming provider does not set a key, the
// existing project file must drop it rather than keep the previous model's
// window.
var ModelScopedEnvKeys = []string{
	EnvMaxContextTokens,
	EnvAutoCompactWindow,
	EnvAutoCompactWindowTokens,
}

// Env holds the Anthropic environment variables that cc-switch-cli understands
// natively. Unknown env values round-trip through Extra verbatim.
type Env struct {
	AuthToken          string
	APIKey             string
	BaseURL            string
	Model              string
	DefaultFableModel  string
	DefaultHaikuModel  string
	DefaultOpusModel   string
	DefaultSonnetModel string

	MaxContextTokens        string
	AutoCompactWindow       string
	AutoCompactWindowTokens string

	// Extra preserves any env keys we don't map to a typed field.
	Extra map[string]json.RawMessage
}

// MarshalJSON emits the env object with typed keys first (in a stable order)
// followed by every Extra key sorted alphabetically. Empty typed values are
// omitted so unset fields don't clutter provider files.
func (e Env) MarshalJSON() ([]byte, error) {
	ordered := []struct {
		key string
		val string
	}{
		{EnvAuthToken, e.AuthToken},
		{EnvAPIKey, e.APIKey},
		{EnvBaseURL, e.BaseURL},
		{EnvModel, e.Model},
		{EnvDefaultFableModel, e.DefaultFableModel},
		{EnvDefaultHaikuModel, e.DefaultHaikuModel},
		{EnvDefaultOpusModel, e.DefaultOpusModel},
		{EnvDefaultSonnetModel, e.DefaultSonnetModel},
		{EnvMaxContextTokens, e.MaxContextTokens},
		{EnvAutoCompactWindow, e.AutoCompactWindow},
		{EnvAutoCompactWindowTokens, e.AutoCompactWindowTokens},
	}

	buf := bytes.NewBufferString("{")
	first := true
	writePair := func(key string, valJSON []byte) error {
		if !first {
			buf.WriteByte(',')
		}
		first = false
		keyJSON, err := json.Marshal(key)
		if err != nil {
			return err
		}
		buf.Write(keyJSON)
		buf.WriteByte(':')
		buf.Write(valJSON)
		return nil
	}
	for _, kv := range ordered {
		if kv.val == "" {
			continue
		}
		valJSON, err := json.Marshal(kv.val)
		if err != nil {
			return nil, err
		}
		if err := writePair(kv.key, valJSON); err != nil {
			return nil, err
		}
	}

	// Extra keys sorted for deterministic output.
	if len(e.Extra) > 0 {
		keys := make([]string, 0, len(e.Extra))
		for k := range e.Extra {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if err := writePair(k, e.Extra[k]); err != nil {
				return nil, err
			}
		}
	}

	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// UnmarshalJSON accepts a flat object of env keys. Values may be any JSON type;
// typed keys are only accepted as strings.
func (e *Env) UnmarshalJSON(data []byte) error {
	raw := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("env must be a JSON object: %w", err)
	}
	*e = Env{}
	typedString := func(key string, dst *string) error {
		if v, ok := raw[key]; ok {
			var s string
			if err := json.Unmarshal(v, &s); err != nil {
				return fmt.Errorf("env.%s must be a string: %w", key, err)
			}
			*dst = s
			delete(raw, key)
		}
		return nil
	}
	if err := typedString(EnvAuthToken, &e.AuthToken); err != nil {
		return err
	}
	if err := typedString(EnvAPIKey, &e.APIKey); err != nil {
		return err
	}
	if err := typedString(EnvBaseURL, &e.BaseURL); err != nil {
		return err
	}
	if err := typedString(EnvModel, &e.Model); err != nil {
		return err
	}
	if err := typedString(EnvDefaultFableModel, &e.DefaultFableModel); err != nil {
		return err
	}
	if err := typedString(EnvDefaultHaikuModel, &e.DefaultHaikuModel); err != nil {
		return err
	}
	if err := typedString(EnvDefaultOpusModel, &e.DefaultOpusModel); err != nil {
		return err
	}
	if err := typedString(EnvDefaultSonnetModel, &e.DefaultSonnetModel); err != nil {
		return err
	}
	if err := typedString(EnvMaxContextTokens, &e.MaxContextTokens); err != nil {
		return err
	}
	if err := typedString(EnvAutoCompactWindow, &e.AutoCompactWindow); err != nil {
		return err
	}
	if err := typedString(EnvAutoCompactWindowTokens, &e.AutoCompactWindowTokens); err != nil {
		return err
	}
	if len(raw) > 0 {
		e.Extra = raw
	}
	return nil
}

// Model represents one provider config on disk.
type Model struct {
	// Name is the provider name (basename of the JSON file). Not serialized.
	Name string `json:"-"`
	// Env is the parsed env sub-object.
	Env Env
	// Rest preserves every top-level key that is not "env". Populated on load
	// and re-emitted on marshal so unknown Claude Code settings survive edits.
	Rest map[string]json.RawMessage
}

// MarshalJSON emits a Claude Code settings.json shape: known top-level keys
// (like "model") come from Rest verbatim, and "env" is the marshaled Env.
func (m Model) MarshalJSON() ([]byte, error) {
	out := map[string]json.RawMessage{}
	for k, v := range m.Rest {
		if k == "env" {
			continue
		}
		out[k] = v
	}
	envJSON, err := json.Marshal(m.Env)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal env: %w", err)
	}
	out["env"] = envJSON
	return json.Marshal(out)
}

// UnmarshalJSON splits "env" (typed) from the rest of the document.
func (m *Model) UnmarshalJSON(data []byte) error {
	raw := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("provider config must be a JSON object: %w", err)
	}
	*m = Model{Rest: map[string]json.RawMessage{}}
	if envRaw, ok := raw["env"]; ok {
		if err := json.Unmarshal(envRaw, &m.Env); err != nil {
			return err
		}
		delete(raw, "env")
	}
	for k, v := range raw {
		m.Rest[k] = v
	}
	return nil
}

// TopLevelModel returns the value of the top-level "model" key ("" if unset).
func (m *Model) TopLevelModel() string {
	if m == nil {
		return ""
	}
	if v, ok := m.Rest["model"]; ok {
		var s string
		if err := json.Unmarshal(v, &s); err == nil {
			return s
		}
	}
	return ""
}

// ModelID picks the primary model identifier for this provider.
// Order of preference: ANTHROPIC_MODEL, ANTHROPIC_DEFAULT_SONNET_MODEL,
// ANTHROPIC_DEFAULT_OPUS_MODEL, ANTHROPIC_DEFAULT_HAIKU_MODEL,
// ANTHROPIC_DEFAULT_FABLE_MODEL.
func (m *Model) ModelID() string {
	if m == nil {
		return ""
	}
	for _, s := range []string{
		m.Env.Model,
		m.Env.DefaultSonnetModel,
		m.Env.DefaultOpusModel,
		m.Env.DefaultHaikuModel,
		m.Env.DefaultFableModel,
	} {
		if s != "" {
			return s
		}
	}
	return ""
}

// ModelTarget is one configured model inside a provider config. A provider can
// carry several slots (the default model plus the per-tier defaults), and each
// slot may point at a different model.
type ModelTarget struct {
	// Slot names the config slot(s) that point at this model. When several
	// slots resolve to the same wire id they are joined with "+", e.g.
	// "sonnet+haiku", so the config's intent stays visible while the model is
	// still tested only once.
	Slot string
	// ID is the resolved model id as stored in the config, including any
	// trailing context-window marker such as "[1m]".
	ID string
	// WireID is the id actually sent in the request body (marker stripped).
	WireID string
}

// Targets returns every distinct configured model across the provider's slots,
// in a stable order: default, top-level, fable, opus, sonnet, haiku. Tier
// aliases are resolved through the matching ANTHROPIC_DEFAULT_*_MODEL value.
// Slots that resolve to the same wire id are tested once, but every slot name
// is kept in Slot (joined with "+") so merged entries still show where they
// came from.
func (m *Model) Targets() []ModelTarget {
	if m == nil {
		return nil
	}
	type slot struct {
		name  string
		value string
	}
	slots := []slot{
		{"default", m.Env.Model},
		{"top-level", m.TopLevelModel()},
		{"fable", m.Env.DefaultFableModel},
		{"opus", m.Env.DefaultOpusModel},
		{"sonnet", m.Env.DefaultSonnetModel},
		{"haiku", m.Env.DefaultHaikuModel},
	}
	index := map[string]int{}
	targets := make([]ModelTarget, 0, len(slots))
	for _, s := range slots {
		v := SanitizeModelID(s.value)
		if v == "" {
			continue
		}
		if _, isTier := tierAliases[strings.ToLower(v)]; isTier {
			if resolved := SanitizeModelID(m.defaultForTier(v)); resolved != "" {
				v = resolved
			}
		}
		wire := WireModelID(v)
		if wire == "" {
			continue
		}
		if i, seen := index[wire]; seen {
			targets[i].Slot += "+" + s.name
			continue
		}
		index[wire] = len(targets)
		targets = append(targets, ModelTarget{Slot: s.name, ID: v, WireID: wire})
	}
	return targets
}

// Token returns the effective authentication token, preferring
// ANTHROPIC_AUTH_TOKEN then ANTHROPIC_API_KEY.
func (m *Model) Token() string {
	if m == nil {
		return ""
	}
	if m.Env.AuthToken != "" {
		return m.Env.AuthToken
	}
	return m.Env.APIKey
}

// BaseURL returns the provider base URL with any trailing slashes trimmed.
func (m *Model) BaseURL() string {
	if m == nil {
		return ""
	}
	return strings.TrimRight(m.Env.BaseURL, "/")
}

// Indent renders the model as pretty-printed JSON with two-space indent and
// a trailing newline (matches the shipped sample provider files).
func (m *Model) Indent() ([]byte, error) {
	data, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, data, "", "  "); err != nil {
		return nil, err
	}
	buf.WriteByte('\n')
	return buf.Bytes(), nil
}

// Parse builds a Model from raw JSON. The Name field is left blank.
func Parse(data []byte) (*Model, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, errors.New("empty provider config")
	}
	var m Model
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// ANSI / terminal-style artifacts sometimes end up pasted into model-id fields
// (for example a bold-on code whose ESC byte was stripped, leaving "[1m]"). A
// real model id never contains '[', so these patterns are safe to remove from
// model-id values specifically.
var (
	// Proper ANSI CSI sequence: ESC [ <params> <intermediates> <final byte>.
	reANSIcsi = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)
	// Other single-char ESC sequences (Fe escapes).
	reANSIesc = regexp.MustCompile(`\x1b[@-Z\\-_]`)
	// OSC sequence ESC ] ... (BEL or ST terminated).
	reANSIosc = regexp.MustCompile(`\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)`)
	// Context-window marker such as [1m], [200k] or [128k] appended to a model
	// id. In Claude Code configs this annotation selects the larger context
	// window for the UI/budget, but it is NOT part of the model name sent over
	// the wire: Claude Code strips it before calling the API, and proxies
	// reject it if it leaks into the request body. We mirror that by stripping
	// it only on the wire (see WireModelID). It is preserved verbatim in the
	// stored config, so a plain SanitizeModelID must leave it alone.
	reContextWindow = regexp.MustCompile(`\[\d+[kKmM]\]\s*$`)
	re1mMarker      = regexp.MustCompile(`(?i)\[1m\]\s*$`)
)

// SanitizeModelID strips only genuine ANSI escape sequences (real ESC bytes)
// from a model identifier and trims surrounding whitespace. It intentionally
// preserves context-window markers such as "[1m]": those are a deliberate
// config annotation for the 1M context window, not corruption, and must stay in
// the stored file. Use WireModelID to obtain the value that should actually be
// sent to a provider.
func SanitizeModelID(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	s = reANSIcsi.ReplaceAllString(s, "")
	s = reANSIosc.ReplaceAllString(s, "")
	s = reANSIesc.ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}

// WireModelID returns the model id with its trailing context-window marker
// (e.g. "[1m]", "[200k]") removed — the form that must be placed in the
// Anthropic request body. Proxies accept the clean name and reject the marker,
// so the tester uses this when building requests while the config keeps the
// marker for Claude Code's own context-window handling.
func WireModelID(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	return strings.TrimSpace(reContextWindow.ReplaceAllString(s, ""))
}

// HasContextMarker reports whether s ends with a trailing [1m]/[200k]-style
// annotation (stripped on the wire).
func HasContextMarker(s string) bool {
	return reContextWindow.FindString(strings.TrimSpace(s)) != ""
}

// Has1MMarker reports whether s ends with [1m]. On an unrecognized model id
// Claude Code assumes a 1M window and ignores CLAUDE_CODE_MAX_CONTEXT_TOKENS.
func Has1MMarker(s string) bool {
	return re1mMarker.MatchString(strings.TrimSpace(s))
}

const (
	minContextWindowTokens = 128_000
	maxContextWindowTokens = 2_000_000
	maxAutoCompactTokens   = 1_000_000
)

// ParseContextWindow accepts the editor/CLI form only: empty, or an integer
// plus a unit (`128k`, `500k`, `1m`, `2m`). Bare numbers such as `500` or
// `500000` are rejected so the typed field and the on-screen value stay in
// the same vocabulary.
func ParseContextWindow(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	n, err := parseWindowWithUnit(s)
	if err != nil {
		return 0, err
	}
	return validateContextWindow(n, s)
}

// parseStoredWindow reads a value that already lives in a provider file.
// Disk always stores a plain integer for Claude Code; older files or a raw
// JSON edit may still use the Nk/Nm spelling.
func parseStoredWindow(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	if n, err := parseWindowWithUnit(s); err == nil {
		return validateContextWindow(n, s)
	}
	if !isAllDigits(s) {
		return 0, fmt.Errorf("invalid context window %q (use 128k, 500k, 1m, 2m)", s)
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("invalid context window %q", s)
	}
	return validateContextWindow(n, s)
}

func parseWindowWithUnit(s string) (int, error) {
	compact := strings.ReplaceAll(s, "_", "")
	compact = strings.ReplaceAll(compact, ",", "")
	compact = strings.TrimSpace(compact)
	if compact == "" {
		return 0, fmt.Errorf("invalid context window %q (use 128k, 500k, 1m, 2m)", s)
	}
	last := compact[len(compact)-1]
	if last != 'k' && last != 'K' && last != 'm' && last != 'M' {
		return 0, fmt.Errorf("invalid context window %q (use 128k, 500k, 1m, 2m)", s)
	}
	num := strings.TrimSpace(compact[:len(compact)-1])
	if num == "" || !isAllDigits(num) {
		return 0, fmt.Errorf("invalid context window %q (use 128k, 500k, 1m, 2m)", s)
	}
	n, err := strconv.Atoi(num)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("invalid context window %q", s)
	}
	if last == 'm' || last == 'M' {
		return n * 1_000_000, nil
	}
	return n * 1_000, nil
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func validateContextWindow(n int, raw string) (int, error) {
	if n < minContextWindowTokens || n > maxContextWindowTokens {
		return 0, fmt.Errorf("context window %q is out of range (128k-2m)", raw)
	}
	return n, nil
}

// FormatContextWindow renders a token count as 500k / 1m / 2m. Values that
// divide evenly by 1M use the m suffix; everything else uses k.
func FormatContextWindow(n int) string {
	if n <= 0 {
		return ""
	}
	if n%1_000_000 == 0 {
		return fmt.Sprintf("%dm", n/1_000_000)
	}
	if n%1000 == 0 {
		return fmt.Sprintf("%dk", n/1000)
	}
	return fmt.Sprintf("%dk", (n+999)/1000)
}

// ContextWindowTokens returns the declared group window from MAX_CONTEXT,
// falling back to AUTO_COMPACT_WINDOW / AUTO_COMPACT_WINDOW_TOKENS. 0 means unset.
func (e Env) ContextWindowTokens() int {
	for _, s := range []string{e.MaxContextTokens, e.AutoCompactWindow, e.AutoCompactWindowTokens} {
		if n, err := parseStoredWindow(s); err == nil && n > 0 {
			return n
		}
	}
	return 0
}

// NormalizeContextWindow rewrites any declared window keys to the integer
// form Claude Code's env parser accepts. "500k" becomes "500000". Compact
// keys are omitted when any model id in the same provider carries a
// trailing [1m]/[Nk] marker, so a per-id 1M override is not compacted at
// the group 500k threshold.
func (m *Model) NormalizeContextWindow() {
	if m == nil {
		return
	}
	n := m.Env.ContextWindowTokens()
	m.ApplyContextWindow(n)
}

// ApplyContextWindow writes CLAUDE_CODE_MAX_CONTEXT_TOKENS as a plain token
// count. AUTO_COMPACT_WINDOW is written only when n is within Claude Code's
// compact range and no model id on this provider has a [1m] suffix.
// n<=0 clears all three keys.
func (m *Model) ApplyContextWindow(n int) {
	if m == nil {
		return
	}
	e := &m.Env
	if n <= 0 {
		e.MaxContextTokens = ""
		e.AutoCompactWindow = ""
		e.AutoCompactWindowTokens = ""
		return
	}
	e.MaxContextTokens = strconv.Itoa(n)
	if n <= maxAutoCompactTokens && !m.HasAny1MMarker() {
		s := strconv.Itoa(n)
		e.AutoCompactWindow = s
		e.AutoCompactWindowTokens = s
		return
	}
	e.AutoCompactWindow = ""
	e.AutoCompactWindowTokens = ""
}

// HasAny1MMarker reports whether any model-id field on this provider ends
// with [1m]. Other [Nk] suffixes are stripped on the wire but do not change
// the window Claude Code assumes for an unrecognized id.
func (m *Model) HasAny1MMarker() bool {
	if m == nil {
		return false
	}
	for _, s := range []string{
		m.Env.Model,
		m.Env.DefaultFableModel,
		m.Env.DefaultHaikuModel,
		m.Env.DefaultOpusModel,
		m.Env.DefaultSonnetModel,
		m.TopLevelModel(),
	} {
		if Has1MMarker(s) {
			return true
		}
	}
	return false
}

// LooksLikeClaudeModel reports whether the resolved id is one Claude Code
// treats as a Claude model (so MAX_CONTEXT alone does not change the window).
func LooksLikeClaudeModel(id string) bool {
	s := strings.ToLower(WireModelID(id))
	if s == "" {
		return false
	}
	if _, isTier := tierAliases[s]; isTier {
		return true
	}
	if strings.HasPrefix(s, "claude-") || strings.HasPrefix(s, "anthropic.claude") {
		return true
	}
	return strings.Contains(s, "claude-")
}

// tierAliases are the Claude Code model tier names that must be resolved to a
// concrete model via the matching ANTHROPIC_DEFAULT_*_MODEL value.
var tierAliases = map[string]struct{}{
	"fable":  {},
	"opus":   {},
	"sonnet": {},
	"haiku":  {},
}

// defaultForTier returns the configured default model for a tier alias.
func (m *Model) defaultForTier(tier string) string {
	switch strings.ToLower(tier) {
	case "fable":
		return m.Env.DefaultFableModel
	case "opus":
		return m.Env.DefaultOpusModel
	case "sonnet":
		return m.Env.DefaultSonnetModel
	case "haiku":
		return m.Env.DefaultHaikuModel
	}
	return ""
}

// ResolvedModelID returns the concrete model id cc-switch-cli should actually
// send to the provider, mirroring how Claude Code resolves the effective model:
//
//  1. ANTHROPIC_MODEL — used directly when it is a concrete name; when it is a
//     tier alias (opus/sonnet/haiku) it is resolved to ANTHROPIC_DEFAULT_*_MODEL.
//  2. The top-level "model" field, resolved the same way.
//  3. A fallback through the sonnet, opus then haiku defaults.
//
// Every candidate is passed through SanitizeModelID so terminal artifacts in the
// stored config cannot produce a bogus model name. Returns "" when nothing
// usable is configured.
func (m *Model) ResolvedModelID() string {
	if m == nil {
		return ""
	}
	resolve := func(v string) string {
		v = SanitizeModelID(v)
		if v == "" {
			return ""
		}
		if _, isTier := tierAliases[strings.ToLower(v)]; isTier {
			return SanitizeModelID(m.defaultForTier(v))
		}
		return v
	}
	if r := resolve(m.Env.Model); r != "" {
		return r
	}
	if r := resolve(m.TopLevelModel()); r != "" {
		return r
	}
	for _, v := range []string{m.Env.DefaultSonnetModel, m.Env.DefaultOpusModel, m.Env.DefaultHaikuModel, m.Env.DefaultFableModel} {
		if s := SanitizeModelID(v); s != "" {
			return s
		}
	}
	return ""
}

// WireModelID is ResolvedModelID with the trailing context-window marker (e.g.
// "[1m]") removed. This is the value that must go into the request body; the
// stored config keeps the marker for Claude Code's context-window selection.
func (m *Model) WireModelID() string {
	return WireModelID(m.ResolvedModelID())
}

// Sanitize cleans terminal/ANSI artifacts out of every model-id field in place
// (ANTHROPIC_MODEL, the three ANTHROPIC_DEFAULT_*_MODEL values and the top-level
// "model"). Tokens, base URL and every other key are left untouched. It reports
// whether any field was changed.
func (m *Model) Sanitize() bool {
	if m == nil {
		return false
	}
	changed := false
	clean := func(cur string, assign func(string)) {
		if s := SanitizeModelID(cur); s != cur {
			assign(s)
			changed = true
		}
	}
	clean(m.Env.Model, func(s string) { m.Env.Model = s })
	clean(m.Env.DefaultFableModel, func(s string) { m.Env.DefaultFableModel = s })
	clean(m.Env.DefaultHaikuModel, func(s string) { m.Env.DefaultHaikuModel = s })
	clean(m.Env.DefaultOpusModel, func(s string) { m.Env.DefaultOpusModel = s })
	clean(m.Env.DefaultSonnetModel, func(s string) { m.Env.DefaultSonnetModel = s })

	if top := m.TopLevelModel(); top != "" {
		if s := SanitizeModelID(top); s != top {
			if enc, err := json.Marshal(s); err == nil {
				if m.Rest == nil {
					m.Rest = map[string]json.RawMessage{}
				}
				m.Rest["model"] = enc
				changed = true
			}
		}
	}
	return changed
}
