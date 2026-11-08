package apply

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/uncleyumo/ccsc/internal/ccstate"
	"github.com/uncleyumo/ccsc/internal/group"
	"github.com/uncleyumo/ccsc/internal/store"
)

const seedSettings = `{
  "env": {
    "KEEP_ME": "yes"
  },
  "permissions": {
    "deny": [
      "WebSearch"
    ]
  },
  "model": "fable"
}
`

func setup(t *testing.T) (store.Paths, string) {
	t.Helper()
	root := t.TempDir()
	configDir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", configDir)
	t.Setenv(store.EnvHome, root)
	settings := filepath.Join(configDir, "settings.json")
	if err := os.WriteFile(settings, []byte(seedSettings), 0o600); err != nil {
		t.Fatal(err)
	}
	return store.Paths{Root: root}, settings
}

func grp(name, model string) *group.Group {
	return &group.Group{
		Schema:  1,
		Name:    name,
		BaseURL: "https://new-api.example.com/",
		Token:   "sk-test",
		Window:  500000,
		Slots: map[group.SlotName]group.Slot{
			group.Fable:  {Model: model},
			group.Opus:   {Model: model},
			group.Sonnet: {Model: model},
			group.Haiku:  {Model: model},
		},
	}
}

func readJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("%s is not valid JSON: %v\n%s", path, err, data)
	}
	return out
}

func TestGlobalApplyWritesOwnedKeysOnly(t *testing.T) {
	paths, settings := setup(t)
	if _, err := Run(Request{Paths: paths, Group: grp("g1", "m-one"), Layer: Global}); err != nil {
		t.Fatal(err)
	}

	doc := readJSON(t, settings)
	env := doc["env"].(map[string]any)

	if got := env["ANTHROPIC_BASE_URL"]; got != "https://new-api.example.com" {
		t.Errorf("base url = %v", got)
	}
	if got := env["ANTHROPIC_AUTH_TOKEN"]; got != "sk-test" {
		t.Errorf("token = %v", got)
	}
	if got := env["ANTHROPIC_DEFAULT_SONNET_MODEL"]; got != "m-one" {
		t.Errorf("sonnet = %v", got)
	}
	if got := env["ANTHROPIC_DEFAULT_MODEL"]; got != "m-one" {
		t.Errorf("startup fallback = %v", got)
	}
	if got := env["CLAUDE_CODE_MAX_CONTEXT_TOKENS"]; got != "500000" {
		t.Errorf("max context = %v", got)
	}
	if _, bad := env["CLAUDE_CODE_AUTO_COMPACT_WINDOW"]; bad {
		t.Error("the shadowing variable must never be written")
	}
	// Untouched keys survive.
	if env["KEEP_ME"] != "yes" {
		t.Error("unrelated env key was lost")
	}
	if _, ok := doc["permissions"]; !ok {
		t.Error("permissions block was lost")
	}
	if doc["model"] != "fable" {
		t.Error("top-level model was touched")
	}

	ms := doc["modelSettings"].(map[string]any)
	entry := ms["m-one"].(map[string]any)
	if got := entry["autoCompactWindow"].(float64); got != 450000 {
		t.Errorf("compact = %v, want 450000", got)
	}
}

func TestGlobalApplyIsIdempotent(t *testing.T) {
	paths, settings := setup(t)
	req := Request{Paths: paths, Group: grp("g1", "m-one"), Layer: Global}
	if _, err := Run(req); err != nil {
		t.Fatal(err)
	}
	first, _ := os.ReadFile(settings)
	if _, err := Run(req); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(settings)
	if string(first) != string(second) {
		t.Fatalf("second apply changed the file:\n--- first ---\n%s\n--- second ---\n%s", first, second)
	}
}

