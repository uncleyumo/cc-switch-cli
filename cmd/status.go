package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/uncleyumo/ccsc/internal/status"
)

var statusReveal bool

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
		out := report.Render(statusReveal)
		fmt.Print(out)
		return nil
	},
}

func init() {
	addProjectFlag(statusCmd)
	statusCmd.Flags().BoolVar(&statusReveal, "reveal", false, "show the full token instead of masking it")
	rootCmd.AddCommand(statusCmd)
}
