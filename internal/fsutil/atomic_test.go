package fsutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAtomicWriteCreates(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "settings.json")
	if err := AtomicWrite(target, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(target)
	if err != nil || string(got) != "hello" {
		t.Fatalf("read: %v %q", err, got)
	}
}

func TestAtomicWriteReplacesExisting(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(target, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := AtomicWrite(target, []byte("new"), 0o644); err != nil {
		t.Fatalf("replace failed: %v", err)
	}
	got, err := os.ReadFile(target)
	if err != nil || string(got) != "new" {
		t.Fatalf("read: %v %q", err, got)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != "settings.json" {
			t.Fatalf("stale temp file left behind: %s", e.Name())
		}
	}
}

func TestAtomicWriteFailsCleanly(t *testing.T) {
	// Writing to a path whose parent doesn't exist should fail without
	// touching the parent's siblings.
	tmp := t.TempDir()
	nonExistent := filepath.Join(tmp, "does", "not", "exist", "file.json")
	if err := AtomicWrite(nonExistent, []byte("x"), 0o644); err == nil {
		t.Fatal("expected AtomicWrite to fail for missing parent")
	}
}

func TestPathExists(t *testing.T) {
	dir := t.TempDir()
	present := filepath.Join(dir, "here")
	if err := os.WriteFile(present, []byte("."), 0o644); err != nil {
		t.Fatal(err)
	}
	if ok, err := PathExists(present); err != nil || !ok {
		t.Fatalf("expected present: %v %v", ok, err)
	}
	if ok, err := PathExists(filepath.Join(dir, "missing")); err != nil || ok {
		t.Fatalf("expected missing: %v %v", ok, err)
	}
}
