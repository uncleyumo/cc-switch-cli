package manager

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"cc-switch-cli/internal/fsutil"
	"cc-switch-cli/internal/mcp"
	"cc-switch-cli/internal/model"
)

// LocalOptions controls how `local` writes, inspects or clears the
// project-scoped settings file.
type LocalOptions struct {
	// Path optionally overrides the target location. When empty the default
	// is <cwd>/.claude/settings.local.json. When Path names a directory, the
	// default filename is appended inside a .claude subdirectory. Otherwise
	// Path is treated as the exact target file.
	Path string
	// Overwrite skips deep-merge and writes the provider file verbatim.
	Overwrite bool
	// ModelOnly, used by ClearLocalConfig, strips provider model/env keys
	// instead of deleting the file.
	ModelOnly bool
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

// ResolveLocalProjectDir picks the project root used for MCP listing.
// Empty path → cwd. A directory (or trailing slash) is used as-is. A file
// named .claude/settings.local.json maps to that file's project root;
// any other file maps to its parent directory.
func ResolveLocalProjectDir(path string) (string, error) {
	if path == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("failed to determine working directory: %w", err)
		}
		return cwd, nil
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("invalid --path %q: %w", path, err)
	}
	trailing := strings.HasSuffix(path, string(os.PathSeparator)) || strings.HasSuffix(path, "/")
	isDir := false
	if info, err := os.Stat(abs); err == nil {
		isDir = info.IsDir()
	}
	if trailing || isDir {
		return abs, nil
	}
	dir := filepath.Dir(abs)
	if strings.EqualFold(filepath.Base(abs), "settings.local.json") &&
		strings.EqualFold(filepath.Base(dir), ".claude") {
		return filepath.Dir(dir), nil
	}
	return dir, nil
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

// ClearOutcome describes what ClearLocalConfig did.
type ClearOutcome struct {
	Path      string
	Missing   bool
	Deleted   bool
	ModelOnly bool
}

// ClearLocalConfig removes the project-local settings file, or — when
// opts.ModelOnly is set — strips the provider-owned model/env keys and
// keeps everything else. A missing file is not an error (Missing=true).
// If model-only stripping leaves an empty document the file is deleted.
func ClearLocalConfig(opts LocalOptions) (ClearOutcome, error) {
	target, err := ResolveLocalTarget(opts.Path)
	if err != nil {
		return ClearOutcome{}, err
	}
	out := ClearOutcome{Path: target, ModelOnly: opts.ModelOnly}

	exists, err := fsutil.PathExists(target)
	if err != nil {
		return out, err
	}
	if !exists {
		out.Missing = true
		return out, nil
	}

	if !opts.ModelOnly {
		if err := os.Remove(target); err != nil {
			return out, fmt.Errorf("failed to remove %s: %w", target, err)
		}
		out.Deleted = true
		return out, nil
	}

	data, err := os.ReadFile(target)
	if err != nil {
		return out, fmt.Errorf("failed to read %s: %w", target, err)
	}
	stripped, emptied, err := model.StripModelKeys(data)
	if err != nil {
		return out, err
	}
	if emptied {
		if err := os.Remove(target); err != nil {
			return out, fmt.Errorf("failed to remove emptied %s: %w", target, err)
		}
		out.Deleted = true
		return out, nil
	}
	if err := fsutil.AtomicWrite(target, stripped, 0o644); err != nil {
		return out, err
	}
	return out, nil
}

// LocalSnapshot is the project-scoped picture printed by `local show`.
type LocalSnapshot struct {
	SettingsPath   string
	SettingsExists bool
	ParseError     string

	Model            *model.Model
	HasModelOverride bool
	OtherKeys        []string
	ExtraEnv         []string
	// MatchedProviders lists store groups whose credentials match this file's
	// env block (see matchLocalProviders). Empty when nothing matches.
	MatchedProviders []string

	ProjectDir     string
	ProjectMCPPath string
	ProjectMCP     []mcp.Entry
	LocalMCP       []mcp.Entry
}

