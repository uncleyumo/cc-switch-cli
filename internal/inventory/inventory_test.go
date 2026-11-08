package inventory

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/uncleyumo/ccsc/internal/ccstate"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestMCPMergesThreeScopes(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", home)

	projectDir := t.TempDir()
	project, err := ccstate.LocateProject(projectDir)
	if err != nil {
		t.Fatal(err)
	}

	write(t, filepath.Join(home, ".claude.json"), `{
	  "mcpServers": {
	    "shared": {"command": "user-cmd"},
	    "user-only": {"command": "u"}
	  },
	  "projects": {
	    "`+project.Key()+`": {
	      "mcpServers": {"shared": {"command": "local-cmd"}},
	      "enabledMcpjsonServers": ["proj-only"]
	    }
	  }
	}`)
	write(t, filepath.Join(projectDir, ".mcp.json"), `{
	  "mcpServers": {
	    "shared": {"command": "project-cmd"},
	    "proj-only": {"command": "p"},
	    "unapproved": {"command": "x"}
	  }
	}`)

	res, err := MCP(project)
	if err != nil {
		t.Fatal(err)
	}

	byName := map[string]Server{}
	shadowed := map[string][]string{}
	for _, s := range res.Servers {
		if s.Shadowed {
			shadowed[s.Name] = append(shadowed[s.Name], s.Scope)
			continue
		}
		byName[s.Name] = s
	}

	if s := byName["shared"]; s.Scope != ScopeLocal || s.Command != "local-cmd" {
		t.Errorf("local should win for shared, got %+v", s)
	}
	if len(shadowed["shared"]) != 2 {
		t.Errorf("the user and project copies should be marked shadowed, got %v", shadowed["shared"])
	}
	if !byName["proj-only"].Approved {
		t.Error("an enabled .mcp.json server should be marked approved")
	}
	if got := byName["unapproved"].Note; got != "pending approval" {
		t.Errorf("an unapproved .mcp.json server should note pending approval, got %q", got)
	}
	if _, ok := byName["user-only"]; !ok {
		t.Error("user-only server missing")
	}
	if len(res.Sources) != 2 {
		t.Errorf("sources = %v", res.Sources)
	}
}

func TestMCPMissingFilesAreNotErrors(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", home)
	project, _ := ccstate.LocateProject(t.TempDir())

	res, err := MCP(project)
	if err != nil {
		t.Fatalf("missing files must not fail: %v", err)
	}
	if len(res.Servers) != 0 {
		t.Errorf("servers = %v", res.Servers)
	}
	if len(res.Sources) == 0 {
		t.Error("sources should still list what was looked at")
	}
}

func TestSkillsProjectWinsOnDuplicate(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", home)
	projectDir := t.TempDir()
	project, _ := ccstate.LocateProject(projectDir)

	write(t, filepath.Join(home, "skills", "dupe", "SKILL.md"), "---\nname: dupe\ndescription: from user\n---\n")
	write(t, filepath.Join(projectDir, ".claude", "skills", "dupe", "SKILL.md"), "---\nname: dupe\ndescription: from project\n---\n")
	write(t, filepath.Join(home, "skills", "solo", "SKILL.md"), "---\ndescription: only\n---\n")

	res, err := Skills(project)
	if err != nil {
		t.Fatal(err)
	}

	var winner *Skill
	var shadowedCount int
	for i := range res.Skills {
		s := &res.Skills[i]
		if s.Name == "dupe" {
			if s.Shadowed {
				shadowedCount++
			} else {
				winner = s
			}
		}
		if s.Name == "solo" && s.Description != "only" {
			t.Errorf("frontmatter description = %q", s.Description)
		}
		if s.Name == "solo" && s.Scope != ScopeUser {
			t.Errorf("solo scope = %q", s.Scope)
		}
	}
	if winner == nil || winner.Description != "from project" {
		t.Fatalf("project should win, got %+v", winner)
	}
	if shadowedCount != 1 {
		t.Fatalf("expected one shadowed dupe, got %d", shadowedCount)
	}
}
