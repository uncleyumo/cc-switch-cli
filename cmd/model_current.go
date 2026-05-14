package cmd

import (
	"fmt"

	"cc-switch-cli/internal/manager"

	"github.com/spf13/cobra"
)

var modelCurrentCmd = &cobra.Command{
	Use:   "current",
	Short: "Show current active model provider",
	RunE: func(cmd *cobra.Command, args []string) error {
		provider, err := manager.GetCurrentProvider()
		if err != nil {
			return err
		}

		if provider == "" {
			fmt.Println("No provider currently set")
			return nil
		}

		fmt.Println(provider)
		return nil
	},
}
