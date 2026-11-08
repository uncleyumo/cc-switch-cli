// Package migrate converts ccsc v1's per-provider settings files into v2
// groups.
//
// A v1 install kept one file per provider under <root>/models/<name>.json,
// each a complete Claude Code settings.json. Only its "env" block matters for
// v2: ccsc now stores an endpoint plus five model slots and renders everything
// else itself.
//
// Migration is one-shot and idempotent: Scan never overwrites an existing
// group, Archive moves the legacy directory aside, and MarkDone records that
// the run happened so the next launch can skip it.
package migrate

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/uncleyumo/ccsc/internal/group"
)

// Env keys read from a v1 provider file.
const (
	// EnvBaseURL is the gateway endpoint (required).
	EnvBaseURL = "ANTHROPIC_BASE_URL"
	// EnvAuthToken is the preferred credential.
	EnvAuthToken = "ANTHROPIC_AUTH_TOKEN"
	// EnvAPIKey is the credential used when EnvAuthToken is absent.
	EnvAPIKey = "ANTHROPIC_API_KEY"
	// EnvContextWindow is the group-level context window, if the user set one.
	EnvContextWindow = "CLAUDE_CODE_MAX_CONTEXT_TOKENS"
	// EnvSubagentModel fills the optional subagent slot.
	EnvSubagentModel = "CLAUDE_CODE_SUBAGENT_MODEL"
)

// LegacyDirName is the v1 directory name under the state root.
const LegacyDirName = "models"

// LegacyModelsDir is where v1 kept provider settings under root.
func LegacyModelsDir(root string) string { return filepath.Join(root, LegacyDirName) }

// Result is the outcome for one legacy provider file.
//
// Group is non-nil when the file was converted and saved. When Group and Err
// are both nil the file was skipped because a group of the same name already
// exists; its source is left untouched.
type Result struct {
	// Source is the legacy file path.
	Source string
	// Name is the group name, taken from the file stem.
	Name string
	// Group is the converted group, nil when the file was skipped or failed.
	Group *group.Group
	// Err is the per-file failure, nil when the file was migrated or skipped.
	Err error
	// Keys lists the env keys consumed from the legacy file, in the order
	// they were read. It is empty for skipped and failed files.
	Keys []string
}

// Skipped reports whether the file was left alone because a group with the
// same name already exists.
func (r Result) Skipped() bool { return r.Group == nil && r.Err == nil }

// FromLegacySettings converts one v1 settings.json into a group.
//
// name is the group name, normally the file stem. Every listed key is
// required except the subagent model and the context window; a model id with a
// trailing "[1m]" (case-insensitive) becomes a slot window of OneMillion with
// the marker stripped from the stored model id.
func FromLegacySettings(data []byte, name string) (*group.Group, error) {
	g, _, err := parseLegacy(data, name)
	return g, err
}

// Scan converts every *.json in legacyDir that has no group of the same name.
//
// A missing legacyDir is not an error. Per-file failures are reported in the
// matching Result rather than aborting the scan, so one broken file cannot
// block the rest. The second return value is non-nil only when legacyDir
// itself cannot be read.
func Scan(legacyDir, groupsDir string) ([]Result, error) {
	entries, err := os.ReadDir(legacyDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("cannot read %s: %w", legacyDir, err)
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".json") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)

	results := make([]Result, 0, len(names))
	for _, fileName := range names {
		src := filepath.Join(legacyDir, fileName)
		name := strings.TrimSuffix(fileName, filepath.Ext(fileName))
		res := Result{Source: src, Name: name}

		if group.Exists(groupsDir, name) {
			results = append(results, res)
			continue
		}

		data, err := os.ReadFile(src)
		if err != nil {
			res.Err = fmt.Errorf("cannot read %s: %w", src, err)
			results = append(results, res)
			continue
		}
		g, keys, err := parseLegacy(data, name)
		if err != nil {
			res.Err = err
			results = append(results, res)
			continue
		}
		if err := group.Save(groupsDir, g); err != nil {
			res.Err = err
			results = append(results, res)
			continue
		}
		res.Group = g
		res.Keys = keys
		results = append(results, res)
	}
	return results, nil
}

