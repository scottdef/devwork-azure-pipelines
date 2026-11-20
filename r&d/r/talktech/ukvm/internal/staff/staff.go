// Package staff is the chief of staff: it takes orders from the principal,
// decides whether they may run, hands each step to the right subordinate,
// and keeps the books.
//
// What the chief does itself: intake, gating, placement, tracking, reporting
// and the audit ledger. What it delegates: every keystroke, every frame
// grab, every conversion that some other machine can do.
package staff

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"ukvm/internal/broker"
	"ukvm/internal/hid"
	"ukvm/internal/intake"
	"ukvm/internal/ledger"
	"ukvm/internal/order"
)

// Job states.
const (
	StateAwait     = "awaiting-confirm"
	StateRunning   = "running"
	StateDone      = "done"
	StateFailed    = "failed"
	StateRefused   = "refused"
	StateCancelled = "cancelled"
)

// Step is one executed call and what came of it.
type Step struct {
	Call   order.Call `json:"call"`
	Node   string     `json:"node,omitempty"`
	Output string     `json:"output,omitempty"`
	Err    string     `json:"error,omitempty"`
	Start  time.Time  `json:"start"`
	End    time.Time  `json:"end"`
}

// Job is one order and its whole history.
type Job struct {
	ID      string       `json:"id"`
	Created time.Time    `json:"created"`
	Source  string       `json:"source"` // text, voice, panel, runbook timer...
	Text    string       `json:"text"`
	Calls   []order.Call `json:"calls"`
	Conf    float64      `json:"confidence"`
	State   string       `json:"state"`
	Reason  string       `json:"reason,omitempty"`
	Steps   []Step       `json:"steps,omitempty"`
}

func (j *Job) clone() Job {
	c := *j
	c.Calls = append([]order.Call(nil), j.Calls...)
	c.Steps = append([]Step(nil), j.Steps...)
	return c
}

// Plan renders the decoded calls: what will actually happen, as opposed to
// what was typed or heard.
func (j Job) Plan() string {
	parts := make([]string, len(j.Calls))
	for i, c := range j.Calls {
		parts[i] = c.String()
	}
	return strings.Join(parts, " → ")
}

// Config is the chief's own settings.
type Config struct {
	Self          string        // this node's name in the registry
	StateDir      string        // jobs, notes, runbooks, converted documents
	DocRoot       string        // the only folder read_pdf may read from
	Settle        time.Duration // wait between typing a command and reading the screen
	ConfirmWindow time.Duration // an unconfirmed order expires after this long
	VoiceConfirm  bool          // allow a spoken "confirm" (off: confirm by touch or CLI)
	Token         string        // bearer token presented to satellites
}

// Deps are the chief's subordinates and instruments.
type Deps struct {
	Policy order.Policy
	Reg    *broker.Registry
	Ledger *ledger.Ledger
	Tools  intake.Tools
	Hands  map[string]Hands // tethers plugged into this node, by target
	Eyes   map[string]Eyes
	Armed  func() bool // false: describe actions instead of performing them
	Now    func() time.Time
	Sleep  func(time.Duration)
	HTTP   *http.Client
	Hear   func(ctx context.Context, wavPath string) (intake.Heard, error) // nil: no speech engine
}

// Chief is the running chief of staff.
type Chief struct {
	cfg Config
	Deps

	runMu sync.Mutex // one order executes at a time

	mu       sync.Mutex // guards everything below
	jobs     map[string]*Job
	seq      int
	focus    string
	notes    map[string]string
	approved map[string]string // runbook name -> SHA-256 it was approved at
}

type saved struct {
	Seq      int               `json:"seq"`
	Focus    string            `json:"focus"`
	Notes    map[string]string `json:"notes"`
	Approved map[string]string `json:"approved"`
}