func TestSwitchingGroupsCleansStaleModelEntries(t *testing.T) {
	paths, settings := setup(t)
	if _, err := Run(Request{Paths: paths, Group: grp("g1", "old-model"), Layer: Global}); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(Request{Paths: paths, Group: grp("g2", "new-model"), Layer: Global}); err != nil {
		t.Fatal(err)
	}

	doc := readJSON(t, settings)
	ms, _ := doc["modelSettings"].(map[string]any)
	if _, stale := ms["old-model"]; stale {
		t.Errorf("stale modelSettings entry survived: %v", ms)
	}
	if _, ok := ms["new-model"]; !ok {
		t.Errorf("new modelSettings entry missing: %v", ms)
	}
}

func TestProjectApplyUsesTopLevelWindow(t *testing.T) {
	paths, _ := setup(t)
	dir := t.TempDir()
	project, err := ccstate.LocateProject(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Run(Request{Paths: paths, Group: grp("g1", "m-one"), Layer: Project, Project: project}); err != nil {
		t.Fatal(err)
	}

	doc := readJSON(t, project.LocalFile)
	if _, bad := doc["modelSettings"]; bad {
		t.Error("project layer must not write modelSettings")
	}
	if got := doc["autoCompactWindow"].(float64); got != 450000 {
		t.Errorf("top-level autoCompactWindow = %v, want 450000", got)
	}
	env := doc["env"].(map[string]any)
	if env["ANTHROPIC_DEFAULT_OPUS_MODEL"] != "m-one" {
		t.Error("project env was not written")
	}
}

func TestClearRemovesOnlyOwnedKeys(t *testing.T) {
	paths, settings := setup(t)
	if _, err := Run(Request{Paths: paths, Group: grp("g1", "m-one"), Layer: Global}); err != nil {
		t.Fatal(err)
	}
	if _, err := Clear(paths, Global, ccstate.Project{}); err != nil {
		t.Fatal(err)
	}

	doc := readJSON(t, settings)
	env, _ := doc["env"].(map[string]any)
	if env["KEEP_ME"] != "yes" {
		t.Error("clear removed an unrelated env key")
	}
	for _, k := range []string{
		"ANTHROPIC_BASE_URL", "ANTHROPIC_AUTH_TOKEN",
		"ANTHROPIC_DEFAULT_FABLE_MODEL", "ANTHROPIC_DEFAULT_MODEL",
		"CLAUDE_CODE_MAX_CONTEXT_TOKENS",
	} {
		if _, ok := env[k]; ok {
			t.Errorf("clear left %s behind", k)
		}
	}
	if _, ok := doc["modelSettings"]; ok {
		t.Error("clear left the modelSettings block behind")
	}
	if doc["model"] != "fable" {
		t.Error("clear touched the top-level model key")
	}
}

func TestRedundantAPIKeyIsRemoved(t *testing.T) {
	paths, settings := setup(t)
	doc := `{"env":{"ANTHROPIC_API_KEY":"sk-test"}}`
	if err := os.WriteFile(settings, []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	r, err := Run(Request{Paths: paths, Group: grp("g1", "m-one"), Layer: Global})
	if err != nil {
		t.Fatal(err)
	}
	out, _ := os.ReadFile(settings)
	if strings.Contains(string(out), "ANTHROPIC_API_KEY") {
		t.Errorf("redundant ANTHROPIC_API_KEY survived:\n%s", out)
	}
	if !strings.Contains(r.Render(), "ANTHROPIC_API_KEY") {
		t.Error("the receipt must say the redundant key was removed")
	}
}

func TestReceiptMentionsTargetAndSource(t *testing.T) {
	paths, settings := setup(t)
	r, err := Run(Request{Paths: paths, Group: grp("g1", "m-one"), Layer: Global})
	if err != nil {
		t.Fatal(err)
	}
	out := r.Render()
	if !strings.Contains(out, "applied g1 to global") {
		t.Errorf("receipt headline missing:\n%s", out)
	}
	if !strings.Contains(out, store.Shorten(settings)) {
		t.Errorf("receipt must name the file it wrote:\n%s", out)
	}
}
