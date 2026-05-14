package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Target TargetConfig `toml:"target"`
}

type TargetConfig struct {
	Path string `toml:"path"`
}

func ConfigDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".cc-switch-cli")
}

func ModelsDir() string {
	return filepath.Join(ConfigDir(), "models")
}

func DefaultTargetPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude", "settings.json")
}

func ConfigFilePath() string {
	return filepath.Join(ConfigDir(), "config.toml")
}

func Load() (*Config, error) {
	path := ConfigFilePath()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("config not found at %s — run `cc-switch-cli init` first", path)
	}

	var cfg Config
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config.toml: %w", err)
	}

	if cfg.Target.Path == "" {
		return nil, fmt.Errorf("target.path is empty in config.toml")
	}

	return &cfg, nil
}

func WriteDefaultConfig() error {
	cfg := Config{
		Target: TargetConfig{
			Path: DefaultTargetPath(),
		},
	}

	f, err := os.Create(ConfigFilePath())
	if err != nil {
		return fmt.Errorf("failed to create config.toml: %w", err)
	}
	defer f.Close()

	if err := toml.NewEncoder(f).Encode(cfg); err != nil {
		return fmt.Errorf("failed to write config.toml: %w", err)
	}

	return nil
}
