package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"cc-switch-cli/internal/manager"
	"cc-switch-cli/internal/tester"
	"cc-switch-cli/internal/ui"
)

var modelListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List every model in ~/.cc-switch-cli/models/",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		names, err := manager.ListModels()
		if err != nil {
			return err
		}
		current, _ := manager.GetCurrentProvider()
		status, _ := tester.LoadStatus()

		if len(names) == 0 {
			fmt.Println("(no models yet; add one with `ccs model add <name>`)")
			return nil
		}
		for _, n := range names {
			marker := "  "
			if n == current {
				marker = ui.Success("* ")
			}
			var trailing string
			if status != nil {
				if s, ok := status.Results[n]; ok {
					trailing = " " + ui.Dim(formatStatus(s))
				}
			}
			fmt.Printf("%s%s%s\n", marker, n, trailing)
		}
		return nil
	},
}

// formatStatus produces the compact "[ok  1.2s @ 2026-07-21 10:04]" style
// suffix shown after each name in `model list`.
func formatStatus(s tester.ProviderStatus) string {
	var latency string
	if s.LatencyMS > 0 {
		latency = " " + (time.Duration(s.LatencyMS) * time.Millisecond).String()
	}
	testedAt := s.TestedAt
	if t, err := time.Parse(time.RFC3339, s.TestedAt); err == nil {
		testedAt = t.Local().Format("2006-01-02 15:04")
	}
	// The stored status string is "unsupported"; for display we render it as
	// "OK~" because it means streaming works (the provider is usable) and only
	// the optional tool-call probe did not pass.
	label := strings.ToUpper(string(s.Status))
	switch s.Status {
	case tester.StatusOK:
		label = ui.Success(label)
	case tester.StatusFail:
		label = ui.Fail(label)
	case tester.StatusUnsupported:
		label = ui.Warn("OK~")
	}
	return fmt.Sprintf("[%s%s @ %s]", label, latency, testedAt)
}
