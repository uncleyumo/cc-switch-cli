package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/uncleyumo/ccsc/internal/version"
)

var rootCmd = &cobra.Command{
	Use:           "ccsc",
	Short:         "model configuration for Claude Code",
	Long:          "Groups of model settings, applied globally or per project.",
	SilenceUsage:  true,
	SilenceErrors: true,
}

// Execute runs the root command and returns the process exit code.
func Execute() int {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 1
	}
	return 0
}

func init() {
	rootCmd.Version = version.String()
	rootCmd.SetVersionTemplate("ccsc {{.Version}}\n")
}
