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
