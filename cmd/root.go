// Package cmd wires the Cobra command tree. Command handlers stay thin: they
// parse args and flags, gate TUI features on TTY availability and delegate all
// real work to the internal packages.
package cmd

import (
	"github.com/spf13/cobra"

	"cc-switch-cli/internal/version"
)

var rootCmd = &cobra.Command{
	Use:           "cc-switch-cli",
	Aliases:       []string{"ccs"},
	Short:         "Manage and switch Claude Code model provider configurations",
	Long:          "cc-switch-cli maintains a library of Claude Code model provider settings and swaps them into ~/.claude/settings.json on demand. See `cc-switch-cli <command> --help` for details.",
	Version:       version.String(),
	SilenceUsage:  true,
	SilenceErrors: false,
}

func init() {
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(useCmd)
	rootCmd.AddCommand(currentCmd)
	rootCmd.AddCommand(localCmd)
	rootCmd.AddCommand(backupCmd)
	rootCmd.AddCommand(modelCmd)
	rootCmd.AddCommand(configCmd)
}

// Execute is the entry point invoked by main.
func Execute() error {
	return rootCmd.Execute()
}
