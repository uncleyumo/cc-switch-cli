package cmd

import (
	"github.com/spf13/cobra"

	"github.com/uncleyumo/ccsc/internal/ccstate"
)

// projectFlag is the shared --dir override. It replaces only the project
// root; it never makes ccsc walk upward looking for a .claude directory.
var projectFlag string

// resolveProject locates the project ccsc was asked about and reports whether
// it is usable (not the home directory and not a filesystem root).
func resolveProject(dir string) (ccstate.Project, bool) {
	project, err := ccstate.LocateProject(dir)
	if err != nil {
		return ccstate.Project{}, false
	}
	return project, ccstate.IsUsableProject(project)
}

func addProjectFlag(cmd *cobra.Command) {
	cmd.Flags().StringVar(&projectFlag, "dir", "", "project directory (defaults to the current one; never searches upward)")
}
