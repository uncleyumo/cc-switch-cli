package ccstate

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/uncleyumo/ccsc/internal/store"
)

// Document is a settings file loaded into an ordered tree so ccsc can change
// a handful of keys and write everything else back untouched.
type Document struct {
	Path            string
	Root            *Object
	indent          string
	trailingNewline bool
	existed         bool
	// createdEnv / createdModelSettings remember that ccsc added the
	// container itself, so it can remove it again once empty instead of
	// leaving a stray "env": {} behind.
	createdEnv           bool
	createdModelSettings bool
}

// LoadDocument reads path, or returns an empty document when it does not
// exist yet. A file that exists but is not a JSON object is an error: ccsc
// never overwrites something it cannot parse.
func LoadDocument(path string) (*Document, error) {
	data, err := store.ReadFileOrEmpty(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", path, err)
	}
	d := &Document{Path: path, indent: "  ", trailingNewline: true, Root: NewObject()}
	if data == nil {
		return d, nil
	}
	d.existed = true
	if len(bytes.TrimSpace(data)) == 0 {
		return d, nil
	}
	v, err := Parse(data)
	if err != nil {
		return nil, fmt.Errorf("%s is not valid JSON: %w", path, err)
	}
	obj, ok := v.(*Object)
	if !ok {
		return nil, fmt.Errorf("%s does not contain a JSON object", path)
	}
	d.Root = obj
	d.indent = detectIndent(data)
	d.trailingNewline = bytes.HasSuffix(data, []byte("\n"))
	return d, nil
}

// Existed reports whether the file was present when loaded.
func (d *Document) Existed() bool { return d.existed }

// Bytes renders the document the way it will be written.
func (d *Document) Bytes() ([]byte, error) {
	b, err := Marshal(d.Root, d.indent)
	if err != nil {
		return nil, err
	}
	if d.trailingNewline {
		b = append(b, '\n')
	}
	return b, nil
}

// ChangedBytes reports whether writing this document would alter the file.
func (d *Document) ChangedBytes() ([]byte, bool, error) {
	out, err := d.Bytes()
	if err != nil {
		return nil, false, err
	}
	current, err := store.ReadFileOrEmpty(d.Path)
	if err != nil {
		return nil, false, err
	}
	if bytes.Equal(current, out) {
		return out, false, nil
	}
	return out, true, nil
}

// Patch is the exact set of changes ccsc is allowed to make. Everything not
// listed here is left alone.
type Patch struct {
	// SetEnv writes env keys. Iterated in sorted order so repeated applies
	// produce byte-identical files.
	SetEnv map[string]string
	// ClearEnv removes env keys ccsc previously wrote.
	ClearEnv []string

	// SetModelCompact writes modelSettings.<id>.autoCompactWindow. Used for
	// the global layer only, where the user's own modelSettings entries live
	// and sibling fields must survive.
	SetModelCompact map[string]int64
	// ClearModelCompact removes modelSettings.<id>.autoCompactWindow for
	// models ccsc wrote and no longer manages.
	ClearModelCompact []string

	// TopCompact writes the single-file-wide autoCompactWindow key. Used for
	// the project layer, where modelSettings merging is not documented.
	TopCompact *int64
	// ClearTopCompact removes it.
	ClearTopCompact bool

	// DropEmptyContainers removes an env / modelSettings block that this
	// patch emptied. Set by Clear, where leaving "env": {} behind is noise.
	DropEmptyContainers bool
}

// Result records what actually changed, for the receipt.
type Result struct {
	EnvSet        []string
	EnvCleared    []string
	ModelsSet     map[string]int64
	ModelsCleared []string
	TopSet        bool
	TopCleared    bool
	Changed       bool
}

// Summary renders the short trailing detail of a receipt line.
func (r Result) Summary() string {
	var parts []string
	if n := len(r.EnvSet); n > 0 {
		parts = append(parts, fmt.Sprintf("%d keys updated", n))
	}
	if n := len(r.EnvCleared) + len(r.ModelsCleared); n > 0 {
		parts = append(parts, fmt.Sprintf("%d cleared", n))
	}
	if len(r.ModelsSet) > 0 {
		parts = append(parts, fmt.Sprintf("%d compact points", len(r.ModelsSet)))
	}
	if r.TopSet {
		parts = append(parts, "compact point set")
	}
	if r.TopCleared {
		parts = append(parts, "compact point cleared")
	}
	if len(parts) == 0 {
		return "no change"
	}
	return strings.Join(parts, ", ")
}

