package tester

import (
	"context"
	"strings"
	"testing"
)

func TestReadSSEBasic(t *testing.T) {
	body := strings.NewReader(strings.Join([]string{
		"event: message_start",
		`data: {"type":"message_start","message":{"id":"1"}}`,
		"",
		"event: content_block_delta",
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"hi"}}`,
		"",
		"event: message_stop",
		`data: {"type":"message_stop"}`,
		"",
	}, "\n"))

	var got []string
	err := readSSE(context.Background(), body, func(ev sseEvent) (bool, error) {
		got = append(got, ev.Type)
		return ev.Type == "message_stop", nil
	})
	if err != nil {
		t.Fatalf("readSSE: %v", err)
	}
	want := []string{"message_start", "content_block_delta", "message_stop"}
	if len(got) != len(want) {
		t.Fatalf("event count mismatch: %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("event %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestReadSSEHandlesCRLFAndComments(t *testing.T) {
	body := strings.NewReader(": comment keep-alive\r\n" +
		"event: message_stop\r\n" +
		"data: {\"type\":\"message_stop\"}\r\n" +
		"\r\n")
	var seen bool
	err := readSSE(context.Background(), body, func(ev sseEvent) (bool, error) {
		if ev.Type == "message_stop" {
			seen = true
			return true, nil
		}
		return false, nil
	})
	if err != nil {
		t.Fatalf("readSSE: %v", err)
	}
	if !seen {
		t.Fatal("did not observe message_stop after CRLF/comment")
	}
}

func TestReadSSEMultilineData(t *testing.T) {
	// Two data: lines are concatenated with \n per the spec.
	payload := "data: {\"type\":\"message_delta\",\n" +
		"data: \"delta\":{\"stop_reason\":\"tool_use\"}}\n\n"
	body := strings.NewReader(payload)
	var stop string
	err := readSSE(context.Background(), body, func(ev sseEvent) (bool, error) {
		if ev.Delta != nil {
			stop = ev.Delta.StopReason
		}
		return true, nil
	})
	if err != nil {
		t.Fatalf("readSSE: %v", err)
	}
	if stop != "tool_use" {
		t.Fatalf("expected multiline data to reconstruct: got %q", stop)
	}
}
