package cmd

import (
	"fmt"

	"cc-switch-cli/internal/manager"

	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List available model providers",
	RunE: func(cmd *cobra.Command, args []string) error {
		providers, err := manager.ListProviders()
		if err != nil {
			return err
		}

		if len(providers) == 0 {
			fmt.Println("No model providers found. Add .json files to ~/.cc-switch-cli/models/")
			return nil
		}

		current, _ := manager.GetCurrentProvider()

		for _, p := range providers {
			if p == current {
				fmt.Printf("  %s  (active)\n", p)
			} else {
				fmt.Printf("  %s\n", p)
			}
		}

		return nil
	},
}
