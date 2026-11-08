package group

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/uncleyumo/ccsc/internal/store"
)

// List returns every group name in dir, sorted.
func List(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("cannot read %s: %w", dir, err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		names = append(names, strings.TrimSuffix(e.Name(), ".json"))
	}
	sort.Strings(names)
	return names, nil
}

// Load reads the named group from dir.
func Load(dir, name string) (*Group, error) {
	if err := ValidateName(name); err != nil {
		return nil, err
	}
	return LoadFile(filepath.Join(dir, name+".json"))
}

// LoadFile reads a group from an explicit path and fills in its Name.
func LoadFile(path string) (*Group, error) {
	data, err := store.ReadFileOrEmpty(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", path, err)
	}
	if data == nil {
		return nil, fmt.Errorf("group %s does not exist", path)
	}
	var g Group
	if err := json.Unmarshal(data, &g); err != nil {
		return nil, fmt.Errorf("%s is not a valid group file: %w", path, err)
	}
	g.Name = strings.TrimSuffix(filepath.Base(path), ".json")
	if err := g.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &g, nil
}

// Save writes the group into dir and stamps UpdatedAt. Group files hold a
// plaintext token, so they are written 0600.
func Save(dir string, g *Group) error {
	if g.Name == "" {
		return errors.New("group has no name")
	}
	if err := ValidateName(g.Name); err != nil {
		return err
	}
	if err := g.Validate(); err != nil {
		return err
	}
	g.Schema = SchemaVersion
	g.UpdatedAt = time.Now()
	data, err := json.MarshalIndent(g, "", "  ")
	if err != nil {
		return fmt.Errorf("cannot encode group: %w", err)
	}
	data = append(data, '\n')
	return store.AtomicWrite(filepath.Join(dir, g.Name+".json"), data, 0o600)
}

// Delete removes the group file. It does not touch anything ccsc already
// wrote into a settings file.
func Delete(dir, name string) error {
	if err := ValidateName(name); err != nil {
		return err
	}
	path := filepath.Join(dir, name+".json")
	if err := os.Remove(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("group %q does not exist", name)
		}
		return fmt.Errorf("cannot delete %s: %w", path, err)
	}
	return nil
}

// Exists reports whether the named group file is present.
func Exists(dir, name string) bool {
	if ValidateName(name) != nil {
		return false
	}
	_, err := os.Stat(filepath.Join(dir, name+".json"))
	return err == nil
}

// ValidateName rejects names that cannot be a safe file stem.
func ValidateName(name string) error {
	if name == "" {
		return errors.New("group name cannot be empty")
	}
	if len(name) > 64 {
		return fmt.Errorf("group name %q is longer than 64 bytes", name)
	}
	if strings.HasPrefix(name, ".") {
		return fmt.Errorf("group name %q cannot start with a dot", name)
	}
	if strings.ContainsAny(name, `/\:*?"<>|`) || strings.Contains(name, "..") {
		return fmt.Errorf("group name %q contains a character that is not allowed in a file name", name)
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("group name %q contains a control character", name)
		}
	}
	switch strings.ToUpper(name) {
	case "CON", "PRN", "AUX", "NUL",
		"COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
		"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		return fmt.Errorf("group name %q is a reserved Windows device name", name)
	}
	return nil
}
