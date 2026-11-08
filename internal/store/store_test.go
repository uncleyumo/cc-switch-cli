package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAtomicWriteCreatesAndReplaces(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")

	if err := AtomicWrite(path, []byte("first"), 0o600); err != nil {
		t.Fatalf("create: %v", err)
	}
	if got := read(t, path); got != "first" {
		t.Fatalf("got %q", got)
	}
	if err := AtomicWrite(path, []byte("second"), 0o600); err != nil {
		t.Fatalf("replace: %v", err)
	}
	if got := read(t, path); got != "second" {
		t.Fatalf("got %q", got)
	}
}

func TestAtomicWriteLeavesNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.json")
	if err := AtomicWrite(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("stray files left behind: %v", names)
	}
}

func TestHomeHonoursCCSCHome(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(EnvHome, dir)
	got, err := Home()
	if err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.Abs(dir)
	if got != want {
		t.Errorf("Home() = %q, want %q", got, want)
	}
}

func TestPathsLayout(t *testing.T) {
	p := Paths{Root: "/root"}
	cases := map[string]string{
		p.ConfigFile():     "/root/config.toml",
		p.GroupsDir():      "/root/groups",
		p.StateFile():      "/root/state.json",
		p.AuditFile():      "/root/audit.jsonl",
		p.BackupsDir():     "/root/backups",
		p.CacheDir():       "/root/cache/models",
		p.GroupFile("abc"): filepath.Join("/root/groups", "abc.json"),
	}
	for got, want := range cases {
		if filepath.ToSlash(got) != filepath.ToSlash(want) {
			t.Errorf("got %q, want %q", got, want)
		}
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
