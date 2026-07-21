package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"cc-switch-cli/internal/manager"
	"cc-switch-cli/internal/ui"
)

var modelShowReveal bool

var modelShowCmd = &cobra.Command{
	Use:   "show <model>",
	Short: "Print a model's JSON to stdout (token masked by default)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		raw, err := manager.GetProviderRaw(args[0])
		if err != nil {
			return err
		}
		if !modelShowReveal {
			raw = ui.MaskSensitive(raw)
		}
		fmt.Print(string(raw))
		return nil
	},
}

func init() {
	modelShowCmd.Flags().BoolVar(&modelShowReveal, "reveal", false, "Show the raw provider file, including secrets")
}
