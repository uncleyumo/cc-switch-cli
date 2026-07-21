package tester

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"

	"cc-switch-cli/internal/ui"
)

// PrintResults renders a lipgloss table of the test results to w.
func PrintResults(w io.Writer, results []Result) {
	rows := make([][]string, 0, len(results))
	for _, r := range results {
		rows = append(rows, []string{
			r.Provider,
			r.Check,
			renderStatus(r.Status),
			renderDuration(r.TTFT),
			renderDuration(r.Latency),
			renderHTTP(r.HTTPStatus),
			truncate(r.Error, 60),
		})
	}
	t := table.New().
		Border(lipgloss.NormalBorder()).
		BorderStyle(lipgloss.NewStyle().Faint(true)).
		Headers("MODEL", "CHECK", "STATUS", "TTFT", "LATENCY", "HTTP", "ERROR").
		Rows(rows...).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return lipgloss.NewStyle().Bold(true).Padding(0, 1)
			}
			return lipgloss.NewStyle().Padding(0, 1)
		})
	fmt.Fprintln(w, t.Render())
}

func renderStatus(s Status) string {
	switch s {
	case StatusOK:
		return ui.Success(strings.ToUpper(string(s)))
	case StatusFail:
		return ui.Fail(strings.ToUpper(string(s)))
	case StatusUnsupported:
		return ui.Warn(strings.ToUpper(string(s)))
	default:
		return string(s)
	}
}

func renderDuration(d time.Duration) string {
	if d <= 0 {
		return "-"
	}
	return d.Round(time.Millisecond).String()
}

func renderHTTP(code int) string {
	if code == 0 {
		return "-"
	}
	return fmt.Sprintf("%d", code)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
