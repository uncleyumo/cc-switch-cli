package model

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// MergeProviderInto deep-merges a provider into an existing settings.local.json
// document. base may be empty (no existing file). The provider's env keys and
// top-level "model" override the base; every other base key is preserved.
//
// Model-scoped env keys (context-window declarations) are an exception: if the
// incoming provider does not set one, the existing value is dropped so a
// previous model's 500k/1m window cannot leak onto the next model.
//
// Returns pretty-printed JSON with a trailing newline. Fails without touching
// the base file if the base is not valid JSON.
func MergeProviderInto(base []byte, provider *Model) ([]byte, error) {
	if provider == nil {
		return nil, fmt.Errorf("provider is nil")
	}

	doc := map[string]json.RawMessage{}
	trimmed := bytes.TrimSpace(base)
	if len(trimmed) > 0 {
		if err := json.Unmarshal(trimmed, &doc); err != nil {
			return nil, fmt.Errorf("existing settings.local.json is not a JSON object: %w", err)
		}
	}

	// Merge env: existing env keys are preserved, provider env keys overlay.
	existingEnv := map[string]json.RawMessage{}
	if raw, ok := doc["env"]; ok && len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, &existingEnv); err != nil {
			return nil, fmt.Errorf("existing env is not a JSON object: %w", err)
		}
	}
	providerEnvRaw, err := json.Marshal(provider.Env)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal provider env: %w", err)
	}
	providerEnv := map[string]json.RawMessage{}
	if err := json.Unmarshal(providerEnvRaw, &providerEnv); err != nil {
		return nil, fmt.Errorf("failed to re-parse provider env: %w", err)
	}
	for k, v := range providerEnv {
		existingEnv[k] = v
	}
	for _, k := range ModelScopedEnvKeys {
		if _, ok := providerEnv[k]; !ok {
			delete(existingEnv, k)
		}
	}
	mergedEnv, err := json.Marshal(existingEnv)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal merged env: %w", err)
	}
	doc["env"] = mergedEnv

	// Top-level "model" carries the Claude Code model selector; override if the
	// provider sets one.
	if raw, ok := provider.Rest["model"]; ok {
		doc["model"] = raw
	}

	pretty, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(pretty, '\n'), nil
}

// IsModelEnvKey reports whether an env key is written by `ccsc local` from a
// provider (Anthropic auth / model / window). Custom keys such as HTTP_PROXY
// or user-added extras are left alone by StripModelKeys.
func IsModelEnvKey(k string) bool {
	return strings.HasPrefix(k, "ANTHROPIC_") || strings.HasPrefix(k, "CLAUDE_CODE_")
}

// StripModelKeys removes the provider-owned bits from a settings.local.json
// document: the top-level "model" key and every env key that `ccsc local`
// would have written (ANTHROPIC_* / CLAUDE_CODE_*). Everything else
// (permissions, hooks, custom env, unknown keys) is kept.
//
// emptied is true when there is nothing left to write — the caller should
// delete the file rather than persist `{}`. An empty or missing base is
// treated as already empty. Invalid JSON is an error and is not rewritten.
func StripModelKeys(base []byte) (out []byte, emptied bool, err error) {
	trimmed := bytes.TrimSpace(base)
	if len(trimmed) == 0 {
		return nil, true, nil
	}
	doc := map[string]json.RawMessage{}
	if err := json.Unmarshal(trimmed, &doc); err != nil {
		return nil, false, fmt.Errorf("existing settings.local.json is not a JSON object: %w", err)
	}

	delete(doc, "model")

	if raw, ok := doc["env"]; ok && len(bytes.TrimSpace(raw)) > 0 {
		env := map[string]json.RawMessage{}
		if err := json.Unmarshal(raw, &env); err != nil {
			return nil, false, fmt.Errorf("existing env is not a JSON object: %w", err)
		}
		for k := range env {
			if IsModelEnvKey(k) {
				delete(env, k)
			}
		}
		if len(env) == 0 {
			delete(doc, "env")
		} else {
			mergedEnv, err := json.Marshal(env)
			if err != nil {
				return nil, false, fmt.Errorf("failed to marshal remaining env: %w", err)
			}
			doc["env"] = mergedEnv
		}
	} else {
		delete(doc, "env")
	}

	if len(doc) == 0 {
		return nil, true, nil
	}
	pretty, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, false, err
	}
	return append(pretty, '\n'), false, nil
}
