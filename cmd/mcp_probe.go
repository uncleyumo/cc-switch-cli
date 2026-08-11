package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/spf13/cobra"

	"cc-switch-cli/internal/mcp"
	"cc-switch-cli/internal/mcptest"
	"cc-switch-cli/internal/ui"
)

var (
	mcpTestScope       string
	mcpTestTimeout     time.Duration
	mcpTestConcurrency int
)

var mcpTestCmd = &cobra.Command{
	Use:   "test [name...]",
	Short: "Probe MCP servers (initialize + tools/list)",
	Long:  "Connects to each named MCP server, runs the initialize handshake and lists tools. Without arguments, probes every server in every scope (or the filtered scope).",
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := projectDirForMCP()
		if err != nil {
			return err
		}
		entries, err := collectMCPEntries(dir, args, mcpTestScope)
		if err != nil {
			return err
		}
		if len(entries) == 0 {
			return fmt.Errorf("no MCP servers to test")
		}

		opts := mcptest.Options{
			Timeout:     mcpTestTimeout,
			Concurrency: mcpTestConcurrency,
			ProjectDir:  dir,
			WithTools:   true,
		}
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
		defer cancel()

		results := mcptest.ProbeAll(ctx, entries, opts, nil)
		printMCPResults(results)

		status, _ := mcptest.LoadStatus()
		if status == nil {
			status = &mcptest.StatusFile{Results: map[string]mcptest.Result{}}
		}
		status.Upsert(results)
		if err := mcptest.SaveStatus(status); err != nil {
			fmt.Fprintln(os.Stderr, ui.Warn("warning: failed to persist mcp test status: "+err.Error()))
		}
		return nil
	},
}

func init() {
	mcpTestCmd.Flags().StringVar(&mcpTestScope, "scope", "", "Restrict to one scope: user, local or project")
	mcpTestCmd.Flags().DurationVar(&mcpTestTimeout, "timeout", 30*time.Second, "Per-server timeout")
	mcpTestCmd.Flags().IntVar(&mcpTestConcurrency, "concurrency", 4, "Maximum concurrent probes")
}

func collectMCPEntries(dir string, names []string, scopeFlag string) ([]mcp.Entry, error) {
	if len(names) == 0 {
		if scopeFlag == "" {
			return mcp.ListAll(dir)
		}
		sc, err := mcp.ParseScope(scopeFlag)
		if err != nil {
			return nil, err
		}
		return mcp.List(sc, dir)
	}
	out := make([]mcp.Entry, 0, len(names))
	for _, n := range names {
		e, err := resolveMCPEntry(dir, n, scopeFlag)
		if err != nil {
			fmt.Fprintln(os.Stderr, ui.Fail("skip "+n+": "+err.Error()))
			continue
		}
		out = append(out, *e)
	}
	return out, nil
}

func printMCPResults(results []mcptest.Result) {
	for _, r := range results {
		status := string(r.Status)
		switch r.Status {
		case mcptest.StatusOK:
			status = ui.Success("OK")
		case mcptest.StatusFail:
			status = ui.Fail("FAIL")
		case mcptest.StatusUnsupported:
			status = ui.Warn("UNSUPPORTED")
		}
		tools := "n/a"
		if r.ToolCount >= 0 {
			tools = fmt.Sprintf("%d", r.ToolCount)
		}
		line := fmt.Sprintf("%-9s %-28s %-7s %-12s tools=%-4s %s",
			"["+r.Scope+"]", r.Name, r.Transport, status, tools,
			(time.Duration(r.LatencyMS) * time.Millisecond).String())
		if r.Error != "" {
			line += "  " + ui.Dim(r.Error)
		}
		fmt.Println(line)
	}
}
