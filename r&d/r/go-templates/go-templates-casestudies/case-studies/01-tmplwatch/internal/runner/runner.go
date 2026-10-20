// Package runner turns snapshots into files: it renders the templates a
// change touches, writes complete renders atomically, and runs commands
// and signals the supervised process afterwards.
package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"math/rand/v2"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/CoolGitOrg/tmplwatch/internal/brain"
	"github.com/CoolGitOrg/tmplwatch/internal/config"
	"github.com/CoolGitOrg/tmplwatch/internal/diff"
	"github.com/CoolGitOrg/tmplwatch/internal/metrics"
	"github.com/CoolGitOrg/tmplwatch/internal/source"
)

// Runner owns the templates and the brain. Update and Reload are
// serialised; Status and Current may be called from any goroutine.
type Runner struct {
	Brain   *brain.Brain
	Metrics *metrics.Metrics
	Log     *log.Logger
	Dry     bool      // print diffs instead of writing
	DryOut  io.Writer // where -dry output goes; default os.Stdout
	Once    bool      // run commands synchronously, without wait or splay
	Exec    *Supervisor

	ctx     context.Context
	mu      sync.Mutex // serialises Update / Reload
	smu     sync.RWMutex
	entries []*entry
	updates int
}

type entry struct {
	cfg  config.Template
	perm os.FileMode
	tmpl *brain.Template

	// guarded by Runner.smu
	last    brain.Result
	lastAt  time.Time
	current []byte // last complete render
	written time.Time
	cmdErr  string

	cmdMu  sync.Mutex // one command at a time per template
	timer  *time.Timer
	timerM sync.Mutex
}

// New parses every template in cfg. A template that fails to parse is an
// error: better to refuse to start than to run half-configured.
func New(ctx context.Context, cfg *config.Config, b *brain.Brain, m *metrics.Metrics, lg *log.Logger) (*Runner, error) {
	r := &Runner{Brain: b, Metrics: m, Log: lg, Dry: cfg.Dry, Once: cfg.Once, DryOut: os.Stdout, ctx: ctx}
	for _, tc := range cfg.Templates {
		e := &entry{cfg: tc}
		e.perm, _ = tc.Mode()
		if err := e.parse(); err != nil {
			return nil, err
		}
		if m != nil {
			m.Template(tc.Name)
		}
		if gone, _ := CleanStale(tc.Destination); len(gone) > 0 {
			lg.Printf("%s: removed stale temp files %v", tc.Name, gone)
		}
		r.entries = append(r.entries, e)
	}
	return r, nil
}

func (e *entry) parse() error {
	miss := "default"
	if e.cfg.ErrorOnMissingKey {
		miss = "error"
	}
	t, err := brain.ParseFile(e.cfg.Name, e.cfg.Source, brain.TemplateOptions{
		Left: e.cfg.LeftDelimiter, Right: e.cfg.RightDelimiter, MissingKey: miss,
	})
	if err != nil {
		return fmt.Errorf("template %s: %w", e.cfg.Name, err)
	}
	e.tmpl = t
	return nil
}

// Outcome summarises one Update for callers such as -once.
type Outcome struct {
	Rendered   []string // templates rendered
	Written    []string // destinations changed (or that would change, in -dry)
	Incomplete []string // templates with missing keys
	Errors     []error  // render, write and (in -once) command errors
}

// Err joins the outcome's problems: errors and incomplete templates.
func (o Outcome) Err() error {
	errs := append([]error(nil), o.Errors...)
	for _, n := range o.Incomplete {
		errs = append(errs, fmt.Errorf("%s: incomplete (missing keys)", n))
	}
	return errors.Join(errs...)
}

// Update installs a snapshot and renders every template it affects.
func (r *Runner) Update(s source.Snapshot) Outcome {
	r.mu.Lock()
	defer r.mu.Unlock()
	changed := r.Brain.Update(s)
	return r.render(changed, false)
}

// Reload re-reads every template file from disk and renders all of them.
// Wired to SIGHUP (the "hangup" note on Plan 9). A template that no longer
// parses keeps its previous version and the error is reported.
func (r *Runner) Reload() Outcome {
	r.mu.Lock()
	defer r.mu.Unlock()
	var errs []error
	for _, e := range r.entries {
		old := e.tmpl
		if err := e.parse(); err != nil {
			e.tmpl = old
			r.Log.Printf("reload: %v (keeping previous version)", err)
			errs = append(errs, err)
		}
	}
	o := r.render(nil, true)
	o.Errors = append(errs, o.Errors...)
	return o
}