// New starts a chief, reloading any jobs and notes left by a previous run.
func New(cfg Config, d Deps) (*Chief, error) {
	if cfg.Self == "" || cfg.StateDir == "" {
		return nil, errors.New("staff: Self and StateDir are required")
	}
	if d.Reg == nil || d.Ledger == nil {
		return nil, errors.New("staff: a registry and a ledger are required")
	}
	if cfg.Settle == 0 {
		cfg.Settle = 1500 * time.Millisecond
	}
	if cfg.ConfirmWindow == 0 {
		cfg.ConfirmWindow = 5 * time.Minute
	}
	if d.Now == nil {
		d.Now = time.Now
	}
	if d.Sleep == nil {
		d.Sleep = time.Sleep
	}
	if d.Armed == nil {
		d.Armed = func() bool { return false }
	}
	if d.HTTP == nil {
		d.HTTP = &http.Client{Timeout: 90 * time.Second}
	}
	if d.Tools.Run == nil {
		d.Tools.Run = intake.Exec
	}
	c := &Chief{cfg: cfg, Deps: d, jobs: map[string]*Job{}, notes: map[string]string{}, approved: map[string]string{}}
	for _, sub := range []string{"jobs", "runbooks", "docs"} {
		if err := os.MkdirAll(filepath.Join(cfg.StateDir, sub), 0o750); err != nil {
			return nil, err
		}
	}
	if b, err := os.ReadFile(filepath.Join(cfg.StateDir, "chief.json")); err == nil {
		var s saved
		if err := json.Unmarshal(b, &s); err != nil {
			return nil, fmt.Errorf("staff: chief.json: %w", err)
		}
		c.seq, c.focus = s.Seq, s.Focus
		if s.Notes != nil {
			c.notes = s.Notes
		}
		if s.Approved != nil {
			c.approved = s.Approved
		}
	}
	files, _ := filepath.Glob(filepath.Join(cfg.StateDir, "jobs", "*.json"))
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var j Job
		if json.Unmarshal(b, &j) == nil && j.ID != "" {
			if j.State == StateRunning { // the previous process died mid-order
				j.State, j.Reason = StateFailed, "interrupted by a restart; not retried"
			}
			c.jobs[j.ID] = &j
		}
	}
	return c, nil
}

// ---- orders in ----

// Submit takes one order, already decoded into calls, and gates it.
func (c *Chief) Submit(ctx context.Context, source, text string, calls []order.Call, conf float64) Job {
	c.runMu.Lock()
	defer c.runMu.Unlock()

	if len(calls) == 1 && (calls[0].Name == "confirm" || calls[0].Name == "cancel") {
		j, err := c.resolve(ctx, "", calls[0].Name == "confirm", source)
		if err != nil {
			return Job{Source: source, Text: text, State: StateRefused, Reason: err.Error(), Created: c.Now()}
		}
		return j
	}

	c.mu.Lock()
	c.seq++
	job := &Job{ID: fmt.Sprintf("j%04d", c.seq), Created: c.Now(), Source: source, Text: text, Calls: calls, Conf: conf}
	c.jobs[job.ID] = job
	c.mu.Unlock()
	c.saveState()

	verdict, reason := order.Gate(calls, conf, c.modeOf)
	c.note("order", map[string]any{"job": job.ID, "source": source, "text": text, "calls": calls,
		"confidence": conf, "verdict": verdict, "reason": reason})
	switch verdict {
	case order.Refuse:
		c.set(job, func(j *Job) { j.State, j.Reason = StateRefused, reason })
	case order.NeedConfirm:
		c.set(job, func(j *Job) { j.State, j.Reason = StateAwait, reason })
	default:
		c.run(ctx, job)
	}
	return c.snapshot(job)
}

// Resolve confirms or cancels an order awaiting confirmation. An empty id
// means the oldest one.
func (c *Chief) Resolve(ctx context.Context, id string, approve bool, source string) (Job, error) {
	c.runMu.Lock()
	defer c.runMu.Unlock()
	return c.resolve(ctx, id, approve, source)
}

func (c *Chief) resolve(ctx context.Context, id string, approve bool, source string) (Job, error) {
	c.mu.Lock()
	var job *Job
	if id == "" {
		for _, jid := range c.idsLocked() {
			if c.jobs[jid].State == StateAwait {
				job = c.jobs[jid]
				break
			}
		}
	} else if j, ok := c.jobs[id]; ok && j.State == StateAwait {
		job = j
	}
	c.mu.Unlock()
	if job == nil {
		return Job{}, errors.New("nothing is awaiting confirmation")
	}
	if !approve {
		c.note("cancel", map[string]any{"job": job.ID, "source": source})
		c.set(job, func(j *Job) { j.State, j.Reason = StateCancelled, "cancelled from "+source })
		return c.snapshot(job), nil
	}
	if source == "voice" && !c.cfg.VoiceConfirm {
		return Job{}, errors.New("spoken confirmation is switched off; confirm on the panel")
	}
	if age := c.Now().Sub(job.Created); age > c.cfg.ConfirmWindow {
		c.set(job, func(j *Job) { j.State, j.Reason = StateCancelled, "confirmation window expired" })
		return c.snapshot(job), errors.New("that order expired unconfirmed; give it again")
	}
	for _, call := range job.Calls { // the policy or a runbook may have changed since
		if _, err := c.modeOf(call); err != nil {
			c.set(job, func(j *Job) { j.State, j.Reason = StateRefused, err.Error() })
			return c.snapshot(job), err
		}
	}
	c.note("confirm", map[string]any{"job": job.ID, "source": source})
	c.run(ctx, job)
	return c.snapshot(job), nil
}

