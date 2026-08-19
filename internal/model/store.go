package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"cc-switch-cli/internal/config"
	"cc-switch-cli/internal/fsutil"
)

// FileNameFor returns the on-disk file path for a provider name.
func FileNameFor(name string) (string, error) {
	if err := ValidateName(name); err != nil {
		return "", err
	}
	dir, err := config.ModelsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, name+".json"), nil
}

// List returns provider names by scanning ~/.cc-switch-cli/models/*.json.
// Names are returned sorted alphabetically.
func List() ([]string, error) {
	dir, err := config.ModelsDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("models directory not found at %s: run `cc-switch-cli init`", dir)
		}
		return nil, fmt.Errorf("failed to read models directory: %w", err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		base := strings.TrimSuffix(name, ".json")
		if ValidateName(base) != nil {
			// Silently skip files that don't match our naming rules — user
			// dropped something unusual in there; don't let it break `list`.
			continue
		}
		names = append(names, base)
	}
	sort.Strings(names)
	return names, nil
}

// ReadRaw returns the byte contents of a provider file. Used by `use` and
// `model show` where we want the exact stored bytes.
func ReadRaw(name string) ([]byte, error) {
	path, err := FileNameFor(name)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("model %q not found", name)
		}
		return nil, fmt.Errorf("failed to read model %q: %w", name, err)
	}
	return data, nil
}

// LoadByName parses a provider file into a Model. Prefer ReadRaw when the
// caller wants exact bytes without paying the parse cost.
func LoadByName(name string) (*Model, error) {
	data, err := ReadRaw(name)
	if err != nil {
		return nil, err
	}
	m, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("model %q is not valid JSON: %w", name, err)
	}
	m.Name = name
	return m, nil
}

// Exists reports whether a provider file exists on disk.
func Exists(name string) (bool, error) {
	path, err := FileNameFor(name)
	if err != nil {
		return false, err
	}
	return fsutil.PathExists(path)
}

// Save serializes the model and atomically writes it to its target file.
// The parent directory is created if missing.
func Save(name string, m *Model) error {
	if err := ValidateName(name); err != nil {
		return err
	}
	dir, err := config.ModelsDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create models directory: %w", err)
	}
	if m != nil {
		m.NormalizeContextWindow()
	}
	data, err := m.Indent()
	if err != nil {
		return err
	}
	path := filepath.Join(dir, name+".json")
	return fsutil.AtomicWrite(path, data, 0o644)
}

// Add creates a new provider file. If from is non-empty, it clones an existing
// provider (parsed round-trip to normalise formatting); otherwise it writes a
// fresh Template().
func Add(name string, from string) error {
	if err := ValidateName(name); err != nil {
		return err
	}
	exists, err := Exists(name)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("model %q already exists", name)
	}
	var m *Model
	if from != "" {
		src, err := LoadByName(from)
		if err != nil {
			return fmt.Errorf("failed to clone from %q: %w", from, err)
		}
		src.Name = name
		m = src
	} else {
		m = Template(name)
	}
	return Save(name, m)
}

// Rename moves a provider file. If the current-marker points at old, it is
// updated to new.
func Rename(oldName, newName string) error {
	if err := ValidateName(oldName); err != nil {
		return fmt.Errorf("invalid current name: %w", err)
	}
	if err := ValidateName(newName); err != nil {
		return fmt.Errorf("invalid new name: %w", err)
	}
	if oldName == newName {
		return nil
	}
	oldPath, err := FileNameFor(oldName)
	if err != nil {
		return err
	}
	newPath, err := FileNameFor(newName)
	if err != nil {
		return err
	}
	if _, err := os.Stat(oldPath); errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("model %q not found", oldName)
	}
	if exists, err := fsutil.PathExists(newPath); err != nil {
		return err
	} else if exists {
		return fmt.Errorf("model %q already exists", newName)
	}
	if err := os.Rename(oldPath, newPath); err != nil {
		return fmt.Errorf("failed to rename model: %w", err)
	}
	return nil
}

// Remove deletes a provider file. Returns nil if it already does not exist.
func Remove(name string) error {
	path, err := FileNameFor(name)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("failed to remove model %q: %w", name, err)
	}
	return nil
}

// Template returns a Model shaped like the shipped claude.json sample with
// placeholder values. Useful as the starting point for `model add`.
func Template(name string) *Model {
	rest := map[string]json.RawMessage{
		"model":                             json.RawMessage(`"sonnet"`),
		"enabledPlugins":                    json.RawMessage(`{}`),
		"extraKnownMarketplaces":            json.RawMessage(`{}`),
		"alwaysThinkingEnabled":             json.RawMessage(`false`),
		"skipDangerousModePermissionPrompt": json.RawMessage(`true`),
	}
	return &Model{
		Name: name,
		Env: Env{
			AuthToken:          "your-anthropic-api-key",
			BaseURL:            "https://api.anthropic.com",
			Model:              "claude-sonnet-4-6",
			DefaultHaikuModel:  "claude-haiku-4-5-20251001",
			DefaultOpusModel:   "claude-opus-4-7",
			DefaultSonnetModel: "claude-sonnet-4-6",
		},
		Rest: rest,
	}
}
