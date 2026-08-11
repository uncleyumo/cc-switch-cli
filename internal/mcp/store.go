package mcp

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"cc-switch-cli/internal/config"
	"cc-switch-cli/internal/fsutil"
)

// ErrNoClaudeConfig reports that ~/.claude.json does not exist yet.
var ErrNoClaudeConfig = errors.New("no Claude Code config found")

// ClaudeJSONPath returns the Claude Code state file path. CLAUDE_CONFIG_DIR
// relocates it, mirroring Claude Code itself.
func ClaudeJSONPath() (string, error) {
	if dir := os.Getenv("CLAUDE_CONFIG_DIR"); dir != "" {
		return filepath.Join(dir, ".claude.json"), nil
	}
	home, err := config.Home()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".claude.json"), nil
}

// NormalizeProjectDir renders a directory the way Claude Code keys its
// projects map: absolute, forward slashes, upper-case drive letter.
func NormalizeProjectDir(dir string) string {
	abs := dir
	if !filepath.IsAbs(abs) {
		if a, err := filepath.Abs(abs); err == nil {
			abs = a
		}
	}
	s := filepath.ToSlash(filepath.Clean(abs))
	if len(s) >= 2 && s[1] == ':' && s[0] >= 'a' && s[0] <= 'z' {
		s = strings.ToUpper(s[:1]) + s[1:]
	}
	return s
}

// List returns the entries of one scope sorted by name. projectDir is
// required for local and project scopes. A missing file or section yields an
// empty list.
func List(scope Scope, projectDir string) ([]Entry, error) {
	switch scope {
	case ScopeUser:
		doc, err := loadClaudeDoc()
		if errors.Is(err, ErrNoClaudeConfig) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		return entriesFromRaw(rawObject(doc, "mcpServers"), ScopeUser), nil
	case ScopeLocal:
		if strings.TrimSpace(projectDir) == "" {
			return nil, errors.New("local scope needs a project directory")
		}
		doc, err := loadClaudeDoc()
		if errors.Is(err, ErrNoClaudeConfig) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		return entriesFromRaw(localServers(doc, projectDir), ScopeLocal), nil
	case ScopeProject:
		if strings.TrimSpace(projectDir) == "" {
			return nil, errors.New("project scope needs a project directory")
		}
		raw, err := readMCPJSON(projectDir)
		if err != nil {
			return nil, err
		}
		return entriesFromRaw(rawObject(raw, "mcpServers"), ScopeProject), nil
	}
	return nil, fmt.Errorf("unknown scope %q", scope)
}

// ListAll returns user, local and project entries ordered by scope precedence
// then name.
func ListAll(projectDir string) ([]Entry, error) {
	var out []Entry
	for _, sc := range AllScopes {
		entries, err := List(sc, projectDir)
		if err != nil {
			return nil, err
		}
		out = append(out, entries...)
	}
	return out, nil
}

// Get returns one server from a scope.
func Get(scope Scope, projectDir, name string) (*Server, error) {
	entries, err := List(scope, projectDir)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.Name == name {
			return e.Server, nil
		}
	}
	return nil, fmt.Errorf("mcp server %q not found in %s scope", name, scope)
}

// Resolve locates a server by name across all scopes. Exactly one hit wins;
// several hits ask for an explicit scope.
func Resolve(projectDir, name string) (*Entry, error) {
	var found []Entry
	for _, sc := range AllScopes {
		srv, err := Get(sc, projectDir, name)
		if err != nil {
			continue
		}
		found = append(found, Entry{Name: name, Scope: sc, Server: srv})
	}
	switch len(found) {
	case 0:
		return nil, fmt.Errorf("mcp server %q not found in any scope", name)
	case 1:
		return &found[0], nil
	}
	scopes := make([]string, len(found))
	for i, f := range found {
		scopes[i] = string(f.Scope)
	}
	return nil, fmt.Errorf("mcp server %q exists in multiple scopes (%s); disambiguate with --scope", name, strings.Join(scopes, ", "))
}

// Set writes one server into a scope, creating structure as needed. Existing
// entries are replaced.
func Set(scope Scope, projectDir, name string, srv *Server) error {
	if err := ValidateName(name); err != nil {
		return err
	}
	if srv == nil {
		return errors.New("empty server entry")
	}
	srv.NormalizeType()
	if err := srv.Validate(); err != nil {
		return err
	}
	data, err := json.Marshal(srv)
	if err != nil {
		return fmt.Errorf("failed to encode server: %w", err)
	}
	switch scope {
	case ScopeUser:
		return setClaudeEntry("", name, data)
	case ScopeLocal:
		if strings.TrimSpace(projectDir) == "" {
			return errors.New("local scope needs a project directory")
		}
		return setClaudeEntry(projectDir, name, data)
	case ScopeProject:
		if strings.TrimSpace(projectDir) == "" {
			return errors.New("project scope needs a project directory")
		}
		return setMCPJSONEntry(projectDir, name, data)
	}
	return fmt.Errorf("unknown scope %q", scope)
}

