package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"cc-switch-cli/internal/manager"
	"cc-switch-cli/internal/ui"
)

var backupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Snapshot the current target settings.json to ~/.cc-switch-cli/",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		path, err := manager.BackupSettings()
		if err != nil {
			return err
		}
		fmt.Println(ui.Success("Backup created: " + path))
		return nil
	},
}
