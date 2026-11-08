// Package config owns the settings panel persisted to <root>/config.toml.
//
// The panel holds what is true for every group: the global settings.json
// override, the values a new group is seeded with, the model-list cache
// lifetime and the interface theme. Anything that describes a single group
// lives in <root>/groups/<name>.json instead.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/uncleyumo/ccsc/internal/ccstate"
	"github.com/uncleyumo/ccsc/internal/group"
	"github.com/uncleyumo/ccsc/internal/store"
)

// Defaults applied when config.toml is missing or leaves a key out.
const (
	// DefaultStartupSlot is the model slot Claude Code starts on.
	DefaultStartupSlot = "sonnet"
	// DefaultTTLHours is how long a /v1/models snapshot stays fresh.
	DefaultTTLHours = 24
	// DefaultTheme is the "follow the terminal" theme name.
	DefaultTheme = "auto"
)

// Target overrides where ccsc writes the global Claude Code settings.
type Target struct {
	// Path replaces the global settings.json when non-empty. A relative path
	// is resolved against the working directory, a leading ~ is expanded.
	Path string `toml:"path"`
}

// Defaults are the values a new group is seeded with.
type Defaults struct {
	// BaseURL is the default group endpoint. It is deliberately empty: ccsc
	// never pre-fills an Anthropic URL.
	BaseURL string `toml:"base_url"`
	// Token is the default group auth token. It is stored in clear text, so
	// config.toml is written with mode 0600.
	Token string `toml:"token"`
	// StartupSlot is the family slot Claude Code starts on: fable, opus,
	// sonnet or haiku. An unknown value falls back to sonnet (see
	// StartupSlot); Validate rejects it.
	StartupSlot string `toml:"startup_slot"`
	// Window is the default hard context cap in tokens. 0 means no cap is
	// written.
	Window int64 `toml:"window"`
	// Compact is the default auto-compact threshold in tokens. 0 means
	// derive 90% of the effective window.
	Compact int64 `toml:"compact"`
}

// Cache controls how long a gateway model-list snapshot is reused.
type Cache struct {
	// TTLHours is the snapshot lifetime in hours; 0 means every command
	// refetches.
	TTLHours int `toml:"ttl_hours"`
}

// UI holds presentation-only preferences.
type UI struct {
	// Theme is the interface theme name, "auto" by default.
	Theme string `toml:"theme"`
}

// Config is the whole settings panel.
type Config struct {
	Target   Target   `toml:"target"`
	Defaults Defaults `toml:"defaults"`
	Cache    Cache    `toml:"cache"`
	UI       UI       `toml:"ui"`
}

// Default returns the panel used when no config.toml exists.
func Default() *Config {
	return &Config{
		Defaults: Defaults{StartupSlot: DefaultStartupSlot},
		Cache:    Cache{TTLHours: DefaultTTLHours},
		UI:       UI{Theme: DefaultTheme},
	}
}

// Load reads <root>/config.toml. A missing file returns Default with no
// error; keys the file omits keep their default. Undecodable TOML is an error.
func Load(p store.Paths) (*Config, error) {
	path := p.ConfigFile()
	data, err := store.ReadFileOrEmpty(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", store.Shorten(path), err)
	}
	c := Default()
	if len(data) == 0 {
		return c, nil
	}
	if _, err := toml.Decode(string(data), c); err != nil {
		return nil, fmt.Errorf("invalid %s: %w", store.Shorten(path), err)
	}
	c.normalize()
	return c, nil
}

// Save writes <root>/config.toml atomically with mode 0600, because the panel
// can hold a default token.
func Save(p store.Paths, c *Config) error {
	if c == nil {
		return errors.New("config: cannot save a nil config")
	}
	var buf bytes.Buffer
	enc := toml.NewEncoder(&buf)
	enc.Indent = ""
	if err := enc.Encode(c); err != nil {
		return fmt.Errorf("cannot encode config: %w", err)
	}
	path := p.ConfigFile()
	if err := store.AtomicWrite(path, buf.Bytes(), 0o600); err != nil {
		return fmt.Errorf("cannot write %s: %w", store.Shorten(path), err)
	}
	return nil
}

// Validate reports whether the panel is usable. startup_slot must be one of
// the four family slots, and none of the numeric settings may be negative.
func (c *Config) Validate() error {
	if c == nil {
		return errors.New("config: nil config")
	}
	if _, ok := familySlot(c.Defaults.StartupSlot); !ok {
		return fmt.Errorf("config: startup_slot must be one of %s, got %q",
			slotList(), c.Defaults.StartupSlot)
	}
	if c.Defaults.Window < 0 {
		return fmt.Errorf("config: window must not be negative, got %d", c.Defaults.Window)
	}
	if c.Defaults.Compact < 0 {
		return fmt.Errorf("config: compact must not be negative, got %d", c.Defaults.Compact)
	}
	if c.Cache.TTLHours < 0 {
		return fmt.Errorf("config: ttl_hours must not be negative, got %d", c.Cache.TTLHours)
	}
	return nil
}

// SettingsPath is the global settings.json ccsc writes: Target.Path when set,
// otherwise the file Claude Code itself would use.
func (c *Config) SettingsPath() (string, error) {
	if c != nil {
		if v := strings.TrimSpace(c.Target.Path); v != "" {
			return absolutePath(v)
		}
	}
	return ccstate.UserSettingsPath()
}

// StartupSlot is the configured startup slot, or group.Sonnet when the value
// is empty or unknown.
func (c *Config) StartupSlot() group.SlotName {
	if c == nil {
		return group.Sonnet
	}
	if n, ok := familySlot(c.Defaults.StartupSlot); ok {
		return n
	}
	return group.Sonnet
}

// normalize trims stray whitespace and restores the startup-slot default after
// decoding, so a hand-edited empty value does not leave the panel unusable.
func (c *Config) normalize() {
	c.Defaults.BaseURL = strings.TrimSpace(c.Defaults.BaseURL)
	c.Defaults.StartupSlot = strings.TrimSpace(c.Defaults.StartupSlot)
	c.UI.Theme = strings.TrimSpace(c.UI.Theme)
	c.Target.Path = strings.TrimSpace(c.Target.Path)
	if c.Defaults.StartupSlot == "" {
		c.Defaults.StartupSlot = DefaultStartupSlot
	}
	if c.UI.Theme == "" {
		c.UI.Theme = DefaultTheme
	}
}

// familySlot parses s as one of the four family slots. It is case-insensitive
// and rejects the subagent slot, which Claude Code never starts on.
func familySlot(s string) (group.SlotName, bool) {
	n := group.SlotName(strings.ToLower(strings.TrimSpace(s)))
	for _, k := range group.ModelSlots {
		if n == k {
			return n, true
		}
	}
	return "", false
}

// slotList renders the allowed startup slots for error messages.
func slotList() string {
	names := make([]string, len(group.ModelSlots))
	for i, k := range group.ModelSlots {
		names[i] = string(k)
	}
	return strings.Join(names, ", ")
}

// absolutePath expands a leading ~ and makes p absolute.
func absolutePath(p string) (string, error) {
	if p == "~" || strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("config: cannot expand %q: %w", p, err)
		}
		p = filepath.Join(home, p[1:])
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", fmt.Errorf("config: cannot resolve %q: %w", p, err)
	}
	return abs, nil
}
