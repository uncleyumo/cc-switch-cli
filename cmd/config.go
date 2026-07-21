package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"cc-switch-cli/internal/config"
	"cc-switch-cli/internal/manager"
	"cc-switch-cli/internal/ui"
)

var configCmd = &cobra.Command{
	Use:     "config",
	Aliases: []string{"cfg"},
	Short:   "Inspect or update cc-switch-cli's own config.toml",
}

func init() {
	configCmd.AddCommand(configShowCmd)
	configCmd.AddCommand(configSetTargetCmd)
}

var configShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Print config paths and derived state",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		r, err := config.Resolve()
		if err != nil {
			return err
		}
		current, _ := manager.GetCurrentProvider()
		fmt.Printf("home         : %s\n", r.Home)
		fmt.Printf("config dir   : %s\n", r.ConfigDir)
		fmt.Printf("models dir   : %s\n", r.ModelsDir)
		fmt.Printf("config file  : %s\n", r.ConfigFile)
		if r.TargetPath == "" {
			fmt.Printf("target       : %s\n", ui.Warn("(not configured; run `ccs init`)"))
		} else {
			fmt.Printf("target       : %s\n", r.TargetPath)
		}
		if current != "" {
			fmt.Printf("current model: %s\n", current)
		} else {
			fmt.Printf("current model: %s\n", ui.Dim("(none)"))
		}
		return nil
	},
}

var configSetTargetCmd = &cobra.Command{
	Use:   "set-target <path>",
	Short: "Point config.toml at a different settings.json path",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.SetTarget(args[0]); err != nil {
			return err
		}
		fmt.Println(ui.Success("target updated"))
		return nil
	},
}
