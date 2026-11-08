// Package receipt renders what a command actually did: which files it read,
// which it backed up, which it wrote and what changed on disk.
//
// Output is plain ASCII on purpose. No ticks, no arrows, no colour-dependent
// glyphs: the wording carries the meaning, and the block stays readable in any
// terminal, in a pipe, and in a log.
package receipt

import (
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/uncleyumo/ccsc/internal/store"
)

// Kind is the verb shown at the start of a receipt line.
type Kind string

const (
	Write  Kind = "write"
	Read   Kind = "read"
	Clear  Kind = "clear"
	Backup Kind = "backup"
	Skip   Kind = "skip"
	Fetch  Kind = "fetch"
)

// Entry is one line of a receipt.
type Entry struct {
	Kind   Kind
	Path   string
	Detail string
}

// Receipt is the whole report for one command.
type Receipt struct {
	Title    string
	Entries  []Entry
	Warnings []string
	Notes    []string
}

// New starts a receipt with a headline such as "applied cost-effective to global".
func New(title string) *Receipt { return &Receipt{Title: title} }

// Add appends a line. Paths under the home directory are shortened to ~.
func (r *Receipt) Add(kind Kind, path, detail string) *Receipt {
	r.Entries = append(r.Entries, Entry{Kind: kind, Path: store.Shorten(path), Detail: detail})
	return r
}

// Warn appends an actionable problem.
func (r *Receipt) Warn(format string, a ...any) *Receipt {
	r.Warnings = append(r.Warnings, fmt.Sprintf(format, a...))
	return r
}

// Note appends a neutral remark.
func (r *Receipt) Note(format string, a ...any) *Receipt {
	r.Notes = append(r.Notes, fmt.Sprintf(format, a...))
	return r
}

// Empty reports whether the receipt has nothing to say.
func (r *Receipt) Empty() bool {
	return len(r.Entries) == 0 && len(r.Warnings) == 0 && len(r.Notes) == 0
}

// Print renders the receipt into w.
func (r *Receipt) Print(w io.Writer) error {
	_, err := io.WriteString(w, r.Render())
	return err
}

// Render returns the printable block:
//
//	applied cost-effective to global
//
//	  read    ~/.cc-switch-cli/groups/cost-effective.json
//	  backup  ~/.cc-switch-cli/backups/.../20261108-142908.163.json
//	  write   ~/.claude/settings.json   8 keys updated, 1 cleared
//
//	  warning: env.ANTHROPIC_MODEL=opus overrides /model and the startup fallback
func (r *Receipt) Render() string {
	var b strings.Builder
	if r.Title != "" {
		b.WriteString(r.Title)
		b.WriteString("\n")
	}

	width := 0
	for _, e := range r.Entries {
		if n := utf8.RuneCountInString(string(e.Kind)); n > width {
			width = n
		}
	}
	pathWidth := 0
	for _, e := range r.Entries {
		if n := len(e.Path); n > pathWidth {
			pathWidth = n
		}
	}
	if pathWidth > 72 {
		pathWidth = 0
	}

	if len(r.Entries) > 0 {
		b.WriteString("\n")
	}
	for _, e := range r.Entries {
		kind := string(e.Kind)
		b.WriteString("  ")
		b.WriteString(kind)
		b.WriteString(strings.Repeat(" ", width-utf8.RuneCountInString(kind)+2))
		b.WriteString(e.Path)
		if e.Detail != "" {
			if pathWidth > 0 {
				b.WriteString(strings.Repeat(" ", pathWidth-len(e.Path)+2))
			} else {
				b.WriteString("  ")
			}
			b.WriteString(e.Detail)
		}
		b.WriteString("\n")
	}

	if len(r.Notes) > 0 || len(r.Warnings) > 0 {
		b.WriteString("\n")
	}
	for _, n := range r.Notes {
		b.WriteString("  " + n + "\n")
	}
	for _, w := range r.Warnings {
		b.WriteString("  warning: " + w + "\n")
	}
	return b.String()
}
