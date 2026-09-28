package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/CoolEngOrg/issueops/internal/agent"
	"github.com/CoolEngOrg/issueops/internal/authz"
	"github.com/CoolEngOrg/issueops/internal/budget"
	"github.com/CoolEngOrg/issueops/internal/copilot"
	"github.com/CoolEngOrg/issueops/internal/engine"
	"github.com/CoolEngOrg/issueops/internal/foundry"
	"github.com/CoolEngOrg/issueops/internal/ghapi"
	"github.com/CoolEngOrg/issueops/internal/issueform"
	"github.com/CoolEngOrg/issueops/internal/state"
)

// ---- scenario format ----------------------------------------------------------
//
// A scenario drives a request through its whole lifecycle offline: the issue
// body is rendered from values with the real issue form, parsed back with the
// Go parser, and every step goes through the same engine calls the workflows
// make (Intake, Handle, Gate, Finish). Execution steps run the real
// type-specific planners against fixtures instead of GitHub/Azure.
//
// Scenarios double as end-to-end tests: `expect` blocks are asserted and
// `go test ./cmd/issueops` runs every examples/*/scenario*.json.

type simDirectory struct {
	// Teams maps "team-slug" (in the registry organization) or "org/team-slug" to logins.
	Teams map[string][]string `json:"teams"`
	// Owners are organization owners; Members are plain members.
	Owners  []string `json:"owners"`
	Members []string `json:"members"`
	// CustomRoles maps an organization role name to logins.
	CustomRoles map[string][]string `json:"custom_roles"`
	// RepoPermissions maps "repo" or "owner/repo" to login -> permission
	// (admin | maintain | write | triage | read).
	RepoPermissions map[string]map[string]string `json:"repo_permissions"`
}

func (d simDirectory) static(org string) *authz.Static {
	s := &authz.Static{Teams: map[string][]string{}, OrgRoles: map[string]string{}, Custom: map[string][]string{}, RepoPerms: map[string]string{}}
	for t, users := range d.Teams {
		key := t
		if !strings.Contains(t, "/") {
			key = org + "/" + t
		}
		s.Teams[strings.ToLower(key)] = users
	}
	for _, u := range d.Members {
		s.OrgRoles[strings.ToLower(u)] = "member"
	}
	for _, u := range d.Owners {
		s.OrgRoles[strings.ToLower(u)] = "admin"
	}
	for r, users := range d.CustomRoles {
		s.Custom[r] = users
	}
	for repo, perms := range d.RepoPermissions {
		full := repo
		if !strings.Contains(repo, "/") {
			full = org + "/" + repo
		}
		for u, p := range perms {
			s.RepoPerms[strings.ToLower(full+"|"+u)] = p
		}
	}
	return s
}

type simExpect struct {
	Action      string   `json:"action,omitempty"`
	Phase       string   `json:"phase,omitempty"`
	Execute     *bool    `json:"execute,omitempty"`
	Environment string   `json:"environment,omitempty"`
	Rejected    *bool    `json:"rejected,omitempty"`
	Escalated   *bool    `json:"escalated,omitempty"`
	Close       string   `json:"close,omitempty"`
	GateRefused *bool    `json:"gate_refused,omitempty"`
	Contains    []string `json:"contains,omitempty"`
	Labels      []string `json:"labels,omitempty"`
}

type simAzure struct {
	Models            string `json:"models"`
	Usage             string `json:"usage"`
	Deployments       string `json:"deployments"`
	Endpoint          string `json:"endpoint,omitempty"`
	ProvisioningState string `json:"provisioning_state,omitempty"`
}

type simExecution struct {
	// Status forces an outcome (completed | failed). Agent runs take their
	// status from the runner result instead.
	Status string `json:"status,omitempty"`
	Error  string `json:"error,omitempty"`
	// copilot-usage-report: fixture directory (see copilot.FixtureSource).
	Fixtures string `json:"fixtures,omitempty"`
	// copilot-budget-request: an existing budget returned by the list call.
	ExistingBudget *ghapi.Budget `json:"existing_budget,omitempty"`
	// foundry-model-deployment: az CLI JSON outputs.
	Azure *simAzure `json:"azure,omitempty"`
	// agentic-task-request: runner log file, or a result encoded into a log.
	RunnerLog   string                `json:"runner_log,omitempty"`
	AgentResult *agent.Result         `json:"agent_result,omitempty"`
	Dispatch    *agent.DispatchResult `json:"dispatch,omitempty"`
}

