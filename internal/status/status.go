// Package status answers one question on one screen: what is actually in
// effect right now, and which layer it came from. It writes nothing.
package status

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/uncleyumo/ccsc/internal/ccstate"
	"github.com/uncleyumo/ccsc/internal/group"
	"github.com/uncleyumo/ccsc/internal/inventory"
	"github.com/uncleyumo/ccsc/internal/render"
	"github.com/uncleyumo/ccsc/internal/store"
	"github.com/uncleyumo/ccsc/internal/ui"
)

// Layer is one settings file and the keys it contributes.
type Layer struct {
	Name    string
	Path    string
	Exists  bool
	Env     map[string]string
	Compact map[string]int64
	Top     int64
}

// Override records a key whose project or local value differs from the user
// layer.
type Override struct {
	Key   string
	Layer string
	Value string
	User  string
}

// Provenance says which ccsc command wrote a layer, or that nothing ccsc
// wrote is in effect.
type Provenance struct {
	Layer  string
	Source string
	Name   string
	At     string
}

// Report is everything the renderer needs.
type Report struct {
	ProjectDir string
	HasProject bool
	Layers     []Layer
	Effective  map[string]string
	Source     map[string]string
	ShellOnly  []string
	Startup    string
	StartupVal string
	Subagent   string
	MCPServers map[string]int
	Skills     map[string]int
	Overrides  []Override
	Provenance []Provenance
	Warnings   []string
}

// Gather reads the three settings layers Claude Code reads.
func Gather(project ccstate.Project, hasProject bool) (*Report, error) {
	r := &Report{MCPServers: map[string]int{}, Skills: map[string]int{}, Source: map[string]string{}}
	if hasProject {
		r.ProjectDir = project.Dir
		r.HasProject = true
	}

	userPath, err := ccstate.UserSettingsPath()
	if err != nil {
		return nil, err
	}

	type target struct {
		name string
		path string
	}
	targets := []target{{"user", userPath}}
	if hasProject {
		targets = append(targets,
			target{"project", project.ProjectFile},
			target{"local", project.LocalFile},
		)
	}

	for _, t := range targets {
		layer, err := loadLayer(t.name, t.path)
		if err != nil {
			r.Warnings = append(r.Warnings, err.Error())
			continue
		}
		r.Layers = append(r.Layers, layer)
	}

	// Precedence, lowest first: user, project, local.
	r.Effective = map[string]string{}
	for _, layer := range r.Layers {
		for k, v := range layer.Env {
			r.Effective[k] = v
			r.Source[k] = layer.Name
		}
	}
	for _, k := range render.OwnedEnvKeys {
		if _, ok := r.Effective[k]; ok {
			continue
		}
		if v := os.Getenv(k); v != "" {
			r.Effective[k] = v
			r.Source[k] = "shell"
			r.ShellOnly = append(r.ShellOnly, k)
		}
	}

	r.Startup = startupSlot(r)
	r.StartupVal = r.Effective[render.DefaultModelKey(group.SlotName(r.Startup))]
	r.Subagent = r.Effective[render.EnvSubagentModel]
	r.Overrides = collectOverrides(r)
	r.Provenance = collectProvenance(project, hasProject)

	if hasProject {
		if mcp, err := inventory.MCP(project); err == nil {
			for _, s := range mcp.Servers {
				if !s.Shadowed {
					r.MCPServers[s.Scope]++
				}
			}
		}
		if sk, err := inventory.Skills(project); err == nil {
			for _, s := range sk.Skills {
				if !s.Shadowed {
					r.Skills[s.Scope]++
				}
			}
		}
	}

	r.Warnings = append(r.Warnings, r.collectWarnings()...)
	return r, nil
}

func loadLayer(name, path string) (Layer, error) {
	l := Layer{Name: name, Path: path, Env: map[string]string{}, Compact: map[string]int64{}}
	doc, err := ccstate.LoadDocument(path)
	if err != nil {
		return l, fmt.Errorf("cannot read the %s layer: %v", name, err)
	}
	l.Exists = doc.Existed()
	if !l.Exists {
		return l, nil
	}
	if env := doc.Root.Object("env"); env != nil {
		for _, k := range env.Keys() {
			if v, ok := env.String(k); ok {
				l.Env[k] = v
			}
		}
	}
	if ms := doc.Root.Object("modelSettings"); ms != nil {
		for _, id := range ms.Keys() {
			entry := ms.Object(id)
			if entry == nil {
				continue
			}
			if v, ok := entry.Number("autoCompactWindow"); ok {
				l.Compact[id] = v
			}
		}
	}
	if v, ok := doc.Root.Number("autoCompactWindow"); ok {
		l.Top = v
	}
	return l, nil
}

func startupSlot(r *Report) string {
	v := r.Effective[render.EnvDefaultModel]
	if v == "" {
		return ""
	}
	for _, name := range []group.SlotName{group.Sonnet, group.Fable, group.Opus, group.Haiku} {
		if r.Effective[render.DefaultModelKey(name)] == v {
			return string(name)
		}
	}
	return "custom"
}

