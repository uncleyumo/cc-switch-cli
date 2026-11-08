// Package render turns a group into the exact set of settings keys ccsc is
// allowed to touch. It is a pure function: no IO, no environment, no clock.
//
// The owned-key list is the whole contract. Anything not named here is left
// byte-identical in the user's settings file.
package render

import (
	"sort"
	"strconv"

	"github.com/uncleyumo/ccsc/internal/group"
)

// Owned env keys.
const (
	EnvBaseURL                 = "ANTHROPIC_BASE_URL"
	EnvToken                   = "ANTHROPIC_AUTH_TOKEN"
	EnvAPIKey                  = "ANTHROPIC_API_KEY"
	EnvDefaultModel            = "ANTHROPIC_DEFAULT_MODEL"
	EnvSubagentModel           = "CLAUDE_CODE_SUBAGENT_MODEL"
	EnvMaxContextTokens        = "CLAUDE_CODE_MAX_CONTEXT_TOKENS"
	EnvAutoCompactWindow       = "CLAUDE_CODE_AUTO_COMPACT_WINDOW"
	EnvAutoCompactWindowTokens = "CLAUDE_CODE_AUTO_COMPACT_WINDOW_TOKENS"
)

// DefaultModelKey is ANTHROPIC_DEFAULT_<SLOT>_MODEL.
func DefaultModelKey(s group.SlotName) string {
	return "ANTHROPIC_DEFAULT_" + s.EnvToken() + "_MODEL"
}

// DefaultModelNameKey is ANTHROPIC_DEFAULT_<SLOT>_MODEL_NAME.
func DefaultModelNameKey(s group.SlotName) string {
	return "ANTHROPIC_DEFAULT_" + s.EnvToken() + "_MODEL_NAME"
}

// OwnedEnvKeys is every env key ccsc may write and is allowed to clear.
var OwnedEnvKeys = func() []string {
	keys := []string{
		EnvBaseURL,
		EnvToken,
		EnvDefaultModel,
		EnvSubagentModel,
		EnvMaxContextTokens,
		EnvAutoCompactWindow,
		EnvAutoCompactWindowTokens,
	}
	for _, s := range group.ModelSlots {
		keys = append(keys, DefaultModelKey(s), DefaultModelNameKey(s))
	}
	sort.Strings(keys)
	return keys
}()

// Options carries the settings-panel choices that affect rendering.
type Options struct {
	// StartupSlot decides what ANTHROPIC_DEFAULT_MODEL points at. It is the
	// last link in Claude Code's startup chain, so it must always resolve to a
	// real model id.
	StartupSlot group.SlotName
}

func (o Options) startup() group.SlotName {
	if o.StartupSlot.Valid() && o.StartupSlot != group.Subagent {
		return o.StartupSlot
	}
	return group.Sonnet
}

// Plan is the set of changes to apply to one settings document.
type Plan struct {
	SetEnv            map[string]string
	ClearEnv          []string
	SetModelCompact   map[string]int64
	ClearModelCompact []string
	TopCompact        *int64
	ClearTopCompact   bool
}

// Global renders the user-level plan: per-model auto-compact entries live in
// the same file as the user's own modelSettings, so siblings survive.
func Global(g *group.Group, opt Options) Plan {
	p := basePlan(g, opt)
	p.SetModelCompact = slotCompacts(g)
	return p.finalize()
}

// Project renders the project-level plan. It writes ONE top-level
// autoCompactWindow instead of modelSettings entries: whether Claude Code
// merges same-model entries across settings files field-by-field is not
// documented, and a wrong guess would wipe the user's effortLevel.
func Project(g *group.Group, opt Options) Plan {
	p := basePlan(g, opt)
	compacts := slotCompacts(g)
	if len(compacts) > 0 {
		min := int64(0)
		for _, v := range compacts {
			if min == 0 || v < min {
				min = v
			}
		}
		p.TopCompact = &min
	}
	return p.finalize()
}

