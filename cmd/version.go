package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/uncleyumo/ccsc/internal/version"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "print the build version",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println(version.String())
		return nil
	},
}

func init() { rootCmd.AddCommand(versionCmd) }
