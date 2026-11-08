// Package status answers one question on one screen: what is actually in
// effect right now, and which layer it came from. It writes nothing.
package status

import (
	"fmt"
	"os"
	"sort"
	"strings"

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
// layer, so the screen can say exactly what is being overridden.
type Override struct {
	Key    string
	Layer  string
	Value  string
	User   string
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

// Render prints the report.
func (r *Report) Render(reveal bool) string {
	var b strings.Builder

	if r.HasProject {
		fmt.Fprintf(&b, "project  %s\n", r.ProjectDir)
	} else {
		b.WriteString("project  none (home directory or a volume root)\n")
	}

	b.WriteString("\nlayers\n")
	nameW := 8
	for _, l := range r.Layers {
		if w := ui.Width(l.Name); w > nameW {
			nameW = w
		}
	}
	for _, l := range r.Layers {
		state := "not configured"
		if l.Exists {
			state = fmt.Sprintf("%s", plural(countModelKeys(l.Env), "model key"))
		}
		fmt.Fprintf(&b, "  %s%s %s\n", ui.Pad(l.Name, nameW),
			ui.Pad(ui.Truncate(store.Shorten(l.Path), 56), 56), state)
	}

	b.WriteString("\nmodel\n")
	row := func(label, value, source string) {
		line := "  " + ui.Pad(label, 14) + value
		if source != "" {
			line = ui.Pad(line, 66) + dim(source)
		}
		b.WriteString(line + "\n")
	}
	if v, ok := r.Effective[render.EnvBaseURL]; ok {
		row("base url", v, r.Source[render.EnvBaseURL])
	} else {
		row("base url", "not set", "")
	}
	if v, ok := r.Effective[render.EnvToken]; ok {
		if !reveal {
			v = group.MaskedToken(v)
		}
		row("token", v, r.Source[render.EnvToken])
	} else {
		row("token", "not set", "")
	}
	if r.Startup != "" {
		row("startup", r.Startup+" -> "+r.StartupVal, r.Source[render.EnvDefaultModel])
	} else {
		row("startup", "not set", "")
	}
	if r.Subagent != "" {
		row("subagent", r.Subagent, r.Source[render.EnvSubagentModel])
	} else {
		row("subagent", "not set, follows the main model", "")
	}
	if v, ok := r.Effective[render.EnvMaxContextTokens]; ok {
		row("context", v+" tokens", r.Source[render.EnvMaxContextTokens])
	} else {
		row("context", "gateway default", "")
	}
	if c := r.effectiveCompact(); c > 0 {
		row("auto-compact", group.FormatWindow(c)+" tokens", "")
	} else {
		row("auto-compact", "not set, Claude Code decides", "")
	}

	b.WriteString("\nslots\n")
	for _, name := range group.ModelSlots {
		v, ok := r.Effective[render.DefaultModelKey(name)]
		if !ok {
			v = "not set"
		}
		fmt.Fprintf(&b, "  %s%s %s\n", ui.Pad(string(name), 10), ui.Pad(v, 44), dim(r.Source[render.DefaultModelKey(name)]))
	}

	if len(r.Overrides) > 0 {
		b.WriteString("\noverrides\n")
		for _, o := range r.Overrides {
			userVal := o.User
			if userVal == "" {
				userVal = "not set"
			}
			if !reveal && (o.Key == render.EnvToken || o.Key == render.EnvAPIKey) {
				userVal = group.MaskedToken(o.User)
				o.Value = group.MaskedToken(o.Value)
			}
			fmt.Fprintf(&b, "  %s\n    %s: %s\n    user: %s\n", o.Key, o.Layer, o.Value, userVal)
		}
	}

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

func dim(s string) string { return s }

// plural renders "1 model key" and "8 model keys".
func plural(n int, unit string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, unit)
	}
	return fmt.Sprintf("%d %ss", n, unit)
}