// slotCompacts resolves one auto-compact trigger per model id.
//
// Two slots can share a model id (a group may point haiku and the subagent
// slot at the same cheap model), so the values have to be reconciled: an
// explicit per-slot value beats a derived 90% one, and among equals the
// smaller value wins because compressing early never overflows.
func slotCompacts(g *group.Group) map[string]int64 {
	out := map[string]int64{}
	explicit := map[string]bool{}
	for _, name := range group.AllSlots {
		s := g.Slot(name)
		if s.Empty() {
			continue
		}
		c := g.CompactFor(name)
		if c <= 0 {
			continue
		}
		id := s.WireModel()
		isExplicit := s.Compact > 0
		cur, seen := out[id]
		switch {
		case !seen:
			out[id] = c
			explicit[id] = isExplicit
		case isExplicit && !explicit[id]:
			out[id] = c
			explicit[id] = true
		case isExplicit == explicit[id] && c < cur:
			out[id] = c
		}
	}
	return out
}

// Clear is the plan for `ccsc local clear`: every owned env key plus the
// top-level compact key. Model-level entries are cleared separately from
// state.json so only models ccsc wrote are touched.
func Clear() Plan {
	p := Plan{
		SetEnv:          map[string]string{},
		SetModelCompact: map[string]int64{},
		ClearEnv:        append([]string(nil), OwnedEnvKeys...),
		ClearTopCompact: true,
	}
	return p.finalize()
}

func basePlan(g *group.Group, opt Options) Plan {
	p := Plan{
		SetEnv:          map[string]string{},
		SetModelCompact: map[string]int64{},
	}
	p.SetEnv[EnvBaseURL] = group.NormalizeBaseURL(g.BaseURL)
	p.SetEnv[EnvToken] = g.Token

	for _, name := range group.ModelSlots {
		s := g.Slot(name)
		p.SetEnv[DefaultModelKey(name)] = s.RenderedModel()
		if s.Name != "" {
			p.SetEnv[DefaultModelNameKey(name)] = s.Name
		} else {
			p.ClearEnv = append(p.ClearEnv, DefaultModelNameKey(name))
		}
	}

	if g.HasSubagent() {
		p.SetEnv[EnvSubagentModel] = g.Slot(group.Subagent).RenderedModel()
	} else {
		p.ClearEnv = append(p.ClearEnv, EnvSubagentModel)
	}

	if g.Window > 0 {
		p.SetEnv[EnvMaxContextTokens] = strconv.FormatInt(g.Window, 10)
	} else {
		p.ClearEnv = append(p.ClearEnv, EnvMaxContextTokens)
	}

	// The startup fallback. Claude Code ignores an alias here, so this is
	// always the resolved model id of a slot.
	p.SetEnv[EnvDefaultModel] = g.Slot(opt.startup()).RenderedModel()

	// These two outrank modelSettings.<id>.autoCompactWindow and would turn
	// every per-model value into dead configuration.
	p.ClearEnv = append(p.ClearEnv, EnvAutoCompactWindow, EnvAutoCompactWindowTokens)

	// Escape hatch: the group's raw env wins, including over the clears above.
	for k, v := range g.Env {
		p.SetEnv[k] = v
	}
	return p
}

// finalize drops clear instructions for keys that are also being set and
// sorts the clear list so repeated applies are deterministic.
func (p Plan) finalize() Plan {
	seen := map[string]bool{}
	cleared := make([]string, 0, len(p.ClearEnv))
	for _, k := range p.ClearEnv {
		if _, set := p.SetEnv[k]; set {
			continue
		}
		if seen[k] {
			continue
		}
		seen[k] = true
		cleared = append(cleared, k)
	}
	sort.Strings(cleared)
	p.ClearEnv = cleared
	if p.SetModelCompact == nil {
		p.SetModelCompact = map[string]int64{}
	}
	return p
}

// Touches reports whether the plan would change anything at all.
func (p Plan) Touches() bool {
	return len(p.SetEnv) > 0 || len(p.ClearEnv) > 0 ||
		len(p.SetModelCompact) > 0 || len(p.ClearModelCompact) > 0 ||
		p.TopCompact != nil || p.ClearTopCompact
}
