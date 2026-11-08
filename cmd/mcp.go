package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/uncleyumo/ccsc/internal/inventory"
	"github.com/uncleyumo/ccsc/internal/probe"
	"github.com/uncleyumo/ccsc/internal/ui"
)

var (
	mcpJSON    bool
	mcpTimeout time.Duration
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "list MCP servers (user, local, project; read-only)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return printMCP()
	},
}

var mcpTestCmd = &cobra.Command{
	Use:   "test [name]",
	Short: "run one real handshake against an MCP server",
	Long:  "start the server, complete the initialize handshake, list tools, then exit. writes no config.",
	RunE: func(cmd *cobra.Command, args []string) error {
		project, _ := resolveProject(projectFlag)
		res, err := inventory.MCP(project)
		if err != nil {
			return err
		}
		want := ""
		if len(args) > 0 {
			want = args[0]
		}

		tested := 0
		failed := 0
		for _, s := range res.Servers {
			if s.Shadowed {
				continue
			}
			if want != "" && s.Name != want {
				continue
			}
			tested++
			out := probe.Test(context.Background(), probe.Target{
				Name:    s.Name,
				Type:    s.Type,
				Command: s.Command,
				Args:    s.Args,
				Env:     s.Env,
				URL:     s.URL,
				Headers: s.Headers,
			}, mcpTimeout)
			if out.OK {
				fmt.Printf("%s %-24s %5dms  %d tools\n", ui.Pad("ok", 6), out.Name, out.Latency.Milliseconds(), out.ToolCount)
				continue
			}
			failed++
			fmt.Printf("%s %-24s %s\n", ui.Pad("error:", 6), out.Name, out.Error)
		}
		if tested == 0 {
			if want != "" {
				return fmt.Errorf("no active MCP server named %q", want)
			}
			return fmt.Errorf("no MCP servers to test")
		}
		if failed > 0 {
			return fmt.Errorf("%d/%d servers failed", failed, tested)
		}
		return nil
	},
}

func printMCP() error {
	project, _ := resolveProject(projectFlag)
	res, err := inventory.MCP(project)
	if err != nil {
		return err
	}
	if mcpJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(res)
	}

	live := 0
	for _, s := range res.Servers {
		if !s.Shadowed {
			live++
		}
	}
	fmt.Printf("mcp: %d active\n\n", live)
	fmt.Printf("  %s %s %s %s %s\n", ui.Pad("scope", 8), ui.Pad("name", 24), ui.Pad("type", 8), ui.Pad("status", 24), "source")
	for _, s := range res.Servers {
		status := s.Note
		if status == "" && s.Shadowed {
			status = "shadowed"
		}
		fmt.Printf("  %s %s %s %s %s  (%s)\n",
			ui.Pad(s.Scope, 8), ui.Pad(ui.Truncate(s.Name, 22), 24), ui.Pad(s.Type, 8),
			ui.Pad(ui.Truncate(status, 22), 24), s.Source, s.KeyPath)
	}
	for _, w := range res.Warnings {
		fmt.Printf("\nwarning: %s\n", w)
	}
	return nil
}

var skillsJSON bool

var skillsCmd = &cobra.Command{
	Use:   "skills",
	Short: "list skills (user and project; read-only)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		project, _ := resolveProject(projectFlag)
		res, err := inventory.Skills(project)
		if err != nil {
			return err
		}
		if skillsJSON {
			enc := json.NewEncoder(os.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(res)
		}
		live := 0
		for _, s := range res.Skills {
			if !s.Shadowed {
				live++
			}
		}
		fmt.Printf("skills: %d active\n\n", live)
		fmt.Printf("  %s %s %s\n", ui.Pad("scope", 8), ui.Pad("name", 28), "dir")
		for _, s := range res.Skills {
			scope := s.Scope
			if s.Shadowed {
				scope += "!"
			}
			fmt.Printf("  %s %s %s\n", ui.Pad(scope, 8), ui.Pad(ui.Truncate(s.Name, 26), 28), s.Dir)
			if desc := strings.TrimSpace(s.Description); desc != "" {
				fmt.Printf("  %s %s   %s\n", ui.Pad("", 8), ui.Pad("", 28), ui.Truncate(desc, 72))
			}
		}
		for _, w := range res.Warnings {
			fmt.Printf("\nwarning: %s", w)
		}
		if len(res.Warnings) > 0 {
			fmt.Println()
		}
		return nil
	},
}

func init() {
	addProjectFlag(mcpCmd)
	addProjectFlag(mcpTestCmd)
	mcpCmd.Flags().BoolVar(&mcpJSON, "json", false, "output raw JSON")
	mcpTestCmd.Flags().DurationVar(&mcpTimeout, "timeout", 15*time.Second, "handshake timeout per server")
	mcpCmd.AddCommand(mcpTestCmd)

	addProjectFlag(skillsCmd)
	skillsCmd.Flags().BoolVar(&skillsJSON, "json", false, "output raw JSON")

	rootCmd.AddCommand(mcpCmd)
	rootCmd.AddCommand(skillsCmd)
}
