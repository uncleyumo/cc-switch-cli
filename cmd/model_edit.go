package cmd

import (
	"errors"

	"github.com/spf13/cobra"

	"cc-switch-cli/internal/tui"
	"cc-switch-cli/internal/ui"
)

var modelEditCmd = &cobra.Command{
	Use:   "edit [name]",
	Short: "Edit a model in the TUI editor",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if !ui.IsTTY() {
			return errors.New("model edit requires a terminal; edit ~/.cc-switch-cli/models/<name>.json directly instead")
		}
		name, err := resolveModelArg(args, "edit")
		if err != nil {
			return err
		}
		return tui.RunEditor(name)
	},
}
