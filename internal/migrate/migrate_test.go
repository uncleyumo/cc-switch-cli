package migrate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/uncleyumo/ccsc/internal/group"
)

const legacySample = `{
  "name": "claude",
  "env": {
    "ANTHROPIC_BASE_URL": "https://new-api.example.com/",
    "ANTHROPIC_AUTH_TOKEN": "sk-legacy",
    "ANTHROPIC_DEFAULT_FABLE_MODEL": "packy-officially/deepseek-flash",
    "ANTHROPIC_DEFAULT_OPUS_MODEL": "packy-cc-sale/claude-opus-5-5[1m]",
    "ANTHROPIC_DEFAULT_SONNET_MODEL": "packy-deepseek-sale/deepseek-flash",
    "ANTHROPIC_DEFAULT_HAIKU_MODEL": "opencode2api-zen/glm-5.3-flash-free",
    "CLAUDE_CODE_SUBAGENT_MODEL": "opencode2api-zen/glm-5.3-flash-free",
    "CLAUDE_CODE_MAX_CONTEXT_TOKENS": "500000"
  },
  "model": "fable"
}`

func TestFromLegacySettings(t *testing.T) {
	g, err := FromLegacySettings([]byte(legacySample), "claude")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if g.Name != "claude" {
		t.Errorf("name = %q", g.Name)
	}
	if g.BaseURL != "https://new-api.example.com" {
		t.Errorf("base_url = %q (trailing slash should be normalised)", g.BaseURL)
	}
	if g.Token != "sk-legacy" {
		t.Errorf("token = %q", g.Token)
	}
	if g.Window != 500000 {
		t.Errorf("window = %d", g.Window)
	}
	if !g.Slot(group.Opus).Is1M() {
		t.Error("the [1m] suffix should set the opus slot window")
	}
	if g.Slot(group.Opus).WireModel() != "packy-cc-sale/claude-opus-5-5" {
		t.Errorf("opus model = %q", g.Slot(group.Opus).WireModel())
	}
	if !g.HasSubagent() {
		t.Error("CLAUDE_CODE_SUBAGENT_MODEL should fill the subagent slot")
	}
	if err := g.Validate(); err != nil {
		t.Errorf("the migrated group is not valid: %v", err)
	}
}

func TestFromLegacySettingsRejectsIncompleteInput(t *testing.T) {
	cases := map[string]string{
		"no token": `{"env":{"ANTHROPIC_BASE_URL":"https://x",
		             "ANTHROPIC_DEFAULT_FABLE_MODEL":"a","ANTHROPIC_DEFAULT_OPUS_MODEL":"b",
		             "ANTHROPIC_DEFAULT_SONNET_MODEL":"c","ANTHROPIC_DEFAULT_HAIKU_MODEL":"d"}}`,
		"missing haiku": `{"env":{"ANTHROPIC_BASE_URL":"https://x","ANTHROPIC_AUTH_TOKEN":"k",
		             "ANTHROPIC_DEFAULT_FABLE_MODEL":"a","ANTHROPIC_DEFAULT_OPUS_MODEL":"b",
		             "ANTHROPIC_DEFAULT_SONNET_MODEL":"c"}}`,
		"not json": `{`,
	}
	for name, data := range cases {
		if _, err := FromLegacySettings([]byte(data), "x"); err == nil {
			t.Errorf("%s: expected an error", name)
		}
	}
}

func TestScanSkipsExistingGroups(t *testing.T) {
	root := t.TempDir()
	legacy := filepath.Join(root, "models")
	groupsDir := filepath.Join(root, "groups")
	if err := os.MkdirAll(legacy, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(groupsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "claude.json"), []byte(legacySample), 0o600); err != nil {
		t.Fatal(err)
	}
	existing := &group.Group{
		Schema: 1, Name: "keep", BaseURL: "https://x", Token: "k", Window: 1000,
		Slots: map[group.SlotName]group.Slot{
			group.Fable: {Model: "a"}, group.Opus: {Model: "b"},
			group.Sonnet: {Model: "c"}, group.Haiku: {Model: "d"},
		},
	}
	if err := group.Save(groupsDir, existing); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "keep.json"), []byte(legacySample), 0o600); err != nil {
		t.Fatal(err)
	}

	results, err := Scan(legacy, groupsDir)
	if err != nil {
		t.Fatal(err)
	}
	var sawClaude, sawKeepSkip bool
	for _, r := range results {
		if r.Name == "claude" && r.Group != nil && r.Err == nil {
			sawClaude = true
		}
		if r.Name == "keep" && r.Skipped() {
			sawKeepSkip = true
		}
	}
	if !sawClaude {
		t.Error("the new group was not migrated")
	}
	if !sawKeepSkip {
		t.Error("an existing group must not be overwritten")
	}
	if !group.Exists(groupsDir, "claude") {
		t.Error("group.Exists(claude) = false after Scan")
	}
}

func TestMarkerRoundTrip(t *testing.T) {
	root := t.TempDir()
	if IsDone(root) {
		t.Fatal("a fresh root should not be marked")
	}
	if err := MarkDone(root); err != nil {
		t.Fatal(err)
	}
	if !IsDone(root) {
		t.Fatal("MarkDone did not take effect")
	}
}

func TestArchiveMovesTheDirectory(t *testing.T) {
	root := t.TempDir()
	legacy := filepath.Join(root, "models")
	if err := os.MkdirAll(legacy, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "a.json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	backups := filepath.Join(root, "backups")

	dst, err := Archive(legacy, backups)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Error("the legacy directory should be gone")
	}
	if _, err := os.Stat(filepath.Join(dst, "a.json")); err != nil {
		t.Errorf("archived file missing: %v", err)
	}
}
