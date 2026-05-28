package cmd

import (
	"fmt"

	"cc-switch-cli/internal/manager"

	"github.com/spf13/cobra"
)

var catCmd = &cobra.Command{
	Use:   "cat [provider]",
	Short: "Print provider configuration content",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		provider := args[0]

		content, err := manager.GetProviderConfig(provider)
		if err != nil {
			return err
		}

		fmt.Print(content)
		return nil
	},
}
