// Package apply is the only place ccsc writes Claude Code configuration.
//
// Every write goes through the same steps: resolve the target, load it as an
// order-preserving document, compute the owned-key patch, back up the original
// bytes, write atomically, then record what happened.
package apply

import (
	"fmt"
	"os"
	"sort"
	"time"

	"github.com/uncleyumo/ccsc/internal/ccstate"
	"github.com/uncleyumo/ccsc/internal/group"
	"github.com/uncleyumo/ccsc/internal/receipt"
	"github.com/uncleyumo/ccsc/internal/render"
	"github.com/uncleyumo/ccsc/internal/store"
)

// Layer selects which settings file is the target.
type Layer int

const (
	// Global writes the user layer (~/.claude/settings.json).
	Global Layer = iota
	// Project writes <project>/.claude/settings.local.json.
	Project
)

// Request is everything Run needs.
type Request struct {
	Paths store.Paths
	Group *group.Group

	Layer   Layer
	Project ccstate.Project
	Options render.Options

	// Action labels the audit entry, e.g. "group.apply".
	Action string
}

// Run applies the group and returns the receipt describing it.
func Run(req Request) (*receipt.Receipt, error) {
	if req.Group == nil {
		return nil, fmt.Errorf("no group selected")
	}
	if err := req.Group.Validate(); err != nil {
		return nil, err
	}
	if req.Layer == Project && !ccstate.IsUsableProject(req.Project) {
		return nil, fmt.Errorf("%s is not a project directory", req.Project.Dir)
	}

	targetPath, err := target(req)
	if err != nil {
		return nil, err
	}

	var plan render.Plan
	switch req.Layer {
	case Global:
		plan = render.Global(req.Group, req.Options)
	case Project:
		plan = render.Project(req.Group, req.Options)
	default:
		return nil, fmt.Errorf("unknown layer")
	}

	state, err := ccstate.LoadState(req.Paths.Root)
	if err != nil {
		return nil, err
	}
	doc, err := ccstate.LoadDocument(targetPath)
	if err != nil {
		return nil, err
	}
	plan.ClearModelCompact = staleModels(previousModels(state, req), plan.SetModelCompact)

	original, err := store.ReadFileOrEmpty(targetPath)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", targetPath, err)
	}

	r := receipt.New(fmt.Sprintf("applied %s to %s", req.Group.Name, layerName(req.Layer)))
	r.Add(receipt.Read, req.Paths.GroupFile(req.Group.Name), "")

	res := doc.Apply(patchFrom(plan))
	warnRedundantAPIKey(doc, req.Group.Token, r)

	out, changed, err := doc.ChangedBytes()
	if err != nil {
		return nil, err
	}
	if !changed {
		r.Note("nothing changed, the file already matches this group")
	} else {
		if original != nil {
			backupPath, err := ccstate.Backup(req.Paths.Root, targetPath, original)
			if err != nil {
				return nil, err
			}
			if backupPath != "" {
				r.Add(receipt.Backup, backupPath, "snapshot before write")
			}
		}
		if err := writeFile(targetPath, out); err != nil {
			return nil, err
		}
		r.Add(receipt.Write, targetPath, res.Summary())
	}

	warnRuntimeOverrides(doc, r)
	warnAncestorSettings(req, r)

	if err := recordState(req, state, plan); err != nil {
		r.Warn("could not update state.json: %v", err)
	}
	audit(req, targetPath, res.Summary())
	return r, nil
}

// Clear removes every key ccsc owns from the layer and reports what went away.
func Clear(paths store.Paths, layer Layer, project ccstate.Project) (*receipt.Receipt, error) {
	if layer == Project && !ccstate.IsUsableProject(project) {
		return nil, fmt.Errorf("%s is not a project directory", project.Dir)
	}
	req := Request{Paths: paths, Layer: layer, Project: project, Action: "local.clear"}

	targetPath, err := target(req)
	if err != nil {
		return nil, err
	}
	state, err := ccstate.LoadState(paths.Root)
	if err != nil {
		return nil, err
	}
	doc, err := ccstate.LoadDocument(targetPath)
	if err != nil {
		return nil, err
	}
	if !doc.Existed() {
		return nil, fmt.Errorf("%s does not exist, nothing to clear", targetPath)
	}

	plan := render.Clear()
	plan.ClearModelCompact = previousModels(state, req)
	if layer == Project {
		if p, ok := state.Projects[project.Key()]; ok && !p.TopCompact {
			// ccsc wrote model entries here but never the top-level window;
			// leave an autoCompactWindow the user wrote alone.
			plan.ClearTopCompact = false
		}
	} else if state.Global == "" {
		plan.ClearTopCompact = false
	}

	original, err := store.ReadFileOrEmpty(targetPath)
	if err != nil {
		return nil, err
	}

	r := receipt.New(fmt.Sprintf("cleared ccsc keys from %s", layerName(layer)))
	res := doc.Apply(patchFrom(plan))
	out, changed, err := doc.ChangedBytes()
	if err != nil {
		return nil, err
	}
	if !changed {
		r.Note("nothing to clear")
	} else {
		if original != nil {
			if backupPath, err := ccstate.Backup(paths.Root, targetPath, original); err == nil && backupPath != "" {
				r.Add(receipt.Backup, backupPath, "snapshot before clear")
			}
		}
		if err := writeFile(targetPath, out); err != nil {
			return nil, err
		}
		r.Add(receipt.Clear, targetPath, res.Summary())
	}

	if doc.Root.Len() == 0 {
		r.Note("%s is now an empty object and can be deleted", targetPath)
	}

	if layer == Global {
		state.Global, state.GlobalAt, state.GlobalModels = "", "", nil
	} else {
		delete(state.Projects, project.Key())
	}
	if err := ccstate.SaveState(paths.Root, state); err != nil {
		r.Warn("could not update state.json: %v", err)
	}
	audit(req, targetPath, res.Summary())
	return r, nil
}

