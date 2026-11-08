package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

// AtomicWrite replaces path with data without ever leaving a half-written
// file behind.
//
// The temp file is created in the target's own directory so the final
// os.Rename stays on one volume; on Windows os.Rename maps to MoveFileEx with
// MOVEFILE_REPLACE_EXISTING, so an existing target is replaced atomically.
// On Windows a rename can fail transiently when another process (an editor, an
// antivirus scanner) briefly holds the file open, so the rename is retried
// with backoff before giving up.
func AtomicWrite(path string, data []byte, perm os.FileMode) (err error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("cannot create %s: %w", dir, err)
	}

	base := filepath.Base(path)
	tmp, err := os.CreateTemp(dir, "."+base+".*.tmp")
	if err != nil {
		return fmt.Errorf("cannot create temp file in %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(tmpName)
		}
	}()

	if _, err = tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("cannot write temp file: %w", err)
	}
	if err = tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("cannot flush temp file: %w", err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("cannot close temp file: %w", err)
	}
	// Best effort: Windows ignores most of the mode bits anyway.
	if perm != 0 {
		_ = os.Chmod(tmpName, perm)
	}

	if err = renameWithRetry(tmpName, path); err != nil {
		return fmt.Errorf("cannot replace %s: %w", path, err)
	}
	syncDir(dir)
	return nil
}

// renameWithRetry retries only on Windows, where a sharing violation is
// transient. Elsewhere the first error is returned unchanged.
func renameWithRetry(oldPath, newPath string) error {
	var err error
	backoff := 10 * time.Millisecond
	for attempt := 0; attempt < 6; attempt++ {
		if err = os.Rename(oldPath, newPath); err == nil {
			return nil
		}
		if runtime.GOOS != "windows" {
			return err
		}
		// Only a sharing/lock failure is worth retrying.
		if !errors.Is(err, os.ErrPermission) && !isSharingViolation(err) {
			return err
		}
		time.Sleep(backoff)
		backoff *= 2
	}
	return err
}

// isSharingViolation matches the Windows error text for ERROR_SHARING_VIOLATION
// and ERROR_ACCESS_DENIED surfaced through os.Rename.
func isSharingViolation(err error) bool {
	msg := err.Error()
	for _, needle := range []string{
		"being used by another process",
		"used by another process",
		"sharing violation",
		"Access is denied",
		"拒绝访问",
	} {
		if contains(msg, needle) {
			return true
		}
	}
	return false
}

// syncDir flushes the directory entry after a rename so the replacement
// survives a power loss. Windows does not allow opening a directory this way,
// and the call is best effort everywhere.
func syncDir(dir string) {
	if runtime.GOOS == "windows" {
		return
	}
	d, err := os.Open(dir)
	if err != nil {
		return
	}
	_ = d.Sync()
	_ = d.Close()
}

// ReadFileOrEmpty returns the file contents, or nil when the file does not
// exist. Any other error is returned.
func ReadFileOrEmpty(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	return data, nil
}

func contains(haystack, needle string) bool {
	return len(needle) == 0 || indexOf(haystack, needle) >= 0
}

func indexOf(haystack, needle string) int {
	n := len(needle)
	for i := 0; i+n <= len(haystack); i++ {
		if haystack[i:i+n] == needle {
			return i
		}
	}
	return -1
}