func (r *Runner) render(changed []string, all bool) Outcome {
	var o Outcome
	r.updates++
	anyWritten := false
	for _, e := range r.entries {
		if !all && !r.Brain.Dirty(e.cfg.Name, changed) {
			continue
		}
		res := r.Brain.Render(e.tmpl)
		o.Rendered = append(o.Rendered, e.cfg.Name)
		if r.Metrics != nil {
			r.Metrics.Render(e.cfg.Name, res.Duration, res.Err, len(res.Missing) > 0)
		}
		r.smu.Lock()
		e.last, e.lastAt = res, time.Now()
		if res.Complete() {
			e.current = res.Out
		}
		r.smu.Unlock()
		switch {
		case res.Err != nil:
			r.Log.Printf("%s: render: %v", e.cfg.Name, res.Err)
			o.Errors = append(o.Errors, fmt.Errorf("%s: %w", e.cfg.Name, res.Err))
			continue
		case len(res.Missing) > 0:
			r.Log.Printf("%s: waiting for keys %s", e.cfg.Name, strings.Join(res.Missing, ", "))
			o.Incomplete = append(o.Incomplete, e.cfg.Name)
			continue
		}
		wrote, err := r.write(e, res.Out)
		if err != nil {
			r.Log.Printf("%s: write %s: %v", e.cfg.Name, e.cfg.Destination, err)
			o.Errors = append(o.Errors, fmt.Errorf("%s: %w", e.cfg.Name, err))
			continue
		}
		if !wrote {
			continue
		}
		anyWritten = true
		o.Written = append(o.Written, e.cfg.Destination)
		if r.Metrics != nil {
			r.Metrics.Write(e.cfg.Name)
		}
		if len(e.cfg.Command) > 0 {
			if err := r.schedule(e); err != nil {
				o.Errors = append(o.Errors, err)
			}
		}
	}
	if !r.Dry && r.Exec != nil {
		switch {
		case anyWritten:
			r.Exec.Changed(r.allRendered())
		case r.allRendered():
			// Destinations already matched at startup: nothing was
			// written, but the child must still start.
			r.Exec.Ready()
		}
	}
	return o
}

// allRendered reports whether every template has a complete render, the
// condition for starting the exec child.
func (r *Runner) allRendered() bool {
	r.smu.RLock()
	defer r.smu.RUnlock()
	for _, e := range r.entries {
		if e.current == nil {
			return false
		}
	}
	return true
}

func (r *Runner) write(e *entry, out []byte) (bool, error) {
	if !r.Dry {
		wrote, err := WriteAtomic(e.cfg.Destination, out, e.perm, e.cfg.Backup)
		if wrote {
			r.smu.Lock()
			e.written = time.Now()
			r.smu.Unlock()
			r.Log.Printf("%s: wrote %s (%d bytes)", e.cfg.Name, e.cfg.Destination, len(out))
		}
		return wrote, err
	}
	old, err := os.ReadFile(e.cfg.Destination)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	if err == nil && string(old) == string(out) {
		return false, nil
	}
	w := r.DryOut
	if w == nil {
		w = os.Stdout
	}
	fmt.Fprintf(w, "# tmplwatch -dry: %s → %s (mode %04o, %d bytes)\n", e.cfg.Name, e.cfg.Destination, e.perm, len(out))
	io.WriteString(w, diff.Unified(e.cfg.Destination, e.cfg.Destination+" (rendered)", string(old), string(out)))
	if len(e.cfg.Command) > 0 {
		fmt.Fprintf(w, "# would run: %s\n", shellWords(e.cfg.Command))
	}
	return true, nil
}

// schedule runs e's command after the template has been quiet for Wait
// plus a random Splay. Every new write restarts the wait, so a burst of
// KV edits causes one command run, not one per edit.
func (r *Runner) schedule(e *entry) error {
	if r.Dry {
		return nil
	}
	if r.Once {
		return r.runCommand(e)
	}
	d := e.cfg.Wait.D()
	if s := e.cfg.Splay.D(); s > 0 {
		d += rand.N(s)
	}
	e.timerM.Lock()
	defer e.timerM.Unlock()
	if e.timer != nil {
		e.timer.Stop()
	}
	e.timer = time.AfterFunc(d, func() { r.runCommand(e) })
	return nil
}

