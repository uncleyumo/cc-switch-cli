package model

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseContextWindow(t *testing.T) {
	cases := []struct {
		in      string
		want    int
		wantErr bool
	}{
		{"", 0, false},
		{"  ", 0, false},
		{"128k", 128_000, false},
		{"200k", 200_000, false},
		{"256k", 256_000, false},
		{"500k", 500_000, false},
		{"500K", 500_000, false},
		{"1m", 1_000_000, false},
		{"1M", 1_000_000, false},
		{"2m", 2_000_000, false},
		{"500000", 0, true},
		{"500", 0, true},
		{"200", 0, true},
		{"1000", 0, true},
		{"500_000", 0, true},
		{"99", 0, true},
		{"1001", 0, true},
		{"3m", 0, true},
		{"abc", 0, true},
		{"50k", 0, true},
		{"100k", 0, true},
	}
	for _, c := range cases {
		got, err := ParseContextWindow(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("ParseContextWindow(%q) = %d, want error", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseContextWindow(%q) unexpected error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("ParseContextWindow(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestParseStoredWindowAcceptsInteger(t *testing.T) {
	n, err := parseStoredWindow("500000")
	if err != nil || n != 500_000 {
		t.Fatalf("parseStoredWindow(500000) = %d, %v", n, err)
	}
	n, err = parseStoredWindow("500k")
	if err != nil || n != 500_000 {
		t.Fatalf("parseStoredWindow(500k) = %d, %v", n, err)
	}
	if _, err := parseStoredWindow("500"); err == nil {
		t.Fatal("bare 500 must be rejected even on disk")
	}
}

func TestFormatContextWindow(t *testing.T) {
	if got := FormatContextWindow(500_000); got != "500k" {
		t.Fatalf("FormatContextWindow(500000) = %q", got)
	}
	if got := FormatContextWindow(1_000_000); got != "1m" {
		t.Fatalf("FormatContextWindow(1000000) = %q", got)
	}
	if got := FormatContextWindow(2_000_000); got != "2m" {
		t.Fatalf("FormatContextWindow(2000000) = %q", got)
	}
	if got := FormatContextWindow(128_000); got != "128k" {
		t.Fatalf("FormatContextWindow(128000) = %q", got)
	}
	if got := FormatContextWindow(0); got != "" {
		t.Fatalf("FormatContextWindow(0) = %q", got)
	}
}

func TestApplyContextWindowRoundTrip(t *testing.T) {
	src := []byte(`{
	  "env": {
	    "ANTHROPIC_BASE_URL": "https://x.test",
	    "ANTHROPIC_MODEL": "grok-4.6",
	    "CLAUDE_CODE_MAX_CONTEXT_TOKENS": "500000",
	    "CLAUDE_CODE_AUTO_COMPACT_WINDOW": "500000",
	    "CLAUDE_CODE_AUTO_COMPACT_WINDOW_TOKENS": "500000"
	  }
	}`)
	m, err := Parse(src)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if m.Env.MaxContextTokens != "500000" {
		t.Fatalf("typed max context not populated: %q", m.Env.MaxContextTokens)
	}
	if m.Env.ContextWindowTokens() != 500_000 {
		t.Fatalf("ContextWindowTokens = %d", m.Env.ContextWindowTokens())
	}
	if _, ok := m.Env.Extra[EnvMaxContextTokens]; ok {
		t.Fatalf("typed context key leaked into Extra")
	}

	out, err := m.Indent()
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, `"CLAUDE_CODE_MAX_CONTEXT_TOKENS": "500000"`) {
		t.Fatalf("max context not re-emitted: %s", s)
	}
	if !strings.Contains(s, `"CLAUDE_CODE_AUTO_COMPACT_WINDOW": "500000"`) {
		t.Fatalf("auto compact window not re-emitted: %s", s)
	}

	m.ApplyContextWindow(0)
	cleared, err := m.Indent()
	if err != nil {
		t.Fatal(err)
	}
	cs := string(cleared)
	if strings.Contains(cs, "CLAUDE_CODE_MAX_CONTEXT_TOKENS") ||
		strings.Contains(cs, "CLAUDE_CODE_AUTO_COMPACT_WINDOW") {
		t.Fatalf("cleared window keys still present: %s", cs)
	}
}

func TestApplyOmitsCompactWhenMarkerPresent(t *testing.T) {
	m := &Model{
		Env: Env{
			BaseURL:           "https://x",
			Model:             "opus",
			DefaultOpusModel:  "grok-4.6[1m]",
			DefaultSonnetModel: "grok-4.6",
			DefaultHaikuModel: "grok-4.6",
		},
	}
	m.ApplyContextWindow(500_000)
	if m.Env.MaxContextTokens != "500000" {
		t.Fatalf("max context = %q", m.Env.MaxContextTokens)
	}
	if m.Env.AutoCompactWindow != "" || m.Env.AutoCompactWindowTokens != "" {
		t.Fatalf("compact keys must be omitted when a sibling id has [1m]: %+v", m.Env)
	}
}

func TestApplyWritesCompactWhenOnlyNon1MMarker(t *testing.T) {
	m := &Model{Env: Env{BaseURL: "https://x", Model: "grok-4.6[200k]"}}
	m.ApplyContextWindow(500_000)
	if m.Env.AutoCompactWindow != "500000" {
		t.Fatalf("[200k] must not suppress AUTO_COMPACT: %+v", m.Env)
	}
}

func TestApplyWritesCompactWithoutMarker(t *testing.T) {
	m := &Model{Env: Env{BaseURL: "https://x", Model: "grok-4.6"}}
	m.ApplyContextWindow(500_000)
	if m.Env.AutoCompactWindow != "500000" || m.Env.AutoCompactWindowTokens != "500000" {
		t.Fatalf("compact keys missing: %+v", m.Env)
	}
}

func TestApplyOmitsCompactAbove1M(t *testing.T) {
	m := &Model{Env: Env{BaseURL: "https://x", Model: "grok-4.6"}}
	m.ApplyContextWindow(2_000_000)
	if m.Env.MaxContextTokens != "2000000" {
		t.Fatalf("max context = %q", m.Env.MaxContextTokens)
	}
	if m.Env.AutoCompactWindow != "" {
		t.Fatalf("AUTO_COMPACT_WINDOW cannot hold 2m: %q", m.Env.AutoCompactWindow)
	}
}

func TestLooksLikeClaudeModel(t *testing.T) {
	if !LooksLikeClaudeModel("claude-sonnet-4-6") {
		t.Fatal("claude- prefix should match")
	}
	if !LooksLikeClaudeModel("us.anthropic.claude-opus-4-8[1m]") {
		t.Fatal("embedded claude- should match")
	}
	if LooksLikeClaudeModel("grok-4.6") {
		t.Fatal("grok should not look like Claude")
	}
	if LooksLikeClaudeModel("grok-4.6[1m]") {
		t.Fatal("grok[1m] should not look like Claude")
	}
}

func TestHasContextMarker(t *testing.T) {
	if !HasContextMarker("grok-4.6[1m]") {
		t.Fatal("expected marker")
	}
	if !HasContextMarker("grok-4.6[200k]") {
		t.Fatal("expected [200k] marker")
	}
	if HasContextMarker("grok-4.6") {
		t.Fatal("no marker")
	}
}

func TestHas1MMarker(t *testing.T) {
	if !Has1MMarker("grok-4.6[1m]") || !Has1MMarker("grok-4.6[1M]") {
		t.Fatal("[1m] should match")
	}
	if Has1MMarker("grok-4.6[200k]") || Has1MMarker("grok-4.6") {
		t.Fatal("[200k] is not a 1M override")
	}
}

func TestMergeDropsUnsetContextWindow(t *testing.T) {
	base := []byte(`{
	  "permissions": {"allow": ["foo"]},
	  "env": {
	    "CUSTOM": "keep-me",
	    "CLAUDE_CODE_MAX_CONTEXT_TOKENS": "500000",
	    "CLAUDE_CODE_AUTO_COMPACT_WINDOW": "500000",
	    "CLAUDE_CODE_AUTO_COMPACT_WINDOW_TOKENS": "500000"
	  }
	}`)
	prov := &Model{Env: Env{BaseURL: "https://y", Model: "z"}}
	out, err := MergeProviderInto(base, prov)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, `"CUSTOM"`) || !strings.Contains(s, `"keep-me"`) {
		t.Fatalf("existing env keys dropped: %s", s)
	}
	if strings.Contains(s, "CLAUDE_CODE_MAX_CONTEXT_TOKENS") ||
		strings.Contains(s, "CLAUDE_CODE_AUTO_COMPACT_WINDOW") {
		t.Fatalf("stale context window leaked across local merge: %s", s)
	}
}

func TestMergeKeepsIncomingContextWindow(t *testing.T) {
	base := []byte(`{"env":{"CLAUDE_CODE_MAX_CONTEXT_TOKENS":"200000"}}`)
	prov := &Model{Env: Env{BaseURL: "https://y", Model: "z"}}
	prov.ApplyContextWindow(500_000)
	out, err := MergeProviderInto(base, prov)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, `"CLAUDE_CODE_MAX_CONTEXT_TOKENS": "500000"`) {
		t.Fatalf("incoming window not applied: %s", s)
	}
	if strings.Contains(s, "200000") {
		t.Fatalf("stale 200k survived: %s", s)
	}
}

func TestContextWindowTypedKeysNotInExtra(t *testing.T) {
	src := []byte(`{"env":{"ANTHROPIC_MODEL":"x","CLAUDE_CODE_MAX_CONTEXT_TOKENS":"500000"}}`)
	m, err := Parse(src)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(m.Env)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]json.RawMessage{}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if string(got[EnvMaxContextTokens]) != `"500000"` {
		t.Fatalf("expected typed emit, got %s", raw)
	}
}
