package transfer

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"cc-switch-cli/internal/fsutil"
	"cc-switch-cli/internal/mcp"
	"cc-switch-cli/internal/model"
)

// ImportOptions controls Apply behavior.
type ImportOptions struct {
	Force bool // overwrite existing entries
}

// ImportReport lists per-entry outcomes of an Apply run.
type ImportReport struct {
	ModelsApplied []string
	ModelsSkipped []string
	MCPsApplied   []string
	MCPsSkipped   []string
}

// Apply writes payload entries into the model library and the user MCP
// scope. Models are written as the exact stored bytes after a parse sanity
// check; MCP servers are validated and written via the user-scope store.
// Existing entries are skipped unless Force is set.
func Apply(p *Payload, opts ImportOptions) (*ImportReport, error) {
	if p == nil {
		return nil, errors.New("empty payload")
	}
	rep := &ImportReport{}

	for _, name := range sortedKeys(p.Models) {
		raw := p.Models[name]
		path, err := model.FileNameFor(name)
		if err != nil {
			return nil, fmt.Errorf("failed to import model %q: %w", name, err)
		}
		exists, err := model.Exists(name)
		if err != nil {
			return nil, fmt.Errorf("failed to import model %q: %w", name, err)
		}
		if exists && !opts.Force {
			rep.ModelsSkipped = append(rep.ModelsSkipped, name)
			continue
		}
		if _, err := model.Parse(raw); err != nil {
			return nil, fmt.Errorf("model %q is malformed: %w", name, err)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, fmt.Errorf("failed to create models directory: %w", err)
		}
		if err := fsutil.AtomicWrite(path, raw, 0o644); err != nil {
			return nil, fmt.Errorf("failed to write model %q: %w", name, err)
		}
		rep.ModelsApplied = append(rep.ModelsApplied, name)
	}

	claudeSeeded := false
	for _, name := range sortedKeys(p.MCPs) {
		raw := p.MCPs[name]
		if err := mcp.ValidateName(name); err != nil {
			return nil, fmt.Errorf("failed to import mcp server: %w", err)
		}
		srv, err := mcp.ParseServer(raw)
		if err != nil {
			return nil, fmt.Errorf("mcp server %q is malformed: %w", name, err)
		}
		if _, err := mcp.Get(mcp.ScopeUser, "", name); err == nil && !opts.Force {
			rep.MCPsSkipped = append(rep.MCPsSkipped, name)
			continue
		}
		if !claudeSeeded {
			if err := ensureClaudeJSON(); err != nil {
				return nil, err
			}
			claudeSeeded = true
		}
		if err := mcp.Set(mcp.ScopeUser, "", name, srv); err != nil {
			return nil, fmt.Errorf("failed to import mcp server %q: %w", name, err)
		}
		rep.MCPsApplied = append(rep.MCPsApplied, name)
	}

	sort.Strings(rep.ModelsApplied)
	sort.Strings(rep.ModelsSkipped)
	sort.Strings(rep.MCPsApplied)
	sort.Strings(rep.MCPsSkipped)
	return rep, nil
}

// ensureClaudeJSON seeds an empty ~/.claude.json when it does not exist so
// user-scope servers can be restored into a fresh installation. The mcp
// store refuses to create the file itself.
func ensureClaudeJSON() error {
	path, err := mcp.ClaudeJSONPath()
	if err != nil {
		return err
	}
	exists, err := fsutil.PathExists(path)
	if err != nil {
		return fmt.Errorf("failed to check %s: %w", path, err)
	}
	if exists {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}
	if err := fsutil.AtomicWrite(path, []byte("{}\n"), 0o600); err != nil {
		return fmt.Errorf("failed to create %s: %w", path, err)
	}
	return nil
}

// ParseSelection resolves a CLI selection flag against the available names.
// "" yields nil (caller decides the default), "all" a copy of available,
// "none" an empty slice, otherwise a comma-separated subset deduplicated
// and reordered to match available. Unknown names produce an error listing
// the offenders.
func ParseSelection(value string, available []string) ([]string, error) {
	switch value {
	case "":
		return nil, nil
	case "all":
		return append([]string(nil), available...), nil
	case "none":
		return []string{}, nil
	}
	known := make(map[string]struct{}, len(available))
	for _, n := range available {
		known[n] = struct{}{}
	}
	requested := map[string]struct{}{}
	offenderSeen := map[string]struct{}{}
	var offenders []string
	for _, part := range strings.Split(value, ",") {
		name := strings.TrimSpace(part)
		if name == "" {
			return nil, fmt.Errorf("invalid selection %q: empty name", value)
		}
		if _, ok := known[name]; ok {
			requested[name] = struct{}{}
			continue
		}
		if _, seen := offenderSeen[name]; !seen {
			offenderSeen[name] = struct{}{}
			offenders = append(offenders, name)
		}
	}
	if len(offenders) > 0 {
		return nil, fmt.Errorf("unknown names in selection: %s", strings.Join(offenders, ", "))
	}
	out := make([]string, 0, len(requested))
	for _, n := range available {
		if _, ok := requested[n]; ok {
			out = append(out, n)
		}
	}
	return out, nil
}
