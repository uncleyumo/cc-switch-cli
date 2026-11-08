package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/uncleyumo/ccsc/internal/ccstate"
	"github.com/uncleyumo/ccsc/internal/config"
	"github.com/uncleyumo/ccsc/internal/store"
)

func TestLoadOptionalSeedReopensThePreviousConfig(t *testing.T) {
	root := t.TempDir()
	configDir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", configDir)
	t.Setenv(store.EnvHome, root)

	projectDir := t.TempDir()
	project, err := ccstate.LocateProject(projectDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(project.LocalFile), 0o755); err != nil {
		t.Fatal(err)
	}
	local := `{"env":{
	  "ANTHROPIC_BASE_URL":"https://gateway.example.com",
	  "ANTHROPIC_AUTH_TOKEN":"sk-seeded",
	  "ANTHROPIC_DEFAULT_FABLE_MODEL":"m/fable",
	  "ANTHROPIC_DEFAULT_OPUS_MODEL":"m/opus",
	  "ANTHROPIC_DEFAULT_SONNET_MODEL":"m/sonnet",
	  "ANTHROPIC_DEFAULT_HAIKU_MODEL":"m/haiku",
	  "ANTHROPIC_DEFAULT_MODEL":"m/opus",
	  "CLAUDE_CODE_MAX_CONTEXT_TOKENS":"500000"
	},"autoCompactWindow":450000}`
	if err := os.WriteFile(project.LocalFile, []byte(local), 0o600); err != nil {
		t.Fatal(err)
	}

	state := &ccstate.State{Projects: map[string]ccstate.ProjectState{
		project.Key(): {Group: "optional", Source: "optional", Startup: "opus"},
	}}
	if err := ccstate.SaveState(root, state); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{}
	cfg.Defaults.StartupSlot = "sonnet"

	seed := loadOptionalSeed(store.Paths{Root: root}, cfg, project)
	if !seed.fromOptional {
		t.Fatal("the panel should have reopened on the previous optional config")
	}
	want := []string{
		"https://gateway.example.com", "sk-seeded", "500k", "450k", "opus",
		"m/fable", "m/opus", "m/sonnet", "m/haiku", "",
	}
	for i, w := range want {
		if got := seed.values[i]; got != w {
			t.Errorf("value[%d] = %q, want %q", i, got, w)
		}
	}
}

func TestLoadOptionalSeedIgnoresAGroupConfig(t *testing.T) {
	root := t.TempDir()
	configDir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", configDir)
	t.Setenv(store.EnvHome, root)

	project, err := ccstate.LocateProject(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	state := &ccstate.State{Projects: map[string]ccstate.ProjectState{
		project.Key(): {Group: "cost-effective", Source: "local"},
	}}
	if err := ccstate.SaveState(root, state); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{}
	cfg.Defaults.StartupSlot = "sonnet"
	seed := loadOptionalSeed(store.Paths{Root: root}, cfg, project)
	if seed.fromOptional {
		t.Fatal("a config written by ccsc local must not be inherited")
	}
}
