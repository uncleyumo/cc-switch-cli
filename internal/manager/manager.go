package manager

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cc-switch-cli/internal/config"
	"cc-switch-cli/internal/logger"
)

func ListProviders() ([]string, error) {
	modelsDir := config.ModelsDir()

	entries, err := os.ReadDir(modelsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("models directory not found at %s — run `cc-switch-cli init`", modelsDir)
		}
		return nil, fmt.Errorf("failed to read models directory: %w", err)
	}

	var providers []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasSuffix(name, ".json") {
			providers = append(providers, strings.TrimSuffix(name, ".json"))
		}
	}

	return providers, nil
}

func SwitchProvider(provider string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	providerFile := filepath.Join(config.ModelsDir(), provider+".json")
	if _, err := os.Stat(providerFile); os.IsNotExist(err) {
		return fmt.Errorf("provider %q not found in models/ directory", provider)
	}

	if err := ensureFirstTimeBackup(cfg.Target.Path); err != nil {
		return err
	}

	os.Remove(cfg.Target.Path)

	data, err := os.ReadFile(providerFile)
	if err != nil {
		return fmt.Errorf("failed to read provider config: %w", err)
	}

	if err := os.WriteFile(cfg.Target.Path, data, 0666); err != nil {
		return fmt.Errorf("failed to write settings: %w", err)
	}

	if err := updateCurrentMarker(provider); err != nil {
		return err
	}

	if err := logger.LogSwitch(provider); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to write log: %v\n", err)
	}

	return nil
}

func GetCurrentProvider() (string, error) {
	matches, err := filepath.Glob(filepath.Join(config.ConfigDir(), "*.current"))
	if err != nil {
		return "", fmt.Errorf("failed to search for current marker: %w", err)
	}

	if len(matches) == 0 {
		return "", nil
	}

	name := filepath.Base(matches[0])
	name = strings.TrimSuffix(name, ".current")
	return name, nil
}

func BackupSettings() (string, error) {
	cfg, err := config.Load()
	if err != nil {
		return "", err
	}

	data, err := os.ReadFile(cfg.Target.Path)
	if err != nil {
		return "", fmt.Errorf("failed to read current settings: %w", err)
	}

	backupName := fmt.Sprintf("backup-%s.json", timestamp())
	backupPath := filepath.Join(config.ConfigDir(), backupName)

	if err := os.WriteFile(backupPath, data, 0666); err != nil {
		return "", fmt.Errorf("failed to write backup: %w", err)
	}

	return backupPath, nil
}

func ensureFirstTimeBackup(targetPath string) error {
	defaultPath := filepath.Join(config.ConfigDir(), "default.json")
	if _, err := os.Stat(defaultPath); err == nil {
		return nil
	}

	data, err := os.ReadFile(targetPath)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("Warning: no existing settings.json to back up")
			return nil
		}
		return fmt.Errorf("failed to read current settings for backup: %w", err)
	}

	if err := os.WriteFile(defaultPath, data, 0666); err != nil {
		return fmt.Errorf("failed to create default backup: %w", err)
	}

	tsName := fmt.Sprintf("default-%s.json", timestamp())
	tsPath := filepath.Join(config.ConfigDir(), tsName)
	if err := os.WriteFile(tsPath, data, 0666); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to create timestamped backup: %v\n", err)
	}

	fmt.Println("Backed up original settings as default.json")
	return nil
}

func updateCurrentMarker(provider string) error {
	matches, _ := filepath.Glob(filepath.Join(config.ConfigDir(), "*.current"))
	for _, m := range matches {
		os.Remove(m)
	}

	markerPath := filepath.Join(config.ConfigDir(), provider+".current")
	return os.WriteFile(markerPath, []byte(provider), 0666)
}

func GetProviderConfig(provider string) (string, error) {
	providerFile := filepath.Join(config.ModelsDir(), provider+".json")
	data, err := os.ReadFile(providerFile)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("provider %q not found in models/ directory", provider)
		}
		return "", fmt.Errorf("failed to read provider config: %w", err)
	}
	return string(data), nil
}

func CreateLocalConfig(provider string, path string, force bool) error {
	providerFile := filepath.Join(config.ModelsDir(), provider+".json")
	data, err := os.ReadFile(providerFile)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("provider %q not found in models/ directory", provider)
		}
		return fmt.Errorf("failed to read provider config: %w", err)
	}

	var targetDir, targetFile string
	if path == "" {
		targetDir, _ = os.Getwd()
		targetFile = "settings.local.json"
	} else {
		absPath, err := filepath.Abs(path)
		if err != nil {
			return fmt.Errorf("invalid path: %w", err)
		}
		if isFilePath(absPath) {
			targetDir = filepath.Dir(absPath)
			targetFile = filepath.Base(absPath)
		} else {
			targetDir = absPath
			targetFile = "settings.local.json"
		}
	}

	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	targetPath := filepath.Join(targetDir, targetFile)
	if !force {
		if _, err := os.Stat(targetPath); err == nil {
			return fmt.Errorf("file %s already exists, use --force to overwrite", targetPath)
		}
	}

	if err := os.WriteFile(targetPath, data, 0666); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	fmt.Printf("Created %s with %s provider config\n", targetPath, provider)
	return nil
}

func isFilePath(path string) bool {
	ext := filepath.Ext(path)
	return ext != ""
}

func timestamp() string {
	return time.Now().Format("20060102-150405")
}
