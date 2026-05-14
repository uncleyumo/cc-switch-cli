package cmd

import (
	"fmt"
	"os"

	"cc-switch-cli/internal/config"

	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize cc-switch-cli configuration",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfgDir := config.ConfigDir()

		if _, err := os.Stat(cfgDir); err == nil {
			fmt.Println("Already initialized. Config directory:", cfgDir)
			return nil
		}

		if err := os.MkdirAll(cfgDir, 0755); err != nil {
			return fmt.Errorf("failed to create config directory: %w", err)
		}

		if err := os.MkdirAll(config.ModelsDir(), 0755); err != nil {
			return fmt.Errorf("failed to create models directory: %w", err)
		}

		if err := config.WriteDefaultConfig(); err != nil {
			return err
		}

		fmt.Println("Initialized cc-switch-cli at:", cfgDir)
		fmt.Println()
		fmt.Println("Next steps:")
		fmt.Printf("  1. Edit %s if the target path is wrong\n", config.ConfigFilePath())
		fmt.Printf("  2. Add model provider JSON files to %s\n", config.ModelsDir())
		fmt.Println("     Example: mimo.json, deepseek.json, claude.json")
		fmt.Println("  3. Run `cc-switch-cli list` to see available providers")
		fmt.Println("  4. Run `cc-switch-cli model set <provider>` to switch")

		return nil
	},
}
