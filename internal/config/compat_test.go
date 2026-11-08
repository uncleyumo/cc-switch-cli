package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/uncleyumo/ccsc/internal/store"
)

// TestLoadReadsV1Config guards the upgrade path: the v1 config.toml only has a
// [target] table, and ccsc v2 must read it without complaint rather than
// refusing to start on an existing installation.
func TestLoadReadsV1Config(t *testing.T) {
	root := t.TempDir()
	t.Setenv(store.EnvHome, root)

	legacy := "[target]\n  path = 'C:\\\\Users\\\\someone\\\\.claude\\\\settings.json'\n"
	if err := os.WriteFile(filepath.Join(root, "config.toml"), []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(store.Paths{Root: root})
	if err != nil {
		t.Fatalf("a v1 config.toml must still load: %v", err)
	}
	if !strings.Contains(cfg.Target.Path, "settings.json") {
		t.Errorf("target path = %q", cfg.Target.Path)
	}
	if cfg.Defaults.BaseURL != "" {
		t.Errorf("default base_url must stay empty, got %q", cfg.Defaults.BaseURL)
	}
	if got := cfg.StartupSlot(); string(got) != "sonnet" {
		t.Errorf("startup slot = %q, want sonnet", got)
	}
	if cfg.Cache.TTLHours != DefaultTTLHours {
		t.Errorf("ttl = %d, want %d", cfg.Cache.TTLHours, DefaultTTLHours)
	}
}
