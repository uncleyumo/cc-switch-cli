// Package version exposes the build-stamped version metadata.
//
// Values are overwritten at build time via:
//
//	go build -ldflags="-X github.com/uncleyumo/ccsc/internal/version.Version=..."
package version

import (
	"fmt"
	"strconv"
	"strings"
)

var (
	// Version is a semver-ish release tag (e.g. "2.2.0"), or "dev" for local builds.
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

// IsDev reports whether this binary was built without a release tag.
func IsDev() bool {
	v := strings.TrimSpace(Version)
	return v == "" || v == "dev"
}

// Compare returns -1, 0 or 1 as a is older, equal to, or newer than b.
// Both sides may carry a leading "v". Non-numeric segments compare as strings,
// which is enough for the plain MAJOR.MINOR.PATCH tags this project publishes.
func Compare(a, b string) int {
	as := splitVersion(a)
	bs := splitVersion(b)
	for i := 0; i < len(as) || i < len(bs); i++ {
		var x, y string
		if i < len(as) {
			x = as[i]
		}
		if i < len(bs) {
			y = bs[i]
		}
		xn, xerr := strconv.Atoi(x)
		yn, yerr := strconv.Atoi(y)
		if xerr == nil && yerr == nil {
			if xn != yn {
				if xn < yn {
					return -1
				}
				return 1
			}
			continue
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

// IsNewer reports whether latest is a strictly newer release than the running
// build. It is always false for a dev build, so `ccsc upgrade` never claims a
// release is older than an unstamped local binary.
func IsNewer(latest, current string) bool {
	if strings.TrimSpace(latest) == "" || IsDevVersion(current) {
		return false
	}
	return Compare(latest, current) > 0
}

// IsDevVersion reports whether v is an unstamped build.
func IsDevVersion(v string) bool {
	v = strings.TrimSpace(v)
	return v == "" || v == "dev"
}

func splitVersion(v string) []string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	v = strings.TrimPrefix(v, "V")
	if v == "" {
		return nil
	}
	// Drop pre-release/build metadata: 1.2.3-rc1+build -> 1.2.3
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	return strings.Split(v, ".")
}
