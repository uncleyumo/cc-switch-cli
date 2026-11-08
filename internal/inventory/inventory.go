// Package inventory is the read-only view of MCP servers and Skills across
// every scope ccsc knows about. It never writes anything.
//
// File locations were measured against Claude Code 2.1.319, not guessed:
//
//	user    <CLAUDE_CONFIG_DIR or ~>/.claude.json            mcpServers
//	local   the same .claude.json                            projects["<abs cwd>"].mcpServers
//	project <projectDir>/.mcp.json                           mcpServers
//
// settings.json files do NOT hold server definitions; the official settings
// reference is self-described as complete and has no such key. They only carry
// approval keys, which is what ccsc reads them for.
package inventory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/uncleyumo/ccsc/internal/ccstate"
	"github.com/uncleyumo/ccsc/internal/store"
)

// Scope names, ordered by precedence (highest first).
const (
	ScopeLocal   = "local"
	ScopeProject = "project"
	ScopeUser    = "user"
)

// Server is one MCP server as it exists on disk.
type Server struct {
	Name    string
	Scope   string
	Source  string // absolute file path it was read from
	KeyPath string // e.g. mcpServers.playwright
	Type    string // stdio | http | sse
	Command string
	Args    []string
	Env     map[string]string
	URL     string
	Headers map[string]string

	// Shadowed marks a definition that lost to a higher-precedence one.
	Shadowed bool
	// Approved marks a project-scope server the user has accepted.
	Approved bool
	// Disabled marks a server listed in disabledMcpjsonServers or deniedMcpServers.
	Disabled bool
	// Note is one short human explanation.
	Note string
}

// MCPResult is the merged view.
type MCPResult struct {
	Servers  []Server
	Sources  []string
	Warnings []string
}

// SkillsResult is the merged skills view.
type SkillsResult struct {
	Skills   []Skill
	Sources  []string
	Warnings []string
}

// Skill is one skill directory or command file.
type Skill struct {
	Name        string
	Scope       string
	Dir         string
	Description string
	Shadowed    bool
}

