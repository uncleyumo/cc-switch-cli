package transfer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSecretFromFile(t *testing.T) {
	dir := t.TempDir()
	write := func(name, content string) string {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
		return path
	}

	cases := []struct {
		name    string
		content string
		want    string
	}{
		{"plain.txt", "my-secret-value\n", "my-secret-value"},
		{"crlf.txt", "my-secret-value\r\n", "my-secret-value"},
		{"no-newline.txt", "my-secret-value", "my-secret-value"},
		{"inner-newline.txt", "line1\nline2\n", "line1\nline2"},
		{"double-newline.txt", "secret\n\n", "secret\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := SecretFromFile(write(tc.name, tc.content))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}

	if _, err := SecretFromFile(write("empty.txt", "")); err == nil {
		t.Fatal("empty secret file must be rejected")
	}
	if _, err := SecretFromFile(write("newline-only.txt", "\r\n")); err == nil {
		t.Fatal("newline-only secret file must be rejected")
	}
	if _, err := SecretFromFile(filepath.Join(dir, "missing.txt")); err == nil {
		t.Fatal("missing secret file must be rejected")
	}
}

func TestSecretFromStdin(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	if _, err := w.WriteString("stdin-secret\n"); err != nil {
		t.Fatalf("write pipe: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close pipe: %v", err)
	}
	old := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = old }()

	got, err := SecretFromFile("-")
	if err != nil {
		t.Fatalf("read stdin: %v", err)
	}
	if got != "stdin-secret" {
		t.Fatalf("got %q", got)
	}
}
