package mcp

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestServerRoundTrip(t *testing.T) {
	in := `{
  "type": "http",
  "url": "https://mcp.example.com/mcp",
  "headers": {"Authorization": "Bearer abc", "X-Trace": "1"},
  "timeout": 600000,
  "alwaysLoad": true
}`
	s, err := ParseServer([]byte(in))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if s.Type != "http" || s.URL != "https://mcp.example.com/mcp" {
		t.Fatalf("typed fields wrong: %+v", s)
	}
	if len(s.Rest) != 2 {
		t.Fatalf("want 2 rest keys, got %v", s.Rest)
	}

	out, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back Server
	if err := json.Unmarshal(out, &back); err != nil {
		t.Fatalf("reparse: %v", err)
	}
	if back.Type != s.Type || back.URL != s.URL {
		t.Fatalf("round-trip typed mismatch: %+v", back)
	}
	if string(back.Rest["timeout"]) != "600000" || string(back.Rest["alwaysLoad"]) != "true" {
		t.Fatalf("round-trip rest mismatch: %v", back.Rest)
	}
	// Typed keys must not leak into Rest on reparse.
	if _, ok := back.Rest["url"]; ok {
		t.Fatal("url leaked into Rest")
	}
}

func TestServerStdioRoundTrip(t *testing.T) {
	in := `{"command": "postgres-mcp", "args": ["postgresql://u@h/db", "--access-mode", "unrestricted"], "env": {"PG_TIMEOUT": "5"}}`
	s, err := ParseServer([]byte(in))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if s.Transport() != TransportStdio {
		t.Fatalf("want stdio, got %s", s.Transport())
	}
	out, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(out), `"type"`) {
		t.Fatalf("typeless stdio entry must not gain a type field: %s", out)
	}
	var back Server
	if err := json.Unmarshal(out, &back); err != nil {
		t.Fatalf("reparse: %v", err)
	}
	if back.Command != "postgres-mcp" || len(back.Args) != 3 || back.Env["PG_TIMEOUT"] != "5" {
		t.Fatalf("stdio round-trip mismatch: %+v", back)
	}
}

