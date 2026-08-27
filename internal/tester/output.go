package tester

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"golang.org/x/term"

	"cc-switch-cli/internal/ui"
)

// PrintResults renders a lipgloss table of the test results to w, followed by
// a full-detail block for every failing check so the error reason is never
// lost to column truncation.
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
			renderError(r),
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
	fmt.Fprintln(w, Summarize(results))

	width := terminalWidth()
	for _, block := range failureBlocks(results, width) {
		fmt.Fprintln(w, block)
	}
}

// failureBlocks renders one indented block per non-OK check with its full,
// untruncated error wrapped to the available width:
//
//	  my-model/stream — auth error
//	    http 401: your credit balance is too low to access this endpoint
func failureBlocks(results []Result, width int) []string {
	blocks := make([]string, 0, len(results))
	for _, r := range results {
		if r.Status == StatusOK || (r.Error == "" && r.ErrorKind == ErrKindNone) {
			continue
		}
		head := fmt.Sprintf("  %s/%s", r.Provider, r.Check)
		if human := r.ErrorKind.Human(); human != "" {
			head += ui.Dim(" — "+human)
		} else {
			head += ui.Dim(" — check failed")
		}
		body := ""
		if r.Error != "" {
			wrap := lipgloss.NewStyle().Width(maxInt(width-4, 20)).MarginLeft(4)
			body = wrap.Render(r.Error)
		}
		blocks = append(blocks, strings.TrimRight(head+"\n"+ui.Dim(body), " \n"))
	}
	if len(blocks) > 0 {
		blocks = append([]string{""}, blocks...) // blank separator after the table
	}
	return blocks
}

// Summarize folds the batch into one headline line with per-provider verdicts:
// "tested 5 providers: 3 ok, 1 fail, 1 unsupported".
func Summarize(results []Result) string {
	if len(results) == 0 {
		return "no results"
	}
	ok := 0
	fail := 0
	unsupported := 0
	byProvider := map[string][]Result{}
	names := make([]string, 0, len(results))
	for _, r := range results {
		if _, seen := byProvider[r.Provider]; !seen {
			names = append(names, r.Provider)
		}
		byProvider[r.Provider] = append(byProvider[r.Provider], r)
	}
	sort.Strings(names)
	for _, name := range names {
		switch statusRank(byProvider[name]) {
		case StatusOK:
			ok++
		case StatusFail:
			fail++
		default:
			unsupported++
		}
	}
	var parts []string
	parts = appendCount(parts, "ok", ok)
	parts = appendCount(parts, "fail", fail)
	parts = appendCount(parts, "unsupported", unsupported)
	return fmt.Sprintf("tested %d providers: %s",
		len(names), strings.Join(parts, ", "))
}

func appendCount(parts []string, label string, n int) []string {
	if n == 0 {
		return parts
	}
	return append(parts, fmt.Sprintf("%d %s", n, label))
}

// renderError renders the table's ERROR cell as "[kind] message…". The kind
// prefix gives an at-a-glance reason; the full message appears below the table.
func renderError(r Result) string {
	if r.Error == "" && r.ErrorKind == ErrKindNone {
		return "-"
	}
	var kind string
	switch r.Status {
	case StatusUnsupported:
		kind = ui.Warn("[" + r.ErrorKind.HumanOrTag() + "]")
	default:
		kind = ui.Fail("[" + r.ErrorKind.HumanOrTag() + "]")
	}
	msg := truncate(r.Error, 46)
	if msg == "" {
		return kind
	}
	return kind + " " + msg
}

// HumanOrTag returns Human() or, for empty kinds, a stable placeholder so the
// bracketed prefix always carries something.
func (k ErrorKind) HumanOrTag() string {
	if h := k.Human(); h != "" {
		return h
	}
	return "error"
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

// truncate cuts s to at most n runes (never bytes), keeping multi-byte text
// such as Chinese error messages intact.
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 1 {
		return "…"
	}
	return string(r[:n-1]) + "…"
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// terminalWidth detects the stdout width for wrapping detail blocks; falls
// back to 80 when not attached to a tty.
func terminalWidth() int {
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 {
		return w
	}
	return 80
}