func collectOverrides(r *Report) []Override {
	user, ok := r.layer("user")
	if !ok {
		return nil
	}
	var out []Override
	for _, k := range render.OwnedEnvKeys {
		userVal, hasUser := user.Env[k]
		for _, name := range []string{"local", "project"} {
			l, ok := r.layer(name)
			if !ok {
				continue
			}
			v, has := l.Env[k]
			if !has {
				continue
			}
			if hasUser && v == userVal {
				continue
			}
			out = append(out, Override{Key: k, Layer: name, Value: v, User: userVal})
			break
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// collectProvenance reports which ccsc command wrote each layer. A layer ccsc
// never wrote is reported as such rather than guessed at.
func collectProvenance(project ccstate.Project, hasProject bool) []Provenance {
	paths, err := store.New()
	if err != nil {
		return nil
	}
	state, err := ccstate.LoadState(paths.Root)
	if err != nil {
		return nil
	}
	var out []Provenance

	user := Provenance{Layer: "user"}
	if state.GlobalSource != "" || state.Global != "" {
		user.Source = state.GlobalSource
		if user.Source == "" {
			user.Source = "group"
		}
		user.Name = state.Global
		user.At = state.GlobalAt
	}
	out = append(out, user)

	if hasProject {
		ps := Provenance{Layer: "project"}
		if p, ok := state.Projects[project.Key()]; ok {
			ps.Source = p.EffectiveSource()
			ps.Name = p.Group
			ps.At = p.At
		}
		out = append(out, ps)
	}
	return out
}

func (r *Report) layer(name string) (Layer, bool) {
	for _, l := range r.Layers {
		if l.Name == name {
			return l, true
		}
	}
	return Layer{}, false
}

func (r *Report) collectWarnings() []string {
	var out []string
	if v, ok := r.Effective["ANTHROPIC_MODEL"]; ok && v != "" {
		out = append(out, fmt.Sprintf("env.ANTHROPIC_MODEL=%s overrides /model and the startup fallback", v))
	}
	key, hasKey := r.Effective[render.EnvAPIKey]
	token, hasToken := r.Effective[render.EnvToken]
	if hasKey && hasToken && key != token {
		out = append(out, "ANTHROPIC_API_KEY and ANTHROPIC_AUTH_TOKEN are both set, with different values")
	}
	if len(r.ShellOnly) > 0 {
		out = append(out, fmt.Sprintf("%s come from the shell environment only, not from any settings file",
			strings.Join(r.ShellOnly, ", ")))
	}
	return out
}

const (
	labelW = 13
	valueW = 52
)

// Render prints the report.
func (r *Report) Render(reveal bool) string {
	var b strings.Builder

	if r.HasProject {
		fmt.Fprintf(&b, "project  %s\n", r.ProjectDir)
	} else {
		b.WriteString("project  none\n")
	}

	b.WriteString("\nlayers\n")
	stateW := 0
	for _, l := range r.Layers {
		if w := ui.Width(layerState(l)); w > stateW {
			stateW = w
		}
	}
	for _, l := range r.Layers {
		fmt.Fprintf(&b, "  %s%s %s\n",
			ui.Pad(l.Name, 8),
			ui.Pad(layerState(l), stateW),
			ui.Truncate(store.Shorten(l.Path), 78))
	}
	if len(r.Overrides) > 0 {
		fmt.Fprintf(&b, "  %s\n", fmt.Sprintf("%d keys differ from the user layer; see ccsc status --diff", len(r.Overrides)))
	}

	b.WriteString("\nccsc\n")
	for _, p := range r.Provenance {
		if p.Source == "" {
			fmt.Fprintf(&b, "  %s%s not written by ccsc\n", ui.Pad(p.Layer, 8), "")
			continue
		}
		detail := p.Source
		if p.Name != "" && p.Source != "optional" {
			detail += " " + p.Name
		}
		fmt.Fprintf(&b, "  %s%s %s\n", ui.Pad(p.Layer, 8), ui.Pad(detail, 28), shortTime(p.At))
	}

	b.WriteString("\nin effect\n")
	// Each block sizes its value column to its own content, so short values
	// do not float two thirds of the way across the terminal.
	writeRows := func(rows [][3]string) {
		maxW := 0
		for _, r := range rows {
			if w := ui.Width(ui.Truncate(r[1], valueW)); w > maxW {
				maxW = w
			}
		}
		for _, r := range rows {
			line := "  " + ui.Pad(r[0], labelW) + ui.Pad(ui.Truncate(r[1], valueW), maxW)
			line = strings.TrimRight(line, " ")
			if r[2] != "" {
				line = ui.Pad(line, 2+labelW+maxW) + "  " + r[2]
			}
			b.WriteString(line + "\n")
		}
	}

	var meta [][3]string
	baseURL, hasBaseURL := r.Effective[render.EnvBaseURL]
	if !hasBaseURL {
		baseURL = "not set"
	}
	meta = append(meta, [3]string{"base url", baseURL, r.Source[render.EnvBaseURL]})

	if token, ok := r.Effective[render.EnvToken]; ok {
		if !reveal {
			token = group.MaskedToken(token)
		}
		meta = append(meta, [3]string{"token", token, r.Source[render.EnvToken]})
	} else {
		meta = append(meta, [3]string{"token", "not set", ""})
	}

	if r.Startup != "" {
		meta = append(meta, [3]string{"startup", r.Startup + " -> " + r.StartupVal, r.Source[render.EnvDefaultModel]})
	} else {
		meta = append(meta, [3]string{"startup", "not set", ""})
	}
	if r.Subagent != "" {
		meta = append(meta, [3]string{"subagent", r.Subagent, r.Source[render.EnvSubagentModel]})
	} else {
		meta = append(meta, [3]string{"subagent", "not set, follows the main model", ""})
	}
	if v, ok := r.Effective[render.EnvMaxContextTokens]; ok {
		meta = append(meta, [3]string{"context", v + " tokens", r.Source[render.EnvMaxContextTokens]})
	} else {
		meta = append(meta, [3]string{"context", "gateway default", ""})
	}
	if c := r.effectiveCompact(); c > 0 {
		meta = append(meta, [3]string{"auto-compact", group.FormatWindow(c) + " tokens", ""})
	} else {
		meta = append(meta, [3]string{"auto-compact", "not set, Claude Code decides", ""})
	}
	writeRows(meta)

	b.WriteString("\n")
	var slots [][3]string
	for _, name := range group.ModelSlots {
		key := render.DefaultModelKey(name)
		v, ok := r.Effective[key]
		if !ok {
			v = "not set"
		}
		slots = append(slots, [3]string{string(name), v, r.Source[key]})
	}
	writeRows(slots)

	b.WriteString("\n")
	mcpTotal, skillTotal := 0, 0
	for _, n := range r.MCPServers {
		mcpTotal += n
	}
	for _, n := range r.Skills {
		skillTotal += n
	}
	if r.HasProject {
		fmt.Fprintf(&b, "mcp      %d in effect (user %d, local %d, project %d)   see: ccsc mcp\n",
			mcpTotal, r.MCPServers[inventory.ScopeUser], r.MCPServers[inventory.ScopeLocal], r.MCPServers[inventory.ScopeProject])
		fmt.Fprintf(&b, "skills   %d in effect (user %d, project %d)   see: ccsc skills\n",
			skillTotal, r.Skills[inventory.ScopeUser], r.Skills[inventory.ScopeProject])
	} else {
		fmt.Fprintf(&b, "mcp      %d in effect (user)   see: ccsc mcp\n", mcpTotal)
		fmt.Fprintf(&b, "skills   %d in effect (user)   see: ccsc skills\n", skillTotal)
	}

	if len(r.Warnings) > 0 {
		b.WriteString("\n")
		for _, w := range r.Warnings {
			fmt.Fprintf(&b, "warning: %s\n", w)
		}
	}
	return b.String()
}

// Diff lists the keys the project or local layer changes, one line each.
func (r *Report) Diff(reveal bool) string {
	if len(r.Overrides) == 0 {
		return "no differences between the user, project and local layers\n"
	}
	keyW := 0
	for _, o := range r.Overrides {
		if w := ui.Width(o.Key); w > keyW {
			keyW = w
		}
	}
	var b strings.Builder
	b.WriteString("layer differences\n\n")
	for _, o := range r.Overrides {
		userVal := o.User
		if userVal == "" {
			userVal = "not set"
		}
		val := o.Value
		if !reveal && (o.Key == render.EnvToken || o.Key == render.EnvAPIKey) {
			userVal = group.MaskedToken(userVal)
			val = group.MaskedToken(val)
		}
		fmt.Fprintf(&b, "  %s  user: %s   %s: %s\n", ui.Pad(o.Key, keyW), userVal, o.Layer, val)
	}
	return b.String()
}

func layerState(l Layer) string {
	if !l.Exists {
		return "not configured"
	}
	return plural(countModelKeys(l.Env), "model key")
}

func (r *Report) effectiveCompact() int64 {
	for i := len(r.Layers) - 1; i >= 0; i-- {
		if r.Layers[i].Top > 0 {
			return r.Layers[i].Top
		}
	}
	var best int64
	for i := len(r.Layers) - 1; i >= 0; i-- {
		ids := make([]string, 0, len(r.Layers[i].Compact))
		for id := range r.Layers[i].Compact {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			if v := r.Layers[i].Compact[id]; v > best {
				best = v
			}
		}
		if best > 0 {
			return best
		}
	}
	return 0
}

func countModelKeys(env map[string]string) int {
	n := 0
	for _, k := range render.OwnedEnvKeys {
		if _, ok := env[k]; ok {
			n++
		}
	}
	return n
}

// shortTime renders a stored RFC3339 stamp as 2026-11-08 15:56.
func shortTime(at string) string {
	t, err := time.Parse(time.RFC3339, at)
	if err != nil {
		return at
	}
	return t.Format("2006-01-02 15:04")
}

// plural renders "1 model key" and "8 model keys".
func plural(n int, unit string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, unit)
	}
	return fmt.Sprintf("%d %ss", n, unit)
}
