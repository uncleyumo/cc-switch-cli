package migrate

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

const (
	// archivePrefix names the directory holding legacy files.
	archivePrefix = "legacy-models-"
	// archiveDateLayout is the YYYYMMDD stamp in that directory name.
	archiveDateLayout = "20060102"
)

// Archive moves legacyDir under backupRoot as legacy-models-<YYYYMMDD> and
// returns the new path.
//
// The rename is attempted first because it is atomic and cheap. Across volumes
// (or anywhere os.Rename refuses) the directory is copied and then removed.
// A second archive on the same day gets a numeric suffix, so an earlier
// archive is never overwritten.
func Archive(legacyDir, backupRoot string) (string, error) {
	info, err := os.Stat(legacyDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("legacy models directory %s does not exist", legacyDir)
		}
		return "", fmt.Errorf("cannot read %s: %w", legacyDir, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", legacyDir)
	}
	if err := os.MkdirAll(backupRoot, 0o755); err != nil {
		return "", fmt.Errorf("cannot create %s: %w", backupRoot, err)
	}

	dest := archivePath(backupRoot, time.Now())
	renameErr := os.Rename(legacyDir, dest)
	if renameErr == nil {
		return dest, nil
	}

	if err := copyTree(legacyDir, dest); err != nil {
		_ = os.RemoveAll(dest)
		return "", fmt.Errorf("cannot archive %s to %s: rename failed (%v), copy failed: %w",
			legacyDir, dest, renameErr, err)
	}
	if err := os.RemoveAll(legacyDir); err != nil {
		return "", fmt.Errorf("archived to %s but cannot remove %s: %w", dest, legacyDir, err)
	}
	return dest, nil
}

// archivePath picks a free legacy-models-<date> directory, suffixing -2, -3,
// ... when one already exists.
func archivePath(backupRoot string, now time.Time) string {
	base := filepath.Join(backupRoot, archivePrefix+now.Format(archiveDateLayout))
	dest := base
	for i := 2; ; i++ {
		_, err := os.Stat(dest)
		if errors.Is(err, os.ErrNotExist) {
			return dest
		}
		if err != nil {
			// Cannot tell whether it exists; hand the path back and let the
			// caller fail loudly rather than loop forever.
			return dest
		}
		dest = fmt.Sprintf("%s-%d", base, i)
	}
}

// copyTree duplicates src into dst, preserving file modes. Symbolic links are
// followed and their contents copied, which is fine for the plain JSON files a
// v1 install leaves behind.
func copyTree(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(path, target)
	})
}

func copyFile(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Sync(); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}
