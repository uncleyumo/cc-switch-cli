package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"cc-switch-cli/internal/mcp"
	"cc-switch-cli/internal/mcptest"
	"cc-switch-cli/internal/ui"
)

var mcpListScope string

var mcpListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List configured MCP servers",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		dir, err := projectDirForMCP()
		if err != nil {
			return err
		}
		var entries []mcp.Entry
		if mcpListScope == "" {
			entries, err = mcp.ListAll(dir)
		} else {
			sc, perr := mcp.ParseScope(mcpListScope)
			if perr != nil {
				return perr
			}
			entries, err = mcp.List(sc, dir)
		}
		if err != nil {
			return err
		}
		if len(entries) == 0 {
			fmt.Println("(no MCP servers; add one with `ccs mcp add <name> -- <command>`)")
			return nil
		}
		status, _ := mcptest.LoadStatus()
		for _, e := range entries {
			line := formatMCPEntry(e, status)
			fmt.Println(line)
		}
		return nil
	},
}

func init() {
	mcpListCmd.Flags().StringVar(&mcpListScope, "scope", "", "Filter by scope: user, local or project")
}

func formatMCPEntry(e mcp.Entry, status *mcptest.StatusFile) string {
	endpoint := mcpEndpointSummary(e.Server)
	scope := fmt.Sprintf("[%s]", e.Scope)
	transport := string(e.Server.Transport())
	base := fmt.Sprintf("%-9s %-28s %-7s %s", scope, e.Name, transport, endpoint)
	if status == nil {
		return base
	}
	if s, ok := status.Results[mcptest.CacheKey(e.Scope, e.Name)]; ok {
		return base + " " + ui.Dim(formatMCPStatus(s))
	}
	return base
}

func mcpEndpointSummary(s *mcp.Server) string {
	if s == nil {
		return ""
	}
	m := s.Masked()
	switch m.Transport() {
	case mcp.TransportStdio:
		parts := []string{m.Command}
		parts = append(parts, m.Args...)
		joined := strings.Join(parts, " ")
		if len(joined) > 60 {
			return joined[:57] + "..."
		}
		return joined
	case mcp.TransportHTTP, mcp.TransportSSE, mcp.TransportWS:
		if len(m.URL) > 60 {
			return m.URL[:57] + "..."
		}
		return m.URL
	default:
		return "(invalid)"
	}
}

func formatMCPStatus(s mcptest.Result) string {
	var latency string
	if s.LatencyMS > 0 {
		latency = " " + (time.Duration(s.LatencyMS) * time.Millisecond).String()
	}
	testedAt := s.TestedAt
	if t, err := time.Parse(time.RFC3339, s.TestedAt); err == nil {
		testedAt = t.Local().Format("2006-01-02 15:04")
	}
	label := strings.ToUpper(string(s.Status))
	switch s.Status {
	case mcptest.StatusOK:
		label = ui.Success(label)
	case mcptest.StatusFail:
		label = ui.Fail(label)
	case mcptest.StatusUnsupported:
		label = ui.Warn(label)
	}
	tools := ""
	if s.ToolCount >= 0 {
		tools = fmt.Sprintf(", %d tools", s.ToolCount)
	}
	return fmt.Sprintf("[%s%s%s @ %s]", label, latency, tools, testedAt)
}
