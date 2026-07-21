package tester

import "testing"

func TestJoinURL(t *testing.T) {
	cases := []struct {
		base string
		path string
		want string
	}{
		{"https://api.example.com", "/v1/messages", "https://api.example.com/v1/messages"},
		{"https://api.example.com/", "/v1/messages", "https://api.example.com/v1/messages"},
		{"https://api.example.com/v1", "/v1/messages", "https://api.example.com/v1/messages"},
		{"https://api.example.com/v1/", "/v1/messages", "https://api.example.com/v1/messages"},
		{"https://proxy.example.com/anthropic", "/v1/messages", "https://proxy.example.com/anthropic/v1/messages"},
	}
	for _, c := range cases {
		got := joinURL(c.base, c.path)
		if got != c.want {
			t.Errorf("joinURL(%q,%q) = %q, want %q", c.base, c.path, got, c.want)
		}
	}
}

func TestExtractErrMsg(t *testing.T) {
	if got := extractErrMsg(`{"error":{"message":"nope"}}`); got != "nope" {
		t.Fatalf("wanted 'nope', got %q", got)
	}
	if got := extractErrMsg("plain text body"); got != "plain text body" {
		t.Fatalf("wanted plain, got %q", got)
	}
	if got := extractErrMsg(""); got != "(empty body)" {
		t.Fatalf("wanted (empty body), got %q", got)
	}
}

func TestExtractErrMsgTopLevelMessage(t *testing.T) {
	// Some gateways put the message at the top level (no "error" wrapper).
	got := extractErrMsg(`{"request_id":"abc","code":"InvalidParameter","message":"Model not exist."}`)
	if got != "Model not exist." {
		t.Fatalf("wanted top-level message, got %q", got)
	}
}

func TestExtractErrMsgSSEErrorEvent(t *testing.T) {
	// Anthropic-compatible gateways (e.g. Aliyun maas) return errors as an SSE
	// event inside HTTP 400.
	body := "event:error\r\ndata:{\"request_id\":\"abc\",\"code\":\"InvalidParameter\",\"message\":\"Model not exist.\"}\r\n\r\n"
	if got := extractErrMsg(body); got != "Model not exist." {
		t.Fatalf("wanted message from SSE data payload, got %q", got)
	}
}

func TestExtractErrMsgSSEWrappedError(t *testing.T) {
	body := "event:error\ndata:{\"error\":{\"message\":\"Insufficient account balance\",\"type\":\"insufficient_balance\"}}\n\n"
	if got := extractErrMsg(body); got != "Insufficient account balance" {
		t.Fatalf("wanted wrapped error message from SSE, got %q", got)
	}
}
