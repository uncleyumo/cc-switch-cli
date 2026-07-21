package manager

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"cc-switch-cli/internal/fsutil"
	"cc-switch-cli/internal/model"
)

// LocalOptions controls how `local` writes the project-scoped settings file.
type LocalOptions struct {
	// Path optionally overrides the target location. When empty the default
	// is <cwd>/.claude/settings.local.json. When Path names a directory, the
	// default filename is appended inside a .claude subdirectory. Otherwise
	// Path is treated as the exact target file.
	Path string
	// Overwrite skips deep-merge and writes the provider file verbatim.
	Overwrite bool
}

// DefaultLocalPath returns the standard target inside the current working dir.
func DefaultLocalPath() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to determine working directory: %w", err)
	}
	return filepath.Join(cwd, ".claude", "settings.local.json"), nil
}

// ResolveLocalTarget picks the final on-disk path for a `local` invocation.
// See LocalOptions.Path for the rules.
func ResolveLocalTarget(path string) (string, error) {
	if path == "" {
		return DefaultLocalPath()
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("invalid --path %q: %w", path, err)
	}
	// Trailing separator or an existing directory → treat as a directory and
	// append the standard filename inside .claude/.
	trailing := strings.HasSuffix(path, string(os.PathSeparator)) || strings.HasSuffix(path, "/")
	isDir := false
	if info, err := os.Stat(abs); err == nil {
		isDir = info.IsDir()
	}
	if trailing || isDir {
		return filepath.Join(abs, ".claude", "settings.local.json"), nil
	}
	return abs, nil
}

// CreateLocalConfig applies the LocalOptions and writes the target file.
// Returns the final target path (useful for user-facing messages).
func CreateLocalConfig(name string, opts LocalOptions) (string, error) {
	provider, err := model.LoadByName(name)
	if err != nil {
		return "", err
	}
	target, err := ResolveLocalTarget(opts.Path)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return "", fmt.Errorf("failed to create %s: %w", filepath.Dir(target), err)
	}
	var out []byte
	if opts.Overwrite {
		raw, err := model.ReadRaw(name)
		if err != nil {
			return "", err
		}
		out = raw
	} else {
		existing, err := os.ReadFile(target)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("failed to read %s: %w", target, err)
		}
		merged, err := model.MergeProviderInto(existing, provider)
		if err != nil {
			return "", err
		}
		out = merged
	}
	if err := fsutil.AtomicWrite(target, out, 0o644); err != nil {
		return "", err
	}
	return target, nil
}
