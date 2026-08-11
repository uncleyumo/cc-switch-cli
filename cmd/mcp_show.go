package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"cc-switch-cli/internal/mcp"
)

var (
	mcpShowScope  string
	mcpShowReveal bool
)

var mcpShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Print an MCP server entry as JSON (secrets masked by default)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := projectDirForMCP()
		if err != nil {
			return err
		}
		entry, err := resolveMCPEntry(dir, args[0], mcpShowScope)
		if err != nil {
			return err
		}
		srv := entry.Server
		if !mcpShowReveal {
			srv = srv.Masked()
		}
		out, err := srv.Indent()
		if err != nil {
			return err
		}
		fmt.Printf("# scope=%s name=%s transport=%s\n", entry.Scope, entry.Name, entry.Server.Transport())
		fmt.Print(string(out))
		return nil
	},
}

func init() {
	mcpShowCmd.Flags().StringVar(&mcpShowScope, "scope", "", "Scope: user, local or project (required when the name exists in more than one)")
	mcpShowCmd.Flags().BoolVar(&mcpShowReveal, "reveal", false, "Show secrets unmasked")
}

// resolveMCPEntry finds a server by name. An explicit scope restricts the
// lookup; otherwise mcp.Resolve is used (errors on ambiguity).
func resolveMCPEntry(projectDir, name, scopeFlag string) (*mcp.Entry, error) {
	if scopeFlag == "" {
		return mcp.Resolve(projectDir, name)
	}
	sc, err := mcp.ParseScope(scopeFlag)
	if err != nil {
		return nil, err
	}
	srv, err := mcp.Get(sc, projectDir, name)
	if err != nil {
		return nil, err
	}
	return &mcp.Entry{Name: name, Scope: sc, Server: srv}, nil
}
