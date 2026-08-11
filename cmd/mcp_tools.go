package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/spf13/cobra"

	"cc-switch-cli/internal/mcptest"
	"cc-switch-cli/internal/ui"
)

var (
	mcpToolsScope   string
	mcpToolsTimeout time.Duration
)

var mcpToolsCmd = &cobra.Command{
	Use:   "tools <name>",
	Short: "List tools exposed by an MCP server",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := projectDirForMCP()
		if err != nil {
			return err
		}
		entry, err := resolveMCPEntry(dir, args[0], mcpToolsScope)
		if err != nil {
			return err
		}
		opts := mcptest.Options{
			Timeout:    mcpToolsTimeout,
			ProjectDir: dir,
			WithTools:  true,
		}
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
		defer cancel()
		r := mcptest.Probe(ctx, *entry, opts)
		if r.Status != mcptest.StatusOK {
			return fmt.Errorf("%s: %s", r.Status, r.Error)
		}
		if r.ToolCount < 0 {
			fmt.Println(ui.Warn("server reachable but tools/list failed: " + r.Error))
			return nil
		}
		if r.ToolCount == 0 {
			fmt.Println("(no tools)")
			return nil
		}
		fmt.Printf("# %s (%s) - %d tools\n", entry.Name, entry.Server.Transport(), r.ToolCount)
		for _, t := range r.Tools {
			desc := t.Description
			if len(desc) > 100 {
				desc = desc[:97] + "..."
			}
			if desc == "" {
				fmt.Printf("  %s\n", t.Name)
			} else {
				fmt.Printf("  %-32s %s\n", t.Name, ui.Dim(desc))
			}
		}
		return nil
	},
}

func init() {
	mcpToolsCmd.Flags().StringVar(&mcpToolsScope, "scope", "", "Scope: user, local or project")
	mcpToolsCmd.Flags().DurationVar(&mcpToolsTimeout, "timeout", 30*time.Second, "Probe timeout")
}
