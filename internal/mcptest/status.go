package mcptest

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"cc-switch-cli/internal/config"
	"cc-switch-cli/internal/fsutil"
	"cc-switch-cli/internal/mcp"
)

const statusFileName = "mcp-test-status.json"

// StatusFile is the on-disk cache of probe results, keyed by CacheKey.
type StatusFile struct {
	Results   map[string]Result `json:"results"`
	Timestamp string            `json:"timestamp,omitempty"`
}

// StatusPath returns the on-disk path of the MCP test-status cache.
func StatusPath() (string, error) {
	dir, err := config.ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, statusFileName), nil
}

// LoadStatus reads the cache. A missing file returns (nil, nil).
func LoadStatus() (*StatusFile, error) {
	path, err := StatusPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read %s: %w", path, err)
	}
	var sf StatusFile
	if err := json.Unmarshal(data, &sf); err != nil {
		return nil, fmt.Errorf("failed to parse %s: %w", path, err)
	}
	if sf.Results == nil {
		sf.Results = map[string]Result{}
	}
	return &sf, nil
}

// SaveStatus atomically writes the cache.
func SaveStatus(sf *StatusFile) error {
	if sf == nil {
		return errors.New("status is nil")
	}
	if sf.Results == nil {
		sf.Results = map[string]Result{}
	}
	sf.Timestamp = time.Now().UTC().Format(time.RFC3339)
	path, err := StatusPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(sf, "", "  ")
	if err != nil {
		return err
	}
	return fsutil.AtomicWrite(path, append(data, '\n'), 0o644)
}

// Upsert folds a fresh batch of results into the cache, keyed by scope/name.
func (sf *StatusFile) Upsert(results []Result) {
	if sf.Results == nil {
		sf.Results = map[string]Result{}
	}
	for _, r := range results {
		scope := mcp.Scope(r.Scope)
		if scope == "" {
			scope = mcp.ScopeUser
		}
		sf.Results[CacheKey(scope, r.Name)] = r
	}
}
