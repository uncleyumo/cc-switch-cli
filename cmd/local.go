package cmd

import (
	"cc-switch-cli/internal/manager"

	"github.com/spf13/cobra"
)

var localForce bool

var localCmd = &cobra.Command{
	Use:   "local [provider] [path]",
	Short: "Create a local settings file from provider config",
	Long: `Create a settings.local.json (or custom-named) file from a provider's configuration.

If path is omitted, creates settings.local.json in the current directory.
If path is a directory, creates settings.local.json inside it.
If path includes a filename (e.g. ./.claude/settings.json.bk), uses that filename.`,
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		provider := args[0]
		path := ""
		if len(args) > 1 {
			path = args[1]
		}

		return manager.CreateLocalConfig(provider, path, localForce)
	},
}

func init() {
	localCmd.Flags().BoolVarP(&localForce, "force", "f", false, "Overwrite existing file without confirmation")
}
