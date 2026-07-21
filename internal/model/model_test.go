package model

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestModelRoundTripPreservesUnknownKeys(t *testing.T) {
	src := []byte(`{
	  "env": {
	    "ANTHROPIC_AUTH_TOKEN": "sk-abc",
	    "ANTHROPIC_BASE_URL": "https://example.test",
	    "ANTHROPIC_MODEL": "claude-sonnet-4-6",
	    "MY_CUSTOM_VAR": "hello",
	    "NUMERIC_ENV": "1"
	  },
	  "enabledPlugins": {"a": true},
	  "extraKnownMarketplaces": {},
	  "alwaysThinkingEnabled": false,
	  "skipDangerousModePermissionPrompt": true,
	  "unknownTopLevel": {"nested": [1, 2, 3]},
	  "model": "sonnet"
	}`)

	m, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if m.Env.AuthToken != "sk-abc" || m.Env.BaseURL != "https://example.test" {
		t.Fatalf("typed env not populated: %+v", m.Env)
	}
	if _, ok := m.Env.Extra["MY_CUSTOM_VAR"]; !ok {
		t.Fatalf("unknown env dropped: %v", m.Env.Extra)
	}
	if _, ok := m.Rest["unknownTopLevel"]; !ok {
		t.Fatalf("unknown top-level key dropped")
	}

	out, err := m.Indent()
	if err != nil {
		t.Fatalf("indent: %v", err)
	}
	// Ensure unknown data survived a round-trip.
	if !strings.Contains(string(out), `"MY_CUSTOM_VAR"`) || !strings.Contains(string(out), `"unknownTopLevel"`) {
		t.Fatalf("round-trip dropped unknown data: %s", string(out))
	}
	if !strings.Contains(string(out), `"ANTHROPIC_AUTH_TOKEN": "sk-abc"`) {
		t.Fatalf("typed env not re-emitted: %s", string(out))
	}
}

func TestEnvOmitsEmptyKnownKeys(t *testing.T) {
	env := Env{BaseURL: "https://x", Model: "claude"}
	data, err := json.Marshal(env)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if strings.Contains(got, "ANTHROPIC_AUTH_TOKEN") || strings.Contains(got, "ANTHROPIC_API_KEY") {
		t.Fatalf("empty known env keys should be omitted: %s", got)
	}
	if !strings.Contains(got, `"ANTHROPIC_BASE_URL":"https://x"`) {
		t.Fatalf("expected known env key emitted: %s", got)
	}
}

func TestNonStringEnvValueRejected(t *testing.T) {
	// ANTHROPIC_MODEL is typed as string; a numeric value must be rejected.
	src := []byte(`{"env": {"ANTHROPIC_MODEL": 42}}`)
	if _, err := Parse(src); err == nil {
		t.Fatalf("expected parse error for non-string typed env")
	}
}

func TestUnknownEnvValueMayBeAnyJSON(t *testing.T) {
	src := []byte(`{"env": {"CUSTOM": [1, 2, 3]}}`)
	m, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, ok := m.Env.Extra["CUSTOM"]; !ok {
		t.Fatalf("unknown env dropped")
	}
	out, err := m.Indent()
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	// json.Indent may spread the array across lines; assert on the pieces.
	if !strings.Contains(s, `"CUSTOM"`) ||
		!strings.Contains(s, "1") ||
		!strings.Contains(s, "2") ||
		!strings.Contains(s, "3") {
		t.Fatalf("unknown env array not preserved: %s", s)
	}
}

func TestModelIDPreference(t *testing.T) {
	m := &Model{Env: Env{DefaultSonnetModel: "s"}}
	if m.ModelID() != "s" {
		t.Fatalf("expected sonnet fallback, got %q", m.ModelID())
	}
	m.Env.Model = "primary"
	if m.ModelID() != "primary" {
		t.Fatalf("primary should win")
	}
}

func TestTokenPreference(t *testing.T) {
	m := &Model{Env: Env{APIKey: "k"}}
	if m.Token() != "k" {
		t.Fatalf("api key fallback")
	}
	m.Env.AuthToken = "t"
	if m.Token() != "t" {
		t.Fatalf("auth token should win")
	}
}

