package tester

import (
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
