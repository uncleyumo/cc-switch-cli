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

// displayLine is one row inside a group of the results view.
type displayLine struct {
	Badge  string // pre-rendered ✓ / ✗ / ~
	Label  string // "stream" or "stream + tool"
	Meta   string // dim context: timings, http code, reason
	Detail string // full error text, quiet gray (may be empty)
}

// displayModel is one configured model inside a provider group.
type displayModel struct {
	Badge string // pre-rendered ✓ / ✗ / ~
	Label string // "sonnet · claude-sonnet-5-5"
	Meta  string // dim context: timings or failure reason
	Lines []displayLine
}

// displayGroup is one provider (or MCP server) block. Model-test groups carry
// one displayModel per configured model; MCP groups use Lines directly.
type displayGroup struct {
	Name   string
	Models []displayModel
	Lines  []displayLine
}

var (
	groupRule  = lipgloss.NewStyle().Faint(true)
	groupName  = lipgloss.NewStyle().Bold(true)
	detailGray = lipgloss.NewStyle().Faint(true)
)

// contentWidth clamps the usable width for the results view so very narrow or
// degenerate terminal sizes still produce readable output.
func contentWidth(width int) int {
	if width < 42 {
		return 40
	}
	return width - 2
}

// renderTestGroups renders a summary line plus one block per provider for a
// bubbles viewport (which does not reflow horizontally, so everything is
// pre-wrapped). Color marks only the verdicts; error bodies stay quiet gray.
func renderTestGroups(summary string, groups []displayGroup, width int) string {
	wrap := lipgloss.NewStyle().Width(contentWidth(width))
	var b strings.Builder
	if summary != "" {
		b.WriteString(wrap.Render(titleStyle.Render(summary)))
		b.WriteString("\n")
	}
	for _, g := range groups {
		b.WriteString("\n")
		b.WriteString(wrap.Render(groupHeader(g.Name, width)))
		b.WriteString("\n")
		for _, dm := range g.Models {
			line := dm.Badge + " " + dm.Label
			if strings.TrimSpace(dm.Meta) != "" {
				line += helpStyle.Render("   " + dm.Meta)
			}
			b.WriteString(wrap.Render("  " + line))
			b.WriteString("\n")
			renderDisplayLines(&b, dm.Lines, "    ", 8, width)
		}
		renderDisplayLines(&b, g.Lines, "  ", 6, width)
	}
	return b.String()
}

// renderDisplayLines writes check rows at the given indent, wrapping detail
// text to the remaining content width.
func renderDisplayLines(b *strings.Builder, lines []displayLine, indent string, detailMargin, width int) {
	wrap := lipgloss.NewStyle().Width(contentWidth(width))
	for _, l := range lines {
		line := l.Badge + " " + l.Label
		if strings.TrimSpace(l.Meta) != "" {
			line += helpStyle.Render("   " + l.Meta)
		}
		b.WriteString(wrap.Render(indent + line))
		b.WriteString("\n")
		if strings.TrimSpace(l.Detail) != "" {
			body := lipgloss.NewStyle().
				MarginLeft(detailMargin).Width(contentWidth(width) - detailMargin).Render(l.Detail)
			b.WriteString(detailGray.Render(trimTrail(body)))
			b.WriteString("\n")
		}
	}
}

// groupHeader renders "── name ─────…" with the rule padded to the content
// width (not the raw terminal width — renderTestGroups wraps at the narrower
// content width, and an overlong header would spill the rule onto its own
// line, where it reads as the previous group's bottom border). The rule is
// faint on purpose: it separates groups without shouting.
func groupHeader(name string, width int) string {
	used := len([]rune(name)) + 4
	pad := contentWidth(width) - used
	if pad < 3 {
		pad = 3
	}
	return groupRule.Render("── ") + groupName.Render(name) + groupRule.Render(" "+strings.Repeat("─", pad))
}

// modelTestGroups converts a model-test batch into grouped display rows using
// the shared grouping/dedup logic from the tester package. Each configured
// model gets its own heading and check list under the provider group.
func modelTestGroups(results []tester.Result) []displayGroup {
	groups := make([]displayGroup, 0)
	for _, g := range tester.GroupByProvider(results) {
		dg := displayGroup{Name: g.Provider}
		for _, mg := range g.Models {
			lines := modelDisplayLines(mg)
			if mg.ModelID == "" && mg.Slot == "" {
				dg.Lines = append(dg.Lines, lines...)
				continue
			}
			dg.Models = append(dg.Models, displayModel{
				Badge: modelBadge(mg.Worst()),
				Label: mg.Label(),
				Meta:  modelMetaText(mg),
				Lines: lines,
			})
		}
		groups = append(groups, dg)
	}
	return groups
}

// modelBadge maps a model verdict to its glyph.
func modelBadge(s tester.Status) string {
	switch s {
	case tester.StatusOK:
		return statusOK("✓")
	case tester.StatusFail:
		return statusFail("✗")
	default:
		return statusWarn("~")
	}
}

