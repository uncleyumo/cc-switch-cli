package manager

// SwitchProvider tests run in a sandboxed HOME: this checkout is a live Claude
// Code project and must never be the switch target. See CLAUDE.md.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"cc-switch-cli/internal/config"
	"cc-switch-cli/internal/model"
)

func sandboxConfig(t *testing.T, home string) {
	t.Helper()
	dir, err := config.ConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := config.WriteDefaultConfig(); err != nil {
		t.Fatal(err)
	}
}

func writeProvider(t *testing.T, name, doc string) {
	t.Helper()
	m, err := model.Parse([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	if err := model.Save(name, m); err != nil {
		t.Fatal(err)
	}
}

func readTargetDoc(t *testing.T, home string) (doc, env map[string]json.RawMessage) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(home, ".claude", "settings.json"))
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("target not valid JSON: %v\n%s", err, string(data))
	}
	env = map[string]json.RawMessage{}
	if raw, ok := doc["env"]; ok {
		if err := json.Unmarshal(raw, &env); err != nil {
			t.Fatalf("target env not an object: %v", err)
		}
	}
	return doc, env
}

func TestSwitchProviderMergesPreservingHooks(t *testing.T) {
	home := sandboxHome(t)
	sandboxConfig(t, home)
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	base := `{"hooks":{"SessionStart":[]},"permissions":{"allow":["Bash"]},` +
		`"env":{"ANTHROPIC_AUTH_TOKEN":"old","HTTP_PROXY":"http://p:8080"}}`
	if err := os.WriteFile(filepath.Join(home, ".claude", "settings.json"), []byte(base), 0o644); err != nil {
		t.Fatal(err)
	}
	writeProvider(t, "prov-a", `{"env":{"ANTHROPIC_AUTH_TOKEN":"tok-a","ANTHROPIC_BASE_URL":"https://a"}}`)
	writeProvider(t, "prov-b", `{"env":{"ANTHROPIC_AUTH_TOKEN":"tok-b","ANTHROPIC_BASE_URL":"https://b","ANTHROPIC_MODEL":"opus"}}`)

	if err := SwitchProvider("prov-a"); err != nil {
		t.Fatal(err)
	}
	doc, env := readTargetDoc(t, home)
	if _, ok := doc["hooks"]; !ok {
		t.Fatalf("hooks dropped by switch: %v", doc)
	}
	if _, ok := doc["permissions"]; !ok {
		t.Fatalf("permissions dropped by switch: %v", doc)
	}
	if string(env["ANTHROPIC_AUTH_TOKEN"]) != `"tok-a"` {
		t.Fatalf("provider env not applied: %v", env)
	}
	if string(env["HTTP_PROXY"]) != `"http://p:8080"` {
		t.Fatalf("non-model env key dropped: %v", env)
	}
	if got, _ := GetCurrentProvider(); got != "prov-a" {
		t.Fatalf("current marker = %q, want prov-a", got)
	}

	// prov-b sets ANTHROPIC_MODEL; switching back to prov-a (which does not)
	// must drop it instead of leaking the old model id.
	if err := SwitchProvider("prov-b"); err != nil {
		t.Fatal(err)
	}
	if _, env = readTargetDoc(t, home); string(env["ANTHROPIC_MODEL"]) != `"opus"` {
		t.Fatalf("prov-b model not applied: %v", env)
	}
	if err := SwitchProvider("prov-a"); err != nil {
		t.Fatal(err)
	}
	if _, env = readTargetDoc(t, home); len(env["ANTHROPIC_MODEL"]) != 0 {
		t.Fatalf("stale ANTHROPIC_MODEL leaked across switch: %v", env)
	}
	if string(env["HTTP_PROXY"]) != `"http://p:8080"` {
		t.Fatalf("non-model env key dropped on second switch: %v", env)
	}
}

func TestSwitchProviderCreatesMissingTarget(t *testing.T) {
	home := sandboxHome(t)
	sandboxConfig(t, home)
	writeProvider(t, "solo", `{"env":{"ANTHROPIC_BASE_URL":"https://solo"}}`)
	if err := SwitchProvider("solo"); err != nil {
		t.Fatal(err)
	}
	doc, env := readTargetDoc(t, home)
	if string(env["ANTHROPIC_BASE_URL"]) != `"https://solo"` {
		t.Fatalf("env not written: %v", env)
	}
	if _, ok := doc["env"]; !ok {
		t.Fatalf("env key missing: %v", doc)
	}
}

func TestSwitchProviderRejectsInvalidTargetJSON(t *testing.T) {
	home := sandboxHome(t)
	sandboxConfig(t, home)
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".claude", "settings.json"), []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeProvider(t, "prov", `{"env":{}}`)
	if err := SwitchProvider("prov"); err == nil {
		t.Fatal("expected error switching onto an invalid target file")
	}
}