type simStep struct {
	Do        string         `json:"do"` // open | edit | comment | execute | reopen
	User      string         `json:"user,omitempty"`
	Body      string         `json:"body,omitempty"`
	Values    map[string]any `json:"values,omitempty"`
	Note      string         `json:"note,omitempty"`
	After     string         `json:"after,omitempty"` // clock advance before the step (Go duration)
	Execution *simExecution  `json:"execution,omitempty"`
	Expect    *simExpect     `json:"expect,omitempty"`
}

type scenario struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Type        string         `json:"type"`
	Start       string         `json:"start"`
	Issue       simIssue       `json:"issue"`
	Values      map[string]any `json:"values"`
	Directory   simDirectory   `json:"directory"`
	Steps       []simStep      `json:"steps"`
}

type simIssue struct {
	Number    int    `json:"number"`
	Title     string `json:"title"`
	Requestor string `json:"requestor"`
}

// ---- simulator ------------------------------------------------------------------

type simDecision struct {
	Step     int              `json:"step"`
	Do       string           `json:"do"`
	User     string           `json:"user,omitempty"`
	Kind     string           `json:"kind"` // intake | command | gate | finish
	Decision *engine.Decision `json:"decision,omitempty"`
	Refused  string           `json:"refused,omitempty"`
}

type sim struct {
	a         *app
	sc        *scenario
	base, out string
	issue     *ghapi.Issue
	comments  []ghapi.Comment
	values    map[string]any
	form      *issueform.Form
	clock     time.Time
	nextID    int64
	runSeq    int
	log       strings.Builder
	decisions []simDecision
	failures  []string
}

func cmdSimulate(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("simulate", flag.ExitOnError)
	var c common
	c.register(fs)
	scenarioPath := fs.String("scenario", "", "scenario JSON file")
	_ = fs.Parse(args)
	c.offline = true
	c.runURL = "" // simulated runs get their own URLs; never the invoking CI run's
	if c.botLogin == "" {
		c.botLogin = "coolengorg-issueops[bot]"
	}
	res, err := runScenario(ctx, &c, *scenarioPath)
	if err != nil {
		return err
	}
	fmt.Printf("scenario %q: %d steps, final phase %s -> %s\n", res.sc.Name, len(res.sc.Steps), phaseFromLabels(res.issue), res.out)
	if len(res.failures) > 0 {
		return fmt.Errorf("%d expectation(s) failed:\n  %s", len(res.failures), strings.Join(res.failures, "\n  "))
	}
	return nil
}

func runScenario(ctx context.Context, c *common, path string) (*sim, error) {
	if path == "" {
		return nil, errors.New("--scenario is required")
	}
	var sc scenario
	if err := readJSON(path, &sc); err != nil {
		return nil, fmt.Errorf("scenario %s: %w", path, err)
	}
	a, err := c.load()
	if err != nil {
		return nil, err
	}
	rt, err := a.reg.ByID(sc.Type)
	if err != nil {
		return nil, err
	}
	form, err := issueform.LoadFormForTemplate(a.formsDir, rt.Template)
	if err != nil {
		return nil, err
	}
	start, err := time.Parse(time.RFC3339, sc.Start)
	if err != nil {
		return nil, fmt.Errorf("scenario start: %w", err)
	}
	if len(sc.Steps) == 0 || sc.Steps[0].Do != "open" {
		return nil, errors.New("scenario: the first step must be \"open\"")
	}
	if sc.Values == nil {
		sc.Values = map[string]any{}
	}
	a.runURL = ""
	a.dir = sc.Directory.static(a.reg.Organization)
	s := &sim{a: a, sc: &sc, base: filepath.Dir(path), out: a.outDir, form: form, clock: start.UTC(), nextID: 1000, values: sc.Values}
	s.header(rt.Name)
	for i, st := range sc.Steps {
		d := 3 * time.Minute
		if st.After != "" {
			if d, err = time.ParseDuration(st.After); err != nil {
				return nil, fmt.Errorf("step %d: after: %w", i+1, err)
			}
		}
		s.clock = s.clock.Add(d)
		a.now = s.clock
		if err := s.step(ctx, i+1, st); err != nil {
			return nil, fmt.Errorf("step %d (%s): %w", i+1, st.Do, err)
		}
	}
	return s, s.finish()
}

