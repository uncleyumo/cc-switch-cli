package tester

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStatusUpsertPicksWorstAndTimestamp(t *testing.T) {
	sf := &StatusFile{Results: map[string]ProviderStatus{}}
	sf.Upsert([]Result{
		{Provider: "a", Check: "stream", Status: StatusOK, Latency: 100 * time.Millisecond},
		{Provider: "a", Check: "tool", Status: StatusUnsupported, Latency: 50 * time.Millisecond, Error: "no tools"},
	})
	a, ok := sf.Results["a"]
	if !ok {
		t.Fatal("provider a missing")
	}
	if a.Status != StatusUnsupported {
		t.Fatalf("worst status should be unsupported, got %s", a.Status)
	}
	if a.LatencyMS < 100 {
		t.Fatalf("expected max latency, got %d", a.LatencyMS)
	}
	if a.TestedAt == "" {
		t.Fatalf("tested_at not stamped")
	}
	if _, err := time.Parse(time.RFC3339, a.TestedAt); err != nil {
		t.Fatalf("tested_at not RFC3339: %v", err)
	}
}

func TestStatusUpsertOKWhenAllOK(t *testing.T) {
	sf := &StatusFile{Results: map[string]ProviderStatus{}}
	sf.Upsert([]Result{
		{Provider: "b", Check: "stream", Status: StatusOK},
		{Provider: "b", Check: "tool", Status: StatusOK},
	})
	if sf.Results["b"].Status != StatusOK {
		t.Fatalf("expected ok, got %s", sf.Results["b"].Status)
	}
	if sf.Results["b"].ErrorKind != ErrKindNone {
		t.Fatalf("ok providers must carry no error kind, got %q", sf.Results["b"].ErrorKind)
	}
}

func TestStatusUpsertCarriesErrorKind(t *testing.T) {
	sf := &StatusFile{Results: map[string]ProviderStatus{}}
	sf.Upsert([]Result{
		{Provider: "d", Check: "stream", Status: StatusFail, Error: "http 401: nope", ErrorKind: ErrKindAuth},
		{Provider: "d", Check: "tool", Status: StatusFail, Error: "http 429: slow", ErrorKind: ErrKindRateLimit},
	})
	got := sf.Results["d"]
	if got.Status != StatusFail {
		t.Fatalf("expected fail, got %s", got.Status)
	}
	if got.Error != "http 401: nope" || got.ErrorKind != ErrKindAuth {
		t.Fatalf("first non-OK row not captured with its kind: %q / %q", got.Error, got.ErrorKind)
	}
}

// TestStatusUpsertStreamIsSourceOfTruth: a failed tool probe (including a
// transient 5xx) must NOT fail a provider whose stream check passed — streaming
// is the availability signal. A provider with no passing check stays FAIL.
func TestStatusUpsertStreamIsSourceOfTruth(t *testing.T) {
	sf := &StatusFile{Results: map[string]ProviderStatus{}}
	sf.Upsert([]Result{
		// stream OK + tool FAIL (e.g. transient 520) -> headline is NOT fail.
		{Provider: "c", Check: "stream", Status: StatusOK},
		{Provider: "c", Check: "tool", Status: StatusFail, Error: "boom"},
		// stream FAIL + tool FAIL -> fail (nothing passed).
		{Provider: "d", Check: "stream", Status: StatusFail, Error: "down"},
		{Provider: "d", Check: "tool", Status: StatusFail, Error: "down"},
	})
	if sf.Results["c"].Status != StatusUnsupported {
		t.Fatalf("stream-OK provider must not be FAIL on a tool error, got %s", sf.Results["c"].Status)
	}
	if sf.Results["c"].Error != "boom" {
		t.Fatalf("tool error message not captured: %q", sf.Results["c"].Error)
	}
	if sf.Results["d"].Status != StatusFail {
		t.Fatalf("provider with no passing check should be FAIL, got %s", sf.Results["d"].Status)
	}
}

// Legacy caches written before error_kind existed must still load cleanly.
func TestLoadStatusLegacyFixture(t *testing.T) {
	home := t.TempDir()
	// Home() resolves via os.UserHomeDir: USERPROFILE on Windows, HOME elsewhere.
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".cc-switch-cli")
	path := filepath.Join(dir, statusFileName)
	legacy := `{"timestamp":"2026-01-01T00:00:00Z","results":{"old":` +
		`{"status":"fail","checks":"stream","error":"http 401: bad key","tested_at":"2026-01-01T00:00:00Z"}}}`
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	sf, err := LoadStatus()
	if err != nil {
		t.Fatalf("legacy fixture failed to load: %v", err)
	}
	if sf == nil || sf.Results["old"].Status != StatusFail {
		t.Fatalf("unexpected load result: %+v", sf)
	}
	if sf.Results["old"].ErrorKind != ErrKindNone {
		t.Fatalf("missing error_kind must decode as none, got %q", sf.Results["old"].ErrorKind)
	}
}
