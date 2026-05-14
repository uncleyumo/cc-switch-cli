package cmd

import (
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "cc-switch-cli",
	Short: "Switch Claude Code model provider configuration",
	Long:  "A CLI tool to switch Claude Code's model provider settings without affecting other configurations.",
}

func init() {
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(modelCmd)
}

func Execute() error {
	return rootCmd.Execute()
}
