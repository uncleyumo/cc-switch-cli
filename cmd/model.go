package cmd

import (
	"github.com/spf13/cobra"
)

var modelCmd = &cobra.Command{
	Use:   "model",
	Short: "Manage model providers",
}

func init() {
	modelCmd.AddCommand(modelSetCmd)
	modelCmd.AddCommand(modelCurrentCmd)
	modelCmd.AddCommand(modelBackupCmd)
}
