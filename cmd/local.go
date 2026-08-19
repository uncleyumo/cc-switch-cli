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
	Short: "Write, inspect or clear the project-local settings.local.json",
	Long: `Create, inspect or clear .claude/settings.local.json in the current project
so Claude Code uses a chosen provider only inside this repository.

  ccsc local <name>             deep-merge the named provider into the file
  ccsc local show               print the current project-level model and MCP
  ccsc local clear              delete the file (drop the project-level model)
  ccsc local clear --model      strip only model/env keys; keep the rest

By default a write is a deep-merge: env keys and the top-level "model" from
the provider are applied while every other project setting (permissions,
hooks) is preserved. Use --overwrite to replace the file wholesale, or
--path to target somewhere other than the current directory.

If a provider is named "clear" or "show", write it with: ccsc local -- <name>`,
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
	localCmd.PersistentFlags().StringVar(&localPath, "path", "", "Custom target: a directory (auto-appends .claude/settings.local.json) or an explicit file path")
	localCmd.Flags().BoolVar(&localOverwrite, "overwrite", false, "Replace the target file wholesale instead of deep-merging")
	localCmd.AddCommand(localClearCmd)
	localCmd.AddCommand(localShowCmd)
}
