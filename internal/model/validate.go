package model

import (
	"errors"
	"fmt"
	"strings"
	"unicode"
)

// windowsReserved lists device names that are illegal file names on Windows
// regardless of extension.
var windowsReserved = map[string]struct{}{
	"con": {}, "prn": {}, "aux": {}, "nul": {},
	"com1": {}, "com2": {}, "com3": {}, "com4": {}, "com5": {}, "com6": {}, "com7": {}, "com8": {}, "com9": {},
	"lpt1": {}, "lpt2": {}, "lpt3": {}, "lpt4": {}, "lpt5": {}, "lpt6": {}, "lpt7": {}, "lpt8": {}, "lpt9": {},
}

// ValidateName ensures a provider name is safe to use as a filename and cannot
// escape ~/.cc-switch-cli/models/. Rules:
//   - non-empty, <= 64 chars
//   - only letters, digits, dot, hyphen, underscore
//   - not "." or ".."
//   - no leading dot (hidden files) or leading/trailing whitespace
//   - not a Windows reserved device name (case-insensitive)
func ValidateName(name string) error {
	if name == "" {
		return errors.New("model name must not be empty")
	}
	if strings.TrimSpace(name) != name {
		return errors.New("model name must not have leading or trailing whitespace")
	}
	if len(name) > 64 {
		return fmt.Errorf("model name %q is too long (max 64 chars)", name)
	}
	if name == "." || name == ".." {
		return fmt.Errorf("model name %q is reserved", name)
	}
	if strings.HasPrefix(name, ".") {
		return fmt.Errorf("model name %q must not start with a dot", name)
	}
	for _, r := range name {
		switch {
		case unicode.IsLetter(r) && r < 128: // ASCII letter
		case unicode.IsDigit(r) && r < 128:
		case r == '-', r == '_', r == '.':
		default:
			return fmt.Errorf("model name %q has invalid character %q (allowed: letters, digits, - _ .)", name, r)
		}
	}
	if _, bad := windowsReserved[strings.ToLower(name)]; bad {
		return fmt.Errorf("model name %q is reserved on Windows", name)
	}
	return nil
}

// Validate checks that a Model has the minimum fields required for cc-switch-cli
// to be useful. BaseURL and a model id are required; token is optional (users may
// supply it via environment).
func Validate(m *Model) error {
	if m == nil {
		return errors.New("model is nil")
	}
	if m.BaseURL() == "" {
		return fmt.Errorf("env.%s must not be empty", EnvBaseURL)
	}
	if len(m.Targets()) == 0 {
		return fmt.Errorf("no model id is configured (set env.%s, the top-level \"model\", or an ANTHROPIC_DEFAULT_*_MODEL)", EnvModel)
	}
	for _, pair := range []struct {
		key, val string
	}{
		{EnvMaxContextTokens, m.Env.MaxContextTokens},
		{EnvAutoCompactWindow, m.Env.AutoCompactWindow},
		{EnvAutoCompactWindowTokens, m.Env.AutoCompactWindowTokens},
	} {
		if pair.val == "" {
			continue
		}
		if _, err := parseStoredWindow(pair.val); err != nil {
			return fmt.Errorf("env.%s: %w", pair.key, err)
		}
	}
	return nil
}