func (s *sim) header(typeName string) {
	sc := s.sc
	fmt.Fprintf(&s.log, "# Scenario: %s\n\n", sc.Name)
	if sc.Description != "" {
		fmt.Fprintf(&s.log, "%s\n\n", sc.Description)
	}
	fmt.Fprintf(&s.log, "> Generated by `issueops simulate` — every decision below was produced by the same engine calls the workflows make, against a static directory and fixtures. Timestamps are simulated.\n\n")
	fmt.Fprintf(&s.log, "| | |\n|---|---|\n| Request type | %s (`%s`) |\n| Issue | %s/%s#%d |\n| Requestor | `%s` |\n| Started | %s |\n\n",
		typeName, sc.Type, s.a.reg.Organization, s.a.reg.Repository, sc.Issue.Number, sc.Issue.Requestor, s.clock.Format(time.RFC1123))
	var teams []string
	for t, u := range sc.Directory.Teams {
		teams = append(teams, fmt.Sprintf("`%s`: %s", t, codeList(u)))
	}
	sort.Strings(teams)
	if len(teams) > 0 {
		fmt.Fprintf(&s.log, "**Directory (simulated):** teams %s", strings.Join(teams, "; "))
		var repos []string
		for r, perms := range sc.Directory.RepoPermissions {
			var ps []string
			for u, p := range perms {
				ps = append(ps, fmt.Sprintf("`%s`=%s", u, p))
			}
			sort.Strings(ps)
			repos = append(repos, fmt.Sprintf("`%s` (%s)", r, strings.Join(ps, ", ")))
		}
		sort.Strings(repos)
		if len(repos) > 0 {
			fmt.Fprintf(&s.log, "; repository permissions %s", strings.Join(repos, "; "))
		}
		if len(sc.Directory.Owners) > 0 {
			fmt.Fprintf(&s.log, "; org owners %s", codeList(sc.Directory.Owners))
		}
		s.log.WriteString(".\n\n")
	}
}

func codeList(v []string) string {
	out := make([]string, len(v))
	for i, x := range v {
		out[i] = "`" + x + "`"
	}
	return strings.Join(out, ", ")
}

func (s *sim) stepHeading(n int, title string) {
	fmt.Fprintf(&s.log, "---\n\n## %d. %s  <sub>%s</sub>\n\n", n, title, s.clock.Format("Mon 15:04 MST"))
}

// body renders the issue body from values and parses it back, exactly like
// GitHub (form -> Markdown) and issue-ops/parser (Markdown -> JSON).
func (s *sim) body() (string, issueform.Values, error) {
	body, err := issueform.Render(s.form, issueform.Values(s.values))
	if err != nil {
		return "", nil, err
	}
	parsed, err := issueform.Parse(body, s.form)
	return body, parsed, err
}

func (s *sim) step(ctx context.Context, n int, st simStep) error {
	switch st.Do {
	case "open":
		body, parsed, err := s.body()
		if err != nil {
			return err
		}
		title := s.sc.Issue.Title
		if title == "" {
			title = s.form.Title
		}
		s.issue = &ghapi.Issue{Number: s.sc.Issue.Number, Title: title, Body: body, State: "open",
			User:      ghapi.User{Login: s.sc.Issue.Requestor, Type: "User"},
			HTMLURL:   fmt.Sprintf("https://github.com/%s/%s/issues/%d", s.a.reg.Organization, s.a.reg.Repository, s.sc.Issue.Number),
			CreatedAt: s.clock, UpdatedAt: s.clock}
		for _, l := range s.form.Labels {
			s.issue.Labels = append(s.issue.Labels, ghapi.Label{Name: l})
		}
		if err := os.WriteFile(filepath.Join(s.out, "issue-body.md"), []byte(body), 0o644); err != nil {
			return err
		}
		if err := writeJSON(filepath.Join(s.out, "parsed.json"), parsed); err != nil {
			return err
		}
		s.stepHeading(n, fmt.Sprintf("`%s` opens the issue from the form", s.sc.Issue.Requestor))
		s.note(st.Note)
		fmt.Fprintf(&s.log, "<details><summary>Issue body (as GitHub renders the form)</summary>\n\n%s\n\n</details>\n\n", fenceMD(body))
		return s.intake(ctx, n, st, "opened", parsed)

	case "edit", "reopen":
		if s.issue == nil {
			return errors.New("issue not opened yet")
		}
		action := "reopened"
		if st.Do == "edit" {
			action = "edited"
			for k, v := range st.Values {
				s.values[k] = v
			}
		}
		body, parsed, err := s.body()
		if err != nil {
			return err
		}
		s.issue.Body, s.issue.UpdatedAt = body, s.clock
		if action == "reopened" {
			s.issue.State, s.issue.ClosedAt = "open", time.Time{}
		}
		who := st.User
		if who == "" {
			who = s.sc.Issue.Requestor
		}
		if action == "edited" {
			var keys []string
			for k := range st.Values {
				keys = append(keys, "`"+k+"`")
			}
			sort.Strings(keys)
			s.stepHeading(n, fmt.Sprintf("`%s` edits the issue (%s)", who, strings.Join(keys, ", ")))
		} else {
			s.stepHeading(n, fmt.Sprintf("`%s` reopens the issue", who))
		}
		s.note(st.Note)
		return s.intake(ctx, n, st, action, parsed)

	case "comment":
		if s.issue == nil {
			return errors.New("issue not opened yet")
		}
		s.nextID++
		cm := ghapi.Comment{ID: s.nextID, Body: st.Body, User: ghapi.User{Login: st.User, Type: "User"}, CreatedAt: s.clock, UpdatedAt: s.clock}
		s.comments = append(s.comments, cm)
		s.stepHeading(n, fmt.Sprintf("`%s` comments", st.User))
		s.note(st.Note)
		fmt.Fprintf(&s.log, "%s\n\n", fenceMD(st.Body))
		cmd := state.ParseCommand(st.Body)
		if cmd == nil {
			s.log.WriteString("_Not a command: the router ignores it._\n\n")
			return nil
		}
		_, parsed, err := s.body()
		if err != nil {
			return err
		}
		r, err := s.load(ctx, parsed)
		if err != nil {
			return err
		}
		d, err := r.Handle(ctx, &state.CommandEvent{Command: cmd, User: st.User, CommentID: cm.ID, At: cm.CreatedAt})
		if err != nil {
			return err
		}
		s.decisions = append(s.decisions, simDecision{Step: n, Do: st.Do, User: st.User, Kind: "command", Decision: d})
		s.apply(d)
		s.describe(d)
		s.check(n, st.Expect, d, "")
		if d.Execute {
			fmt.Fprintf(&s.log, "➡️ The router dispatches `%s` (environment `%s`) for issue #%d.\n\n", d.Workflow, d.Environment, s.issue.Number)
		}
		return nil

	case "execute":
		return s.execute(ctx, n, st)
	}
	return fmt.Errorf("unknown step %q", st.Do)
}