func TestTransportDetection(t *testing.T) {
	cases := []struct {
		name string
		json string
		want Transport
	}{
		{"typeless command", `{"command": "srv"}`, TransportStdio},
		{"explicit stdio", `{"type": "stdio", "command": "srv"}`, TransportStdio},
		{"http", `{"type": "http", "url": "https://x/mcp"}`, TransportHTTP},
		{"streamable-http alias", `{"type": "streamable-http", "url": "https://x/mcp"}`, TransportHTTP},
		{"sse", `{"type": "sse", "url": "https://x/sse"}`, TransportSSE},
		{"ws", `{"type": "ws", "url": "wss://x/socket"}`, TransportWS},
		{"url without type", `{"url": "https://x/mcp"}`, TransportInvalid},
		{"unknown type", `{"type": "carrier-pigeon", "url": "https://x"}`, TransportInvalid},
		{"empty", `{}`, TransportInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, err := ParseServer([]byte(tc.json))
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if got := s.Transport(); got != tc.want {
				t.Fatalf("got %s, want %s", got, tc.want)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	valid := []string{
		`{"command": "npx", "args": ["-y", "server"]}`,
		`{"type": "http", "url": "https://mcp.example.com/mcp"}`,
		`{"type": "sse", "url": "http://127.0.0.1:8002/sse"}`,
		`{"type": "ws", "url": "wss://example.com/socket"}`,
	}
	for _, in := range valid {
		s, err := ParseServer([]byte(in))
		if err != nil {
			t.Fatalf("parse %s: %v", in, err)
		}
		if err := s.Validate(); err != nil {
			t.Fatalf("validate %s: %v", in, err)
		}
	}

	invalid := map[string]string{
		"stdio without command": `{"type": "stdio"}`,
		"http without url":      `{"type": "http"}`,
		"http with bad scheme":  `{"type": "http", "url": "ftp://x"}`,
		"url without type":      `{"url": "https://x/mcp"}`,
		"mixed command and url": `{"type": "http", "url": "https://x", "command": "srv"}`,
		"empty":                 `{}`,
	}
	for name, in := range invalid {
		s, err := ParseServer([]byte(in))
		if err != nil {
			t.Fatalf("parse %s: %v", in, err)
		}
		if err := s.Validate(); err == nil {
			t.Fatalf("%s: expected validation error", name)
		}
	}

	s, _ := ParseServer([]byte(`{"url": "https://x/mcp"}`))
	err := s.Validate()
	if err == nil || !strings.Contains(err.Error(), `"type"`) {
		t.Fatalf("url-without-type error should mention the missing type, got %v", err)
	}
}

func TestValidateName(t *testing.T) {
	for _, ok := range []string{"fetch-mcp", "db_2", "A1", "x"} {
		if err := ValidateName(ok); err != nil {
			t.Fatalf("%q should be valid: %v", ok, err)
		}
	}
	for _, bad := range []string{"", "has space", "dot.name", "semi:colon", strings.Repeat("a", 65)} {
		if err := ValidateName(bad); err == nil {
			t.Fatalf("%q should be rejected", bad)
		}
	}
	for _, reserved := range []string{"workspace", "claude-in-chrome", "computer-use", "Claude_Preview", "claude_browser"} {
		if err := ValidateName(reserved); err == nil {
			t.Fatalf("reserved name %q accepted", reserved)
		}
	}
}

func TestNormalizeType(t *testing.T) {
	s, _ := ParseServer([]byte(`{"type": "streamable-http", "url": "https://x"}`))
	s.NormalizeType()
	if s.Type != "http" {
		t.Fatalf("want http, got %q", s.Type)
	}
}

func TestMasked(t *testing.T) {
	in := `{
  "type": "http",
  "url": "https://user:hunter2@mcp.example.com/mcp",
  "headers": {"Authorization": "Bearer supersecrettoken", "X-Trace": "keep"},
  "env": {},
  "command": ""
}`
	s, err := ParseServer([]byte(in))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	// Give it stdio-ish extras to test env/args masking on one entry.
	s.Command = ""
	s.Args = []string{"postgresql://postgres:s3cretpw@172.16.0.1:5432/db", "--flag"}
	s.Env = map[string]string{"MYSQL_PASSWORD": "plainpw", "MYSQL_HOST": "db.local"}

	m := s.Masked()

	if m.Headers["X-Trace"] != "keep" {
		t.Fatalf("non-secret header altered: %v", m.Headers)
	}
	if strings.Contains(m.Headers["Authorization"], "supersecrettoken") {
		t.Fatalf("authorization header not masked: %v", m.Headers["Authorization"])
	}
	if strings.Contains(m.Env["MYSQL_PASSWORD"], "plainpw") {
		t.Fatalf("password env not masked: %v", m.Env["MYSQL_PASSWORD"])
	}
	if m.Env["MYSQL_HOST"] != "db.local" {
		t.Fatalf("non-secret env altered: %v", m.Env)
	}
	if strings.Contains(m.Args[0], "s3cretpw") || !strings.Contains(m.Args[0], "****") {
		t.Fatalf("dsn password not redacted: %v", m.Args[0])
	}
	if m.Args[1] != "--flag" {
		t.Fatalf("plain arg altered: %v", m.Args[1])
	}
	if !strings.Contains(m.URL, "****") || strings.Contains(m.URL, "hunter2") {
		t.Fatalf("url password not redacted: %v", m.URL)
	}
	// Original untouched.
	if s.Args[0] == m.Args[0] {
		t.Fatal("Masked must copy args")
	}
	if s.Headers["Authorization"] != "Bearer supersecrettoken" {
		t.Fatal("Masked mutated the receiver")
	}
}

func TestParseScope(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want Scope
		ok   bool
	}{
		{"user", ScopeUser, true},
		{"LOCAL", ScopeLocal, true},
		{"project", ScopeProject, true},
		{"", "", false},
		{"global", "", false},
	} {
		got, err := ParseScope(tc.in)
		if tc.ok && (err != nil || got != tc.want) {
			t.Fatalf("ParseScope(%q) = %q, %v", tc.in, got, err)
		}
		if !tc.ok && err == nil {
			t.Fatalf("ParseScope(%q) should fail", tc.in)
		}
	}
}