// Remove deletes one server from a scope. Absent entries are an error.
func Remove(scope Scope, projectDir, name string) error {
	if err := ValidateName(name); err != nil {
		return err
	}
	switch scope {
	case ScopeUser, ScopeLocal:
		dir := ""
		if scope == ScopeLocal {
			if strings.TrimSpace(projectDir) == "" {
				return errors.New("local scope needs a project directory")
			}
			dir = projectDir
		}
		return removeClaudeEntry(dir, name)
	case ScopeProject:
		if strings.TrimSpace(projectDir) == "" {
			return errors.New("project scope needs a project directory")
		}
		return removeMCPJSONEntry(projectDir, name)
	}
	return fmt.Errorf("unknown scope %q", scope)
}

// NormalizeType maps accepted aliases onto canonical values. Claude Code
// accepts "streamable-http" but writes "http".
func (s *Server) NormalizeType() {
	if s == nil {
		return
	}
	if strings.EqualFold(s.Type, "streamable-http") {
		s.Type = "http"
	}
}

// --- ~/.claude.json mechanics ---

func loadClaudeDoc() (map[string]json.RawMessage, error) {
	path, err := ClaudeJSONPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s (run Claude Code once to create it)", ErrNoClaudeConfig, path)
		}
		return nil, fmt.Errorf("failed to read %s: %w", path, err)
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", path, err)
	}
	return doc, nil
}

func saveClaudeDoc(doc map[string]json.RawMessage) error {
	path, err := ClaudeJSONPath()
	if err != nil {
		return err
	}
	if err := backupClaudeJSON(path); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to back up %s: %v\n", path, err)
	}
	data, err := marshalDoc(doc)
	if err != nil {
		return err
	}
	if err := fsutil.AtomicWrite(path, data, 0o600); err != nil {
		return fmt.Errorf("failed to update %s: %w", path, err)
	}
	return nil
}

func marshalDoc(doc map[string]json.RawMessage) ([]byte, error) {
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to encode config: %w", err)
	}
	return append(data, '\n'), nil
}

const backupKeep = 5

// backupClaudeJSON snapshots the state file before a mutation. Called
// best-effort: the caller warns on failure and proceeds.
func backupClaudeJSON(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	dir, err := config.ConfigDir()
	if err != nil {
		return err
	}
	bdir := filepath.Join(dir, "backups")
	if err := os.MkdirAll(bdir, 0o755); err != nil {
		return fmt.Errorf("failed to create backup directory: %w", err)
	}
	name := fmt.Sprintf("claude-json-%s.json", time.Now().Format("20060102-150405"))
	if err := fsutil.AtomicWrite(filepath.Join(bdir, name), data, 0o644); err != nil {
		return err
	}
	pruneClaudeBackups(bdir)
	return nil
}

func pruneClaudeBackups(dir string) {
	dirEntries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	var names []string
	for _, e := range dirEntries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), "claude-json-") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	for _, n := range names[:max(0, len(names)-backupKeep)] {
		_ = os.Remove(filepath.Join(dir, n))
	}
}

// projectKeyIn finds the projects-map key for a directory. Keys compare
// exactly, so an existing key is matched case-insensitively before falling
// back to the normalized form (Windows drive-letter case differs across
// tools, GH anthropics/claude-code#45195).
func projectKeyIn(projects map[string]json.RawMessage, projectDir string) (string, bool) {
	want := NormalizeProjectDir(projectDir)
	if _, ok := projects[want]; ok {
		return want, true
	}
	for k := range projects {
		if strings.EqualFold(k, want) {
			return k, true
		}
	}
	return "", false
}

func localServers(doc map[string]json.RawMessage, projectDir string) map[string]json.RawMessage {
	projects := rawObject(doc, "projects")
	key, ok := projectKeyIn(projects, projectDir)
	if !ok {
		return nil
	}
	entry := map[string]json.RawMessage{}
	if err := json.Unmarshal(projects[key], &entry); err != nil {
		return nil
	}
	return rawObject(entry, "mcpServers")
}

// setClaudeEntry writes into the user scope when projectDir is empty,
// otherwise into projects.<projectDir>.mcpServers.
func setClaudeEntry(projectDir, name string, data json.RawMessage) error {
	doc, err := loadClaudeDoc()
	if err != nil {
		return err
	}
	if projectDir == "" {
		servers := rawObject(doc, "mcpServers")
		servers[name] = data
		if doc["mcpServers"], err = json.Marshal(servers); err != nil {
			return fmt.Errorf("failed to encode mcpServers: %w", err)
		}
		return saveClaudeDoc(doc)
	}

	projects := rawObject(doc, "projects")
	key, ok := projectKeyIn(projects, projectDir)
	if !ok {
		key = NormalizeProjectDir(projectDir)
	}
	entry := map[string]json.RawMessage{}
	if raw, exists := projects[key]; exists {
		if err := json.Unmarshal(raw, &entry); err != nil {
			return fmt.Errorf("failed to parse project entry %q: %w", key, err)
		}
	}
	servers := rawObject(entry, "mcpServers")
	servers[name] = data
	entryJSON, err := json.Marshal(servers)
	if err != nil {
		return fmt.Errorf("failed to encode mcpServers: %w", err)
	}
	entry["mcpServers"] = entryJSON
	if projects[key], err = json.Marshal(entry); err != nil {
		return fmt.Errorf("failed to encode project entry: %w", err)
	}
	if doc["projects"], err = json.Marshal(projects); err != nil {
		return fmt.Errorf("failed to encode projects: %w", err)
	}
	return saveClaudeDoc(doc)
}

