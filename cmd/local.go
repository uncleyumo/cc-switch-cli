package cmd

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/uncleyumo/ccsc/internal/apply"
	"github.com/uncleyumo/ccsc/internal/ccstate"
	"github.com/uncleyumo/ccsc/internal/group"
	"github.com/uncleyumo/ccsc/internal/render"
	"github.com/uncleyumo/ccsc/internal/store"
	"github.com/uncleyumo/ccsc/internal/workbench"
)

var localCmd = &cobra.Command{
	Use:   "local",
	Short: "bind a group to this project",
	Long:  "writes <project>/.claude/settings.local.json. The project root is the current directory; ccsc never walks upward.",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		paths, cfg, err := load()
		if err != nil {
			return err
		}
		project, ok := resolveProject(projectFlag)
		if !ok {
			return fmt.Errorf("%s is not a project directory", projectFlag)
		}
		if err := requireTTY(); err != nil {
			return err
		}

		names, err := group.List(paths.GroupsDir())
		if err != nil {
			return err
		}
		header := []string{
			project.Dir,
			"target: " + project.LocalFile,
			"",
			currentConfigLine(paths, project),
		}
		res, err := workbench.RunList(workbench.ListOptions{
			Title:  "project",
			Items:  names,
			Header: header,
			Bindings: []workbench.Binding{
				{Key: "enter", Label: "apply", Action: workbench.ActApply},
				{Key: "q", Label: "quit", Action: workbench.ActQuit},
			},
			Empty:  "the group library is empty; create one with ccsc group",
			Cursor: boundIndex(paths, project, names),
		})
		if err != nil {
			return err
		}
		if res.Action != workbench.ActApply {
			return nil
		}
		g, err := group.Load(paths.GroupsDir(), res.Item)
		if err != nil {
			return err
		}
		r, err := apply.Run(apply.Request{
			Paths:   paths,
			Group:   g,
			Layer:   apply.Project,
			Project: project,
			Options: render.Options{StartupSlot: cfg.StartupSlot()},
			Action:  "local.apply",
			Source:  "local",
		})
		if err != nil {
			return err
		}
		printReceipt(r)
		return nil
	},
}

var localClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "remove the model keys ccsc wrote into this project",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		paths, _, err := load()
		if err != nil {
			return err
		}
		project, ok := resolveProject(projectFlag)
		if !ok {
			return fmt.Errorf("%s is not a project directory", projectFlag)
		}
		r, err := apply.Clear(paths, apply.Project, project)
		if err != nil {
			return err
		}
		printReceipt(r)
		return nil
	},
}

// currentConfigLine says what already sits in the project layer, and which
// command put it there.
func currentConfigLine(paths store.Paths, project ccstate.Project) string {
	state, err := ccstate.LoadState(paths.Root)
	if err == nil {
		if ps, ok := state.Projects[project.Key()]; ok && ps.EffectiveSource() != "" {
			src := ps.EffectiveSource()
			detail := ""
			if src == "local" && ps.Group != "" {
				detail = " (group " + ps.Group + ")"
			}
			return fmt.Sprintf("current: written by ccsc %s%s, applied %s", src, detail, shortTime(ps.At))
		}
	}
	if _, err := os.Stat(project.LocalFile); err == nil {
		return "current: a project config cc-switch did not write"
	}
	return "current: nothing configured here"
}

// shortTime renders a stored RFC3339 stamp as 2026-11-08 15:56.
func shortTime(at string) string {
	t, err := time.Parse(time.RFC3339, at)
	if err != nil {
		return at
	}
	return t.Format("2006-01-02 15:04")
}

// boundIndex puts the group this project is already using under the cursor.
func boundIndex(paths store.Paths, project ccstate.Project, names []string) int {
	state, err := ccstate.LoadState(paths.Root)
	if err != nil {
		return 0
	}
	ps, ok := state.Projects[project.Key()]
	if !ok {
		return 0
	}
	for i, name := range names {
		if name == ps.Group {
			return i
		}
	}
	return 0
}

func init() {
	addProjectFlag(localCmd)
	addProjectFlag(localClearCmd)
	localCmd.AddCommand(localClearCmd)
	rootCmd.AddCommand(localCmd)
}