// modelDisplayLines flattens one model group into rows: passing checks first,
// then merged failures with their full error body.
func modelDisplayLines(mg tester.ModelGroup) []displayLine {
	lines := make([]displayLine, 0, len(mg.OKs)+len(mg.Failures))
	for _, ok := range mg.OKs {
		lines = append(lines, displayLine{
			Badge: statusOK("✓"),
			Label: ok.Check,
			Meta: strings.Join(testNonEmpty(
				"ttft "+renderDur(ok.TTFT),
				"latency "+renderDur(ok.Latency),
				renderHTTPShort(ok.HTTPCode),
			), "  "),
		})
	}
	for _, f := range mg.Failures {
		lines = append(lines, failureLine(f))
	}
	return lines
}

// modelMetaText summarizes timings for a healthy model, or the first failure's
// reason/http/latency for a broken one.
func modelMetaText(mg tester.ModelGroup) string {
	if len(mg.Failures) > 0 {
		f := mg.Failures[0]
		parts := []string{f.Kind.HumanOrTag()}
		if f.HTTPCode != 0 {
			parts = append(parts, fmt.Sprintf("http %d", f.HTTPCode))
		}
		if f.Latency > 0 {
			parts = append(parts, renderDur(f.Latency))
		}
		return strings.Join(parts, " · ")
	}
	var ttft, latency time.Duration
	for _, ok := range mg.OKs {
		if ok.Check == "stream" {
			ttft = ok.TTFT
		}
		if ok.Latency > latency {
			latency = ok.Latency
		}
	}
	if ttft == 0 {
		for _, ok := range mg.OKs {
			if ok.TTFT > 0 && (ttft == 0 || ok.TTFT < ttft) {
				ttft = ok.TTFT
			}
		}
	}
	return strings.Join(testNonEmpty(
		"ttft "+renderDur(ttft),
		"latency "+renderDur(latency),
	), " · ")
}

// failureLine tints only the verdict glyph and the reason word; timings and
// the http code ride along dim.
func failureLine(f tester.Failure) displayLine {
	badge := statusFail("✗")
	reasonColor := statusFail
	if f.Status == tester.StatusUnsupported {
		badge = statusWarn("~")
		reasonColor = statusWarn
	}
	meta := ""
	if f.Kind != tester.ErrKindOther && f.Kind != tester.ErrKindNone {
		meta = reasonColor(f.Kind.HumanOrTag())
	} else {
		meta = helpStyle.Render(f.Kind.HumanOrTag())
	}
	if f.HTTPCode != 0 {
		meta += helpStyle.Render(fmt.Sprintf(" · http %d", f.HTTPCode))
	}
	if f.Latency > 0 {
		meta += helpStyle.Render(" · " + renderDur(f.Latency))
	}
	return displayLine{
		Badge:  badge,
		Label:  strings.Join(f.Checks, " + "),
		Meta:   meta,
		Detail: ensureUTF8(f.Error),
	}
}

// mcpTestGroups converts an MCP probe batch into one group per server. MCP
// errors are arbitrary program/HTTP output, so no classification is applied.
func mcpTestGroups(results []mcptest.Result) []displayGroup {
	groups := make([]displayGroup, 0, len(results))
	for _, r := range results {
		name := r.Name
		if r.Scope != "" {
			name = "[" + r.Scope + "] " + r.Name
		}
		line := displayLine{
			Badge:  statusFail("✗"),
			Label:  r.Transport,
			Meta:   strings.Join(testNonEmpty(mcpLatency(r), renderToolCount(r.ToolCount)), "  "),
			Detail: ensureUTF8(r.Error),
		}
		switch r.Status {
		case mcptest.StatusOK:
			line.Badge = statusOK("✓")
		case mcptest.StatusUnsupported:
			line.Badge = statusWarn("~")
		}
		groups = append(groups, displayGroup{Name: name, Lines: []displayLine{line}})
	}
	return groups
}

// summarizeMCPStyled folds a probe batch into a headline with tinted counts.
func summarizeMCPStyled(results []mcptest.Result) string {
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
		parts = append(parts, statusOK(fmt.Sprintf("%d ok", ok)))
	}
	if fail > 0 {
		parts = append(parts, statusFail(fmt.Sprintf("%d fail", fail)))
	}
	if unavailable > 0 {
		parts = append(parts, statusWarn(fmt.Sprintf("%d n/a", unavailable)))
	}
	return fmt.Sprintf("tested %d servers: %s", len(results), strings.Join(parts, ", "))
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

// trimTrail strips the right-side padding lipgloss adds when wrapping to a
// fixed width, so rendered blocks carry no trailing whitespace.
func trimTrail(s string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = strings.TrimRight(l, " \t")
	}
	return strings.Join(lines, "\n")
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
