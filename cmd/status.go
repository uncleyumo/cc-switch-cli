package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/uncleyumo/ccsc/internal/status"
)

var (
	statusReveal bool
	statusDiff   bool
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "what is in effect right now (read-only)",
	Long:  "Resolves every model key across the user, project and local settings files, shows the winning value and where it came from, then MCP and skills counts.",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		project, hasProject := resolveProject(projectFlag)
		report, err := status.Gather(project, hasProject)
		if err != nil {
			return err
		}
		if statusReveal {
			fmt.Println("warning: the full token is shown below; mind screen sharing and terminal logs")
			fmt.Println()
		}
		fmt.Print(report.Render(statusReveal))
		if statusDiff {
			fmt.Println()
			fmt.Print(report.Diff(statusReveal))
		}
		return nil
	},
}

func init() {
	addProjectFlag(statusCmd)
	statusCmd.Flags().BoolVar(&statusReveal, "reveal", false, "show the full token instead of masking it")
	statusCmd.Flags().BoolVar(&statusDiff, "diff", false, "list every key the project or local layer changes")
	rootCmd.AddCommand(statusCmd)
}
