package ui

import (
	"strings"
	"testing"
)

func TestMaskToken(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"short", "*****"},
		{"12345678", "********"},
		{"abcd12345678efgh", "abcd******efgh"},
	}
	for _, c := range cases {
		if got := MaskToken(c.in); got != c.want {
			t.Errorf("MaskToken(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestMaskSensitiveHidesToken(t *testing.T) {
	raw := []byte(`{"env":{"ANTHROPIC_AUTH_TOKEN":"sk-veryveryverylong","ANTHROPIC_BASE_URL":"https://x"}}`)
	out := MaskSensitive(raw)
	s := string(out)
	if strings.Contains(s, "sk-veryveryverylong") {
		t.Fatalf("token leaked: %s", s)
	}
	if !strings.Contains(s, "https://x") {
		t.Fatalf("non-sensitive fields must survive: %s", s)
	}
}

func TestMaskSensitiveNoEnvUnchanged(t *testing.T) {
	raw := []byte(`{"other":"value"}`)
	if string(MaskSensitive(raw)) != string(raw) {
		t.Fatal("input without env should pass through")
	}
}

func TestScrubTokens(t *testing.T) {
	got := ScrubTokens("bad key sk-veryveryverylong found", "sk-veryveryverylong")
	if strings.Contains(got, "sk-veryveryverylong") {
		t.Fatalf("token not scrubbed: %s", got)
	}
}
