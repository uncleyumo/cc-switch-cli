package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Config maps the on-disk config.toml.
type Config struct {
	Target TargetConfig `toml:"target"`
}

// TargetConfig describes where the active settings.json lives.
type TargetConfig struct {
	Path string `toml:"path"`
}

// Resolved is a bundle of paths and derived state used by `config show`.
type Resolved struct {
	Home       string
	ConfigDir  string
	ModelsDir  string
	ConfigFile string
	TargetPath string
}

// Home wraps os.UserHomeDir with a stable error message.
func Home() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to resolve user home directory: %w", err)
	}
	if home == "" {
		return "", errors.New("user home directory is empty")
	}
	return home, nil
}

// ConfigDir returns ~/.cc-switch-cli.
func ConfigDir() (string, error) {
	home, err := Home()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".cc-switch-cli"), nil
}

// ModelsDir returns the directory that holds provider JSON files.
func ModelsDir() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "models"), nil
}

// DefaultTargetPath is the shipping default for target.path.
func DefaultTargetPath() (string, error) {
	home, err := Home()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude", "settings.json"), nil
}

// ConfigFilePath returns ~/.cc-switch-cli/config.toml.
func ConfigFilePath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.toml"), nil
}

// Load reads and validates config.toml.
func Load() (*Config, error) {
	path, err := ConfigFilePath()
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("config not found at %s: run `cc-switch-cli init` first", path)
	} else if err != nil {
		return nil, fmt.Errorf("failed to stat config.toml: %w", err)
	}

	var cfg Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config.toml: %w", err)
	}

	if cfg.Target.Path == "" {
		return nil, errors.New("target.path is empty in config.toml")
	}
	return &cfg, nil
}

// WriteDefaultConfig creates config.toml with target.path=DefaultTargetPath().
// The parent directory must already exist.
func WriteDefaultConfig() error {
	path, err := ConfigFilePath()
	if err != nil {
		return err
	}
	target, err := DefaultTargetPath()
	if err != nil {
		return err
	}
	cfg := Config{Target: TargetConfig{Path: target}}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create config.toml: %w", err)
	}
	defer f.Close()

	if err := toml.NewEncoder(f).Encode(cfg); err != nil {
		return fmt.Errorf("failed to write config.toml: %w", err)
	}
	return nil
}

// SetTarget updates config.toml's target.path (creating config.toml if needed).
func SetTarget(path string) error {
	if path == "" {
		return errors.New("target path must not be empty")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("invalid target path: %w", err)
	}

	dir, err := ConfigDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	cfgPath, err := ConfigFilePath()
	if err != nil {
		return err
	}
	cfg := Config{Target: TargetConfig{Path: abs}}
	f, err := os.Create(cfgPath)
	if err != nil {
		return fmt.Errorf("failed to create config.toml: %w", err)
	}
	defer f.Close()
	if err := toml.NewEncoder(f).Encode(cfg); err != nil {
		return fmt.Errorf("failed to write config.toml: %w", err)
	}
	return nil
}

// Resolve gathers every derived path in one call for `config show`.
func Resolve() (*Resolved, error) {
	home, err := Home()
	if err != nil {
		return nil, err
	}
	dir, err := ConfigDir()
	if err != nil {
		return nil, err
	}
	models, err := ModelsDir()
	if err != nil {
		return nil, err
	}
	cfgFile, err := ConfigFilePath()
	if err != nil {
		return nil, err
	}
	target := ""
	if cfg, loadErr := Load(); loadErr == nil {
		target = cfg.Target.Path
	}
	return &Resolved{
		Home:       home,
		ConfigDir:  dir,
		ModelsDir:  models,
		ConfigFile: cfgFile,
		TargetPath: target,
	}, nil
}
