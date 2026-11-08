package group

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseWindow reads a context window the way people actually type it.
//
//	""        -> 0   (do not write)
//	"500k"    -> 500000
//	"567k"    -> 567000
//	"1m"      -> 1000000
//	"2m"      -> 2000000
//	"600000"  -> 600000
//
// A bare number is still accepted so old files and scripts keep working.
func ParseWindow(s string) (int64, error) {
	// Control characters can only come from a mangled paste or a terminal
	// quirk; they are never part of a size.
	raw := strings.TrimSpace(strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, s))
	if raw == "" {
		return 0, nil
	}
	lower := strings.ToLower(raw)
	mult := int64(1)
	switch {
	case strings.HasSuffix(lower, "k"):
		mult = 1_000
		lower = strings.TrimSuffix(lower, "k")
	case strings.HasSuffix(lower, "m"):
		mult = 1_000_000
		lower = strings.TrimSuffix(lower, "m")
	}
	lower = strings.TrimSpace(lower)
	if lower == "" {
		return 0, fmt.Errorf("%q is not a size", raw)
	}
	n, err := strconv.ParseFloat(lower, 64)
	if err != nil {
		return 0, fmt.Errorf("%q is not a size", raw)
	}
	if n < 0 {
		return 0, fmt.Errorf("size cannot be negative")
	}
	return int64(n * float64(mult)), nil
}

// FormatWindow renders a token count the way it was most likely typed:
// 500k rather than 500000, 1m rather than 1000000.
func FormatWindow(n int64) string {
	switch {
	case n <= 0:
		return ""
	case n%1_000_000 == 0:
		return strconv.FormatInt(n/1_000_000, 10) + "m"
	case n%1_000 == 0:
		return strconv.FormatInt(n/1_000, 10) + "k"
	default:
		return strconv.FormatInt(n, 10)
	}
}
