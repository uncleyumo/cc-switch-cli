package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/uncleyumo/ccsc/internal/group"
	"github.com/uncleyumo/ccsc/internal/store"
)

func testPaths(t *testing.T) store.Paths {
	t.Helper()
	return store.Paths{Root: t.TempDir()}
}

func TestLoadMissingReturnsDefaults(t *testing.T) {
	p := testPaths(t)
	c, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if want := Default(); !reflect.DeepEqual(c, want) {
		t.Fatalf("Load = %#v, want %#v", c, want)
	}
	if c.Defaults.BaseURL != "" {
		t.Errorf("BaseURL = %q, want empty (never pre-fill an endpoint)", c.Defaults.BaseURL)
	}
	if c.Defaults.StartupSlot != "sonnet" {
		t.Errorf("StartupSlot = %q, want sonnet", c.Defaults.StartupSlot)
	}
	if c.Cache.TTLHours != 24 {
		t.Errorf("TTLHours = %d, want 24", c.Cache.TTLHours)
	}
	if c.UI.Theme != "auto" {
		t.Errorf("Theme = %q, want auto", c.UI.Theme)
	}
	if _, err := os.Stat(p.ConfigFile()); !os.IsNotExist(err) {
		t.Errorf("Load created %s (stat err = %v)", p.ConfigFile(), err)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	p := testPaths(t)
	original := &Config{
		Target: Target{Path: filepath.Join(t.TempDir(), "settings.json")},
		Defaults: Defaults{
			BaseURL:     "https://new-api.example.com/",
			Token:       "sk-test-token",
			StartupSlot: "opus",
			Window:      200000,
			Compact:     180000,
		},
		Cache: Cache{TTLHours: 6},
		UI:    UI{Theme: "dark"},
	}
	if err := Save(p, original); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := os.Stat(p.ConfigFile()); err != nil {
		t.Fatalf("config.toml not written: %v", err)
	}

	got, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !reflect.DeepEqual(got, original) {
		t.Fatalf("round trip = %#v, want %#v", got, original)
	}

	// The on-disk shape must stay stable: snake_case tables and keys.
	raw, err := os.ReadFile(p.ConfigFile())
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	text := string(raw)
	for _, want := range []string{
		"[target]", "[defaults]", "[cache]", "[ui]",
		"path =", "base_url =", "token =", "startup_slot =", "window =", "compact =", "ttl_hours =", "theme =",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("config.toml missing %q:\n%s", want, text)
		}
	}
}

func TestSaveReplacesExistingFile(t *testing.T) {
	p := testPaths(t)
	if err := Save(p, Default()); err != nil {
		t.Fatalf("first Save: %v", err)
	}
	c := Default()
	c.UI.Theme = "light"
	if err := Save(p, c); err != nil {
		t.Fatalf("second Save: %v", err)
	}
	got, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.UI.Theme != "light" {
		t.Fatalf("Theme = %q, want light", got.UI.Theme)
	}
	entries, err := os.ReadDir(p.Root)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}
}

func TestSaveNilConfig(t *testing.T) {
	if err := Save(testPaths(t), nil); err == nil {
		t.Fatal("Save(nil) = nil, want error")
	}
}

func TestLoadPartialFileKeepsDefaults(t *testing.T) {
	p := testPaths(t)
	body := "[ui]\ntheme = \"dark\"\n\n[defaults]\nbase_url = \"https://gw.example.com/\"\n"
	if err := store.AtomicWrite(p.ConfigFile(), []byte(body), 0o600); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	c, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.UI.Theme != "dark" {
		t.Errorf("Theme = %q, want dark", c.UI.Theme)
	}
	if c.Defaults.BaseURL != "https://gw.example.com/" {
		t.Errorf("BaseURL = %q", c.Defaults.BaseURL)
	}
	if c.Defaults.StartupSlot != "sonnet" {
		t.Errorf("StartupSlot = %q, want default sonnet", c.Defaults.StartupSlot)
	}
	if c.Cache.TTLHours != 24 {
		t.Errorf("TTLHours = %d, want default 24", c.Cache.TTLHours)
	}
}

