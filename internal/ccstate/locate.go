package ccstate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ConfigDir mirrors Claude Code's own rule: CLAUDE_CONFIG_DIR when set, else
// ~/.claude.
func ConfigDir() (string, error) {
	if v := strings.TrimSpace(os.Getenv("CLAUDE_CONFIG_DIR")); v != "" {
		abs, err := filepath.Abs(v)
		if err != nil {
			return "", fmt.Errorf("invalid CLAUDE_CONFIG_DIR %q: %w", v, err)
		}
		return abs, nil
	}
	h, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot locate home directory: %w", err)
	}
	return filepath.Join(h, ".claude"), nil
}

// UserSettingsPath is the user layer: <config dir>/settings.json.
func UserSettingsPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "settings.json"), nil
}

// ClaudeJSONPath is where user-scope and local-scope MCP servers live. Note it
// is NOT inside ~/.claude: without CLAUDE_CONFIG_DIR it is ~/.claude.json,
// with it, $CLAUDE_CONFIG_DIR/.claude.json.
func ClaudeJSONPath() (string, error) {
	if v := strings.TrimSpace(os.Getenv("CLAUDE_CONFIG_DIR")); v != "" {
		abs, err := filepath.Abs(v)
		if err != nil {
			return "", fmt.Errorf("invalid CLAUDE_CONFIG_DIR %q: %w", v, err)
		}
		return filepath.Join(abs, ".claude.json"), nil
	}
	h, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot locate home directory: %w", err)
	}
	return filepath.Join(h, ".claude.json"), nil
}

// Project is the directory a Claude Code session runs in, plus the two files
// that belong to it.
type Project struct {
	Dir         string
	LocalFile   string
	ProjectFile string
}

// Key is the absolute path spelled the way ~/.claude.json keys it: forward
// slashes, case untouched.
func (p Project) Key() string { return strings.ReplaceAll(p.Dir, `\`, "/") }

// LocateProject resolves dir (or the current working directory) into a
// Project. It never walks upward: Claude Code reads project settings only from
// the directory the session was started in, and writing anywhere else would
// silently do nothing.
func LocateProject(dir string) (Project, error) {
	if strings.TrimSpace(dir) == "" {
		wd, err := os.Getwd()
		if err != nil {
			return Project{}, fmt.Errorf("cannot determine the working directory: %w", err)
		}
		dir = wd
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return Project{}, fmt.Errorf("invalid directory %q: %w", dir, err)
	}
	abs = filepath.Clean(abs)
	info, err := os.Stat(abs)
	if err != nil {
		return Project{}, fmt.Errorf("cannot open %s: %w", abs, err)
	}
	if !info.IsDir() {
		return Project{}, fmt.Errorf("%s is not a directory", abs)
	}
	return Project{
		Dir:         abs,
		LocalFile:   filepath.Join(abs, ".claude", "settings.local.json"),
		ProjectFile: filepath.Join(abs, ".claude", "settings.json"),
	}, nil
}

// IsUsableProject rejects the home directory and filesystem roots, where a
// project-level configuration makes no sense.
func IsUsableProject(project Project) bool {
	home, err := os.UserHomeDir()
	if err == nil && home != "" && strings.EqualFold(filepath.Clean(project.Dir), filepath.Clean(home)) {
		return false
	}
	vol := filepath.VolumeName(project.Dir)
	clean := filepath.Clean(project.Dir)
	if vol != "" && strings.EqualFold(clean, vol+string(os.PathSeparator)) {
		return false
	}
	if clean == string(os.PathSeparator) {
		return false
	}
	return true
}

// AncestorProjectSettings walks upward from project.Dir and returns the first
// ancestor holding a .claude/settings.json, or "".
//
// This is used only to warn. Claude Code does not read that file when it is
// started in project.Dir, and that mismatch is the most dangerous way a user
// can end up with a configuration that appears applied and is never read.
func AncestorProjectSettings(project Project) string {
	cur := filepath.Dir(project.Dir)
	for {
		candidate := filepath.Join(cur, ".claude", "settings.json")
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return ""
		}
		cur = parent
	}
}