func (s *sim) note(n string) {
	if n != "" {
		fmt.Fprintf(&s.log, "_%s_\n\n", n)
	}
}

func (s *sim) load(ctx context.Context, parsed issueform.Values) (*engine.Request, error) {
	return s.a.env().Load(ctx, s.issue, s.comments, parsed, nil)
}

func (s *sim) intake(ctx context.Context, n int, st simStep, action string, parsed issueform.Values) error {
	r, err := s.load(ctx, parsed)
	if err != nil {
		return err
	}
	d, err := r.Intake(ctx, action)
	if err != nil {
		return err
	}
	s.decisions = append(s.decisions, simDecision{Step: n, Do: st.Do, User: s.sc.Issue.Requestor, Kind: "intake", Decision: d})
	s.apply(d)
	s.describe(d)
	s.check(n, st.Expect, d, "")
	return nil
}

// apply mirrors what the workflows do with a decision: create or update the
// comment (peter-evans/find-comment + create-or-update-comment), project
// labels (issue-ops/labeler) and close the issue.
func (s *sim) apply(d *engine.Decision) {
	if d.Comment != nil {
		updated := false
		if d.Comment.UpdatePrefix != "" {
			for i := len(s.comments) - 1; i >= 0; i-- {
				c := &s.comments[i]
				if c.User.Login == s.a.botLogin && strings.HasPrefix(c.Body, d.Comment.UpdatePrefix) {
					c.Body, c.UpdatedAt, updated = d.Comment.Body, s.clock, true
					break
				}
			}
		}
		if !updated {
			s.nextID++
			s.comments = append(s.comments, ghapi.Comment{ID: s.nextID, Body: d.Comment.Body,
				User: ghapi.User{Login: s.a.botLogin, Type: "Bot"}, CreatedAt: s.clock, UpdatedAt: s.clock})
		}
	}
	remove := map[string]bool{}
	for _, l := range d.LabelsRemove {
		remove[l] = true
	}
	var labels []ghapi.Label
	have := map[string]bool{}
	for _, l := range s.issue.Labels {
		if !remove[l.Name] && !have[l.Name] {
			labels = append(labels, l)
			have[l.Name] = true
		}
	}
	for _, l := range d.LabelsAdd {
		if !have[l] {
			labels = append(labels, ghapi.Label{Name: l})
			have[l] = true
		}
	}
	s.issue.Labels = labels
	if d.Close != "" {
		s.issue.State, s.issue.ClosedAt = "closed", s.clock
	}
}

func (s *sim) stateLabels() []string {
	var out []string
	for _, l := range s.issue.LabelNames() {
		if strings.HasPrefix(l, "issueops:") && !isTypeLabel(s.a, l) {
			out = append(out, l)
		}
	}
	sort.Strings(out)
	return out
}

