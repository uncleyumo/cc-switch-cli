// Package ui holds tiny helpers shared between plain command output and the
// Bubble Tea TUI. Keep it dependency-light: the TUI packages import styles from
// here, but the plain-output commands must be usable without lipgloss loaded.
package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"
)

// IsTTY reports whether stdout is attached to a terminal. Used to gate the TUI.
func IsTTY() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

// Reusable styles. lipgloss auto-detects the terminal profile and honors
// NO_COLOR / non-TTY output, so we do not have to gate these ourselves.
var (
	Success = lipgloss.NewStyle().Foreground(lipgloss.Color("42")).Render  // green
	Fail    = lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Render // red
	Warn    = lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Render // amber
	Info    = lipgloss.NewStyle().Foreground(lipgloss.Color("39")).Render  // blue
	Dim     = lipgloss.NewStyle().Faint(true).Render
	Bold    = lipgloss.NewStyle().Bold(true).Render
)

// MaskToken returns a value with the middle redacted so it can be shown in
// list output or logs without leaking the full secret. Anything with fewer
// than 8 chars is redacted entirely.
func MaskToken(token string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return ""
	}
	if len(token) <= 8 {
		return strings.Repeat("*", len(token))
	}
	head := token[:4]
	tail := token[len(token)-4:]
	return head + strings.Repeat("*", 6) + tail
}

// MaskSensitive rewrites a provider JSON document so that
// ANTHROPIC_AUTH_TOKEN / ANTHROPIC_API_KEY values are masked. Input that is
// not valid JSON is returned unchanged.
func MaskSensitive(raw []byte) []byte {
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(raw, &doc); err != nil {
		return raw
	}
	envRaw, ok := doc["env"]
	if !ok {
		return raw
	}
	env := map[string]json.RawMessage{}
	if err := json.Unmarshal(envRaw, &env); err != nil {
		return raw
	}
	changed := false
	for _, key := range []string{"ANTHROPIC_AUTH_TOKEN", "ANTHROPIC_API_KEY"} {
		v, ok := env[key]
		if !ok {
			continue
		}
		var s string
		if err := json.Unmarshal(v, &s); err != nil {
			continue
		}
		masked, err := json.Marshal(MaskToken(s))
		if err != nil {
			continue
		}
		env[key] = masked
		changed = true
	}
	if !changed {
		return raw
	}
	newEnv, err := json.Marshal(env)
	if err != nil {
		return raw
	}
	doc["env"] = newEnv
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return raw
	}
	return append(out, '\n')
}

// ScrubTokens returns s with any occurrences of the given tokens replaced by
// their masked form. Empty tokens are ignored.
func ScrubTokens(s string, tokens ...string) string {
	for _, t := range tokens {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		s = strings.ReplaceAll(s, t, MaskToken(t))
	}
	return s
}

// Println is a thin wrapper that lets tests capture output without importing
// lipgloss.
func Println(args ...any) { fmt.Println(args...) }
