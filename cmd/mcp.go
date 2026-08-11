package cmd

import (
	"errors"
	"os"

	"github.com/spf13/cobra"

	"cc-switch-cli/internal/tui"
	"cc-switch-cli/internal/ui"
)

var mcpCmd = &cobra.Command{
	Use:   "mcp",
	Short: "Manage Claude Code MCP servers (user / local / project scopes)",
	Long: `Inspect, add, remove and probe the MCP servers Claude Code loads from
~/.claude.json (user and local scopes) and .mcp.json (project scope).

With no subcommand in a terminal, opens the interactive MCP manager. Edits
made while a Claude Code session is running may be overwritten when that
session exits — prefer editing with Claude Code closed.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		if !ui.IsTTY() {
			return errors.New("interactive MCP manager requires a terminal; use a subcommand such as `mcp list` or `mcp test`")
		}
		return tui.RunMCPManager()
	},
}

var mcpTUICmd = &cobra.Command{
	Use:   "tui",
	Short: "Open the interactive MCP manager (same as bare `mcp`)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if !ui.IsTTY() {
			return errors.New("mcp tui requires a terminal")
		}
		return tui.RunMCPManager()
	},
}

func init() {
	mcpCmd.AddCommand(mcpListCmd)
	mcpCmd.AddCommand(mcpShowCmd)
	mcpCmd.AddCommand(mcpAddCmd)
	mcpCmd.AddCommand(mcpAddJSONCmd)
	mcpCmd.AddCommand(mcpRemoveCmd)
	mcpCmd.AddCommand(mcpTestCmd)
	mcpCmd.AddCommand(mcpToolsCmd)
	mcpCmd.AddCommand(mcpTUICmd)
}

// projectDirForMCP returns the working directory used as the project key for
// local and project scopes.
func projectDirForMCP() (string, error) {
	return os.Getwd()
}