// Apply mutates the document in place and reports what changed.
func (d *Document) Apply(p Patch) Result {
	res := Result{ModelsSet: map[string]int64{}}

	if len(p.SetEnv) > 0 || len(p.ClearEnv) > 0 {
		env := d.Root.Object("env")
		if env == nil {
			env = d.Root.EnsureObject("env")
			d.createdEnv = true
		}
		keys := make([]string, 0, len(p.SetEnv))
		for k := range p.SetEnv {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			val := p.SetEnv[k]
			if cur, ok := env.String(k); ok && cur == val {
				continue
			}
			env.Set(k, val)
			res.EnvSet = append(res.EnvSet, k)
		}
		for _, k := range p.ClearEnv {
			if !env.Has(k) {
				continue
			}
			if _, ok := env.Get(k); ok {
				env.Delete(k)
				res.EnvCleared = append(res.EnvCleared, k)
			}
		}
		if (d.createdEnv || p.DropEmptyContainers) && env.Len() == 0 {
			d.Root.Delete("env")
		}
	}

	if len(p.SetModelCompact) > 0 || len(p.ClearModelCompact) > 0 {
		ms := d.Root.Object("modelSettings")
		if ms == nil {
			ms = d.Root.EnsureObject("modelSettings")
			d.createdModelSettings = true
		}
		ids := make([]string, 0, len(p.SetModelCompact))
		for id := range p.SetModelCompact {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			w := p.SetModelCompact[id]
			entry := ms.EnsureObject(id)
			if cur, ok := entry.Number("autoCompactWindow"); ok && cur == w {
				continue
			}
			entry.Set("autoCompactWindow", NewNumber(w))
			res.ModelsSet[id] = w
		}
		for _, id := range p.ClearModelCompact {
			entry := ms.Object(id)
			if entry == nil {
				continue
			}
			if !entry.Delete("autoCompactWindow") {
				continue
			}
			res.ModelsCleared = append(res.ModelsCleared, id)
			if entry.Len() == 0 {
				ms.Delete(id)
			}
		}
		if (d.createdModelSettings || p.DropEmptyContainers) && ms.Len() == 0 {
			d.Root.Delete("modelSettings")
		}
	}

	if p.TopCompact != nil {
		if cur, ok := d.Root.Number("autoCompactWindow"); !ok || cur != *p.TopCompact {
			d.Root.Set("autoCompactWindow", NewNumber(*p.TopCompact))
			res.TopSet = true
		}
	}
	if p.ClearTopCompact && d.Root.Delete("autoCompactWindow") {
		res.TopCleared = true
	}

	res.Changed = len(res.EnvSet) > 0 || len(res.EnvCleared) > 0 ||
		len(res.ModelsSet) > 0 || len(res.ModelsCleared) > 0 ||
		res.TopSet || res.TopCleared
	return res
}

// Write persists the document atomically and preserves the file mode of an
// existing file.
func (d *Document) Write() error {
	out, changed, err := d.ChangedBytes()
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}
	return store.AtomicWrite(d.Path, out, 0o600)
}

// detectIndent guesses the indentation unit from the first indented line.
func detectIndent(data []byte) string {
	i := bytes.IndexByte(data, '\n')
	if i < 0 {
		return "  "
	}
	line := data[i+1:]
	for len(line) > 0 && (line[0] == '\n' || line[0] == '\r') {
		j := bytes.IndexByte(line, '\n')
		if j < 0 {
			return "  "
		}
		line = line[j+1:]
	}
	spaces := 0
	for spaces < len(line) && line[spaces] == ' ' {
		spaces++
	}
	if spaces > 0 {
		return strings.Repeat(" ", spaces)
	}
	if len(line) > 0 && line[0] == '\t' {
		return "\t"
	}
	return "  "
}
