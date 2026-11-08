// Package store owns every path ccsc writes and the atomic write primitive
// that all of them go through.
//
// Everything ccsc persists lives under a single root (by default
// ~/.cc-switch-cli). Deleting that directory and the binary leaves no trace.
package store

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// EnvHome overrides the state directory. Tests set it to a temp dir; users
// rarely need it.
const EnvHome = "CCSC_HOME"

// Paths is the fixed layout of the state directory.
type Paths struct {
	Root string
}

// New resolves the state root from the environment.
func New() (Paths, error) {
	root, err := Home()
	if err != nil {
		return Paths{}, err
	}
	return Paths{Root: root}, nil
}

// Home returns the state directory. CCSC_HOME wins over ~/.cc-switch-cli.
func Home() (string, error) {
	if v := strings.TrimSpace(os.Getenv(EnvHome)); v != "" {
		abs, err := filepath.Abs(v)
		if err != nil {
			return "", fmt.Errorf("invalid %s %q: %w", EnvHome, v, err)
		}
		return abs, nil
	}
	h, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot locate home directory: %w", err)
	}
	return filepath.Join(h, ".cc-switch-cli"), nil
}

// ConfigFile is the TOML settings panel.
func (p Paths) ConfigFile() string { return filepath.Join(p.Root, "config.toml") }

// GroupsDir holds one JSON file per group.
func (p Paths) GroupsDir() string { return filepath.Join(p.Root, "groups") }

// GroupFile is the path of a named group.
func (p Paths) GroupFile(name string) string {
	return filepath.Join(p.GroupsDir(), name+".json")
}

// CacheDir holds gateway model-list snapshots.
func (p Paths) CacheDir() string { return filepath.Join(p.Root, "cache", "models") }

// StateFile records what ccsc last applied where.
func (p Paths) StateFile() string { return filepath.Join(p.Root, "state.json") }

// BackupsDir holds timestamped snapshots of every file ccsc overwrites.
func (p Paths) BackupsDir() string { return filepath.Join(p.Root, "backups") }

// AuditFile is the append-only JSONL history.
func (p Paths) AuditFile() string { return filepath.Join(p.Root, "audit.jsonl") }

// Ensure creates every directory the store needs.
func (p Paths) Ensure() error {
	for _, dir := range []string{p.Root, p.GroupsDir(), p.CacheDir(), p.BackupsDir()} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("cannot create %s: %w", dir, err)
		}
	}
	return nil
}

// Shorten replaces the user's home directory with ~ so receipts stay readable.
func Shorten(path string) string {
	h, err := os.UserHomeDir()
	if err != nil || h == "" {
		return path
	}
	if path == h {
		return "~"
	}
	sep := string(os.PathSeparator)
	if strings.HasPrefix(path, h+sep) {
		return "~" + path[len(h):]
	}
	return path
}
