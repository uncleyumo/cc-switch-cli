package tester

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTruncateRuneSafe(t *testing.T) {
	// CJK text previously got cut on a byte boundary, tearing a rune in half.
	long := strings.Repeat("模", 100)
	got := truncate(long, 60)
	if !utf8.ValidString(got) {
		t.Fatal("truncate produced invalid UTF-8 for multi-byte input")
	}
	if n := len([]rune(got)); n != 60 {
		t.Errorf("truncate rune count = %d, want 60", n)
	}
	if truncate("short", 60) != "short" {
		t.Error("short strings must pass through unchanged")
	}
	if got := truncate("abcdef", 3); got != "ab…" {
		t.Errorf("truncate = %q, want %q", got, "ab…")
	}
}

func TestRenderErrorPrefix(t *testing.T) {
	r := Result{Status: StatusFail, ErrorKind: ErrKindAuth,
		Error: "http 401: your credit balance is too low to access this endpoint"}
	got := renderError(r)
	// With colors on, the prefix and message are still contiguous substrings.
	if !strings.Contains(got, "[auth error]") {
		t.Errorf("renderError missing kind prefix: %q", got)
	}
	if !strings.Contains(got, "http 401") {
		t.Errorf("renderError missing message head: %q", got)
	}
	// Kind only (no message) must not end with a dangling space.
	bare := renderError(Result{Status: StatusFail, ErrorKind: ErrKindRateLimit})
	if strings.HasSuffix(bare, " ") {
		t.Errorf("renderError has trailing space: %q", bare)
	}
	if renderError(Result{Status: StatusOK}) != "-" {
		t.Error("clean results should render '-'")
	}
}

func TestTruncatedRenderErrorLength(t *testing.T) {
	r := Result{Status: StatusFail, ErrorKind: ErrKindOther, Error: strings.Repeat("汉", 80)}
	got := renderError(r)
	if !utf8.ValidString(got) {
		t.Fatal("renderError produced invalid UTF-8")
	}
}

func TestSummarize(t *testing.T) {
	results := []Result{
		{Provider: "a", Check: "stream", Status: StatusOK},
		{Provider: "b", Check: "stream", Status: StatusFail, Error: "down", ErrorKind: ErrKindNetwork},
		{Provider: "c", Check: "stream", Status: StatusOK},
		{Provider: "c", Check: "tool", Status: StatusUnsupported, Error: "no tools"},
	}
	got := Summarize(results)
	// Provider c folds to unsupported: its tool check failed while stream passed.
	want := "tested 3 providers: 1 ok, 1 fail, 1 unsupported"
	if got != want {
		t.Errorf("Summarize = %q, want %q", got, want)
	}
	if Summarize(nil) != "no results" {
		t.Error("empty results should summarize as 'no results'")
	}
}

func TestPrintResultsIncludesFullDetail(t *testing.T) {
	full := "http 401: your credit balance is too low to access this endpoint for model claude-test"
	results := []Result{
		{Provider: "good", Check: "stream", Status: StatusOK},
		{Provider: "bad", Check: "stream", Status: StatusFail, Error: full, ErrorKind: ErrKindAuth, HTTPStatus: 401},
	}
	w := &strings.Builder{}
	PrintResults(w, results)
	out := w.String()
	if !strings.Contains(out, "── bad ──") {
		t.Errorf("missing provider group header:\n%s", out)
	}
	// lipgloss wraps detail lines to terminal width; flatten whitespace before
	// asserting the full untruncated error survived.
	flat := strings.Join(strings.Fields(out), " ")
	if !strings.Contains(flat, full) {
		t.Errorf("detail block lost the full untruncated error:\n%s", out)
	}
	if !strings.Contains(out, "auth error") {
		t.Errorf("failure line missing tinted reason:\n%s", out)
	}
}

func TestGroupByProviderMergesIdenticalFailures(t *testing.T) {
	same := "http 403: anomalous client"
	results := []Result{
		{Provider: "b", Check: "stream", Status: StatusOK, TTFT: 1},
		{Provider: "a", Check: "stream", Status: StatusFail, Error: same, ErrorKind: ErrKindAuth, HTTPStatus: 403},
		{Provider: "a", Check: "tool", Status: StatusFail, Error: same, ErrorKind: ErrKindAuth, HTTPStatus: 403},
		{Provider: "a", Check: "extra", Status: StatusFail, Error: "different", ErrorKind: ErrKindRateLimit},
	}
	groups := GroupByProvider(results)
	if len(groups) != 2 || groups[0].Provider != "a" {
		t.Fatalf("expected 2 groups sorted by name, got %+v", groups)
	}
	g := groups[0]
	if len(g.Failures) != 2 {
		t.Fatalf("identical failures should merge, distinct stay separate: %+v", g.Failures)
	}
	if strings.Join(g.Failures[0].Checks, ",") != "stream,tool" {
		t.Errorf("merged checks = %v, want stream,tool", g.Failures[0].Checks)
	}
	if g.Worst() != StatusFail {
		t.Errorf("worst = %q, want fail", g.Worst())
	}
	if groups[1].Worst() != StatusOK || len(groups[1].Failures) != 0 {
		t.Errorf("all-ok group should have no failures: %+v", groups[1])
	}
}

func TestSummarizeStyledColorsCounts(t *testing.T) {
	results := []Result{
		{Provider: "a", Check: "stream", Status: StatusOK},
		{Provider: "b", Check: "stream", Status: StatusFail, Error: "down"},
	}
	plain := Summarize(results)
	t.Setenv("NO_COLOR", "1")
	if got := SummarizeStyled(results); got != plain {
		t.Errorf("with NO_COLOR, SummarizeStyled = %q, want %q", got, plain)
	}
}