func (c *Chief) modeOf(call order.Call) (order.Mode, error) {
	m, err := order.Validate(call, c.Policy)
	if err != nil {
		return m, err
	}
	if call.Name == "runbook_run" {
		name := call.Str("name")
		sum, err := c.runbookSum(name)
		if err != nil {
			return "", fmt.Errorf("there is no runbook called %q", name)
		}
		c.mu.Lock()
		ok := c.approved[name] == sum
		c.mu.Unlock()
		if ok {
			return order.Auto, nil // approved, and unchanged since approval
		}
	}
	return m, nil
}

// ---- execution ----

func (c *Chief) run(ctx context.Context, job *Job) {
	c.set(job, func(j *Job) { j.State, j.Reason = StateRunning, "" })
	for _, call := range job.Calls {
		st := Step{Call: call, Start: c.Now()}
		out, node, err := c.step(ctx, call, 0)
		st.End, st.Node, st.Output = c.Now(), node, out
		if err != nil {
			st.Err = err.Error()
		}
		c.set(job, func(j *Job) { j.Steps = append(j.Steps, st) })
		c.note("result", map[string]any{"job": job.ID, "call": call, "node": node, "error": st.Err, "output": clip(out, 2000)})
		if err != nil {
			// Stop at the first failure and never retry: a half-typed command
			// typed again is worse than a job that needs a human.
			c.set(job, func(j *Job) { j.State, j.Reason = StateFailed, err.Error() })
			return
		}
	}
	c.set(job, func(j *Job) { j.State = StateDone })
}

func (c *Chief) step(ctx context.Context, call order.Call, depth int) (out, node string, err error) {
	self := c.cfg.Self
	switch call.Name {
	case "select_target":
		t := call.Str("target")
		c.mu.Lock()
		c.focus = t
		c.mu.Unlock()
		c.saveState()
		return "attention is on " + t, self, nil
	case "status":
		return c.StatusText(), self, nil
	case "note":
		c.mu.Lock()
		c.notes[call.Str("key")] = call.Str("value")
		c.mu.Unlock()
		c.saveState()
		return "noted " + call.Str("key"), self, nil
	case "recall":
		c.mu.Lock()
		v, ok := c.notes[call.Str("key")]
		c.mu.Unlock()
		if !ok {
			return "nothing recorded under " + call.Str("key"), self, nil
		}
		return v, self, nil
	case "read_pdf":
		return c.readPDF(ctx, call.Str("path"))
	case "runbook_run":
		if depth > 0 {
			return "", self, errors.New("a runbook cannot start another runbook")
		}
		return c.runbook(ctx, call.Str("name"))
	case "confirm", "cancel":
		return "", self, fmt.Errorf("%q must be given on its own", call.Name)
	}

	target := c.Focus()
	if target == "" {
		return "", self, errors.New(`no computer is selected; say "target NAME" first`)
	}
	if call.Name == "run_command" {
		// A shell on the target is exact; a keyboard and a camera are not.
		// Use the target's own resources when it offers them.
		if n, err := c.Reg.Place(broker.Need{Cap: "exec:" + target}, c.Now()); err == nil && n.SSH != "" {
			return c.sshRun(ctx, n, target, call.Str("command"))
		}
	}
	return c.onTarget(ctx, target, call)
}