func isTypeLabel(a *app, l string) bool {
	for _, rt := range a.reg.RequestTypes {
		if rt.Label == l {
			return true
		}
	}
	return false
}

func (s *sim) describe(d *engine.Decision) {
	fmt.Fprintf(&s.log, "**Decision:** `%s` · phase `%s` · labels %s", d.Action, d.Phase, codeList(s.stateLabels()))
	if d.Execute {
		fmt.Fprintf(&s.log, " · execute in environment `%s`", d.Environment)
	}
	if d.Close != "" {
		fmt.Fprintf(&s.log, " · issue closed as `%s`", d.Close)
	}
	if d.Rejected {
		s.log.WriteString(" · command rejected")
	}
	s.log.WriteString("\n\n")
	if d.Comment == nil {
		return
	}
	marker, body, _ := strings.Cut(d.Comment.Body, "\n")
	verb := "posts"
	if d.Comment.UpdatePrefix != "" {
		verb = "creates or updates"
	}
	fmt.Fprintf(&s.log, "IssueOps bot %s a comment (`%s`):\n\n`%s`\n\n", verb, d.Comment.Template, marker)
	for _, l := range strings.Split(strings.TrimRight(body, "\n"), "\n") {
		s.log.WriteString(strings.TrimRight("> "+l, " ") + "\n")
	}
	s.log.WriteString("\n")
}

func (s *sim) check(step int, e *simExpect, d *engine.Decision, refused string) {
	if e == nil {
		return
	}
	fail := func(format string, a ...any) {
		s.failures = append(s.failures, fmt.Sprintf("step %d: ", step)+fmt.Sprintf(format, a...))
	}
	if e.GateRefused != nil && *e.GateRefused != (refused != "") {
		fail("gate refused = %v, want %v (%s)", refused != "", *e.GateRefused, refused)
	}
	if d == nil {
		return
	}
	if e.Action != "" && d.Action != e.Action {
		fail("action %q, want %q", d.Action, e.Action)
	}
	if e.Phase != "" && d.Phase != e.Phase {
		fail("phase %q, want %q", d.Phase, e.Phase)
	}
	if e.Execute != nil && d.Execute != *e.Execute {
		fail("execute %v, want %v", d.Execute, *e.Execute)
	}
	if e.Environment != "" && d.Environment != e.Environment {
		fail("environment %q, want %q", d.Environment, e.Environment)
	}
	if e.Rejected != nil && d.Rejected != *e.Rejected {
		fail("rejected %v, want %v (%s)", d.Rejected, *e.Rejected, d.Reason)
	}
	if e.Escalated != nil && d.Escalated != *e.Escalated {
		fail("escalated %v, want %v", d.Escalated, *e.Escalated)
	}
	if e.Close != "" && d.Close != e.Close {
		fail("close %q, want %q", d.Close, e.Close)
	}
	body := ""
	if d.Comment != nil {
		body = d.Comment.Body
	}
	for _, want := range e.Contains {
		if !strings.Contains(body, want) {
			fail("comment does not contain %q", want)
		}
	}
	have := map[string]bool{}
	for _, l := range s.issue.LabelNames() {
		have[l] = true
	}
	for _, l := range e.Labels {
		if !have[l] {
			fail("issue lacks label %q (has %v)", l, s.issue.LabelNames())
		}
	}
}

// ---- execution ------------------------------------------------------------------