// InspectLocalConfig reads the project-local settings file and the
// project / local MCP scopes for the same directory.
func InspectLocalConfig(opts LocalOptions) (*LocalSnapshot, error) {
	target, err := ResolveLocalTarget(opts.Path)
	if err != nil {
		return nil, err
	}
	projectDir, err := ResolveLocalProjectDir(opts.Path)
	if err != nil {
		return nil, err
	}
	snap := &LocalSnapshot{
		SettingsPath:   target,
		ProjectDir:     projectDir,
		ProjectMCPPath: filepath.Join(projectDir, ".mcp.json"),
	}

	exists, err := fsutil.PathExists(target)
	if err != nil {
		return nil, err
	}
	snap.SettingsExists = exists
	if exists {
		data, err := os.ReadFile(target)
		if err != nil {
			return nil, fmt.Errorf("failed to read %s: %w", target, err)
		}
		if err := populateSettings(snap, data); err != nil {
			snap.ParseError = err.Error()
		} else {
			snap.MatchedProviders = matchLocalProviders(snap.Model)
		}
	}

	projectEntries, err := mcp.List(mcp.ScopeProject, projectDir)
	if err != nil {
		return nil, err
	}
	snap.ProjectMCP = projectEntries

	localEntries, err := mcp.List(mcp.ScopeLocal, projectDir)
	if err != nil {
		return nil, err
	}
	snap.LocalMCP = localEntries

	return snap, nil
}

func populateSettings(snap *LocalSnapshot, data []byte) error {
	m, err := model.Parse(data)
	if err != nil {
		return err
	}
	snap.Model = m

	for k := range m.Rest {
		if k != "model" {
			snap.OtherKeys = append(snap.OtherKeys, k)
		}
	}
	sort.Strings(snap.OtherKeys)

	for k := range m.Env.Extra {
		if model.IsModelEnvKey(k) {
			snap.HasModelOverride = true
			continue
		}
		snap.ExtraEnv = append(snap.ExtraEnv, k)
	}
	sort.Strings(snap.ExtraEnv)

	if m.TopLevelModel() != "" ||
		m.Env.AuthToken != "" ||
		m.Env.APIKey != "" ||
		m.Env.BaseURL != "" ||
		m.Env.Model != "" ||
		m.Env.DefaultHaikuModel != "" ||
		m.Env.DefaultOpusModel != "" ||
		m.Env.DefaultSonnetModel != "" ||
		m.Env.MaxContextTokens != "" ||
		m.Env.AutoCompactWindow != "" ||
		m.Env.AutoCompactWindowTokens != "" {
		snap.HasModelOverride = true
	}
	return nil
}

// matchLocalProviders scans the cc-switch-cli provider store and returns the
// group names whose credentials match the given settings env block. A group
// matches when its base URL and token both equal the file's; when neither side
// carries a token, equality of the resolved model id is accepted instead.
// Store read/parse failures are swallowed — grouping is a display aid, not a
// precondition, so `local show` must not fail over it. Sorted by model.List.
func matchLocalProviders(m *model.Model) []string {
	if m == nil {
		return nil
	}
	names, err := model.List()
	if err != nil {
		return nil
	}
	var matched []string
	for _, name := range names {
		p, err := model.LoadByName(name)
		if err != nil {
			continue
		}
		if sameCredentials(p, m) {
			matched = append(matched, name)
		}
	}
	return matched
}

func sameCredentials(a, b *model.Model) bool {
	if a == nil || b == nil {
		return false
	}
	if a.BaseURL() != b.BaseURL() {
		return false
	}
	at, bt := a.Token(), b.Token()
	if at != "" || bt != "" {
		return at != "" && at == bt
	}
	id := a.ResolvedModelID()
	return id != "" && id == b.ResolvedModelID()
}
