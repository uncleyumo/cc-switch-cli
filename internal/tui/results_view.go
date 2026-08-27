package tui

import (
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"

	"cc-switch-cli/internal/mcptest"
	"cc-switch-cli/internal/tester"
)

// testResultRow is one entry in the shared post-test detail view shown by the
// model and MCP managers after pressing t / T.
type testResultRow struct {
	Title  string // e.g. "my-model / stream" or "[user] github-server"
	Badge  string // pre-rendered OK / FAIL / OK~ status
	Meta   string // "ttft 120ms  latency 480ms  http 401" (may be empty)
	Detail string // full, untruncated error text (may be empty)
}

// contentWidth clamps the usable width for the results view so very narrow or
// degenerate terminal sizes still produce readable output.
func contentWidth(width int) int {
	if width < 42 {
		return 40
	}
	return width - 2
}

// renderTestResults renders a summary line plus the rows into fixed-width,
// pre-wrapped text for a bubbles viewport (which does not reflow horizontally).
func renderTestResults(summary string, rows []testResultRow, width int) string {
	wrap := lipgloss.NewStyle().Width(contentWidth(width))
	var b strings.Builder
	if summary != "" {
		b.WriteString(wrap.Render(titleStyle.Render(summary)))
		b.WriteString("\n\n")
	}
	for i, r := range rows {
		head := r.Badge + " " + titleStyle.Render(r.Title)
		if r.Meta != "" {
			head += helpStyle.Render("   " + r.Meta)
		}
		b.WriteString(wrap.Render(head))
		b.WriteString("\n")
		if strings.TrimSpace(r.Detail) != "" {
			body := lipgloss.NewStyle().MarginLeft(3).Width(contentWidth(width) - 3).Render(r.Detail)
			b.WriteString(errorStyle.Render(body))
			b.WriteString("\n")
		}
		if i < len(rows)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

// renderModelTestResults converts model-test results into result-view rows.
func renderModelTestResults(results []tester.Result) []testResultRow {
	rows := make([]testResultRow, 0, len(results))
	for _, r := range results {
		row := testResultRow{
			Title: fmt.Sprintf("%s / %s", r.Provider, r.Check),
			Meta: strings.Join(testNonEmpty(
				fmt.Sprintf("ttft %s", renderDur(r.TTFT)),
				fmt.Sprintf("latency %s", renderDur(r.Latency)),
				renderHTTPShort(r.HTTPStatus),
			), "  "),
			Detail: r.Error,
		}
		switch r.Status {
		case tester.StatusOK:
			row.Badge = statusOK("OK")
		case tester.StatusUnsupported:
			row.Badge = statusWarn("OK~")
		default:
			reason := r.ErrorKind.Human()
			row.Badge = statusFail("FAIL")
			if reason == "" && r.Error == "" {
				reason = "check failed"
			}
			if reason != "" {
				if row.Detail == "" {
					row.Detail = reason
				} else if r.ErrorKind != tester.ErrKindOther || !strings.EqualFold(reason, "error") {
					// Put the classified reason in front of the raw message.
					row.Detail = reason + ": " + r.Error
				}
			}
		}
		rows = append(rows, row)
	}
	return rows
}

// renderMCPTestResults converts MCP probe results into result-view rows. MCP
// errors are arbitrary program/HTTP output, so no classification is applied —
// only the shared renderer is reused.
func renderMCPTestResults(results []mcptest.Result) []testResultRow {
	rows := make([]testResultRow, 0, len(results))
	for _, r := range results {
		scope := r.Scope
		if scope != "" {
			scope = "[" + scope + "] "
		}
		row := testResultRow{
			Title:  scope + r.Name,
			Detail: ensureUTF8(r.Error),
			Meta: strings.Join(testNonEmpty(
				mcpLatency(r),
				renderToolCount(r.ToolCount),
			), "  "),
		}
		switch r.Status {
		case mcptest.StatusOK:
			row.Badge = statusOK("OK")
		case mcptest.StatusUnsupported:
			row.Badge = statusWarn("N/A")
		default:
			row.Badge = statusFail("FAIL")
		}
		rows = append(rows, row)
	}
	return rows
}

func mcpLatency(r mcptest.Result) string {
	if r.LatencyMS <= 0 {
		return ""
	}
	return (time.Duration(r.LatencyMS) * time.Millisecond).Round(time.Millisecond).String()
}

func renderToolCount(n int) string {
	if n < 0 {
		return ""
	}
	if n == 1 {
		return "1 tool"
	}
	return fmt.Sprintf("%d tools", n)
}

// summarizeMCP folds a probe batch into one headline line:
// "tested 4 servers: 3 ok, 1 fail".
func summarizeMCP(results []mcptest.Result) string {
	if len(results) == 0 {
		return "no results"
	}
	ok := 0
	fail := 0
	unavailable := 0
	for _, r := range results {
		switch r.Status {
		case mcptest.StatusOK:
			ok++
		case mcptest.StatusFail:
			fail++
		default:
			unavailable++
		}
	}
	var parts []string
	if ok > 0 {
		parts = append(parts, fmt.Sprintf("%d ok", ok))
	}
	if fail > 0 {
		parts = append(parts, fmt.Sprintf("%d fail", fail))
	}
	if unavailable > 0 {
		parts = append(parts, fmt.Sprintf("%d n/a", unavailable))
	}
	return fmt.Sprintf("tested %d servers: %s", len(results), strings.Join(parts, ", "))
}

func testNonEmpty(parts ...string) []string {
	out := parts[:0]
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			out = append(out, p)
		}
	}
	return out
}

func renderDur(d time.Duration) string {
	if d <= 0 {
		return "-"
	}
	return d.Round(time.Millisecond).String()
}

func renderHTTPShort(code int) string {
	if code == 0 {
		return ""
	}
	return fmt.Sprintf("http %d", code)
}

// ensureUTF8 normalizes probe output for the results view: CRLF (typical of
// stdio server stderr) becomes LF and malformed byte runs are coerced so they
// cannot render as garbage.
func ensureUTF8(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	if utf8.ValidString(s) {
		return s
	}
	return string([]rune(s))
}