func (s *sim) execute(ctx context.Context, n int, st simStep) error {
	if s.issue == nil {
		return errors.New("issue not opened yet")
	}
	s.runSeq++
	s.a.runURL = fmt.Sprintf("https://github.com/%s/%s/actions/runs/%d", s.a.reg.Organization, s.a.reg.Repository, 9100000000+int64(s.sc.Issue.Number)*100+int64(s.runSeq))
	defer func() { s.a.runURL = "" }()
	rt, _ := s.a.reg.ByID(s.sc.Type)
	s.stepHeading(n, fmt.Sprintf("`%s` runs (gate → execute → finish)", rt.Execute.Workflow))
	s.note(st.Note)

	_, parsed, err := s.body()
	if err != nil {
		return err
	}
	r, err := s.load(ctx, parsed)
	if err != nil {
		return err
	}
	gd, err := r.Gate(ctx, false)
	var ge *engine.GateError
	if errors.As(err, &ge) {
		s.decisions = append(s.decisions, simDecision{Step: n, Do: st.Do, Kind: "gate", Refused: ge.Reason})
		fmt.Fprintf(&s.log, "⛔ **Gate refused:** %s\n\nThe execution job never starts; nothing is changed.\n\n", ge.Reason)
		s.check(n, st.Expect, nil, ge.Reason)
		return nil
	}
	if err != nil {
		return err
	}
	s.decisions = append(s.decisions, simDecision{Step: n, Do: st.Do, Kind: "gate", Decision: gd})
	s.log.WriteString("**Gate job** (`issueops gate`):\n\n")
	s.apply(gd)
	s.describe(gd)
	digest := gd.Digest

	ex := st.Execution
	if ex == nil {
		ex = &simExecution{}
	}
	dir := filepath.Join(s.out, "execution")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	// The execution job re-loads the request and enforces the gate's digest.
	r, err = s.load(ctx, parsed)
	if err != nil {
		return err
	}
	if r.Snap.CurrentDigest != digest {
		return fmt.Errorf("digest drift between gate and execution")
	}
	status, errMsg, files, err := s.run(ctx, r, ex, dir)
	if err != nil {
		return err
	}
	if ex.Status != "" && s.sc.Type != "agentic-task-request" {
		status = ex.Status
	}
	if ex.Error != "" {
		errMsg = ex.Error
	}
	fmt.Fprintf(&s.log, "**Execution job** (environment `%s`, digest `%s`): status `%s`", gd.Environment, state.Short(digest), status)
	if errMsg != "" {
		fmt.Fprintf(&s.log, " — %s", errMsg)
	}
	s.log.WriteString("\n\n")
	if len(files) > 0 {
		s.log.WriteString("Artifacts produced:\n\n")
		for _, f := range files {
			rel, _ := filepath.Rel(s.out, f)
			fmt.Fprintf(&s.log, "- [`%s`](%s)\n", rel, filepath.ToSlash(rel))
		}
		s.log.WriteString("\n")
	}

	outcome := filepath.Join(dir, fmt.Sprintf("outcome-%d.json", s.runSeq))
	var result any
	if _, statErr := os.Stat(outcome); statErr == nil {
		if result, err = loadResult(s.sc.Type, outcome, status); err != nil {
			return err
		}
	}
	r, err = s.load(ctx, parsed)
	if err != nil {
		return err
	}
	fd, err := r.Finish(status, result, errMsg)
	if err != nil {
		return err
	}
	s.decisions = append(s.decisions, simDecision{Step: n, Do: st.Do, Kind: "finish", Decision: fd})
	s.log.WriteString("**Report job** (`issueops finish`):\n\n")
	s.apply(fd)
	s.describe(fd)
	s.check(n, st.Expect, fd, "")
	return nil
}

func (s *sim) path(p string) string {
	if p == "" || filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(s.base, p)
}

