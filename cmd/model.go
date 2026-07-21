package cmd

import (
	"errors"

	"github.com/spf13/cobra"

	"cc-switch-cli/internal/tui"
	"cc-switch-cli/internal/ui"
)

var modelCmd = &cobra.Command{
	Use:     "model",
	Aliases: []string{"m"},
	Short:   "Manage the model library (list / add / edit / test / ...)",
	Long:    "With no subcommand, opens the interactive TUI when stdout is a terminal. See `cc-switch-cli model --help` for the subcommands.",
	RunE: func(cmd *cobra.Command, args []string) error {
		if !ui.IsTTY() {
			return errors.New("interactive model manager requires a terminal; use a subcommand such as `model list` or `model test`")
		}
		return tui.RunManager()
	},
}

func init() {
	modelCmd.AddCommand(modelListCmd)
	modelCmd.AddCommand(modelShowCmd)
	modelCmd.AddCommand(modelAddCmd)
	modelCmd.AddCommand(modelEditCmd)
	modelCmd.AddCommand(modelRenameCmd)
	modelCmd.AddCommand(modelRemoveCmd)
	modelCmd.AddCommand(modelTestCmd)
	modelCmd.AddCommand(modelDoctorCmd)
	modelCmd.AddCommand(modelTUICmd)
}

var modelTUICmd = &cobra.Command{
	Use:   "tui",
	Short: "Open the interactive model manager (same as bare `model`)",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if !ui.IsTTY() {
			return errors.New("model tui requires a terminal")
		}
		return tui.RunManager()
	},
}
