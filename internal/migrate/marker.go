package migrate

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/uncleyumo/ccsc/internal/store"
)

// MarkerName is the file whose presence means the v1 migration already ran.
const MarkerName = ".migrated-v2"

// MarkerPath is the marker file under root.
func MarkerPath(root string) string { return filepath.Join(root, MarkerName) }

// IsDone reports whether the v1 to v2 migration was recorded as complete.
func IsDone(root string) bool {
	info, err := os.Stat(MarkerPath(root))
	return err == nil && !info.IsDir()
}

// MarkDone records that the migration finished. Writing the marker is atomic,
// so an interrupted run leaves either the old state or the marker, never a
// half-written file.
func MarkDone(root string) error {
	content := fmt.Sprintf("migrated to v2 at %s\n", time.Now().Format(time.RFC3339))
	if err := store.AtomicWrite(MarkerPath(root), []byte(content), 0o644); err != nil {
		return fmt.Errorf("cannot mark migration complete: %w", err)
	}
	return nil
}