// parseLegacy does the actual conversion and also reports which env keys were
// consumed.
func parseLegacy(data []byte, name string) (*group.Group, []string, error) {
	var file struct {
		Env map[string]json.RawMessage `json:"env"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, nil, fmt.Errorf("%s: not a valid settings file: %w", providerLabel(name), err)
	}

	var keys []string
	note := func(key string) { keys = append(keys, key) }

	baseURL, ok := envString(file.Env, EnvBaseURL)
	if !ok {
		return nil, nil, fmt.Errorf("%s: env.%s is empty", providerLabel(name), EnvBaseURL)
	}
	baseURL = group.NormalizeBaseURL(baseURL)
	if err := group.ValidateBaseURL(baseURL); err != nil {
		return nil, nil, fmt.Errorf("%s: %w", providerLabel(name), err)
	}
	note(EnvBaseURL)

	tokenKey := EnvAuthToken
	token, ok := envString(file.Env, tokenKey)
	if !ok {
		tokenKey = EnvAPIKey
		token, ok = envString(file.Env, tokenKey)
	}
	if !ok {
		return nil, nil, fmt.Errorf("%s: no token in env.%s or env.%s",
			providerLabel(name), EnvAuthToken, EnvAPIKey)
	}
	note(tokenKey)

	var window int64
	if raw, ok := envString(file.Env, EnvContextWindow); ok {
		// An unparsable or non-positive window is treated as "not set": the
		// group schema cannot express a negative or zero cap.
		if n, err := strconv.ParseInt(raw, 10, 64); err == nil && n > 0 {
			window = n
		}
		note(EnvContextWindow)
	}

	g := &group.Group{
		BaseURL: baseURL,
		Token:   token,
		Window:  window,
		Name:    name,
	}

	var missing []string
	for _, slotName := range group.ModelSlots {
		modelKey := "ANTHROPIC_DEFAULT_" + slotName.EnvToken() + "_MODEL"
		raw, ok := envString(file.Env, modelKey)
		if !ok {
			missing = append(missing, string(slotName))
			continue
		}

		slot := group.Slot{Model: raw}
		if slot.Is1M() {
			slot.Model = slot.WireModel()
			slot.Window = group.OneMillion
		}
		if slot.Empty() {
			// A bare "[1m]" is not a model id.
			missing = append(missing, string(slotName))
			continue
		}
		note(modelKey)

		nameKey := "ANTHROPIC_DEFAULT_" + slotName.EnvToken() + "_MODEL_NAME"
		if display, ok := envString(file.Env, nameKey); ok {
			slot.Name = display
			note(nameKey)
		}
		g.SetSlot(slotName, slot)
	}

	if len(missing) > 0 {
		return nil, nil, fmt.Errorf("%s: missing model slots: %s",
			providerLabel(name), strings.Join(missing, ", "))
	}

	if raw, ok := envString(file.Env, EnvSubagentModel); ok {
		note(EnvSubagentModel)
		slot := group.Slot{Model: raw}
		if slot.Is1M() {
			slot.Model = slot.WireModel()
			slot.Window = group.OneMillion
		}
		if !slot.Empty() {
			g.SetSlot(group.Subagent, slot)
		}
	}

	if err := g.Validate(); err != nil {
		return nil, nil, fmt.Errorf("%s: %w", providerLabel(name), err)
	}
	return g, keys, nil
}

func providerLabel(name string) string {
	if strings.TrimSpace(name) == "" {
		return "legacy provider"
	}
	return fmt.Sprintf("legacy provider %q", name)
}

// envString returns the trimmed string value of an env key and whether it was
// present and non-empty. Non-string values are ignored rather than failing the
// whole file.
func envString(env map[string]json.RawMessage, key string) (string, bool) {
	raw, ok := env[key]
	if !ok {
		return "", false
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", false
	}
	value = strings.TrimSpace(value)
	return value, value != ""
}
