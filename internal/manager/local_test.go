package manager

// Tests that resolve an implicit cwd (empty --path) must t.Chdir into t.TempDir
// first. This checkout is a live Claude Code project; see CLAUDE.md.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"cc-switch-cli/internal/mcp"
)

func TestResolveLocalTargetDefault(t *testing.T) {
	// Resolve against a temp cwd, never this repo — the checkout is live
	// Claude Code config and must not be used as an implicit local target.
	dir := t.TempDir()
	t.Chdir(dir)
	got, err := ResolveLocalTarget("")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, ".claude", "settings.local.json")
	if got != want {
		t.Fatalf("default target = %s, want %s", got, want)
	}
}

func TestResolveLocalTargetDirectory(t *testing.T) {
	dir := t.TempDir()
	got, err := ResolveLocalTarget(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, ".claude", "settings.local.json")
	if got != want {
		t.Fatalf("dir target = %s, want %s", got, want)
	}
}

func TestResolveLocalTargetFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "custom.json")
	got, err := ResolveLocalTarget(file)
	if err != nil {
		t.Fatal(err)
	}
	if got != file {
		t.Fatalf("file target = %s, want %s", got, file)
	}
}

func TestResolveLocalProjectDirDefault(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	got, err := ResolveLocalProjectDir("")
	if err != nil {
		t.Fatal(err)
	}
	if got != dir {
		t.Fatalf("default project = %s, want %s", got, dir)
	}
}

func TestResolveLocalProjectDir(t *testing.T) {
	dir := t.TempDir()
	got, err := ResolveLocalProjectDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != dir {
		t.Fatalf("dir project = %s, want %s", got, dir)
	}

	settings := filepath.Join(dir, ".claude", "settings.local.json")
	got, err = ResolveLocalProjectDir(settings)
	if err != nil {
		t.Fatal(err)
	}
	if got != dir {
		t.Fatalf("settings file project = %s, want %s", got, dir)
	}

	other := filepath.Join(dir, "custom.json")
	got, err = ResolveLocalProjectDir(other)
	if err != nil {
		t.Fatal(err)
	}
	if got != dir {
		t.Fatalf("other file project = %s, want %s", got, dir)
	}
}

func TestClearLocalConfigMissingIsOK(t *testing.T) {
	dir := t.TempDir()
	out, err := ClearLocalConfig(LocalOptions{Path: dir})
	if err != nil {
		t.Fatal(err)
	}
	if !out.Missing || out.Deleted {
		t.Fatalf("missing file should report Missing, got %+v", out)
	}
}

func TestClearLocalConfigDeletesFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, ".claude", "settings.local.json")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(`{"model":"sonnet"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := ClearLocalConfig(LocalOptions{Path: dir})
	if err != nil {
		t.Fatal(err)
	}
	if !out.Deleted || out.Missing {
		t.Fatalf("want deleted, got %+v", out)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("file still exists: %v", err)
	}
}

func TestClearLocalConfigModelOnlyKeepsPermissions(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, ".claude", "settings.local.json")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	src := `{
  "permissions": {"allow": ["Bash"]},
  "model": "sonnet",
  "env": {
    "ANTHROPIC_MODEL": "x",
    "HTTP_PROXY": "http://p"
  }
}`
	if err := os.WriteFile(target, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := ClearLocalConfig(LocalOptions{Path: dir, ModelOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if out.Deleted || out.Missing || !out.ModelOnly {
		t.Fatalf("want model-only rewrite, got %+v", out)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if !strings.Contains(s, `"permissions"`) || !strings.Contains(s, `"HTTP_PROXY"`) {
		t.Fatalf("kept keys missing: %s", s)
	}
	if strings.Contains(s, `"model"`) || strings.Contains(s, "ANTHROPIC_") {
		t.Fatalf("model keys survived: %s", s)
	}
}

func TestClearLocalConfigModelOnlyDeletesEmpty(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, ".claude", "settings.local.json")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(`{"model":"sonnet","env":{"ANTHROPIC_MODEL":"x"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := ClearLocalConfig(LocalOptions{Path: dir, ModelOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if !out.Deleted || !out.ModelOnly {
		t.Fatalf("want emptied+deleted, got %+v", out)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("emptied file still exists: %v", err)
	}
}

func TestClearLocalConfigModelOnlyRejectsInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, ".claude", "settings.local.json")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ClearLocalConfig(LocalOptions{Path: dir, ModelOnly: true}); err == nil {
		t.Fatal("expected error for invalid json")
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "not json" {
		t.Fatalf("invalid file was rewritten: %q", data)
	}
}

func TestInspectLocalConfigMissingFile(t *testing.T) {
	sandboxHome(t)
	dir := t.TempDir()
	snap, err := InspectLocalConfig(LocalOptions{Path: dir})
	if err != nil {
		t.Fatal(err)
	}
	if snap.SettingsExists {
		t.Fatal("expected missing settings")
	}
	if snap.HasModelOverride {
		t.Fatal("missing file should not report a model override")
	}
	if snap.ProjectDir != dir {
		t.Fatalf("project dir = %s, want %s", snap.ProjectDir, dir)
	}
	if len(snap.ProjectMCP) != 0 || len(snap.LocalMCP) != 0 {
		t.Fatalf("expected no MCP, got project=%v local=%v", snap.ProjectMCP, snap.LocalMCP)
	}
}

func TestInspectLocalConfigReadsModelAndMCP(t *testing.T) {
	home := sandboxHome(t)
	dir := t.TempDir()
	target := filepath.Join(dir, ".claude", "settings.local.json")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	settings := `{
  "permissions": {"allow": ["Bash"]},
  "model": "sonnet",
  "env": {
    "ANTHROPIC_AUTH_TOKEN": "sk-abcdefghijklmnop",
    "ANTHROPIC_BASE_URL": "https://api.example.com",
    "ANTHROPIC_MODEL": "claude-sonnet-4-6",
    "HTTP_PROXY": "http://proxy"
  }
}`
	if err := os.WriteFile(target, []byte(settings), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".mcp.json"), []byte(`{
  "mcpServers": {"fetch": {"command": "mcp-server-fetch"}}
}`), 0o644); err != nil {
		t.Fatal(err)
	}

	projKey := mcp.NormalizeProjectDir(dir)
	claude := map[string]any{
		"projects": map[string]any{
			projKey: map[string]any{
				"mcpServers": map[string]any{
					"local-srv": map[string]any{"command": "echo"},
				},
			},
		},
	}
	raw, err := json.Marshal(claude)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".claude.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}

	snap, err := InspectLocalConfig(LocalOptions{Path: dir})
	if err != nil {
		t.Fatal(err)
	}
	if !snap.SettingsExists || !snap.HasModelOverride || snap.Model == nil {
		t.Fatalf("expected model override, got %+v", snap)
	}
	if snap.Model.TopLevelModel() != "sonnet" {
		t.Fatalf("top-level model = %q", snap.Model.TopLevelModel())
	}
	if snap.Model.Env.Model != "claude-sonnet-4-6" {
		t.Fatalf("env model = %q", snap.Model.Env.Model)
	}
	if len(snap.OtherKeys) != 1 || snap.OtherKeys[0] != "permissions" {
		t.Fatalf("other keys = %v", snap.OtherKeys)
	}
	if len(snap.ExtraEnv) != 1 || snap.ExtraEnv[0] != "HTTP_PROXY" {
		t.Fatalf("extra env = %v", snap.ExtraEnv)
	}
	if len(snap.ProjectMCP) != 1 || snap.ProjectMCP[0].Name != "fetch" {
		t.Fatalf("project MCP = %v", snap.ProjectMCP)
	}
	if len(snap.LocalMCP) != 1 || snap.LocalMCP[0].Name != "local-srv" {
		t.Fatalf("local MCP = %v", snap.LocalMCP)
	}
}

func TestInspectLocalConfigMatchesProviderGroup(t *testing.T) {
	home := sandboxHome(t)
	modelsDir := filepath.Join(home, ".cc-switch-cli", "models")
	if err := os.MkdirAll(modelsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Two store groups plus one that shares the base URL but not the token —
	// only "group-a" should match.
	groups := map[string]string{
		"group-a": `{"env":{"ANTHROPIC_AUTH_TOKEN":"sk-local-token","ANTHROPIC_BASE_URL":"https://api.example.com","ANTHROPIC_MODEL":"claude-sonnet-4-6"}}`,
		"other":   `{"env":{"ANTHROPIC_AUTH_TOKEN":"sk-other","ANTHROPIC_BASE_URL":"https://api.example.com"}}`,
	}
	for name, raw := range groups {
		if err := os.WriteFile(filepath.Join(modelsDir, name+".json"), []byte(raw), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	dir := t.TempDir()
	target := filepath.Join(dir, ".claude", "settings.local.json")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	settings := `{
  "model": "sonnet",
  "env": {
    "ANTHROPIC_AUTH_TOKEN": "sk-local-token",
    "ANTHROPIC_BASE_URL": "https://api.example.com"
  }
}`
	if err := os.WriteFile(target, []byte(settings), 0o644); err != nil {
		t.Fatal(err)
	}

	snap, err := InspectLocalConfig(LocalOptions{Path: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.MatchedProviders) != 1 || snap.MatchedProviders[0] != "group-a" {
		t.Fatalf("matched providers = %v, want [group-a]", snap.MatchedProviders)
	}
}

func TestInspectLocalConfigNoMatchingGroup(t *testing.T) {
	sandboxHome(t)
	dir := t.TempDir()
	target := filepath.Join(dir, ".claude", "settings.local.json")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(`{"env":{"ANTHROPIC_AUTH_TOKEN":"sk-x","ANTHROPIC_BASE_URL":"https://x.example.com"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	snap, err := InspectLocalConfig(LocalOptions{Path: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(snap.MatchedProviders) != 0 {
		t.Fatalf("expected no match, got %v", snap.MatchedProviders)
	}
}

func sandboxHome(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	return dir
}