func (c *Chief) sshRun(ctx context.Context, n broker.Node, target, cmd string) (string, string, error) {
	armed := c.Armed()
	if err := c.act(map[string]any{"via": "ssh", "node": n.Name, "target": target, "command": cmd, "armed": armed}); err != nil {
		return "", n.Name, err
	}
	if !armed {
		return fmt.Sprintf("DRY-RUN: would run %q on %s over SSH", cmd, target), n.Name, nil
	}
	out, err := c.Tools.Run(ctx, nil, "ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=5", n.SSH, "--", cmd)
	return strings.TrimRight(string(out), "\n"), n.Name, err
}

// onTarget performs a call on whichever node holds the target's tether.
func (c *Chief) onTarget(ctx context.Context, target string, call order.Call) (string, string, error) {
	need := "hands:" + target
	if call.Name == "read_screen" {
		need = "eyes:" + target
	}
	n, err := c.Reg.Place(broker.Need{Cap: need}, c.Now())
	if err != nil {
		return "", "", fmt.Errorf("%s has no reachable tether (%s)", target, need)
	}
	if n.Name == c.cfg.Self {
		out, err := c.local(ctx, target, call)
		return out, n.Name, err
	}
	if call.Name != "read_screen" {
		armed := c.Armed()
		if err := c.act(map[string]any{"via": "satellite", "node": n.Name, "target": target, "call": call, "armed": armed}); err != nil {
			return "", n.Name, err
		}
		if !armed {
			return fmt.Sprintf("DRY-RUN: would ask %s to perform %s on %s", n.Name, call, target), n.Name, nil
		}
	}
	out, err := c.remote(ctx, n, target, call)
	return out, n.Name, err
}

