package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"cc-switch-cli/internal/manager"
	"cc-switch-cli/internal/ui"
)

var modelRenameCmd = &cobra.Command{
	Use:   "rename <old> <new>",
	Short: "Rename a model",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := manager.RenameProvider(args[0], args[1]); err != nil {
			return err
		}
		fmt.Println(ui.Success(fmt.Sprintf("Renamed %s -> %s", args[0], args[1])))
		return nil
	},
}