func removeClaudeEntry(projectDir, name string) error {
	doc, err := loadClaudeDoc()
	if err != nil {
		return err
	}
	scope := ScopeUser
	var servers map[string]json.RawMessage
	var writeBack func() error

	if projectDir == "" {
		servers = rawObject(doc, "mcpServers")
		writeBack = func() error {
			var err error
			doc["mcpServers"], err = json.Marshal(servers)
			return err
		}
	} else {
		scope = ScopeLocal
		projects := rawObject(doc, "projects")
		key, ok := projectKeyIn(projects, projectDir)
		if !ok {
			return fmt.Errorf("mcp server %q not found in local scope", name)
		}
		entry := map[string]json.RawMessage{}
		if err := json.Unmarshal(projects[key], &entry); err != nil {
			return fmt.Errorf("failed to parse project entry %q: %w", key, err)
		}
		servers = rawObject(entry, "mcpServers")
		writeBack = func() error {
			serversJSON, err := json.Marshal(servers)
			if err != nil {
				return err
			}
			entry["mcpServers"] = serversJSON
			entryJSON, err := json.Marshal(entry)
			if err != nil {
				return err
			}
			projects[key] = entryJSON
			doc["projects"], err = json.Marshal(projects)
			return err
		}
	}

	if _, ok := servers[name]; !ok {
		return fmt.Errorf("mcp server %q not found in %s scope", name, scope)
	}
	delete(servers, name)
	if err := writeBack(); err != nil {
		return fmt.Errorf("failed to encode config: %w", err)
	}
	return saveClaudeDoc(doc)
}

// --- .mcp.json mechanics ---

func mcpJSONPath(projectDir string) string {
	return filepath.Join(projectDir, ".mcp.json")
}

// readMCPJSON loads the project-scope file. A missing file is an empty map,
// not an error; malformed JSON is.
func readMCPJSON(projectDir string) (map[string]json.RawMessage, error) {
	data, err := os.ReadFile(mcpJSONPath(projectDir))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read .mcp.json: %w", err)
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf(".mcp.json is malformed (not valid JSON): %w", err)
	}
	return doc, nil
}

func setMCPJSONEntry(projectDir, name string, data json.RawMessage) error {
	doc, err := readMCPJSON(projectDir)
	if err != nil {
		return err
	}
	if doc == nil {
		doc = map[string]json.RawMessage{}
	}
	servers := rawObject(doc, "mcpServers")
	servers[name] = data
	if doc["mcpServers"], err = json.Marshal(servers); err != nil {
		return fmt.Errorf("failed to encode mcpServers: %w", err)
	}
	out, err := marshalDoc(doc)
	if err != nil {
		return err
	}
	return fsutil.AtomicWrite(mcpJSONPath(projectDir), out, 0o644)
}

func removeMCPJSONEntry(projectDir, name string) error {
	doc, err := readMCPJSON(projectDir)
	if err != nil {
		return err
	}
	servers := rawObject(doc, "mcpServers")
	if _, ok := servers[name]; !ok {
		return fmt.Errorf("mcp server %q not found in project scope", name)
	}
	delete(servers, name)
	if doc == nil {
		doc = map[string]json.RawMessage{}
	}
	if doc["mcpServers"], err = json.Marshal(servers); err != nil {
		return fmt.Errorf("failed to encode mcpServers: %w", err)
	}
	out, err := marshalDoc(doc)
	if err != nil {
		return err
	}
	return fsutil.AtomicWrite(mcpJSONPath(projectDir), out, 0o644)
}

// --- shared helpers ---

// rawObject decodes a nested JSON object; missing or malformed values yield
// an empty map so callers can treat every level as writable.
func rawObject(doc map[string]json.RawMessage, key string) map[string]json.RawMessage {
	out := map[string]json.RawMessage{}
	if doc == nil {
		return out
	}
	if raw, ok := doc[key]; ok {
		_ = json.Unmarshal(raw, &out)
	}
	return out
}

func entriesFromRaw(raw map[string]json.RawMessage, scope Scope) []Entry {
	names := make([]string, 0, len(raw))
	for n := range raw {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]Entry, 0, len(names))
	for _, n := range names {
		var s Server
		if err := json.Unmarshal(raw[n], &s); err != nil {
			// Unparseable entries are skipped here; they still surface when
			// the raw file is viewed.
			continue
		}
		out = append(out, Entry{Name: n, Scope: scope, Server: &s})
	}
	return out
}