func (c *Chief) remote(ctx context.Context, n broker.Node, target string, call order.Call) (string, error) {
	body, _ := json.Marshal(map[string]any{"target": target, "call": call})
	url := n.Addr
	if !strings.Contains(url, "://") {
		url = "http://" + url
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url+"/v1/act", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("satellite %s: %w", n.Name, err)
	}
	defer resp.Body.Close()
	var r struct {
		Output string `json:"output"`
		Error  string `json:"error"`
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err := json.Unmarshal(raw, &r); err != nil {
		return "", fmt.Errorf("satellite %s: HTTP %d: %s", n.Name, resp.StatusCode, clip(string(raw), 200))
	}
	if r.Error != "" {
		return r.Output, fmt.Errorf("satellite %s: %s", n.Name, r.Error)
	}
	return r.Output, nil
}

// Act is the satellite side: perform one tether action that the chief has
// already gated. The satellite still checks it against its own policy copy.
func (c *Chief) Act(ctx context.Context, target string, call order.Call) (string, error) {
	if _, err := order.Validate(call, c.Policy); err != nil {
		return "", err
	}
	switch call.Name {
	case "read_screen", "press_keys", "type_text", "click", "click_text", "run_command":
	default:
		return "", fmt.Errorf("%s is not a tether action", call.Name)
	}
	c.runMu.Lock()
	defer c.runMu.Unlock()
	return c.local(ctx, target, call)
}

// local performs a tether action with hands and eyes plugged into this node.
func (c *Chief) local(ctx context.Context, target string, call order.Call) (string, error) {
	switch call.Name {
	case "read_screen":
		return c.screenText(ctx, target)
	case "press_keys":
		rep, err := hid.ParseChord(call.Str("chord"))
		if err != nil {
			return "", err
		}
		return c.keys(target, call, []hid.KeyReport{rep, hid.Release})
	case "type_text":
		reps, err := hid.Type(call.Str("text"))
		if err != nil {
			return "", err
		}
		return c.keys(target, call, reps)
	case "click":
		x, _ := call.Int("x")
		y, _ := call.Int("y")
		return c.click(target, call, x, y)
	case "click_text":
		e, ok := c.Eyes[target]
		if !ok {
			return "", fmt.Errorf("%s has no display capture on %s", target, c.cfg.Self)
		}
		path, done, err := e.Frame(ctx)
		if err != nil {
			return "", err
		}
		defer done()
		x, y, err := c.Tools.Locate(ctx, path, call.Str("text"))
		if err != nil {
			return "", err
		}
		return c.click(target, call, x, y)
	case "run_command":
		cmd := call.Str("command")
		if txt, err := c.screenText(ctx, target); err == nil && atLogin(txt) {
			return "", errors.New("the screen shows a login or password prompt; not typing a command into it")
		}
		reps, err := hid.Type(cmd + "\n")
		if err != nil {
			return "", err
		}
		out, err := c.keys(target, call, reps)
		if err != nil || !c.Armed() {
			return out, err
		}
		c.Sleep(c.cfg.Settle)
		txt, err := c.screenText(ctx, target)
		if err != nil {
			return out + "\n(typed, but the screen could not be read back: " + err.Error() + ")", nil
		}
		return tail(txt, 15), nil
	}
	return "", fmt.Errorf("%s cannot run on a tether", call.Name)
}

func (c *Chief) keys(target string, call order.Call, reps []hid.KeyReport) (string, error) {
	armed := c.Armed()
	if err := c.act(map[string]any{"via": "hid", "node": c.cfg.Self, "target": target, "call": call, "reports": len(reps), "armed": armed}); err != nil {
		return "", err
	}
	if !armed {
		return fmt.Sprintf("DRY-RUN: would send %d key reports to %s", len(reps), target), nil
	}
	h, ok := c.Hands[target]
	if !ok {
		return "", fmt.Errorf("%s has no keyboard tether on %s", target, c.cfg.Self)
	}
	if err := h.Keys(reps); err != nil {
		return "", err
	}
	return fmt.Sprintf("sent %d key reports to %s", len(reps), target), nil
}

func (c *Chief) click(target string, call order.Call, x, y int) (string, error) {
	armed := c.Armed()
	if err := c.act(map[string]any{"via": "hid", "node": c.cfg.Self, "target": target, "call": call, "x": x, "y": y, "armed": armed}); err != nil {
		return "", err
	}
	if !armed {
		return fmt.Sprintf("DRY-RUN: would click at %d,%d on %s", x, y, target), nil
	}
	h, ok := c.Hands[target]
	if !ok {
		return "", fmt.Errorf("%s has no pointer tether on %s", target, c.cfg.Self)
	}
	if err := h.Pointer(0, x, y); err != nil { // move, press, release
		return "", err
	}
	if err := h.Pointer(1, x, y); err != nil {
		return "", err
	}
	if err := h.Pointer(0, x, y); err != nil {
		return "", err
	}
	return fmt.Sprintf("clicked at %d,%d on %s", x, y, target), nil
}

func (c *Chief) screenText(ctx context.Context, target string) (string, error) {
	e, ok := c.Eyes[target]
	if !ok {
		return "", fmt.Errorf("%s has no display capture on %s", target, c.cfg.Self)
	}
	path, done, err := e.Frame(ctx)
	if err != nil {
		return "", err
	}
	defer done()
	// Reading is work like any other: lend it out when someone can take it.
	if n, err := c.Reg.Place(broker.Need{Cap: "ocr", MemMB: 200}, c.Now()); err == nil && n.SSH != "" {
		if txt, err := c.Tools.ImageTextVia(ctx, n.SSH, path); err == nil {
			return txt, nil
		}
	}
	return c.Tools.ImageText(ctx, path)
}

func (c *Chief) readPDF(ctx context.Context, rel string) (string, string, error) {
	self := c.cfg.Self
	if c.cfg.DocRoot == "" {
		return "", self, errors.New("no document folder is configured")
	}
	root, err := filepath.EvalSymlinks(c.cfg.DocRoot)
	if err != nil {
		return "", self, err
	}
	full, err := filepath.EvalSymlinks(filepath.Join(root, filepath.Clean("/"+rel)))
	if err != nil || !strings.HasPrefix(full, root+string(filepath.Separator)) {
		return "", self, fmt.Errorf("no document %q in the document folder", rel)
	}
	n, err := c.Reg.Place(broker.Need{Cap: "pdf", MemMB: 128}, c.Now())
	if err != nil {
		return "", self, err
	}
	where, text := self, ""
	if n.SSH != "" {
		where = n.Name
		text, err = c.Tools.PDFTextVia(ctx, n.SSH, full)
		if err != nil || len(strings.Fields(text)) == 0 { // lender failed, or the PDF is a scan
			where = self
			text, err = c.Tools.PDFText(ctx, full)
		}
	} else {
		text, err = c.Tools.PDFText(ctx, full)
	}
	if err != nil {
		return "", where, err
	}
	out := filepath.Join(c.cfg.StateDir, "docs", strings.TrimSuffix(filepath.Base(full), filepath.Ext(full))+".txt")
	if err := os.WriteFile(out, []byte(text), 0o640); err != nil {
		return "", where, err
	}
	return fmt.Sprintf("%d characters of text saved to %s\n%s", len(text), out, head(text, 12)), where, nil
}

// ---- runbooks ----

func (c *Chief) runbookPath(name string) string {
	return filepath.Join(c.cfg.StateDir, "runbooks", name+".ord")
}

func (c *Chief) runbookSum(name string) (string, error) {
	b, err := os.ReadFile(c.runbookPath(name))
	if err != nil {
		return "", err
	}
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:]), nil
}

