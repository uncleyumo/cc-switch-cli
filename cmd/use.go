package cmd

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"cc-switch-cli/internal/manager"
	"cc-switch-cli/internal/tui"
	"cc-switch-cli/internal/ui"
)

var useCmd = &cobra.Command{
	Use:   "use [model]",
	Short: "Activate a model provider globally",
	Long:  "Merge the named provider into ~/.claude/settings.json: keys the provider declares override the file, keys it does not declare (hooks, permissions, ...) are preserved. With no argument in an interactive terminal, opens a picker.",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name, err := resolveModelArg(args, "use")
		if err != nil {
			return err
		}
		if err := manager.SwitchProvider(name); err != nil {
			return err
		}
		fmt.Println(ui.Success("Switched to " + name))
		return nil
	},
}

// resolveModelArg reads the model name from args or launches the picker when
// stdin is a TTY. If neither is available it returns a clear error so scripts
// don't accidentally hang.
func resolveModelArg(args []string, action string) (string, error) {
	if len(args) == 1 {
		return args[0], nil
	}
	if !ui.IsTTY() {
		return "", fmt.Errorf("no model name provided (interactive picker requires a terminal); pass one explicitly")
	}
	names, err := manager.ListModels()
	if err != nil {
		return "", err
	}
	if len(names) == 0 {
		return "", errors.New("no models found: add one with `ccsc model add <name>`")
	}
	current, _ := manager.GetCurrentProvider()
	title := "Choose a model to " + action
	chosen, err := tui.RunPicker(names, current, title)
	if err != nil {
		return "", err
	}
	if chosen == "" {
		return "", errors.New("cancelled")
	}
	return chosen, nil
}
