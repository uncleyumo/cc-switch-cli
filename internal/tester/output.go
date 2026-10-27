package tester

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"

	"cc-switch-cli/internal/ui"
)

// PrintResults renders the batch to w: one summary line, then one block per
// provider and, inside it, one block per configured model with its own verdict
// and full check detail. Full error messages stay in quiet gray so color marks
// only the verdicts.
func PrintResults(w io.Writer, results []Result) {
	fmt.Fprintln(w, SummarizeStyled(results))
	if len(results) == 0 {
		return
	}
	fmt.Fprintln(w, renderGroups(GroupByProvider(results), terminalWidth()))
}

// renderGroups draws one block per provider:
//
//	── mimo ─────────────────────────────
//	  ✓ default · claude-opus-5-5   ttft 210ms · latency 1.1s
//	      ✓ stream   ttft 210ms · latency 1.1s · http 200
//	      ✓ tool     ttft 180ms · latency 900ms · http 200
//	  ✗ sonnet · claude-sonnet-5-5  auth error · http 401
//	      ✗ stream + tool   auth error · http 401
//	          Insufficient account balance
func renderGroups(groups []ProviderGroup, width int) string {
	var b strings.Builder
	for gi, g := range groups {
		if gi > 0 {
			b.WriteString("\n")
		}
		b.WriteString(groupHeader(g.Provider, width))
		b.WriteString("\n")
		for _, mg := range g.Models {
			if mg.Label() == "" {
				renderModelLines(&b, mg, "  ", width)
				continue
			}
			b.WriteString(renderModelHeader(mg))
			b.WriteString("\n")
			renderModelLines(&b, mg, "    ", width)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// renderModelHeader is the one-line verdict for one configured model, with its
// best timings when it works or the failure reason when it does not.
func renderModelHeader(mg ModelGroup) string {
	badge := ui.Success("✓")
	if mg.Worst() == StatusUnsupported {
		badge = ui.Warn("~")
	} else if mg.Worst() == StatusFail {
		badge = ui.Fail("✗")
	}
	line := "  " + badge + " " + ui.Bold(mg.Label())
	if meta := modelMeta(mg); meta != "" {
		line += ui.Dim("   " + meta)
	}
	return line
}

// modelMeta summarizes timings for a healthy model, or the first failure's
// reason/http/latency for a broken one.
func modelMeta(mg ModelGroup) string {
	if len(mg.Failures) > 0 {
		f := mg.Failures[0]
		parts := []string{f.Kind.HumanOrTag()}
		if f.HTTPCode != 0 {
			parts = append(parts, "http "+itoaHTTP(f.HTTPCode))
		}
		if f.Latency > 0 {
			parts = append(parts, f.Latency.Round(time.Millisecond).String())
		}
		return strings.Join(parts, " · ")
	}
	var ttft time.Duration
	var latency time.Duration
	for _, ok := range mg.OKs {
		// Stream is the availability signal; prefer its TTFT over the tool
		// probe's when both passed.
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
		"ttft "+renderDuration(ttft),
		"latency "+renderDuration(latency),
	), " · ")
}

// renderModelLines writes the per-check rows for one model at the given indent.
func renderModelLines(b *strings.Builder, mg ModelGroup, indent string, width int) {
	for _, l := range mg.OKs {
		meta := strings.Join(testNonEmpty(
			"ttft "+renderDuration(l.TTFT),
			"latency "+renderDuration(l.Latency),
			renderHTTPShort(l.HTTPCode),
		), " · ")
		line := indent + ui.Success("✓") + " " + l.Check
		if meta != "" {
			line += ui.Dim("   " + meta)
		}
		b.WriteString(line + "\n")
	}
	for _, f := range mg.Failures {
		b.WriteString(indent + renderFailureLine(f) + "\n")
		if msg := strings.TrimSpace(f.Error); msg != "" {
			wrap := lipgloss.NewStyle().Width(maxInt(width-len(indent)-6, 20)).MarginLeft(len(indent) + 2)
			b.WriteString(ui.Dim(trimTrail(wrap.Render(msg))) + "\n")
		}
	}
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
	if f.Latency > 0 {
		parts = append(parts, ui.Dim(f.Latency.Round(time.Millisecond).String()))
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

// Summarize folds the batch into one headline line with per-model verdicts:
// "tested 5 providers: 3 ok, 1 fail, 1 unsupported".
func Summarize(results []Result) string {
	if len(results) == 0 {
		return "no results"
	}
	providers, models, ok, fail, unsupported := modelTally(results)
	var parts []string
	parts = appendCount(parts, "ok", ok)
	parts = appendCount(parts, "fail", fail)
	parts = appendCount(parts, "unsupported", unsupported)
	return summarizeLine(providers, models) + ": " + strings.Join(parts, ", ")
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

func renderDuration(d time.Duration) string {
	if d <= 0 {
		return "-"
	}
	return d.Round(time.Millisecond).String()
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
