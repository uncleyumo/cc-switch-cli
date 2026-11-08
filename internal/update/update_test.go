package update

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAssetNameEndsWithZip(t *testing.T) {
	if got := AssetName(); !strings.HasSuffix(got, ".zip") || !strings.HasPrefix(got, "ccsc_") {
		t.Fatalf("AssetName = %q", got)
	}
}

func TestChecksumURL(t *testing.T) {
	rel := &Release{Assets: map[string]string{"checksums.txt": "https://x/checksums.txt"}}
	if got := ChecksumURL(rel); got != "https://x/checksums.txt" {
		t.Fatalf("ChecksumURL = %q", got)
	}
	if got := ChecksumURL(&Release{}); got != "" {
		t.Fatalf("missing checksums.txt should yield empty, got %q", got)
	}
}

func TestVerifySHA256(t *testing.T) {
	data := []byte("binary")
	sum := sha256.Sum256(data)
	good := hex.EncodeToString(sum[:]) + "  ccsc_windows_amd64.zip\n"

	if err := VerifySHA256(data, []byte(good), "ccsc_windows_amd64.zip"); err != nil {
		t.Fatalf("valid checksum rejected: %v", err)
	}
	if err := VerifySHA256([]byte("other"), []byte(good), "ccsc_windows_amd64.zip"); err == nil {
		t.Fatal("mismatched checksum accepted")
	}
	if err := VerifySHA256(data, []byte(good), "ccsc_linux_arm64.zip"); err == nil {
		t.Fatal("missing entry accepted")
	}
}

func TestExtractFindsTheMember(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("ccsc")
	_, _ = w.Write([]byte("payload"))
	_ = zw.Close()

	got, err := Extract(buf.Bytes(), "ccsc")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "payload" {
		t.Fatalf("got %q", got)
	}
	if _, err := Extract(buf.Bytes(), "missing"); err == nil {
		t.Fatal("expected an error for a missing member")
	}
}

func TestReplaceKeepsADatedBackup(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, BinaryName())
	if err := os.WriteFile(exe, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}

	backup, err := Replace(exe, []byte("new"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(filepath.Base(backup), ".bak.") {
		t.Fatalf("backup name = %q", backup)
	}
	got, _ := os.ReadFile(exe)
	if string(got) != "new" {
		t.Fatalf("executable = %q", got)
	}
	old, _ := os.ReadFile(backup)
	if string(old) != "old" {
		t.Fatalf("backup = %q", old)
	}

	// A second replace keeps only the newest backup.
	if _, err := Replace(exe, []byte("newest")); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	backups := 0
	for _, e := range entries {
		if strings.Contains(e.Name(), ".bak.") {
			backups++
		}
	}
	if backups != 1 {
		t.Fatalf("expected exactly one backup, found %d", backups)
	}
}