type mcpEntry struct {
	Type    string            `json:"type"`
	Command string            `json:"command"`
	Args    []string          `json:"args"`
	Env     map[string]string `json:"env"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
}

type claudeJSON struct {
	MCPServers map[string]mcpEntry `json:"mcpServers"`
	Projects   map[string]struct {
		MCPServers             map[string]mcpEntry `json:"mcpServers"`
		EnabledMcpjsonServers  []string            `json:"enabledMcpjsonServers"`
		DisabledMcpjsonServers []string            `json:"disabledMcpjsonServers"`
	} `json:"projects"`
}

// MCP merges the three scopes and marks the losers.
func MCP(project ccstate.Project) (MCPResult, error) {
	var res MCPResult

	claudePath, err := ccstate.ClaudeJSONPath()
	if err != nil {
		return res, err
	}
	res.Sources = append(res.Sources, claudePath)

	var root claudeJSON
	data, err := store.ReadFileOrEmpty(claudePath)
	if err != nil {
		res.Warnings = append(res.Warnings, fmt.Sprintf("cannot read %s: %v", claudePath, err))
	} else if data != nil {
		if err := json.Unmarshal(data, &root); err != nil {
			res.Warnings = append(res.Warnings, fmt.Sprintf("%s is not valid JSON: %v", claudePath, err))
		}
	}

	proj := root.Projects[project.Key()]

	byName := map[string][]Server{}
	for name, e := range root.MCPServers {
		byName[name] = append(byName[name], toServer(name, ScopeUser, claudePath, "mcpServers."+name, e))
	}
	for name, e := range proj.MCPServers {
		byName[name] = append(byName[name], toServer(name, ScopeLocal, claudePath, "projects."+project.Key()+".mcpServers."+name, e))
	}

	mcpPath := filepath.Join(project.Dir, ".mcp.json")
	res.Sources = append(res.Sources, mcpPath)
	if data, err := store.ReadFileOrEmpty(mcpPath); err != nil {
		res.Warnings = append(res.Warnings, fmt.Sprintf("cannot read %s: %v", mcpPath, err))
	} else if data != nil {
		var doc struct {
			MCPServers map[string]mcpEntry `json:"mcpServers"`
		}
		if err := json.Unmarshal(data, &doc); err != nil {
			res.Warnings = append(res.Warnings, fmt.Sprintf("%s is not valid JSON: %v", mcpPath, err))
		}
		for name, e := range doc.MCPServers {
			s := toServer(name, ScopeProject, mcpPath, "mcpServers."+name, e)
			s.Approved = contains(proj.EnabledMcpjsonServers, name)
			s.Disabled = contains(proj.DisabledMcpjsonServers, name)
			s.Note = approvalNote(s)
			byName[name] = append(byName[name], s)
		}
	}

	denied := deniedServers(project, &res)

	names := make([]string, 0, len(byName))
	for name := range byName {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		defs := byName[name]
		sort.SliceStable(defs, func(i, j int) bool {
			return rank(defs[i].Scope) < rank(defs[j].Scope)
		})
		winner := defs[0]
		if contains(denied, name) {
			winner.Disabled = true
			winner.Note = joinNote(winner.Note, "denied in settings")
		}
		res.Servers = append(res.Servers, winner)
		for _, loser := range defs[1:] {
			loser.Shadowed = true
			loser.Note = joinNote(loser.Note, "overridden by "+winner.Scope)
			res.Servers = append(res.Servers, loser)
		}
	}
	return res, nil
}

func toServer(name, scope, source, keyPath string, e mcpEntry) Server {
	t := e.Type
	if t == "" {
		if e.URL != "" {
			t = "http"
		} else {
			t = "stdio"
		}
	}
	return Server{
		Name:    name,
		Scope:   scope,
		Source:  source,
		KeyPath: keyPath,
		Type:    t,
		Command: e.Command,
		Args:    e.Args,
		Env:     e.Env,
		URL:     e.URL,
		Headers: e.Headers,
	}
}

func approvalNote(s Server) string {
	switch {
	case s.Disabled:
		return "disabled"
	case s.Approved:
		return "approved"
	default:
		return "pending approval"
	}
}

// deniedServers merges deniedMcpServers from every settings file.
func deniedServers(project ccstate.Project, res *MCPResult) []string {
	var out []string
	paths := []string{project.LocalFile, project.ProjectFile}
	if user, err := ccstate.UserSettingsPath(); err == nil {
		paths = append(paths, user)
	}
	for _, p := range paths {
		data, err := store.ReadFileOrEmpty(p)
		if err != nil || data == nil {
			continue
		}
		var doc struct {
			DeniedMcpServers []struct {
				ServerName string `json:"serverName"`
			} `json:"deniedMcpServers"`
			DisabledMcpjsonServers []string `json:"disabledMcpjsonServers"`
		}
		if err := json.Unmarshal(data, &doc); err != nil {
			res.Warnings = append(res.Warnings, fmt.Sprintf("%s is not valid JSON: %v", p, err))
			continue
		}
		for _, d := range doc.DeniedMcpServers {
			if d.ServerName != "" {
				out = append(out, d.ServerName)
			}
		}
		out = append(out, doc.DisabledMcpjsonServers...)
	}
	return out
}

func rank(scope string) int {
	switch scope {
	case ScopeLocal:
		return 0
	case ScopeProject:
		return 1
	default:
		return 2
	}
}

// Skills merges skill and command directories, project over user.
func Skills(project ccstate.Project) (SkillsResult, error) {
	var res SkillsResult

	configDir, err := ccstate.ConfigDir()
	if err != nil {
		return res, err
	}

	sources := []struct {
		dir   string
		scope string
	}{
		{filepath.Join(configDir, "skills"), ScopeUser},
		{filepath.Join(project.Dir, ".claude", "skills"), ScopeProject},
		{filepath.Join(configDir, "commands"), ScopeUser},
		{filepath.Join(project.Dir, ".claude", "commands"), ScopeProject},
	}

	grouped := map[string][]Skill{}
	for _, src := range sources {
		res.Sources = append(res.Sources, src.dir)
		skills, err := scanSkills(src.dir, src.scope)
		if err != nil {
			res.Warnings = append(res.Warnings, fmt.Sprintf("cannot read %s: %v", src.dir, err))
			continue
		}
		for _, s := range skills {
			grouped[s.Name] = append(grouped[s.Name], s)
		}
	}

	names := make([]string, 0, len(grouped))
	for name := range grouped {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		defs := grouped[name]
		sort.SliceStable(defs, func(i, j int) bool { return rank(defs[i].Scope) < rank(defs[j].Scope) })
		res.Skills = append(res.Skills, defs[0])
		for _, loser := range defs[1:] {
			loser.Shadowed = true
			res.Skills = append(res.Skills, loser)
		}
	}
	return res, nil
}

func scanSkills(dir, scope string) ([]Skill, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Skill
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		sub := filepath.Join(dir, e.Name())
		file := filepath.Join(sub, "SKILL.md")
		if _, err := os.Stat(file); err != nil {
			continue
		}
		name, desc := frontmatter(file)
		if name == "" {
			name = e.Name()
		}
		out = append(out, Skill{Name: name, Scope: scope, Dir: sub, Description: desc})
	}
	return out, nil
}

// frontmatter reads name and description from a leading --- block. The subset
// is tiny on purpose: a full YAML dependency is not worth one field.
func frontmatter(path string) (name, description string) {
	data, err := store.ReadFileOrEmpty(path)
	if err != nil || data == nil {
		return "", ""
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return "", ""
	}
	for _, line := range lines[1:] {
		if strings.TrimSpace(line) == "---" {
			break
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		switch strings.TrimSpace(key) {
		case "name":
			name = value
		case "description":
			description = value
		}
	}
	return name, description
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

func joinNote(a, b string) string {
	if a == "" {
		return b
	}
	return a + ", " + b
}
