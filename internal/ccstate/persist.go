package ccstate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/uncleyumo/ccsc/internal/store"
)

// State records what ccsc last wrote where, so it can clean up only the keys
// it owns and leave everything else alone.
type State struct {
	Global       string                  `json:"global,omitempty"`
	GlobalAt     string                  `json:"global_at,omitempty"`
	GlobalModels []string                `json:"global_models,omitempty"`
	Projects     map[string]ProjectState `json:"projects,omitempty"`
}

// ProjectState is the per-project bookkeeping.
type ProjectState struct {
	Group      string   `json:"group,omitempty"`
	At         string   `json:"at,omitempty"`
	Models     []string `json:"models,omitempty"`
	TopCompact bool     `json:"top_compact,omitempty"`
}

// LoadState reads state.json, returning an empty state when it is absent.
func LoadState(root string) (*State, error) {
	data, err := store.ReadFileOrEmpty(filepath.Join(root, "state.json"))
	if err != nil {
		return nil, fmt.Errorf("cannot read state.json: %w", err)
	}
	s := &State{Projects: map[string]ProjectState{}}
	if data == nil {
		return s, nil
	}
	if err := json.Unmarshal(data, s); err != nil {
		return nil, fmt.Errorf("state.json is corrupt (delete %s to reset): %w", filepath.Join(root, "state.json"), err)
	}
	if s.Projects == nil {
		s.Projects = map[string]ProjectState{}
	}
	return s, nil
}

// SaveState writes state.json atomically.
func SaveState(root string, s *State) error {
	if s.Projects == nil {
		s.Projects = map[string]ProjectState{}
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("cannot encode state: %w", err)
	}
	data = append(data, '\n')
	return store.AtomicWrite(filepath.Join(root, "state.json"), data, 0o600)
}

const (
	auditMaxBytes = 5 << 20
	auditKeep     = 3
	backupsKeep   = 10
)

// Event is one line of audit.jsonl.
type Event struct {
	At     string `json:"at"`
	Action string `json:"action"`
	Target string `json:"target,omitempty"`
	Source string `json:"source,omitempty"`
	Detail string `json:"detail,omitempty"`
}

// AppendAudit appends an event, rotating at 5 MB and keeping three archives.
// Failure to write the audit log never fails the operation that produced it.
func AppendAudit(root string, ev Event) error {
	if ev.At == "" {
		ev.At = time.Now().Format(time.RFC3339)
	}
	path := filepath.Join(root, "audit.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	rotateAudit(path)
	line, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(line, '\n'))
	return err
}

func rotateAudit(path string) {
	info, err := os.Stat(path)
	if err != nil || info.Size() < auditMaxBytes {
		return
	}
	for i := auditKeep - 1; i >= 1; i-- {
		_ = os.Rename(fmt.Sprintf("%s.%d", path, i), fmt.Sprintf("%s.%d", path, i+1))
	}
	_ = os.Rename(path, path+".1")
}

// Backup snapshots the pre-write bytes of a target file under
// backups/<sanitised target>/ and keeps the ten most recent snapshots.
func Backup(root, targetPath string, data []byte) (string, error) {
	if data == nil {
		return "", nil
	}
	dir := filepath.Join(root, "backups", sanitizeTarget(targetPath))
	name := time.Now().Format("20060102-150405.000") + ".json"
	dst := filepath.Join(dir, name)
	if err := store.AtomicWrite(dst, data, 0o600); err != nil {
		return "", fmt.Errorf("cannot back up %s: %w", targetPath, err)
	}
	pruneBackups(dir, backupsKeep)
	return dst, nil
}

func sanitizeTarget(path string) string {
	abs, err := filepath.Abs(path)
	if err == nil {
		path = abs
	}
	r := strings.NewReplacer(":", "", `\`, "_", "/", "_", " ", "_")
	s := r.Replace(path)
	s = strings.Trim(s, "_")
	if len(s) > 80 {
		s = s[len(s)-80:]
	}
	if s == "" {
		s = "target"
	}
	return s
}

func pruneBackups(dir string, keep int) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			names = append(names, e.Name())
		}
	}
	if len(names) <= keep {
		return
	}
	sort.Strings(names)
	for _, name := range names[:len(names)-keep] {
		_ = os.Remove(filepath.Join(dir, name))
	}
}
