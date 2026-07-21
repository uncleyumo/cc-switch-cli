// Package manager is the orchestration layer between commands and disk. It
// wraps model IO, atomic file replacement of the target settings, current-
// provider tracking and audit logging.
package manager

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"cc-switch-cli/internal/config"
	"cc-switch-cli/internal/fsutil"
	"cc-switch-cli/internal/logger"
	"cc-switch-cli/internal/model"
)

// ListModels returns every provider name known to the store.
func ListModels() ([]string, error) {
	return model.List()
}

// SwitchProvider replaces the target settings file with a byte-copy of the
// provider file, updates the current marker and appends to the audit log.
// Byte-copy is intentional: it keeps the global switch hot-path lossless and
// independent of the Model parser.
func SwitchProvider(name string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	data, err := model.ReadRaw(name)
	if err != nil {
		return err
	}
	if err := ensureFirstTimeBackup(cfg.Target.Path); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(cfg.Target.Path), 0o755); err != nil {
		return fmt.Errorf("failed to create target directory: %w", err)
	}
	if err := fsutil.AtomicWrite(cfg.Target.Path, data, 0o644); err != nil {
		return fmt.Errorf("failed to update target settings: %w", err)
	}
	if err := updateCurrentMarker(name); err != nil {
		return fmt.Errorf("target updated but failed to record current provider: %w", err)
	}
	if err := logger.LogSwitch(name); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to write log: %v\n", err)
	}
	return nil
}

// GetCurrentProvider returns the name of the active provider, or "" if none is
// recorded yet.
func GetCurrentProvider() (string, error) {
	dir, err := config.ConfigDir()
	if err != nil {
		return "", err
	}
	matches, err := filepath.Glob(filepath.Join(dir, "*.current"))
	if err != nil {
		return "", fmt.Errorf("failed to search for current marker: %w", err)
	}
	if len(matches) == 0 {
		return "", nil
	}
	base := filepath.Base(matches[0])
	return strings.TrimSuffix(base, ".current"), nil
}

// BackupSettings copies the current target settings to
// ~/.cc-switch-cli/backup-YYYYMMDD-HHMMSS.json and returns its path.
func BackupSettings() (string, error) {
	cfg, err := config.Load()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(cfg.Target.Path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("no settings file at %s to back up", cfg.Target.Path)
		}
		return "", fmt.Errorf("failed to read current settings: %w", err)
	}
	dir, err := config.ConfigDir()
	if err != nil {
		return "", err
	}
	backupPath := filepath.Join(dir, fmt.Sprintf("backup-%s.json", timestamp()))
	if err := fsutil.AtomicWrite(backupPath, data, 0o644); err != nil {
		return "", fmt.Errorf("failed to write backup: %w", err)
	}
	return backupPath, nil
}

// ensureFirstTimeBackup snapshots the pre-first-switch settings to
// default.json (and a timestamped twin) so users can always recover.
func ensureFirstTimeBackup(targetPath string) error {
	dir, err := config.ConfigDir()
	if err != nil {
		return err
	}
	defaultPath := filepath.Join(dir, "default.json")
	if _, err := os.Stat(defaultPath); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to stat default backup: %w", err)
	}

	data, err := os.ReadFile(targetPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fmt.Fprintln(os.Stderr, "note: no existing settings.json to back up")
			return nil
		}
		return fmt.Errorf("failed to read current settings for backup: %w", err)
	}
	if err := fsutil.AtomicWrite(defaultPath, data, 0o644); err != nil {
		return fmt.Errorf("failed to create default backup: %w", err)
	}
	tsPath := filepath.Join(dir, fmt.Sprintf("default-%s.json", timestamp()))
	if err := fsutil.AtomicWrite(tsPath, data, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to create timestamped backup: %v\n", err)
	}
	fmt.Fprintln(os.Stderr, "note: backed up original settings as default.json")
	return nil
}

// updateCurrentMarker enforces the "at most one *.current file" invariant.
func updateCurrentMarker(provider string) error {
	dir, err := config.ConfigDir()
	if err != nil {
		return err
	}
	matches, _ := filepath.Glob(filepath.Join(dir, "*.current"))
	for _, m := range matches {
		_ = os.Remove(m)
	}
	markerPath := filepath.Join(dir, provider+".current")
	if err := os.WriteFile(markerPath, []byte(provider), 0o644); err != nil {
		return fmt.Errorf("failed to write current marker: %w", err)
	}
	return nil
}

// GetProviderRaw returns the raw bytes of a provider file.
func GetProviderRaw(name string) ([]byte, error) {
	return model.ReadRaw(name)
}

// clearCurrentIf clears the current marker if it points at the given name.
// Used by delete/rename to keep state consistent.
func clearCurrentIf(name string) error {
	current, err := GetCurrentProvider()
	if err != nil {
		return err
	}
	if current != name {
		return nil
	}
	dir, err := config.ConfigDir()
	if err != nil {
		return err
	}
	return os.Remove(filepath.Join(dir, name+".current"))
}

// DeleteProvider removes a provider file and clears the current marker if it
// pointed at that provider.
func DeleteProvider(name string) error {
	if err := clearCurrentIf(name); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return model.Remove(name)
}

// RenameProvider renames a provider file and updates the current marker if it
// pointed at the old name.
func RenameProvider(oldName, newName string) error {
	current, err := GetCurrentProvider()
	if err != nil {
		return err
	}
	if err := model.Rename(oldName, newName); err != nil {
		return err
	}
	if current == oldName {
		if err := updateCurrentMarker(newName); err != nil {
			return fmt.Errorf("renamed but failed to update current marker: %w", err)
		}
	}
	return nil
}

func timestamp() string {
	return time.Now().Format("20060102-150405")
}
