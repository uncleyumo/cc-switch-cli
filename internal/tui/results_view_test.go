package tui

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"cc-switch-cli/internal/mcptest"
	"cc-switch-cli/internal/tester"
)

func TestRenderTestResultsWrapsAndStaysUTF8(t *testing.T) {
	longErr := strings.Repeat("测试错误信息，用于验证换行与 UTF-8 安全。", 30)
	rows := []testResultRow{
		{Title: "good / stream", Badge: statusOK("OK"), Meta: "ttft 120ms"},
		{Title: "bad / stream", Badge: statusFail("FAIL"), Meta: "http 401", Detail: longErr},
	}
	out := renderTestResults("tested 2 providers: 1 ok, 1 fail", rows, 50)
	if !utf8.ValidString(out) {
		t.Fatal("results view produced invalid UTF-8")
	}
	for i, line := range strings.Split(out, "\n") {
		if w := visibleWidth(line); w > contentWidth(50) {
			t.Errorf("line %d overflows width: %d > %d (%q)", i, w, contentWidth(50), line)
		}
	}
	if !strings.Contains(out, "good / stream") || !strings.Contains(out, "bad / stream") {
		t.Errorf("row titles missing:\n%s", out)
	}
}

func TestRenderTestResultsNarrowFloor(t *testing.T) {
	rows := []testResultRow{{Title: "x / stream", Badge: statusFail("FAIL"), Detail: "boom"}}
	out := renderTestResults("", rows, 10)
	if !strings.Contains(out, "boom") {
		t.Errorf("detail missing in ultra-narrow render:\n%s", out)
	}
}

func TestContentWidthFloor(t *testing.T) {
	if got := contentWidth(5); got != 40 {
		t.Errorf("contentWidth(5) = %d, want floor 40", got)
	}
	if got := contentWidth(100); got != 98 {
		t.Errorf("contentWidth(100) = %d, want 98", got)
	}
}

// visibleWidth strips ANSI escape sequences before measuring; lipgloss inserts
// color codes around styled runs.
func visibleWidth(s string) int {
	var b strings.Builder
	inEscape := false
	for _, r := range s {
		switch {
		case r == '\x1b':
			inEscape = true
		case inEscape && r == 'm':
			inEscape = false
		case !inEscape:
			b.WriteRune(r)
		}
	}
	return len([]rune(b.String()))
}

func TestRenderModelTestResults(t *testing.T) {
	results := []tester.Result{
		{Provider: "a", Check: "stream", Status: tester.StatusOK,
			TTFT: 120 * time.Millisecond, Latency: 480 * time.Millisecond},
		{Provider: "b", Check: "stream", Status: tester.StatusFail,
			ErrorKind: tester.ErrKindAuth, Error: "http 401: bad key", HTTPStatus: 401},
	}
	rows := renderModelTestResults(results)
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if !strings.Contains(rows[0].Meta, "ttft 120ms") || !strings.Contains(rows[0].Meta, "latency 480ms") {
		t.Errorf("ok row meta incomplete: %q", rows[0].Meta)
	}
	if !strings.Contains(rows[0].Badge, "OK") {
		t.Errorf("ok row badge wrong: %q", rows[0].Badge)
	}
	if rows[1].Detail != "auth error: http 401: bad key" {
		t.Errorf("fail row should prefix the classified reason, got %q", rows[1].Detail)
	}
	if !strings.Contains(rows[1].Meta, "http 401") {
		t.Errorf("fail row meta missing http code: %q", rows[1].Meta)
	}
}

func TestRenderMCPTestResults(t *testing.T) {
	results := []mcptest.Result{
		{Name: "gh", Scope: "user", Status: mcptest.StatusOK, ToolCount: 8, LatencyMS: 340},
		{Name: "bad", Scope: "project", Status: mcptest.StatusFail, Error: "stdio: exit code 1\r\n boom"},
	}
	rows := renderMCPTestResults(results)
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0].Title != "[user] gh" || !strings.Contains(rows[0].Meta, "8 tools") {
		t.Errorf("ok row mismatch: %+v", rows[0])
	}
	if rows[1].Detail != "stdio: exit code 1\n boom" { // \r stripped
		t.Errorf("error text not sanitized: %q", rows[1].Detail)
	}
}