// runCommand runs the template's command as an argument vector: no shell
// is involved, so values in the template can never become shell syntax.
func (r *Runner) runCommand(e *entry) error {
	e.cmdMu.Lock()
	defer e.cmdMu.Unlock()
	ctx := r.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, e.cfg.CommandTimeout.D())
	defer cancel()
	args := e.cfg.Command
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	start := time.Now()
	err := cmd.Run()
	result := "ok"
	if err != nil {
		result = "error"
		err = fmt.Errorf("%s: command %s: %w", e.cfg.Name, shellWords(args), err)
		r.Log.Print(err)
	} else {
		r.Log.Printf("%s: command %s ok in %v", e.cfg.Name, shellWords(args), time.Since(start).Round(time.Millisecond))
	}
	if r.Metrics != nil {
		r.Metrics.Command(e.cfg.Name, result)
	}
	r.smu.Lock()
	e.cmdErr = ""
	if err != nil {
		e.cmdErr = err.Error()
	}
	r.smu.Unlock()
	return err
}

// Stop cancels pending command timers.
func (r *Runner) Stop() {
	for _, e := range r.entries {
		e.timerM.Lock()
		if e.timer != nil {
			e.timer.Stop()
		}
		e.timerM.Unlock()
	}
}

// Names returns the template names in configuration order.
func (r *Runner) Names() []string {
	var out []string
	for _, e := range r.entries {
		out = append(out, e.cfg.Name)
	}
	return out
}

// Current returns the last complete render of the named template.
func (r *Runner) Current(name string) ([]byte, bool) {
	r.smu.RLock()
	defer r.smu.RUnlock()
	for _, e := range r.entries {
		if e.cfg.Name == name && e.current != nil {
			return e.current, true
		}
	}
	return nil, false
}

// TemplateStatus is one template's state, as served by /status.
type TemplateStatus struct {
	Name        string    `json:"name"`
	Source      string    `json:"source"`
	Destination string    `json:"destination"`
	Complete    bool      `json:"complete"`
	Missing     []string  `json:"missing,omitempty"`
	Error       string    `json:"error,omitempty"`
	CommandErr  string    `json:"command_error,omitempty"`
	Deps        []string  `json:"dependencies"`
	Bytes       int       `json:"bytes"`
	SHA256      string    `json:"sha256,omitempty"`
	Version     string    `json:"version,omitempty"`
	RenderedAt  time.Time `json:"rendered_at,omitzero"`
	WrittenAt   time.Time `json:"written_at,omitzero"`
}

// Status is the whole daemon's state.
type Status struct {
	Version   string           `json:"source_version"`
	Keys      int              `json:"keys"`
	Updates   int              `json:"updates"`
	Healthy   bool             `json:"healthy"`
	Dry       bool             `json:"dry"`
	Exec      *ExecStatus      `json:"exec,omitempty"`
	Templates []TemplateStatus `json:"templates"`
}

// Status reports the current state.
func (r *Runner) Status() Status {
	snap := r.Brain.Snapshot()
	r.smu.RLock()
	st := Status{Version: snap.Version, Keys: len(snap.Data), Updates: r.updates, Healthy: true, Dry: r.Dry}
	for _, e := range r.entries {
		ts := TemplateStatus{
			Name: e.cfg.Name, Source: e.cfg.Source, Destination: e.cfg.Destination,
			Complete: e.last.Complete() && !e.lastAt.IsZero(), Missing: e.last.Missing,
			Deps: r.Brain.Deps(e.cfg.Name).List(), Bytes: len(e.current), Version: e.last.Version,
			RenderedAt: e.lastAt, WrittenAt: e.written, CommandErr: e.cmdErr,
		}
		if e.last.Err != nil {
			ts.Error = e.last.Err.Error()
		}
		if e.current != nil {
			ts.SHA256 = sha256hex(e.current)
		}
		if ts.Deps == nil {
			ts.Deps = []string{}
		}
		if !ts.Complete {
			st.Healthy = false
		}
		st.Templates = append(st.Templates, ts)
	}
	r.smu.RUnlock()
	if r.Exec != nil {
		es := r.Exec.Status()
		st.Exec = &es
	}
	return st
}

// Incomplete returns the names of templates not yet rendered completely.
func (r *Runner) Incomplete() []string {
	var out []string
	for _, t := range r.Status().Templates {
		if !t.Complete {
			out = append(out, t.Name)
		}
	}
	return out
}

// shellWords prints argv the way a person would type it, for logs only.
func shellWords(args []string) string {
	q := make([]string, len(args))
	for i, a := range args {
		if a != "" && !strings.ContainsAny(a, " \t\n'\"\\$`;&|<>(){}*?[]#~") {
			q[i] = a
		} else {
			q[i] = "'" + strings.ReplaceAll(a, "'", `'\''`) + "'"
		}
	}
	return strings.Join(q, " ")
}
