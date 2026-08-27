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
// a grouped detail section: one block per provider, identical errors merged,
// full messages in quiet gray so color marks only the verdicts.
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
	fmt.Fprintln(w, SummarizeStyled(results))
	fmt.Fprintln(w, renderGroups(GroupByProvider(results), terminalWidth()))
}

// renderGroups draws one block per provider:
//
//	── mimo ─────────────────────────────
//	  ✗ stream + tool  auth error · http 402 · 165ms
//	      Insufficient account balance
func renderGroups(groups []ProviderGroup, width int) string {
	var b strings.Builder
	for gi, g := range groups {
		if gi > 0 {
			b.WriteString("\n")
		}
		b.WriteString(groupHeader(g.Provider, width))
		b.WriteString("\n")
		for _, l := range g.OKs {
			meta := strings.Join(testNonEmpty(
				"ttft "+renderDuration(l.TTFT),
				"latency "+renderDuration(l.Latency),
				renderHTTPShort(l.HTTPCode),
			), "  ")
			line := ui.Success("✓") + " " + l.Check
			if meta != "" {
				line += ui.Dim("   " + meta)
			}
			b.WriteString("  " + line + "\n")
		}
		for _, f := range g.Failures {
			b.WriteString("  " + renderFailureLine(f) + "\n")
			if msg := strings.TrimSpace(f.Error); msg != "" {
				wrap := lipgloss.NewStyle().Width(maxInt(width-8, 20)).MarginLeft(6)
				b.WriteString(ui.Dim(trimTrail(wrap.Render(msg))) + "\n")
			}
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// groupHeader renders "── name ─────…" faint, with the name bold and the rule
// padded to the terminal width. The rule is quiet on purpose: it separates
// without shouting.
func groupHeader(name string, width int) string {
	used := len([]rune(name)) + 4
	pad := maxInt(width-used, 3)
	return ui.Dim("── ") + ui.Bold(name) + ui.Dim(" "+strings.Repeat("─", pad))
}

// renderFailureLine renders a merged failure: red ✗, the merged check names,
// then the tinted reason and dim context. Only the verdict words carry color.
func renderFailureLine(f Failure) string {
	badge := ui.Fail("✗")
	reasonColor := ui.Fail
	if f.Status == StatusUnsupported {
		badge = ui.Warn("~")
		reasonColor = ui.Warn
	}
	reason := f.Kind.HumanOrTag()
	parts := []string{strings.Join(f.Checks, " + ")}
	if f.Kind != ErrKindOther && f.Kind != ErrKindNone {
		parts = append(parts, reasonColor(reason))
	} else {
		parts = append(parts, ui.Dim(reason))
	}
	if f.HTTPCode != 0 {
		parts = append(parts, ui.Dim("http "+itoaHTTP(f.HTTPCode)))
	}
	return badge + " " + strings.Join(parts, ui.Dim(" · "))
}

func testNonEmpty(parts ...string) []string {
	out := parts[:0]
	for _, p := range parts {
		if strings.TrimSpace(p) != "" && p != "ttft -" && p != "latency -" {
			out = append(out, p)
		}
	}
	return out
}

func renderHTTPShort(code int) string {
	if code == 0 {
		return ""
	}
	return "http " + itoaHTTP(code)
}

func itoaHTTP(code int) string {
	return fmt.Sprintf("%d", code)
}

// trimTrail strips the right-side padding lipgloss adds when wrapping to a
// fixed width, so rendered blocks carry no trailing whitespace.
func trimTrail(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t")
	}
	return strings.Join(lines, "\n")
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
