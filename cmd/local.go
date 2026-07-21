package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"cc-switch-cli/internal/manager"
	"cc-switch-cli/internal/ui"
)

var (
	localOverwrite bool
	localPath      string
)

var localCmd = &cobra.Command{
	Use:   "local [model]",
	Short: "Write a project-local settings.local.json for the named model",
	Long: `Create or update .claude/settings.local.json in the current project so Claude Code
uses the chosen provider only inside this repository.

By default the file is deep-merged: env keys and the top-level "model" from the
provider are applied while every other project setting (permissions, hooks,
MCP servers) is preserved. Use --overwrite to replace the file wholesale, or
--path to write somewhere other than the current directory.`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name, err := resolveModelArg(args, "write locally")
		if err != nil {
			return err
		}
		target, err := manager.CreateLocalConfig(name, manager.LocalOptions{
			Path:      localPath,
			Overwrite: localOverwrite,
		})
		if err != nil {
			return err
		}
		mode := "merged"
		if localOverwrite {
			mode = "overwritten"
		}
		fmt.Println(ui.Success(fmt.Sprintf("%s: %s (%s)", name, target, mode)))
		return nil
	},
}

func init() {
	localCmd.Flags().BoolVar(&localOverwrite, "overwrite", false, "Replace the target file wholesale instead of deep-merging")
	localCmd.Flags().StringVar(&localPath, "path", "", "Custom target: a directory (auto-appends .claude/settings.local.json) or an explicit file path")
}
