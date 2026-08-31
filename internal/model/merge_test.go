package model

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMergeIntoEmptyBase(t *testing.T) {
	prov := &Model{
		Env:  Env{BaseURL: "https://x", Model: "c"},
		Rest: map[string]json.RawMessage{"model": json.RawMessage(`"sonnet"`)},
	}
	out, err := MergeProviderInto(nil, prov)
	if err != nil {
		t.Fatal(err)
	}
	doc := map[string]json.RawMessage{}
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatalf("output not valid JSON: %v\n%s", err, string(out))
	}
	if _, ok := doc["env"]; !ok {
		t.Fatalf("merged env missing")
	}
	if string(doc["model"]) != `"sonnet"` {
		t.Fatalf("top-level model not applied: %s", string(doc["model"]))
	}
}

func TestMergePreservesExistingPermissions(t *testing.T) {
	base := []byte(`{
	  "permissions": {"allow": ["foo"]},
	  "env": {"CUSTOM": "keep-me"},
	  "model": "haiku"
	}`)
	prov := &Model{
		Env:  Env{BaseURL: "https://y", Model: "z"},
		Rest: map[string]json.RawMessage{"model": json.RawMessage(`"sonnet"`)},
	}
	out, err := MergeProviderInto(base, prov)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	s := string(out)
	if !strings.Contains(s, `"permissions"`) || !strings.Contains(s, `"foo"`) {
		t.Fatalf("permissions dropped: %s", s)
	}
	if !strings.Contains(s, `"CUSTOM"`) || !strings.Contains(s, `"keep-me"`) {
		t.Fatalf("existing env keys dropped: %s", s)
	}
	if !strings.Contains(s, `"ANTHROPIC_BASE_URL"`) {
		t.Fatalf("provider env not injected: %s", s)
	}
	if !strings.Contains(s, `"sonnet"`) {
		t.Fatalf("model not overridden: %s", s)
	}
}

func TestMergeRejectsInvalidBase(t *testing.T) {
	if _, err := MergeProviderInto([]byte("not json"), &Model{}); err == nil {
		t.Fatal("expected error for invalid base json")
	}
}

