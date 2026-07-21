package cmd

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"cc-switch-cli/internal/config"
	"cc-switch-cli/internal/fsutil"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize the cc-switch-cli configuration directory",
	Long:  "Create ~/.cc-switch-cli/, the models/ subdirectory and a default config.toml pointing at ~/.claude/settings.json. Safe to re-run: only missing pieces are created.",
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := config.ConfigDir()
		if err != nil {
			return err
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("failed to create %s: %w", dir, err)
		}
		modelsDir, err := config.ModelsDir()
		if err != nil {
			return err
		}
		if err := os.MkdirAll(modelsDir, 0o755); err != nil {
			return fmt.Errorf("failed to create %s: %w", modelsDir, err)
		}
		cfgPath, err := config.ConfigFilePath()
		if err != nil {
			return err
		}
		exists, err := fsutil.PathExists(cfgPath)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if !exists {
			if err := config.WriteDefaultConfig(); err != nil {
				return err
			}
			fmt.Printf("Created %s\n", cfgPath)
		} else {
			fmt.Printf("Config already exists at %s\n", cfgPath)
		}

		fmt.Println()
		fmt.Println("Next steps:")
		fmt.Printf("  1. Drop provider JSON files under %s\n", modelsDir)
		fmt.Println("  2. Run `ccs model list` to inspect them")
		fmt.Println("  3. Run `ccs use <name>` to activate one, or `ccs model` for the TUI")
		return nil
	},
}
