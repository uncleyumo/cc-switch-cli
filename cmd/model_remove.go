package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"cc-switch-cli/internal/manager"
	"cc-switch-cli/internal/ui"
)

var modelRemoveYes bool

var modelRemoveCmd = &cobra.Command{
	Use:     "remove <model>",
	Aliases: []string{"rm"},
	Short:   "Delete a model file",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		if !modelRemoveYes {
			if !ui.IsTTY() {
				return errors.New("refusing to delete without --yes when not attached to a terminal")
			}
			fmt.Printf("Delete model %q? [y/N] ", name)
			reader := bufio.NewReader(os.Stdin)
			line, _ := reader.ReadString('\n')
			line = strings.TrimSpace(strings.ToLower(line))
			if line != "y" && line != "yes" {
				fmt.Println("aborted")
				return nil
			}
		}
		if err := manager.DeleteProvider(name); err != nil {
			return err
		}
		fmt.Println(ui.Success("Removed model: " + name))
		return nil
	},
}

func init() {
	modelRemoveCmd.Flags().BoolVarP(&modelRemoveYes, "yes", "y", false, "Skip confirmation")
}