func TestSanitizeModelID(t *testing.T) {
	cases := []struct{ in, want string }{
		{"claude-opus-4-8", "claude-opus-4-8"},                       // clean untouched
		{"claude-opus-4-8[1m]", "claude-opus-4-8[1m]"},               // context marker PRESERVED (not corruption)
		{"glm-5.2[1m]", "glm-5.2[1m]"},                               // dots + marker preserved
		{"\x1b[1mclaude\x1b[0m", "claude"},                           // real ANSI wrapping stripped
		{"\x1b[38;5;208mqwen3.8-max-preview", "qwen3.8-max-preview"}, // 256-color prefix stripped
		{"  claude-sonnet-4-6  ", "claude-sonnet-4-6"},               // whitespace trim
		{"", ""},
	}
	for _, c := range cases {
		if got := SanitizeModelID(c.in); got != c.want {
			t.Errorf("SanitizeModelID(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestWireModelID(t *testing.T) {
	cases := []struct{ in, want string }{
		{"claude-opus-4-8[1m]", "claude-opus-4-8"}, // 1M marker stripped on the wire
		{"qwen3.8-max-preview[1m]", "qwen3.8-max-preview"},
		{"some-model[200k]", "some-model"},     // 200k marker
		{"some-model[128K]", "some-model"},     // uppercase K
		{"claude-opus-4-8", "claude-opus-4-8"}, // no marker untouched
		{"mimo-v2.5-pro", "mimo-v2.5-pro"},     // version dots untouched
		{"", ""},
	}
	for _, c := range cases {
		if got := WireModelID(c.in); got != c.want {
			t.Errorf("WireModelID(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestResolvedModelID(t *testing.T) {
	// Concrete ANTHROPIC_MODEL wins; the [1m] context marker is preserved
	// (ResolvedModelID reflects the stored value, WireModelID strips it).
	m := &Model{Env: Env{Model: "claude-opus-4-8[1m]", DefaultSonnetModel: "claude-sonnet-4-6"}}
	if got := m.ResolvedModelID(); got != "claude-opus-4-8[1m]" {
		t.Fatalf("concrete ANTHROPIC_MODEL should win with marker preserved, got %q", got)
	}

	// Tier alias in ANTHROPIC_MODEL resolves to the matching default (marker kept).
	m = &Model{Env: Env{Model: "opus", DefaultOpusModel: "qwen3.8-max-preview[1m]"}}
	if got := m.ResolvedModelID(); got != "qwen3.8-max-preview[1m]" {
		t.Fatalf("opus alias should resolve to opus default (marker kept), got %q", got)
	}

	// Tier alias with no matching default falls through to the top-level model,
	// then to the first available default (mirrors the qwen-plan config).
	m = &Model{
		Env:  Env{Model: "opus", DefaultSonnetModel: "qwen3.8-max-preview[1m]"},
		Rest: map[string]json.RawMessage{"model": json.RawMessage(`"opus"`)},
	}
	if got := m.ResolvedModelID(); got != "qwen3.8-max-preview[1m]" {
		t.Fatalf("should fall back to sonnet default (marker kept), got %q", got)
	}

	// Top-level tier alias resolves via its default (packy-bridge: model=haiku).
	m = &Model{
		Env:  Env{DefaultSonnetModel: "gpt-5.6-terra[1m]", DefaultHaikuModel: "grok-4.5[1m]"},
		Rest: map[string]json.RawMessage{"model": json.RawMessage(`"haiku"`)},
	}
	if got := m.ResolvedModelID(); got != "grok-4.5[1m]" {
		t.Fatalf("haiku alias should resolve to haiku default (marker kept), got %q", got)
	}

	// Nothing configured.
	if got := (&Model{}).ResolvedModelID(); got != "" {
		t.Fatalf("empty config should resolve to empty, got %q", got)
	}
}

func TestModelWireModelIDStripsContextMarker(t *testing.T) {
	// qwen-plan shape: top-level + ANTHROPIC_MODEL are the "opus" alias, which
	// resolves to the [1m]-annotated opus default; the wire form drops [1m].
	m := &Model{
		Env:  Env{Model: "opus", DefaultOpusModel: "qwen3.8-max-preview[1m]", DefaultSonnetModel: "qwen3.8-max-preview[1m]"},
		Rest: map[string]json.RawMessage{"model": json.RawMessage(`"opus"`)},
	}
	if got := m.WireModelID(); got != "qwen3.8-max-preview" {
		t.Fatalf("wire id should drop [1m], got %q", got)
	}
	if got := m.ResolvedModelID(); got != "qwen3.8-max-preview[1m]" {
		t.Fatalf("resolved id should keep [1m], got %q", got)
	}
}

func TestSanitizeOnlyStripsRealEscapeBytes(t *testing.T) {
	m := &Model{
		Env: Env{
			AuthToken:          "sk-secret[1m]", // must NOT be touched
			BaseURL:            "https://x.test",
			Model:              "\x1b[1mclaude-opus-4-8\x1b[0m", // real ESC bytes -> stripped
			DefaultSonnetModel: "claude-sonnet-4-6[1m]",         // context marker -> kept
		},
		Rest: map[string]json.RawMessage{"model": json.RawMessage(`"sonnet"`)},
	}
	if !m.Sanitize() {
		t.Fatalf("expected Sanitize to report a change (real ESC bytes)")
	}
	if m.Env.Model != "claude-opus-4-8" {
		t.Fatalf("real ESC bytes should be stripped from Model: %q", m.Env.Model)
	}
	if m.Env.DefaultSonnetModel != "claude-sonnet-4-6[1m]" {
		t.Fatalf("context marker must be preserved: %q", m.Env.DefaultSonnetModel)
	}
	if m.Env.AuthToken != "sk-secret[1m]" {
		t.Fatalf("token must never be modified, got %q", m.Env.AuthToken)
	}
	if m.TopLevelModel() != "sonnet" {
		t.Fatalf("clean top-level model should be untouched: %q", m.TopLevelModel())
	}
	// Second pass reports nothing left to fix.
	if m.Sanitize() {
		t.Fatalf("second Sanitize should be a no-op")
	}
}