// writeFile preserves the mode of an existing settings file.
func writeFile(path string, data []byte) error {
	mode := os.FileMode(0o600)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	return store.AtomicWrite(path, data, mode)
}

func target(req Request) (string, error) {
	if req.Layer == Project {
		if req.Project.LocalFile == "" {
			return "", fmt.Errorf("no project selected")
		}
		return req.Project.LocalFile, nil
	}
	return ccstate.UserSettingsPath()
}

func layerName(l Layer) string {
	if l == Project {
		return "the project"
	}
	return "global"
}

func patchFrom(p render.Plan) ccstate.Patch {
	return ccstate.Patch{
		SetEnv:              p.SetEnv,
		ClearEnv:            p.ClearEnv,
		SetModelCompact:     p.SetModelCompact,
		ClearModelCompact:   p.ClearModelCompact,
		TopCompact:          p.TopCompact,
		ClearTopCompact:     p.ClearTopCompact,
		DropEmptyContainers: len(p.SetEnv) == 0 && len(p.SetModelCompact) == 0,
	}
}

func previousModels(state *ccstate.State, req Request) []string {
	if req.Layer == Global {
		return state.GlobalModels
	}
	return state.Projects[req.Project.Key()].Models
}

func staleModels(previous []string, current map[string]int64) []string {
	var out []string
	for _, id := range previous {
		if _, still := current[id]; !still {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}

func recordState(req Request, state *ccstate.State, plan render.Plan) error {
	now := time.Now().Format(time.RFC3339)
	models := make([]string, 0, len(plan.SetModelCompact))
	for id := range plan.SetModelCompact {
		models = append(models, id)
	}
	sort.Strings(models)

	switch req.Layer {
	case Global:
		state.Global = req.Group.Name
		state.GlobalAt = now
		state.GlobalModels = models
	case Project:
		if state.Projects == nil {
			state.Projects = map[string]ccstate.ProjectState{}
		}
		state.Projects[req.Project.Key()] = ccstate.ProjectState{
			Group:      req.Group.Name,
			At:         now,
			Models:     models,
			TopCompact: plan.TopCompact != nil,
		}
	}
	return ccstate.SaveState(req.Paths.Root, state)
}

func audit(req Request, targetPath, detail string) {
	action := req.Action
	if action == "" {
		action = "apply"
	}
	source := ""
	if req.Group != nil {
		source = req.Group.Name
	}
	_ = ccstate.AppendAudit(req.Paths.Root, ccstate.Event{
		Action: action,
		Target: targetPath,
		Source: source,
		Detail: detail,
	})
}

func warnRedundantAPIKey(doc *ccstate.Document, token string, r *receipt.Receipt) {
	env := doc.Root.Object("env")
	if env == nil {
		return
	}
	key, ok := env.String("ANTHROPIC_API_KEY")
	if !ok {
		return
	}
	if key == token {
		env.Delete("ANTHROPIC_API_KEY")
		r.Add(receipt.Clear, "env.ANTHROPIC_API_KEY", "same value as the token, removed")
		return
	}
	r.Warn("ANTHROPIC_API_KEY is also set and differs from the token; it decides the auth header")
}

func warnRuntimeOverrides(doc *ccstate.Document, r *receipt.Receipt) {
	env := doc.Root.Object("env")
	if env == nil {
		return
	}
	if v, ok := env.String("ANTHROPIC_MODEL"); ok && v != "" {
		r.Warn("env.ANTHROPIC_MODEL=%s overrides /model and the startup fallback", v)
	}
}

func warnAncestorSettings(req Request, r *receipt.Receipt) {
	if req.Layer != Project {
		return
	}
	if data, err := store.ReadFileOrEmpty(req.Project.ProjectFile); err == nil && data != nil {
		return
	}
	if ancestor := ccstate.AncestorProjectSettings(req.Project); ancestor != "" {
		r.Warn("Claude Code started in %s will not read %s", req.Project.Dir, ancestor)
	}
}