// run executes the type-specific planner offline and writes the outcome file.
func (s *sim) run(ctx context.Context, r *engine.Request, ex *simExecution, dir string) (status, errMsg string, files []string, err error) {
	outcome := filepath.Join(dir, fmt.Sprintf("outcome-%d.json", s.runSeq))
	// werr keeps the first artifact write error; branches return it so a
	// failed write is never masked by a shadowed err.
	var werr error
	save := func(name string, v any) string {
		p := filepath.Join(dir, name)
		var e error
		if b, ok := v.([]byte); ok {
			e = os.WriteFile(p, b, 0o644)
		} else if str, ok := v.(string); ok {
			e = os.WriteFile(p, []byte(str), 0o644)
		} else {
			e = writeJSON(p, v)
		}
		if e != nil && werr == nil {
			werr = e
		}
		files = append(files, p)
		return p
	}
	switch s.sc.Type {
	case "copilot-budget-request":
		plan, errs := budget.BuildPlan(r.RC)
		if len(errs) > 0 {
			return "failed", strings.Join(errs, "; "), nil, nil
		}
		save("budget-plan.json", map[string]any{"plan": plan, "create_payload": plan.CreatePayload(), "update_payload": plan.UpdatePayload()})
		res := simulateBudget(plan, ex.ExistingBudget, r.Snap.CurrentDigest)
		save(filepath.Base(outcome), budgetOutcome{Result: res, Plan: plan})
		return "completed", "", files, werr

	case "copilot-usage-report":
		sp, settings, errs, _ := copilot.BuildSpec(r.RC)
		if len(errs) > 0 {
			return "failed", strings.Join(errs, "; "), nil, nil
		}
		if ex.Fixtures == "" {
			return "", "", nil, errors.New("copilot-usage-report execution needs `fixtures`")
		}
		rep, err := copilot.Build(ctx, &copilot.FixtureSource{Dir: s.path(ex.Fixtures)}, sp, copilot.Options{
			Org: s.a.reg.Organization, Pricing: s.a.reg.Pricing, Settings: *settings, Now: s.a.now, WithReviews: true})
		if err != nil {
			return "", "", nil, err
		}
		arts, err := copilot.WriteAll(rep, filepath.Join(s.out, "report"), settings.AssetsHost)
		if err != nil {
			return "", "", nil, err
		}
		for _, p := range []string{arts.HTML, arts.JSON, arts.Prom} {
			if p != "" {
				files = append(files, p)
			}
		}
		files = append(files, arts.CSV...)
		save(filepath.Base(outcome), reportOutcome{Report: rep, Artifacts: arts, RetentionDays: 30, Grafana: sp.Grafana,
			ArtifactURL: s.a.runURL + "/artifacts/" + fmt.Sprint(4200000000+s.sc.Issue.Number)})
		return "completed", "", files, werr

	case "foundry-model-deployment":
		plan, errs := foundry.BuildPlan(r.RC)
		if len(errs) > 0 {
			return "failed", strings.Join(errs, "; "), nil, nil
		}
		params, perr := plan.BicepParameters()
		if perr != nil {
			return "", "", nil, perr
		}
		save("parameters.json", params)
		save("foundry-plan.json", plan)
		if ex.Azure == nil {
			return "", "", nil, errors.New("foundry execution needs `azure` fixtures")
		}
		var ms []foundry.AzModel
		var us []foundry.AzUsage
		var ds []foundry.AzDeployment
		for p, v := range map[string]any{ex.Azure.Models: &ms, ex.Azure.Usage: &us, ex.Azure.Deployments: &ds} {
			if err := readJSON(s.path(p), v); err != nil {
				return "", "", nil, fmt.Errorf("azure fixture %s: %w", p, err)
			}
		}
		strict := true
		var fs foundry.Settings
		if s.a.reg != nil && r.Type.DecodeSettings(&fs) == nil {
			strict = fs.QuotaStrict
		}
		pre := foundry.Preflight(plan, ms, us, ds, strict)
		save("preflight.json", pre)
		if !pre.OK {
			return "failed", "pre-flight failed: " + strings.Join(pre.Errors, " "), files, werr
		}
		o := foundryOutcome{Plan: plan, Preflight: pre, Endpoint: ex.Azure.Endpoint, ProvisioningState: ex.Azure.ProvisioningState}
		if o.Endpoint == "" {
			o.Endpoint = "https://" + plan.Account + ".openai.azure.com/"
		}
		if o.ProvisioningState == "" {
			o.ProvisioningState = "Succeeded"
		}
		save(filepath.Base(outcome), o)
		if !strings.EqualFold(o.ProvisioningState, "Succeeded") {
			return "failed", "deployment provisioning state is " + o.ProvisioningState, files, werr
		}
		return "completed", "", files, werr

	case "agentic-task-request":
		st, err := agent.LoadSettings(r.RC)
		if err != nil {
			return "", "", nil, err
		}
		if r.QA == nil || !r.QA.Complete {
			return "failed", "clarifying questions are not complete", nil, nil
		}
		sp := agent.BuildSpec(r.RC, r.QA, r.Snap.CurrentDigest, r.Issue.HTMLURL)
		be := st.Backends[sp.Backend]
		save("agent-spec.json", sp)
		switch sp.Backend {
		case agent.BackendCopilot:
			body, err := s.a.set.Render("agent.copilot-issue.md.tmpl", sp)
			if err != nil {
				return "", "", nil, err
			}
			instr, err := s.a.set.Render("agent.copilot-instructions.md.tmpl", sp)
			if err != nil {
				return "", "", nil, err
			}
			save("copilot-issue.md", body)
			save("copilot-custom-instructions.md", instr)
			d := ex.Dispatch
			if d == nil {
				d = &agent.DispatchResult{Backend: agent.BackendCopilot, Kind: "issue", URL: fmt.Sprintf("https://github.com/%s/issues/%d", sp.TargetRepo, 700+s.sc.Issue.Number),
					Reference: fmt.Sprintf("%s#%d", sp.TargetRepo, 700+s.sc.Issue.Number),
					Message:   "Issue created in the target repository and assigned to the Copilot cloud agent; it will open a draft pull request."}
			}
			save(filepath.Base(outcome), agentOutcome{Dispatch: d, Spec: sp})
			return "completed", "", files, werr
		case agent.BackendCatalog:
			entry := st.Catalog[sp.Catalog]
			inputs := map[string]string{"issueops_request": sp.RequestURL}
			for _, q := range r.QA.Questions {
				for in, qid := range entry.Inputs {
					if qid == q.ID {
						inputs[in] = q.Answer
					}
				}
			}
			save("workflow-dispatch-inputs.json", map[string]any{"workflow": entry.Workflow, "ref": "main", "inputs": inputs})
			d := ex.Dispatch
			if d == nil {
				d = &agent.DispatchResult{Backend: agent.BackendCatalog, Kind: "workflow", Workflow: entry.Workflow,
					URL:     fmt.Sprintf("https://github.com/%s/actions/workflows/%s", sp.TargetRepo, entry.Workflow),
					Message: "Agentic workflow dispatched on `main`."}
			}
			save(filepath.Base(outcome), agentOutcome{Dispatch: d, Spec: sp})
			return "completed", "", files, werr
		case agent.BackendAKS:
			manifest, jd, err := agent.RenderAKS(s.a.set, sp, be, r.Issue.Number)
			if err != nil {
				return "failed", err.Error(), files, nil
			}
			save(fmt.Sprintf("agent-job-%d.yaml", s.runSeq), manifest)
			var logText string
			switch {
			case ex.RunnerLog != "":
				b, err := os.ReadFile(s.path(ex.RunnerLog))
				if err != nil {
					return "", "", nil, err
				}
				logText = string(b)
			case ex.AgentResult != nil:
				logText = fmt.Sprintf("{\"level\":\"info\",\"msg\":\"job %s started\"}\n", jd.JobName) + agent.EncodeResult(ex.AgentResult)
			default:
				return "", "", nil, errors.New("aks execution needs `runner_log` or `agent_result`")
			}
			save(fmt.Sprintf("runner-%d.log", s.runSeq), logText)
			res, perr := agent.ParseResult(logText)
			if perr != nil {
				res = &agent.Result{Status: "failed", Error: perr.Error()}
			}
			save(filepath.Base(outcome), agentOutcome{Agent: res})
			return res.Status, res.Error, files, werr
		}
		return "", "", nil, fmt.Errorf("unknown backend %q", sp.Backend)
	}
	return "", "", nil, fmt.Errorf("no simulated executor for %s", s.sc.Type)
}