func (c *Chief) runbookCalls(text string) ([]order.Call, error) {
	calls, err := order.Parse(text)
	if err != nil {
		return nil, err
	}
	for _, call := range calls {
		switch call.Name {
		case "runbook_run", "confirm", "cancel":
			return nil, fmt.Errorf("%q is not allowed inside a runbook", call.Name)
		}
		if _, err := order.Validate(call, c.Policy); err != nil {
			return nil, err
		}
	}
	return calls, nil
}

// SaveRunbook stores a named list of orders after checking every line
// against the policy. A saved runbook still needs approval before it will
// run unattended, and any later edit withdraws that approval.
func (c *Chief) SaveRunbook(name, text string) error {
	if _, err := order.Validate(order.Call{Name: "runbook_run", Args: map[string]any{"name": name}}, c.Policy); err != nil {
		return err
	}
	if _, err := c.runbookCalls(text); err != nil {
		return err
	}
	if err := os.WriteFile(c.runbookPath(name), []byte(text), 0o640); err != nil {
		return err
	}
	sum, _ := c.runbookSum(name)
	c.note("runbook-save", map[string]any{"name": name, "sha256": sum})
	return nil
}

// ApproveRunbook records the principal's approval of a runbook exactly as it
// stands now.
func (c *Chief) ApproveRunbook(name string) (string, error) {
	sum, err := c.runbookSum(name)
	if err != nil {
		return "", fmt.Errorf("there is no runbook called %q", name)
	}
	c.mu.Lock()
	c.approved[name] = sum
	c.mu.Unlock()
	c.saveState()
	c.note("runbook-approve", map[string]any{"name": name, "sha256": sum})
	return sum, nil
}

func (c *Chief) runbook(ctx context.Context, name string) (string, string, error) {
	self := c.cfg.Self
	b, err := os.ReadFile(c.runbookPath(name))
	if err != nil {
		return "", self, fmt.Errorf("there is no runbook called %q", name)
	}
	calls, err := c.runbookCalls(string(b))
	if err != nil {
		return "", self, fmt.Errorf("runbook %s: %w", name, err)
	}
	var out strings.Builder
	for _, call := range calls {
		o, node, err := c.step(ctx, call, 1)
		fmt.Fprintf(&out, "[%s on %s]\n%s\n", call, node, o)
		if err != nil {
			return out.String(), self, fmt.Errorf("runbook %s stopped at %s: %w", name, call.Name, err)
		}
	}
	return strings.TrimRight(out.String(), "\n"), self, nil
}

// ---- tracking and reporting ----

// Focus returns the computer currently selected.
func (c *Chief) Focus() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.focus
}

// Jobs returns up to limit jobs, newest first.
func (c *Chief) Jobs(limit int) []Job {
	c.mu.Lock()
	defer c.mu.Unlock()
	ids := c.idsLocked()
	var out []Job
	for i := len(ids) - 1; i >= 0 && len(out) < limit; i-- {
		out = append(out, c.jobs[ids[i]].clone())
	}
	return out
}

// Job returns one job by id.
func (c *Chief) Job(id string) (Job, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	j, ok := c.jobs[id]
	if !ok {
		return Job{}, false
	}
	return j.clone(), true
}

