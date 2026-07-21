package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"cc-switch-cli/internal/manager"
)

var currentCmd = &cobra.Command{
	Use:   "current",
	Short: "Print the currently active model",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		name, err := manager.GetCurrentProvider()
		if err != nil {
			return err
		}
		if name == "" {
			fmt.Println("(no model is currently active)")
			return nil
		}
		fmt.Println(name)
		return nil
	},
}
