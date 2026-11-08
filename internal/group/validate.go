package group

import (
	"fmt"
	"net/url"
	"strings"
)

// Validate checks everything ccsc needs to render a settings file. It is run
// on load and on save so a hand-edited group fails loudly instead of silently
// writing a broken configuration.
func (g *Group) Validate() error {
	if g.Schema != 0 && g.Schema != SchemaVersion {
		return fmt.Errorf("unsupported schema %d (this build understands %d)", g.Schema, SchemaVersion)
	}
	if strings.TrimSpace(g.BaseURL) == "" {
		return fmt.Errorf("base_url is empty")
	}
	if err := ValidateBaseURL(g.BaseURL); err != nil {
		return err
	}
	if strings.TrimSpace(g.Token) == "" {
		return fmt.Errorf("token is empty")
	}
	if g.Window < 0 {
		return fmt.Errorf("window must not be negative")
	}
	for _, name := range ModelSlots {
		if g.Slots[name].Empty() {
			return fmt.Errorf("slot %s has no model", name)
		}
	}
	for name, slot := range g.Slots {
		if !name.Valid() {
			return fmt.Errorf("unknown slot %q", name)
		}
		if slot.Empty() {
			continue
		}
		if slot.Window != 0 && slot.Window < OneMillion {
			return fmt.Errorf("slot %s window %d: only 0 or 1000000 can be expressed per model; use the group-level window for anything else", name, slot.Window)
		}
		if slot.Compact < 0 {
			return fmt.Errorf("slot %s compact must not be negative", name)
		}
	}
	return nil
}

// ValidateBaseURL accepts only absolute http(s) URLs.
func ValidateBaseURL(raw string) error {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("base_url %q is not a valid URL: %w", raw, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("base_url %q must start with http:// or https://", raw)
	}
	if u.Host == "" {
		return fmt.Errorf("base_url %q has no host", raw)
	}
	return nil
}

// NormalizeBaseURL trims whitespace and a trailing slash so the same endpoint
// always renders identically.
func NormalizeBaseURL(raw string) string {
	return strings.TrimRight(strings.TrimSpace(raw), "/")
}

// Host returns the host[:port] of the group's endpoint, for display.
func (g *Group) Host() string {
	u, err := url.Parse(g.BaseURL)
	if err != nil || u.Host == "" {
		return g.BaseURL
	}
	return u.Host
}

// MaskedToken renders the token so two keys can be told apart without either
// being usable: sk-tes******mnop.
func MaskedToken(token string) string {
	t := strings.TrimSpace(token)
	if t == "" {
		return "(not set)"
	}
	r := []rune(t)
	if len(r) <= 12 {
		return strings.Repeat("*", len(r))
	}
	return string(r[:6]) + "******" + string(r[len(r)-4:])
}

// Masked is the group's token in masked form.
func (g *Group) Masked() string { return MaskedToken(g.Token) }