func TestOverlayProviderPreservesUndeclaredKeys(t *testing.T) {
	base := []byte(`{
	  "hooks": {"SessionStart": [{"hooks": [{"type": "command"}]}]},
	  "permissions": {"allow": ["Bash"]},
	  "statusLine": {"type": "command", "command": "old"},
	  "env": {"ANTHROPIC_BASE_URL": "http://old", "HTTP_PROXY": "http://proxy:8080"}
	}`)
	prov, err := Parse([]byte(`{
	  "env": {"ANTHROPIC_AUTH_TOKEN": "sk", "ANTHROPIC_BASE_URL": "https://new"},
	  "statusLine": {"type": "command", "command": "new"}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	out, err := OverlayProvider(base, prov)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, `"SessionStart"`) || !strings.Contains(s, `"Bash"`) {
		t.Fatalf("undeclared target keys dropped: %s", s)
	}
	if !strings.Contains(s, `"command": "new"`) || strings.Contains(s, `"old"`) {
		t.Fatalf("declared key not replaced wholesale: %s", s)
	}
	if !strings.Contains(s, `"HTTP_PROXY"`) {
		t.Fatalf("non-model env key dropped: %s", s)
	}
	for _, stale := range []string{"ANTHROPIC_MODEL", "CLAUDE_CODE_MAX_CONTEXT_TOKENS", "http://old"} {
		if strings.Contains(s, stale) {
			t.Fatalf("stale model env key %q survived: %s", stale, s)
		}
	}
}

func TestOverlayProviderEmptyBase(t *testing.T) {
	prov, err := Parse([]byte(`{"env":{"ANTHROPIC_BASE_URL":"https://x"}}`))
	if err != nil {
		t.Fatal(err)
	}
	out, err := OverlayProvider(nil, prov)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"https://x"`) {
		t.Fatalf("provider env missing from fresh document: %s", string(out))
	}
}

func TestOverlayProviderRejectsInvalidBase(t *testing.T) {
	prov, err := Parse([]byte(`{"env":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := OverlayProvider([]byte("not json"), prov); err == nil {
		t.Fatal("expected error for invalid base json")
	}
}

func TestMergeProviderEnvOverridesSameKey(t *testing.T) {
	base := []byte(`{"env":{"ANTHROPIC_BASE_URL":"http://old","ANTHROPIC_MODEL":"old"}}`)
	prov := &Model{Env: Env{BaseURL: "https://new", Model: "new"}}
	out, err := MergeProviderInto(base, prov)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"ANTHROPIC_BASE_URL": "https://new"`) {
		t.Fatalf("provider env did not override existing: %s", string(out))
	}
	if strings.Contains(string(out), `"http://old"`) {
		t.Fatalf("stale env value survived merge: %s", string(out))
	}
}

func TestStripModelKeysPreservesProjectSettings(t *testing.T) {
	base := []byte(`{
	  "permissions": {"allow": ["Bash"]},
	  "hooks": {"PreToolUse": []},
	  "model": "sonnet",
	  "env": {
	    "ANTHROPIC_AUTH_TOKEN": "sk-secret",
	    "ANTHROPIC_BASE_URL": "https://x",
	    "ANTHROPIC_MODEL": "claude-sonnet-4-6",
	    "CLAUDE_CODE_MAX_CONTEXT_TOKENS": "500000",
	    "HTTP_PROXY": "http://proxy.local:8080"
	  }
	}`)
	out, emptied, err := StripModelKeys(base)
	if err != nil {
		t.Fatal(err)
	}
	if emptied {
		t.Fatal("expected remaining project keys")
	}
	s := string(out)
	if !strings.Contains(s, `"permissions"`) || !strings.Contains(s, `"hooks"`) {
		t.Fatalf("project keys dropped: %s", s)
	}
	if !strings.Contains(s, `"HTTP_PROXY"`) {
		t.Fatalf("custom env dropped: %s", s)
	}
	for _, leak := range []string{"ANTHROPIC_", "CLAUDE_CODE_", `"model"`} {
		if strings.Contains(s, leak) {
			t.Fatalf("model key %q survived strip: %s", leak, s)
		}
	}
}

func TestStripModelKeysEmptiesWhenOnlyModel(t *testing.T) {
	base := []byte(`{"model":"sonnet","env":{"ANTHROPIC_MODEL":"x"}}`)
	out, emptied, err := StripModelKeys(base)
	if err != nil {
		t.Fatal(err)
	}
	if !emptied || out != nil {
		t.Fatalf("want emptied, got emptied=%v out=%q", emptied, out)
	}
}

func TestStripModelKeysEmptyBase(t *testing.T) {
	if _, emptied, err := StripModelKeys(nil); err != nil || !emptied {
		t.Fatalf("empty base should be emptied, err=%v emptied=%v", err, emptied)
	}
	if _, emptied, err := StripModelKeys([]byte("  \n")); err != nil || !emptied {
		t.Fatalf("whitespace base should be emptied, err=%v emptied=%v", err, emptied)
	}
}

func TestStripModelKeysRejectsInvalidJSON(t *testing.T) {
	if _, _, err := StripModelKeys([]byte("not json")); err == nil {
		t.Fatal("expected error for invalid json")
	}
}

func TestIsModelEnvKey(t *testing.T) {
	yes := []string{"ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_FOO", "CLAUDE_CODE_MAX_CONTEXT_TOKENS"}
	no := []string{"HTTP_PROXY", "CUSTOM", "anthropic_lower", "CLAUDE_OTHER"}
	for _, k := range yes {
		if !IsModelEnvKey(k) {
			t.Errorf("IsModelEnvKey(%q) = false, want true", k)
		}
	}
	for _, k := range no {
		if IsModelEnvKey(k) {
			t.Errorf("IsModelEnvKey(%q) = true, want false", k)
		}
	}
}
