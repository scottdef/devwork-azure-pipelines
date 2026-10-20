// Package brain holds the current key/value snapshot and knows which
// templates depend on which keys.
//
// Each render gets a recording Store: every key, ls and tree call is noted.
// When a new snapshot arrives the brain computes the set of changed keys,
// and only templates whose recorded dependencies intersect it are rendered
// again. A render that asked for a key that does not exist is incomplete
// and must not be written.
package brain

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/CoolGitOrg/tmplwatch/internal/funcs"
	"github.com/CoolGitOrg/tmplwatch/internal/source"
)

// Deps is the set of keys and prefixes one render read.
type Deps struct {
	Keys     map[string]bool
	Prefixes map[string]bool // normalised to end in "/" ("" means everything)
}

// Touches reports whether any of the changed keys could alter a render
// with these dependencies.
func (d Deps) Touches(changed []string) bool {
	for _, k := range changed {
		if d.Keys[k] {
			return true
		}
		for p := range d.Prefixes {
			if strings.HasPrefix(k, p) {
				return true
			}
		}
	}
	return false
}

// List returns the dependencies as sorted strings; prefixes end in "/".
func (d Deps) List() []string {
	var out []string
	for k := range d.Keys {
		out = append(out, k)
	}
	for p := range d.Prefixes {
		if p == "" {
			p = "/"
		}
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// Template is a parsed template ready to render.
type Template struct {
	Name string
	Path string
	t    *template.Template
	dir  fs.FS
	miss string // missingkey option, re-applied after Clone
}

// TemplateOptions control parsing.
type TemplateOptions struct {
	Left, Right string // delimiters; empty means {{ }}
	MissingKey  string // text/template missingkey option: default, zero or error
}

// ParseFile reads and parses the template at path.
func ParseFile(name, path string, o TemplateOptions) (*Template, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(name, path, string(b), os.DirFS(filepath.Dir(path)), o)
}

// Parse parses text as a template. dir roots the file function.
func Parse(name, path, text string, dir fs.FS, o TemplateOptions) (*Template, error) {
	switch o.MissingKey {
	case "":
		o.MissingKey = "default"
	case "default", "zero", "error", "invalid":
	default:
		return nil, fmt.Errorf("%s: missingkey %q: want default, zero or error", name, o.MissingKey)
	}
	// Parse with a placeholder map so the parser knows the function names;
	// each render rebinds them to a recording store via Clone.
	t, err := template.New(name).
		Delims(o.Left, o.Right).
		Option("missingkey=" + o.MissingKey).
		Funcs(funcs.Map(funcs.Options{})).
		Parse(text)
	if err != nil {
		return nil, err
	}
	return &Template{Name: name, Path: path, t: t, dir: dir, miss: o.MissingKey}, nil
}

// Result is the outcome of one render.
type Result struct {
	Name     string
	Out      []byte
	Deps     Deps
	Missing  []string // keys that were absent; non-empty means incomplete
	Err      error
	Duration time.Duration
	Version  string // snapshot version rendered against
}

// Complete reports whether the result may be written.
func (r Result) Complete() bool { return r.Err == nil && len(r.Missing) == 0 }

// Brain holds the latest snapshot and per-template dependencies.
// It is safe for concurrent use.
type Brain struct {
	// Now and Env are passed to every render; tests pin them.
	Now func() time.Time
	Env func(string) string

	mu   sync.RWMutex
	snap source.Snapshot
	deps map[string]Deps
	have bool
}

// New returns an empty brain.
func New() *Brain { return &Brain{deps: map[string]Deps{}} }

// Snapshot returns the current snapshot.
func (b *Brain) Snapshot() source.Snapshot {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.snap
}

// Update installs s and returns the sorted keys that were added, removed
// or modified. The first update reports every key.
func (b *Brain) Update(s source.Snapshot) []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	changed := Diff(b.snap.Data, s.Data)
	b.snap, b.have = s, true
	return changed
}

// Diff returns the sorted keys whose presence or value differs.
func Diff(old, new map[string]string) []string {
	var out []string
	for k, v := range new {
		if ov, ok := old[k]; !ok || ov != v {
			out = append(out, k)
		}
	}
	for k := range old {
		if _, ok := new[k]; !ok {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// Dirty reports whether name must be rendered again after changed keys.
// A template never rendered is always dirty.
func (b *Brain) Dirty(name string, changed []string) bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	d, ok := b.deps[name]
	return !ok || d.Touches(changed)
}

// Deps returns the dependencies recorded by name's last render.
func (b *Brain) Deps(name string) Deps {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.deps[name]
}

// Render executes t against the current snapshot, records its
// dependencies, and returns the result. Dependencies are recorded even
// for incomplete or failed renders, so that the key that was missing
// triggers the next attempt.
func (b *Brain) Render(t *Template) Result {
	b.mu.RLock()
	snap := b.snap
	b.mu.RUnlock()
	rec := &recorder{data: snap.Data, deps: Deps{Keys: map[string]bool{}, Prefixes: map[string]bool{}}}
	start := time.Now()
	res := Result{Name: t.Name, Version: snap.Version}
	tc, err := t.t.Clone()
	if err != nil {
		res.Err = err
		return res
	}
	// Clone does not carry Option settings (Go 1.24), so set missingkey again.
	tc.Option("missingkey=" + t.miss)
	tc.Funcs(funcs.Map(funcs.Options{Store: rec, Dir: t.dir, Now: b.Now, Env: b.Env}))
	var buf bytes.Buffer
	res.Err = safeExecute(tc, &buf)
	res.Duration = time.Since(start)
	res.Out = buf.Bytes()
	res.Deps = rec.deps
	res.Missing = rec.missing
	b.mu.Lock()
	b.deps[t.Name] = rec.deps
	b.mu.Unlock()
	return res
}

// safeExecute runs the template, converting a panic in a template
// function (text/template already recovers most) into an error so one bad
// template cannot take the daemon down.
func safeExecute(t *template.Template, buf *bytes.Buffer) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("template panic: %v", r)
		}
	}()
	return t.Execute(buf, nil)
}

// recorder is the funcs.Store handed to one render.
type recorder struct {
	data    map[string]string
	deps    Deps
	missing []string
}

func (r *recorder) Get(k string) (string, bool) {
	r.deps.Keys[k] = true
	v, ok := r.data[k]
	return v, ok
}

func (r *recorder) List(prefix string, recursive bool) []funcs.KVPair {
	p := strings.Trim(prefix, "/")
	if p != "" {
		p += "/"
	}
	r.deps.Prefixes[p] = true
	return funcs.List(r.data, prefix, recursive)
}

func (r *recorder) Missing(k string) {
	for _, m := range r.missing {
		if m == k {
			return
		}
	}
	r.missing = append(r.missing, k)
}