func TestLoadEmptySlotFallsBack(t *testing.T) {
	p := testPaths(t)
	if err := store.AtomicWrite(p.ConfigFile(), []byte("[defaults]\nstartup_slot = \"\"\n"), 0o600); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	c, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Defaults.StartupSlot != "sonnet" {
		t.Errorf("StartupSlot = %q, want sonnet", c.Defaults.StartupSlot)
	}
	if err := c.Validate(); err != nil {
		t.Errorf("Validate after Load: %v", err)
	}
}

func TestLoadInvalidTOML(t *testing.T) {
	p := testPaths(t)
	if err := store.AtomicWrite(p.ConfigFile(), []byte("this is not = = toml"), 0o600); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	if _, err := Load(p); err == nil {
		t.Fatal("Load of broken TOML = nil error, want error")
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantErr bool
	}{
		{"defaults", func(*Config) {}, false},
		{"fable", func(c *Config) { c.Defaults.StartupSlot = "fable" }, false},
		{"haiku", func(c *Config) { c.Defaults.StartupSlot = "haiku" }, false},
		{"upper case", func(c *Config) { c.Defaults.StartupSlot = "Opus" }, false},
		{"bad slot", func(c *Config) { c.Defaults.StartupSlot = "gpt-4o" }, true},
		{"empty slot", func(c *Config) { c.Defaults.StartupSlot = "" }, true},
		{"subagent slot", func(c *Config) { c.Defaults.StartupSlot = "subagent" }, true},
		{"negative window", func(c *Config) { c.Defaults.Window = -1 }, true},
		{"negative compact", func(c *Config) { c.Defaults.Compact = -1 }, true},
		{"negative ttl", func(c *Config) { c.Cache.TTLHours = -1 }, true},
		{"zero numerics", func(c *Config) {
			c.Defaults.Window = 0
			c.Defaults.Compact = 0
			c.Cache.TTLHours = 0
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := Default()
			tt.mutate(c)
			err := c.Validate()
			if tt.wantErr && err == nil {
				t.Fatal("Validate = nil, want error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("Validate = %v, want nil", err)
			}
		})
	}
}

func TestSettingsPathOverride(t *testing.T) {
	p := testPaths(t)
	_ = p
	custom := filepath.Join(t.TempDir(), "elsewhere", "settings.json")
	c := Default()
	c.Target.Path = custom
	got, err := c.SettingsPath()
	if err != nil {
		t.Fatalf("SettingsPath: %v", err)
	}
	if got != custom {
		t.Fatalf("SettingsPath = %q, want %q", got, custom)
	}
}

func TestSettingsPathMakesRelativeAbsolute(t *testing.T) {
	c := Default()
	c.Target.Path = filepath.Join("rel", "settings.json")
	got, err := c.SettingsPath()
	if err != nil {
		t.Fatalf("SettingsPath: %v", err)
	}
	if !filepath.IsAbs(got) {
		t.Fatalf("SettingsPath = %q, want an absolute path", got)
	}
	if !strings.HasSuffix(got, filepath.Join("rel", "settings.json")) {
		t.Fatalf("SettingsPath = %q, want it to end with rel/settings.json", got)
	}
}

func TestSettingsPathFallback(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", dir)

	c := Default()
	got, err := c.SettingsPath()
	if err != nil {
		t.Fatalf("SettingsPath: %v", err)
	}
	want := filepath.Join(dir, "settings.json")
	if got != want {
		t.Fatalf("SettingsPath = %q, want %q", got, want)
	}

	// A blank override is the same as no override.
	c.Target.Path = "   "
	got, err = c.SettingsPath()
	if err != nil {
		t.Fatalf("SettingsPath with blank override: %v", err)
	}
	if got != want {
		t.Fatalf("SettingsPath = %q, want %q", got, want)
	}
}

func TestStartupSlot(t *testing.T) {
	tests := []struct {
		in   string
		want group.SlotName
	}{
		{"opus", group.Opus},
		{"FABLE", group.Fable},
		{" haiku ", group.Haiku},
		{"sonnet", group.Sonnet},
		{"", group.Sonnet},
		{"subagent", group.Sonnet},
		{"nonsense", group.Sonnet},
	}
	for _, tt := range tests {
		c := Default()
		c.Defaults.StartupSlot = tt.in
		if got := c.StartupSlot(); got != tt.want {
			t.Errorf("StartupSlot(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
	if got := (*Config)(nil).StartupSlot(); got != group.Sonnet {
		t.Errorf("nil StartupSlot = %q, want sonnet", got)
	}
}
