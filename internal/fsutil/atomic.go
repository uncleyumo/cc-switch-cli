// Package fsutil provides small filesystem helpers shared across the CLI.
package fsutil

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// AtomicWrite writes data to path via a temp file in the same directory and
// os.Rename. On Windows, os.Rename maps to MoveFileEx with REPLACE_EXISTING,
// so already-existing targets are replaced atomically. The parent directory
// must exist; use MkdirAll before calling if unsure.
//
// If the write fails at any stage, the original file at path is left intact
// and the temp file is cleaned up.
func AtomicWrite(path string, data []byte, perm os.FileMode) (retErr error) {
	dir := filepath.Dir(path)
	base := filepath.Base(path)

	// CreateTemp guarantees the temp file lives in dir (same volume as target),
	// which is required for os.Rename to work on Windows.
	tmp, err := os.CreateTemp(dir, "."+base+".*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp file in %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			// Best-effort cleanup; don't shadow the real error.
			_ = os.Remove(tmpName)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("failed to sync temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("failed to close temp file: %w", err)
	}
	// Chmod after close: some Windows AV setups deny mode changes on open handles.
	if perm != 0 {
		if err := os.Chmod(tmpName, perm); err != nil {
			// Non-fatal on Windows where perms are largely ignored; log via return.
			return fmt.Errorf("failed to chmod temp file: %w", err)
		}
	}

	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("failed to replace %s: %w", path, err)
	}
	cleanup = false
	return nil
}

// CopyFile copies src to dst using AtomicWrite semantics on dst.
func CopyFile(src, dst string, perm os.FileMode) error {
	f, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open %s: %w", src, err)
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", src, err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("failed to create %s: %w", filepath.Dir(dst), err)
	}
	return AtomicWrite(dst, data, perm)
}

// PathExists reports whether path exists (any type). Errors other than
// os.ErrNotExist propagate.
func PathExists(path string) (bool, error) {
	if _, err := os.Stat(path); err == nil {
		return true, nil
	} else if os.IsNotExist(err) {
		return false, nil
	} else {
		return false, err
	}
}
