package cmd

import (
	"fmt"

	"cc-switch-cli/internal/manager"

	"github.com/spf13/cobra"
)

var modelSetCmd = &cobra.Command{
	Use:   "set [provider]",
	Short: "Switch to a model provider",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		provider := args[0]

		if err := manager.SwitchProvider(provider); err != nil {
			return err
		}

		fmt.Printf("Switched to provider: %s\n", provider)
		return nil
	},
}
