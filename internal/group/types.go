// Package group defines the on-disk shape of a ccsc group: one endpoint plus
// the five model slots ccsc renders into Claude Code settings.
//
// A group deliberately does NOT describe a whole settings.json. It only names
// models and the window they live in; turning that into settings keys is a
// one-way, unit-tested transformation in internal/render.
package group

import (
	"strings"
	"time"
)

const (
	// SchemaVersion is the only group schema ccsc v2 writes.
	SchemaVersion = 1

	// OneMillion is the only per-slot window ccsc can express, because
	// Claude Code's only per-model control is the "[1m]" marker on a model id.
	OneMillion int64 = 1_000_000

	// CompactRatioNum/Den is the default auto-compact trigger: 90% of the
	// effective window.
	CompactRatioNum = 9
	CompactRatioDen = 10
)

// SlotName identifies one of the five model slots.
type SlotName string

const (
	Fable    SlotName = "fable"
	Opus     SlotName = "opus"
	Sonnet   SlotName = "sonnet"
	Haiku    SlotName = "haiku"
	Subagent SlotName = "subagent"
)

// ModelSlots are the four family slots Claude Code resolves /model against.
// All four are required: they are what makes the startup fallback always valid.
var ModelSlots = []SlotName{Fable, Opus, Sonnet, Haiku}

// AllSlots includes the subagent slot, which is optional.
var AllSlots = []SlotName{Fable, Opus, Sonnet, Haiku, Subagent}

// Valid reports whether s is a known slot.
func (s SlotName) Valid() bool {
	for _, k := range AllSlots {
		if k == s {
			return true
		}
	}
	return false
}

// EnvToken is the upper-case fragment used in ANTHROPIC_DEFAULT_<X>_MODEL.
func (s SlotName) EnvToken() string { return strings.ToUpper(string(s)) }

// Label is the human-facing name used in the workbench and receipts.
func (s SlotName) Label() string {
	switch s {
	case Fable:
		return "Fable"
	case Opus:
		return "Opus"
	case Sonnet:
		return "Sonnet"
	case Haiku:
		return "Haiku"
	case Subagent:
		return "subagent"
	}
	return string(s)
}

// Slot is one model mapping.
type Slot struct {
	// Model is the model id exactly as the gateway spells it. A trailing
	// "[1m]" is tolerated and normalised rather than duplicated.
	Model string `json:"model"`
	// Name is the display label written to ANTHROPIC_DEFAULT_*_MODEL_NAME.
	Name string `json:"name,omitempty"`
	// Window is either 0 (whatever the gateway defaults to) or OneMillion
	// (render the id with a "[1m]" marker).
	Window int64 `json:"window,omitempty"`
	// Compact is the per-model auto-compact trigger. 0 means "derive it from
	// the effective window".
	Compact int64 `json:"compact,omitempty"`
}

// Empty reports whether the slot has no model assigned.
func (s Slot) Empty() bool { return strings.TrimSpace(s.Model) == "" }

// Is1M reports whether this slot asks for the 1M window.
func (s Slot) Is1M() bool { return s.Window >= OneMillion || hasMarker(s.Model) }

// WireModel is the model id with any window marker stripped. It is the key
// used under modelSettings, where Claude Code keys by the real model id.
func (s Slot) WireModel() string {
	base, _ := splitMarker(s.Model)
	return base
}

// RenderedModel is the value written into ANTHROPIC_DEFAULT_*_MODEL.
func (s Slot) RenderedModel() string {
	base, marked := splitMarker(s.Model)
	if s.Window >= OneMillion || marked {
		return base + marker
	}
	return base
}

const marker = "[1m]"

func hasMarker(model string) bool {
	_, ok := splitMarker(model)
	return ok
}

// splitMarker removes a trailing "[1m]" (case-insensitive).
func splitMarker(model string) (string, bool) {
	s := strings.TrimSpace(model)
	if len(s) >= len(marker) && strings.EqualFold(s[len(s)-len(marker):], marker) {
		return strings.TrimSpace(s[:len(s)-len(marker)]), true
	}
	return s, false
}

// Group is one endpoint plus its model slots.
type Group struct {
	Schema    int               `json:"schema"`
	BaseURL   string            `json:"base_url"`
	Token     string            `json:"token"`
	Window    int64             `json:"window,omitempty"`
	Slots     map[SlotName]Slot `json:"slots"`
	Env       map[string]string `json:"env,omitempty"`
	UpdatedAt time.Time         `json:"updated_at"`

	// Name is the file stem and is never serialised into the file itself.
	Name string `json:"-"`
}

// Slot returns the slot, or a zero Slot when it is unset.
func (g *Group) Slot(n SlotName) Slot { return g.Slots[n] }

// SetSlot assigns a slot, creating the map on first use.
func (g *Group) SetSlot(n SlotName, s Slot) {
	if g.Slots == nil {
		g.Slots = map[SlotName]Slot{}
	}
	g.Slots[n] = s
}

// EffectiveWindow is the context window a slot will actually have: its own
// 1M marker if present, else the group-level hard cap, else 0 (unknown).
func (g *Group) EffectiveWindow(n SlotName) int64 {
	s := g.Slots[n]
	if s.Is1M() {
		return OneMillion
	}
	return g.Window
}

// CompactFor returns the auto-compact trigger for a slot: the explicit value
// when set, otherwise 90% of the effective window, otherwise 0 (unset).
func (g *Group) CompactFor(n SlotName) int64 {
	if s := g.Slots[n]; s.Compact > 0 {
		return s.Compact
	}
	w := g.EffectiveWindow(n)
	if w <= 0 {
		return 0
	}
	return w * CompactRatioNum / CompactRatioDen
}

// HasSubagent reports whether the optional subagent slot is filled in.
func (g *Group) HasSubagent() bool { return !g.Slots[Subagent].Empty() }
