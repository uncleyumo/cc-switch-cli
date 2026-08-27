package tui

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"cc-switch-cli/internal/mcptest"
	"cc-switch-cli/internal/tester"
)

func TestRenderTestGroupsWrapsAndStaysUTF8(t *testing.T) {
	longErr := strings.Repeat("测试错误信息，用于验证换行与 UTF-8 安全。", 30)
	groups := []displayGroup{
		{Name: "good", Lines: []displayLine{
			{Badge: statusOK("✓"), Label: "stream", Meta: "ttft 120ms"},
		}},
		{Name: "bad", Lines: []displayLine{
			{Badge: statusFail("✗"), Label: "stream + tool", Meta: "auth error · http 401", Detail: longErr},
		}},
	}
	out := renderTestGroups("tested 2 providers: 1 ok, 1 fail", groups, 50)
	if !utf8.ValidString(out) {
		t.Fatal("results view produced invalid UTF-8")
	}
	for i, line := range strings.Split(out, "\n") {
		if w := visibleWidth(line); w > contentWidth(50) {
			t.Errorf("line %d overflows width: %d > %d (%q)", i, w, contentWidth(50), line)
		}
	}
	if !strings.Contains(out, "── good") || !strings.Contains(out, "── bad") {
		t.Errorf("group headers missing:\n%s", out)
	}
	// The header rule must stay on the same line as the name; a line made up
	// only of ─ means the rule wrapped and now reads as the previous group's
	// bottom border.
	for i, line := range strings.Split(out, "\n") {
		if line != "" && strings.Trim(line, "─") == "" {
			t.Errorf("line %d is a bare rule (header wrapped):\n%s", i, out)
		}
	}
	if !strings.Contains(out, "stream + tool") {
		t.Errorf("merged failure label missing:\n%s", out)
	}
}

func TestRenderTestGroupsNarrowFloor(t *testing.T) {
	groups := []displayGroup{{Name: "x", Lines: []displayLine{
		{Badge: statusFail("✗"), Label: "stream", Detail: "boom"},
	}}}
	out := renderTestGroups("", groups, 10)
	if !strings.Contains(out, "boom") {
		t.Errorf("detail missing in ultra-narrow render:\n%s", out)
	}
	if !strings.Contains(out, "── x") {
		t.Errorf("group header missing in narrow render:\n%s", out)
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

func TestModelTestGroups(t *testing.T) {
	results := []tester.Result{
		{Provider: "a", Check: "stream", Status: tester.StatusOK,
			TTFT: 120 * time.Millisecond, Latency: 480 * time.Millisecond, HTTPStatus: 200},
		{Provider: "b", Check: "stream", Status: tester.StatusFail,
			ErrorKind: tester.ErrKindAuth, Error: "http 401: bad key", HTTPStatus: 401},
		{Provider: "b", Check: "tool", Status: tester.StatusFail,
			ErrorKind: tester.ErrKindAuth, Error: "http 401: bad key", HTTPStatus: 401},
	}
	groups := modelTestGroups(results)
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}
	b := groups[1]
	if len(b.Lines) != 1 {
		t.Fatalf("identical stream+tool failures should merge into one line, got %d", len(b.Lines))
	}
	line := b.Lines[0]
	if line.Label != "stream + tool" {
		t.Errorf("merged label = %q", line.Label)
	}
	if !strings.Contains(line.Meta, "auth error") || !strings.Contains(line.Meta, "http 401") {
		t.Errorf("failure meta missing reason/http: %q", line.Meta)
	}
	if !strings.Contains(line.Detail, "bad key") {
		t.Errorf("failure detail lost message: %q", line.Detail)
	}
	okLine := groups[0].Lines[0]
	if !strings.Contains(okLine.Meta, "ttft 120ms") || !strings.Contains(okLine.Meta, "http 200") {
		t.Errorf("ok line meta incomplete: %q", okLine.Meta)
	}
}

func TestMCPTestGroups(t *testing.T) {
	results := []mcptest.Result{
		{Name: "gh", Scope: "user", Status: mcptest.StatusOK, Transport: "stdio",
			ToolCount: 8, LatencyMS: 340},
		{Name: "bad", Scope: "project", Status: mcptest.StatusFail, Transport: "http",
			Error: "stdio: exit code 1\r\n boom"},
	}
	groups := mcpTestGroups(results)
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(groups))
	}
	if groups[0].Name != "[user] gh" || !strings.Contains(groups[0].Lines[0].Meta, "8 tools") {
		t.Errorf("ok group mismatch: %+v", groups[0])
	}
	if !strings.Contains(groups[1].Lines[0].Detail, "exit code 1\n boom") { // \r stripped
		t.Errorf("error text not sanitized: %q", groups[1].Lines[0].Detail)
	}
}

func TestSummarizeMCPStyled(t *testing.T) {
	results := []mcptest.Result{
		{Status: mcptest.StatusOK},
		{Status: mcptest.StatusFail},
		{Status: mcptest.StatusUnsupported},
	}
	got := summarizeMCPStyled(results)
	plain := visiblePlain(got)
	if !strings.Contains(plain, "tested 3 servers: 1 ok, 1 fail, 1 n/a") {
		t.Errorf("summarizeMCPStyled plain = %q", plain)
	}
}

// visiblePlain strips ANSI codes for assertion readability.
func visiblePlain(s string) string {
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
	return b.String()
}
