package cmd

import (
	"fmt"

	"cc-switch-cli/internal/manager"

	"github.com/spf13/cobra"
)

var modelBackupCmd = &cobra.Command{
	Use:   "backup",
	Short: "Create a manual backup of current settings",
	RunE: func(cmd *cobra.Command, args []string) error {
		path, err := manager.BackupSettings()
		if err != nil {
			return err
		}

		fmt.Printf("Backup created: %s\n", path)
		return nil
	},
}
