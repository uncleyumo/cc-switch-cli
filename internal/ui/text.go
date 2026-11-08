// Package ui holds the small text helpers the plain-text commands share.
package ui

import "strings"

// Width is the number of terminal columns s occupies, counting CJK runes as
// two. Go's fmt padding counts bytes, which makes any table with Chinese
// labels drift.
func Width(s string) int {
	w := 0
	for _, r := range s {
		w += runeWidth(r)
	}
	return w
}

// Pad pads s on the right to the given display width.
func Pad(s string, width int) string {
	if w := Width(s); w >= width {
		return s
	} else {
		return s + strings.Repeat(" ", width-w)
	}
}

// Truncate cuts s to at most width columns, ending with "...". If width is too
// small to hold the marker it returns dots alone.
func Truncate(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if Width(s) <= width {
		return s
	}
	const marker = "..."
	if width <= len(marker) {
		return marker[:width]
	}
	w := 0
	for i, r := range s {
		rw := runeWidth(r)
		if w+rw > width-len(marker) {
			return s[:i] + marker
		}
		w += rw
	}
	return s
}

func runeWidth(r rune) int {
	switch {
	case r < 0x20:
		return 0
	case r < 0x1100:
		return 1
	case r >= 0x1100 && r <= 0x115F, // Hangul Jamo
		r >= 0x2E80 && r <= 0x303E, // CJK radicals, punctuation
		r >= 0x3041 && r <= 0x33FF, // kana, CJK compatibility
		r >= 0x3400 && r <= 0x4DBF, // CJK ext A
		r >= 0x4E00 && r <= 0x9FFF, // CJK unified
		r >= 0xA000 && r <= 0xA4CF, // Yi
		r >= 0xAC00 && r <= 0xD7A3, // Hangul syllables
		r >= 0xF900 && r <= 0xFAFF, // CJK compatibility ideographs
		r >= 0xFE30 && r <= 0xFE6F, // CJK compatibility forms
		r >= 0xFF00 && r <= 0xFF60, // fullwidth forms
		r >= 0xFFE0 && r <= 0xFFE6,
		r >= 0x1F300 && r <= 0x1F64F, // emoji
		r >= 0x1F900 && r <= 0x1F9FF,
		r >= 0x20000 && r <= 0x3FFFD: // CJK ext B+
		return 2
	}
	return 1
}
