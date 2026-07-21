package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"cc-switch-cli/internal/model"
	"cc-switch-cli/internal/tui"
	"cc-switch-cli/internal/ui"
)

var (
	modelAddFrom   string
	modelAddNoEdit bool
)

var modelAddCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Create a new model provider file",
	Long:  "Writes a starter provider JSON under ~/.cc-switch-cli/models/. Use --from to clone an existing model as the base. In an interactive terminal the editor TUI opens after creation unless --no-edit is set.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		if err := model.Add(name, modelAddFrom); err != nil {
			return err
		}
		fmt.Println(ui.Success("Created model: " + name))
		if modelAddNoEdit || !ui.IsTTY() {
			return nil
		}
		return tui.RunEditor(name)
	},
}

func init() {
	modelAddCmd.Flags().StringVar(&modelAddFrom, "from", "", "Clone from an existing model instead of the built-in template")
	modelAddCmd.Flags().BoolVar(&modelAddNoEdit, "no-edit", false, "Skip opening the editor TUI after creation")
}
