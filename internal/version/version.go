// Package version exposes the build-stamped version metadata.
//
// Values are overwritten at build time via:
//
//	go build -ldflags="-X cc-switch-cli/internal/version.Version=..."
package version

import "fmt"

var (
	// Version is a semver-ish release tag (e.g. "2.0.0").
	Version = "dev"
	// Commit is the short git SHA.
	Commit = "none"
	// Date is the ISO-8601 build date.
	Date = "unknown"
)

// String returns a compact, single-line representation.
func String() string {
	return fmt.Sprintf("%s (commit %s, built %s)", Version, Commit, Date)
}
