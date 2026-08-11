package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"cc-switch-cli/internal/mcp"
	"cc-switch-cli/internal/ui"
)

var (
	mcpRemoveScope string
	mcpRemoveYes   bool
)

var mcpRemoveCmd = &cobra.Command{
	Use:     "remove <name>",
	Aliases: []string{"rm"},
	Short:   "Remove an MCP server",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := projectDirForMCP()
		if err != nil {
			return err
		}
		entry, err := resolveMCPEntry(dir, args[0], mcpRemoveScope)
		if err != nil {
			return err
		}
		if !mcpRemoveYes {
			if !ui.IsTTY() {
				return fmt.Errorf("refusing to remove %q without -y outside a terminal", entry.Name)
			}
			fmt.Printf("remove %q from %s scope? [y/N] ", entry.Name, entry.Scope)
			var answer string
			_, _ = fmt.Scanln(&answer)
			if answer != "y" && answer != "Y" {
				fmt.Println("cancelled")
				return nil
			}
		}
		if err := mcp.Remove(entry.Scope, dir, entry.Name); err != nil {
			return err
		}
		fmt.Println(ui.Success(fmt.Sprintf("removed %s (%s)", entry.Name, entry.Scope)))
		return nil
	},
}

func init() {
	mcpRemoveCmd.Flags().StringVar(&mcpRemoveScope, "scope", "", "Scope: user, local or project")
	mcpRemoveCmd.Flags().BoolVarP(&mcpRemoveYes, "yes", "y", false, "Skip the confirmation prompt")
}