func (c *Chief) idsLocked() []string {
	ids := make([]string, 0, len(c.jobs))
	for id := range c.jobs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// Status is the chief's report.
type Status struct {
	Focus  string         `json:"focus"`
	Armed  bool           `json:"armed"`
	Nodes  []NodeStatus   `json:"nodes"`
	Counts map[string]int `json:"jobs"`
	Ledger uint64         `json:"ledger_records"`
	Head   string         `json:"ledger_head"`
}

// NodeStatus is one tracked device in a report.
type NodeStatus struct {
	broker.Node
	Up bool `json:"up"`
}

// Report gathers the current state of devices and jobs.
func (c *Chief) Report() Status {
	now := c.Now()
	s := Status{Focus: c.Focus(), Armed: c.Armed(), Counts: map[string]int{}}
	for _, n := range c.Reg.Snapshot() {
		s.Nodes = append(s.Nodes, NodeStatus{n, c.Reg.Alive(n, now)})
	}
	c.mu.Lock()
	for _, j := range c.jobs {
		s.Counts[j.State]++
	}
	c.mu.Unlock()
	s.Ledger, s.Head = c.Ledger.Head()
	return s
}

// StatusText is the report as the chief would say it.
func (c *Chief) StatusText() string {
	s := c.Report()
	now := c.Now()
	var b strings.Builder
	focus := s.Focus
	if focus == "" {
		focus = "none"
	}
	mode := "dry run (not armed)"
	if s.Armed {
		mode = "ARMED"
	}
	fmt.Fprintf(&b, "attention: %s   mode: %s\n", focus, mode)
	for _, n := range s.Nodes {
		state := "up  "
		if !n.Up {
			state = "DOWN"
		}
		seen := "never seen"
		if n.Tier == broker.Self {
			seen = "this node"
		} else if !n.Seen.IsZero() {
			seen = "seen " + now.Sub(n.Seen).Round(time.Second).String() + " ago"
		}
		fmt.Fprintf(&b, "  %-10s %-6s %s  %-16s %s\n", n.Name, n.Tier, state, seen, strings.Join(n.Caps, " "))
	}
	keys := make([]string, 0, len(s.Counts))
	for k := range s.Counts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = strconv.Itoa(s.Counts[k]) + " " + k
	}
	if len(parts) == 0 {
		parts = []string{"none yet"}
	}
	fmt.Fprintf(&b, "jobs: %s\nledger: %d records", strings.Join(parts, ", "), s.Ledger)
	return b.String()
}

// ---- bookkeeping ----

// act writes an action to the ledger before it happens. If the ledger
// cannot be written the action must not proceed.
func (c *Chief) act(data map[string]any) error {
	if err := c.Ledger.Append("act", data); err != nil {
		return fmt.Errorf("audit ledger unavailable, refusing to act: %w", err)
	}
	return nil
}

func (c *Chief) note(kind string, data map[string]any) {
	if err := c.Ledger.Append(kind, data); err != nil {
		fmt.Fprintf(os.Stderr, "cos: ledger: %v\n", err)
	}
}

func (c *Chief) set(job *Job, f func(*Job)) {
	c.mu.Lock()
	f(job)
	snap := job.clone()
	c.mu.Unlock()
	writeJSON(filepath.Join(c.cfg.StateDir, "jobs", snap.ID+".json"), snap)
}

func (c *Chief) snapshot(job *Job) Job {
	c.mu.Lock()
	defer c.mu.Unlock()
	return job.clone()
}

func (c *Chief) saveState() {
	c.mu.Lock()
	s := saved{Seq: c.seq, Focus: c.focus, Notes: map[string]string{}, Approved: map[string]string{}}
	for k, v := range c.notes {
		s.Notes[k] = v
	}
	for k, v := range c.approved {
		s.Approved[k] = v
	}
	c.mu.Unlock()
	writeJSON(filepath.Join(c.cfg.StateDir, "chief.json"), s)
}

func writeJSON(path string, v any) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return
	}
	tmp := path + ".tmp"
	if os.WriteFile(tmp, append(b, '\n'), 0o640) == nil {
		os.Rename(tmp, path)
	}
}

func atLogin(screen string) bool {
	lines := strings.Split(strings.TrimSpace(screen), "\n")
	last := strings.ToLower(strings.TrimSpace(lines[len(lines)-1]))
	return strings.HasSuffix(last, "login:") || strings.HasSuffix(last, "password:")
}

// tail returns the last n non-blank lines: OCR output is full of empty lines
// and the bottom of a console is where the answer is.
func tail(s string, n int) string {
	var lines []string
	for _, l := range strings.Split(s, "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, strings.TrimRight(l, " \r"))
		}
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

func head(s string, n int) string {
	lines := strings.Split(strings.TrimLeft(s, "\n"), "\n")
	if len(lines) > n {
		lines = append(lines[:n], "…")
	}
	return strings.Join(lines, "\n")
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