// simulateBudget mirrors budget.Apply without the API.
func simulateBudget(p *budget.Plan, existing *ghapi.Budget, digest string) *budget.Result {
	if existing != nil {
		before := *existing
		if budget.Unchanged(&before, p) {
			return &budget.Result{Action: "unchanged", Before: &before, After: &before}
		}
		after := before
		after.BudgetAmount, after.PreventFurtherUsage, after.ExpiresAt = float64(p.Amount), p.Prevent, p.ExpiresAt
		up := p.UpdatePayload()
		if up.BudgetAlerting != nil {
			after.BudgetAlerting = *up.BudgetAlerting
		}
		return &budget.Result{Action: "updated", Before: &before, After: &after, Update: &up}
	}
	cr := p.CreatePayload()
	after := &ghapi.Budget{ID: "bdg_" + state.Short(digest), BudgetType: p.BudgetType, BudgetProductSKU: p.SKU, BudgetScope: p.Scope,
		BudgetEntityName: p.EntityName, BudgetAmount: float64(p.Amount), PreventFurtherUsage: p.Prevent, BudgetAlerting: p.Alerting,
		User: p.User, ExpiresAt: p.ExpiresAt}
	return &budget.Result{Action: "created", After: after, Create: &cr}
}

func (s *sim) finish() error {
	fmt.Fprintf(&s.log, "---\n\n## Result\n\nFinal state: issue **%s**, labels %s, %d comments (%d from the IssueOps bot).\n",
		s.issue.State, codeList(s.issue.LabelNames()), len(s.comments), s.botComments())
	if len(s.failures) > 0 {
		fmt.Fprintf(&s.log, "\n**Expectation failures:**\n\n- %s\n", strings.Join(s.failures, "\n- "))
	}
	if err := os.WriteFile(filepath.Join(s.out, "transcript.md"), []byte(s.log.String()), 0o644); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(s.out, "decisions.json"), s.decisions); err != nil {
		return err
	}
	return writeJSON(filepath.Join(s.out, "timeline.json"), []opsIssue{{Issue: *s.issue, Comments: s.comments}})
}

func (s *sim) botComments() int {
	n := 0
	for _, c := range s.comments {
		if c.User.Login == s.a.botLogin {
			n++
		}
	}
	return n
}

// fenceMD wraps text in a fence longer than any backtick run inside it.
func fenceMD(s string) string {
	fence := "```"
	for strings.Contains(s, fence) {
		fence += "`"
	}
	return fence + "markdown\n" + strings.TrimRight(s, "\n") + "\n" + fence
}
