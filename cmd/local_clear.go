package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"cc-switch-cli/internal/manager"
	"cc-switch-cli/internal/ui"
)

var localClearModelOnly bool

var localClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Remove the project-local settings.local.json",
	Long: `Delete .claude/settings.local.json so this project falls back to the
global ~/.claude/settings.json (and any committed .claude/settings.json).

  ccsc local clear           delete the file
  ccsc local clear --model   strip only the provider model/env keys and keep
                             permissions, hooks, custom env and other keys

A missing file is not an error. MCP servers (.mcp.json and the local scope
in ~/.claude.json) are never touched. --path is inherited from local.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		out, err := manager.ClearLocalConfig(manager.LocalOptions{
			Path:      localPath,
			ModelOnly: localClearModelOnly,
		})
		if err != nil {
			return err
		}
		switch {
		case out.Missing:
			fmt.Println(ui.Dim(fmt.Sprintf("already absent: %s", out.Path)))
		case out.Deleted && out.ModelOnly:
			fmt.Println(ui.Success(fmt.Sprintf("cleared model (file emptied and removed): %s", out.Path)))
		case out.Deleted:
			fmt.Println(ui.Success(fmt.Sprintf("removed %s", out.Path)))
		case out.ModelOnly:
			fmt.Println(ui.Success(fmt.Sprintf("cleared model keys: %s", out.Path)))
		default:
			fmt.Println(ui.Success(fmt.Sprintf("cleared %s", out.Path)))
		}
		return nil
	},
}

func init() {
	localClearCmd.Flags().BoolVar(&localClearModelOnly, "model", false, "Only strip model/env keys; keep permissions, hooks and other project settings")
	localClearCmd.Flags().BoolVar(&localClearModelOnly, "model-only", false, "Alias for --model")
	_ = localClearCmd.Flags().MarkHidden("model-only")
}
