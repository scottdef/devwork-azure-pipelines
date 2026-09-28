// Package engine is the IssueOps state machine. It loads a request (type,
// parsed body, policy facts, approval plan, Q&A, timeline snapshot) and
// produces Decisions: which comment to post, which state label to project,
// whether to execute, in which GitHub environment, and whether to close.
//
// The engine never mutates GitHub itself; workflows apply decisions with the
// documented IssueOps actions (issue-ops/labeler, peter-evans/*-comment).
package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/CoolEngOrg/issueops/internal/agent"
	"github.com/CoolEngOrg/issueops/internal/authz"
	"github.com/CoolEngOrg/issueops/internal/budget"
	"github.com/CoolEngOrg/issueops/internal/config"
	"github.com/CoolEngOrg/issueops/internal/copilot"
	"github.com/CoolEngOrg/issueops/internal/foundry"
	"github.com/CoolEngOrg/issueops/internal/ghapi"
	"github.com/CoolEngOrg/issueops/internal/issueform"
	"github.com/CoolEngOrg/issueops/internal/policy"
	"github.com/CoolEngOrg/issueops/internal/qa"
	"github.com/CoolEngOrg/issueops/internal/request"
	"github.com/CoolEngOrg/issueops/internal/state"
	"github.com/CoolEngOrg/issueops/internal/tmpl"
)

// Env holds shared dependencies.
type Env struct {
	Registry  *config.Registry
	GH        *ghapi.Client // nil when offline
	Dir       authz.Directory
	BotLogin  string
	Templates *tmpl.Set
	Now       func() time.Time
	RunURL    string
	Handlers  map[string]request.Handler
}

// DefaultHandlers maps request types to their handlers.
func DefaultHandlers() map[string]request.Handler {
	return map[string]request.Handler{
		"copilot-budget-request":   budget.Handler{},
		"copilot-usage-report":     copilot.Handler{},
		"foundry-model-deployment": foundry.Handler{},
		"agentic-task-request":     agent.Handler{},
	}
}

// Request is a fully loaded request.
type Request struct {
	Env       *Env
	Type      *config.RequestType
	Handler   request.Handler
	Issue     *ghapi.Issue
	Values    issueform.Values
	RC        *request.Context
	Facts     policy.Facts
	Plan      *policy.Plan
	Snap      *state.Snapshot
	Questions []qa.Question
	QA        *qa.Status
	Subject   authz.Subject
	Errors    []string
	Warnings  []string
}

// Valid reports whether the request passed all validation.
func (r *Request) Valid() bool { return len(r.Errors) == 0 }

func (e *Env) now() time.Time {
	if e.Now != nil {
		return e.Now()
	}
	return time.Now().UTC()
}

// Load builds a Request. formErrors are errors reported by issue-ops/validator.
func (e *Env) Load(ctx context.Context, issue *ghapi.Issue, comments []ghapi.Comment, values issueform.Values, formErrors []string) (*Request, error) {
	rt, err := e.Registry.ResolveLabels(issue.LabelNames())
	if err != nil {
		return nil, err
	}
	h := e.Handlers[rt.ID]
	if h == nil {
		return nil, fmt.Errorf("engine: no handler for %s", rt.ID)
	}
	rc := &request.Context{Registry: e.Registry, Type: rt, Values: values, Requestor: issue.User.Login, Issue: issue.Number, GH: e.GH, Now: e.now()}
	r := &Request{Env: e, Type: rt, Handler: h, Issue: issue, Values: values, RC: rc}
	r.Errors = append(r.Errors, formErrors...)
	errs, warns := h.Validate(ctx, rc)
	r.Errors = append(r.Errors, errs...)
	r.Warnings = append(r.Warnings, warns...)

	r.Facts = policy.FactsFromValues(values)
	extra, err := h.Facts(rc)
	if err != nil {
		return nil, err
	}
	r.Facts.Merge(extra).Merge(map[string]any{"$requestor": issue.User.Login})
	if r.Plan, err = policy.BuildPlan(rt, r.Facts); err != nil {
		return nil, err
	}

	r.Snap = state.Build(rt.ID, issue, comments, e.BotLogin)
	var answers map[string]string
	if rt.QA {
		s, err := agent.LoadSettings(rc)
		if err != nil {
			return nil, err
		}
		var dynamic []qa.Question
		for _, m := range r.Snap.MarkersOf(state.EventQuestions) {
			if raw, ok := m.Data["dynamic"]; ok {
				b, _ := json.Marshal(raw)
				var qs []qa.Question
				if json.Unmarshal(b, &qs) == nil {
					dynamic = append(dynamic, qs...)
				}
			}
		}
		qs, qerr := agent.Questions(s, values.String("agent_backend"), values.String("agent_catalog_workflow"), dynamic)
		if qerr == nil {
			r.Questions = qs
			var texts []string
			for _, c := range r.Snap.AnswerComments {
				texts = append(texts, c.Args+"\n"+c.Body)
			}
			r.QA = qa.Collect(qs, texts)
			answers = r.QA.Answers
		} else if r.Valid() {
			r.Errors = append(r.Errors, "No clarifying questions are configured for this backend/catalog combination.")
		}
	}
	r.Snap.Finalize(state.Digest(rt.ID, issue.Body, answers))

	target, fieldUsers := h.Subject(rc)
	r.Subject = authz.Subject{Org: e.Registry.Organization, IssueOpsRepo: e.Registry.Repository, Requestor: issue.User.Login, TargetRepo: target, FieldUsers: fieldUsers}
	return r, nil
}

// CheckRequestor verifies the requestor may submit this request type.
func (r *Request) CheckRequestor(ctx context.Context) (bool, string, error) {
	rq := r.Type.Requestors
	if rq == nil || rq.AnyOf.IsEmpty() || r.Env.Dir == nil {
		return true, "", nil
	}
	sub := r.Subject
	m, err := authz.Evaluate(ctx, r.Env.Dir, rq.AnyOf, r.Issue.User.Login, sub)
	if err != nil {
		return false, "", err
	}
	if !m.OK {
		return false, "You are not eligible to request this: requests are limited to " + state.Describe(r.Env.Registry.Organization, rq.AnyOf) + ".", nil
	}
	return true, m.Reason, nil
}

// Phase is the phase to project, combining the timeline with live validation.
func (r *Request) Phase() string {
	s := r.Snap
	if state.IsTerminal(s.Phase) {
		return s.Phase
	}
	switch s.Phase {
	case state.PhaseExecuting:
		return state.PhaseExecuting
	case state.PhaseApproved:
		if s.ApprovedValid() && r.Valid() {
			return state.PhaseApproved
		}
	case state.PhaseSubmitted, state.PhaseFailed:
		if s.SubmitValid() && r.Valid() {
			return s.Phase
		}
	}
	if !r.Valid() {
		return state.PhaseInvalid
	}
	if r.Type.QA && (r.QA == nil || !r.QA.Complete) {
		return state.PhaseAwaiting
	}
	return state.PhaseValidated
}

// Escalated reports whether policy escalations apply.
func (r *Request) Escalated() bool { return len(r.Plan.Escalations) > 0 }

// ApprovalOptions returns the policy switches.
func (r *Request) ApprovalOptions() state.ApprovalOptions {
	return state.ApprovalOptions{AllowSelfApproval: r.Type.Approval.AllowSelfApproval, DistinctApprovers: r.Type.Approval.DistinctApprovers}
}

// Approvals evaluates current votes.
func (r *Request) Approvals(ctx context.Context) (*state.ApprovalResult, error) {
	if r.Env.Dir == nil {
		return nil, fmt.Errorf("engine: no directory for authorization")
	}
	return state.EvaluateApprovals(ctx, r.Env.Dir, r.Env.Registry.Organization, r.Plan.Rules, r.Snap.Votes, r.Subject, r.ApprovalOptions())
}

// Allowlist returns the github/command allowlist for approver commands.
func (r *Request) Allowlist() string {
	return policy.StaticAllowlist(r.Env.Registry.Organization, r.Plan.Rules)
}

// ---- decisions ----------------------------------------------------------

// Comment is the comment a decision wants posted.
type Comment struct {
	Event        string         `json:"event"`
	Data         map[string]any `json:"data"`
	Template     string         `json:"template"`
	UpdatePrefix string         `json:"update_prefix,omitempty"`
	Body         string         `json:"-"`
}

// Decision is the outcome of an intake, command, gate or finish step.
type Decision struct {
	Action       string   `json:"action"`
	Phase        string   `json:"phase"`
	Escalated    bool     `json:"escalated"`
	Comment      *Comment `json:"comment,omitempty"`
	Execute      bool     `json:"execute"`
	Environment  string   `json:"environment,omitempty"`
	Workflow     string   `json:"workflow,omitempty"`
	RequestType  string   `json:"request_type"`
	Close        string   `json:"close,omitempty"`
	Rejected     bool     `json:"rejected"`
	Reason       string   `json:"reason,omitempty"`
	Allowlist    string   `json:"allowlist,omitempty"`
	Digest       string   `json:"digest"`
	LabelsAdd    []string `json:"labels_add"`
	LabelsRemove []string `json:"labels_remove"`
}

// RuleView is a rule prepared for templates.
type RuleView struct {
	Name string
	Min  int
	Who  string
}

// View is the data handed to comment templates.
type View struct {
	Org, Repo, DocsURL, RunURL string
	TypeID, TypeName, TypeDesc string
	Issue                      int
	IssueURL                   string
	Requestor                  string
	Title                      string
	Rows                       []request.Row
	Errors, Warnings           []string
	Plan                       *policy.Plan
	Rules                      []RuleView
	Mentions                   []string
	AutoApproved               bool
	Approval                   *state.ApprovalResult
	Digest                     string
	Actor                      string
	Command                    string
	Note                       string
	QA                         *qa.Status
	Info                       map[string]any
	Result                     any
	Phase                      string
	Stale                      bool
	Reason                     string
	Commands                   []string
}

// View builds template data.
func (r *Request) View() *View {
	reg := r.Env.Registry
	v := &View{
		Org: reg.Organization, Repo: reg.Repository, DocsURL: reg.DocsURL, RunURL: r.Env.RunURL,
		TypeID: r.Type.ID, TypeName: r.Type.Name, TypeDesc: r.Type.Description,
		Issue: r.Issue.Number, IssueURL: r.Issue.HTMLURL, Requestor: r.Issue.User.Login, Title: r.Issue.Title,
		Rows: r.Handler.Summary(r.RC), Errors: r.Errors, Warnings: r.Warnings, Plan: r.Plan,
		AutoApproved: r.Plan.AutoApproved || len(r.Plan.Rules) == 0,
		Digest:       r.Snap.CurrentDigest, QA: r.QA, Info: r.RC.Info, Phase: r.Phase(), Stale: r.Snap.Stale,
	}
	seen := map[string]bool{}
	for _, rule := range r.Plan.Rules {
		v.Rules = append(v.Rules, RuleView{Name: rule.Name, Min: rule.Min, Who: state.Describe(reg.Organization, rule.AnyOf)})
		for _, t := range rule.AnyOf.Teams {
			m := "@" + reg.Organization + "/" + t
			if strings.Contains(t, "/") {
				m = "@" + t
			}
			if !seen[m] {
				seen[m] = true
				v.Mentions = append(v.Mentions, m)
			}
		}
		for _, u := range rule.AnyOf.Users {
			m := "@" + strings.TrimPrefix(u, "@")
			if !seen[m] {
				seen[m] = true
				v.Mentions = append(v.Mentions, m)
			}
		}
	}
	sort.Strings(v.Mentions)
	return v
}

func (r *Request) decision(action, phase string) *Decision {
	d := &Decision{Action: action, Phase: phase, Escalated: r.Escalated(), RequestType: r.Type.ID, Digest: r.Snap.CurrentDigest, Allowlist: r.Allowlist(), Workflow: r.Type.Execute.Workflow}
	d.LabelsAdd, d.LabelsRemove = state.LabelsFor(phase, d.Escalated)
	return d
}

// render fills d.Comment.Body = marker line + rendered template.
func (r *Request) render(d *Decision, event, template, updatePrefix string, data map[string]any, v *View) error {
	if data == nil {
		data = map[string]any{}
	}
	if _, ok := data["digest"]; !ok {
		data["digest"] = r.Snap.CurrentDigest
	}
	if v == nil {
		v = r.View()
	}
	body, err := r.Env.Templates.Render(template, v)
	if err != nil {
		return err
	}
	d.Comment = &Comment{Event: event, Data: data, Template: template, UpdatePrefix: updatePrefix,
		Body: state.Marker{Event: event, Data: data}.Format() + "\n" + body}
	return nil
}

func (r *Request) reject(reason string, cmd *state.CommandEvent) (*Decision, error) {
	d := r.decision("rejected", r.Phase())
	d.Rejected, d.Reason = true, reason
	v := r.View()
	v.Reason, v.Actor = reason, cmd.User
	v.Command = cmd.Trigger()
	return d, r.render(d, state.EventNotice, "comment.rejected.md.tmpl", "", map[string]any{"command": cmd.Name, "actor": cmd.User}, v)
}

// Intake handles issues opened/edited/reopened.
func (r *Request) Intake(ctx context.Context, action string) (*Decision, error) {
	snapPhase := r.Snap.Phase
	if state.IsTerminal(snapPhase) && action != "reopened" {
		return r.decision("noop", snapPhase), nil
	}
	if ok, why, err := r.CheckRequestor(ctx); err != nil {
		return nil, err
	} else if !ok {
		r.Errors = append(r.Errors, why)
	}
	if action == "reopened" && state.IsTerminal(snapPhase) {
		r.Snap.Phase = state.PhaseValidated // reset marker below revives the request
	}
	phase := r.Phase()
	d := r.decision("summary", phase)
	v := r.View()
	data := map[string]any{"valid": r.Valid(), "action": action}
	if r.Type.QA {
		data["qa_complete"] = r.QA != nil && r.QA.Complete
	}
	event, tpl := state.EventSummary, "comment.summary.md.tmpl"
	if !r.Valid() {
		tpl = "comment.invalid.md.tmpl"
	}
	if action == "reopened" && state.IsTerminal(snapPhase) {
		event, data["phase"] = state.EventReset, phase
		return d, r.render(d, event, tpl, "", data, v)
	}
	return d, r.render(d, event, tpl, state.Prefix(state.EventSummary), data, v)
}

// Handle processes a command comment (already included in the timeline).
func (r *Request) Handle(ctx context.Context, cmd *state.CommandEvent) (*Decision, error) {
	s := r.Snap
	isRequestor := strings.EqualFold(cmd.User, r.Issue.User.Login)
	phase := r.Phase()
	switch cmd.Name {
	case "help":
		d := r.decision("help", phase)
		return d, r.render(d, state.EventNotice, "comment.help.md.tmpl", "", nil, nil)

	case "status":
		d := r.decision("status", phase)
		v := r.View()
		if s.SubmitValid() && r.Env.Dir != nil {
			ar, err := r.Approvals(ctx)
			if err != nil {
				return nil, err
			}
			v.Approval = ar
		}
		return d, r.render(d, state.EventNotice, "comment.status.md.tmpl", "", nil, v)

	case "cancel":
		if state.IsTerminal(phase) || phase == state.PhaseExecuting {
			return r.reject("The request is "+phase+" and can no longer be cancelled.", cmd)
		}
		if !isRequestor {
			ok := false
			if r.Env.Dir != nil {
				p, err := r.Env.Dir.RepoPermission(ctx, r.Env.Registry.Organization, r.Env.Registry.Repository, cmd.User)
				if err != nil {
					return nil, err
				}
				ok = p == "admin" || p == "maintain"
			}
			if !ok {
				return r.reject("Only the requestor or IssueOps repository maintainers can cancel a request.", cmd)
			}
		}
		d := r.decision("cancelled", state.PhaseCancelled)
		d.Close = "not_planned"
		v := r.View()
		v.Actor, v.Note = cmd.User, cmd.Args
		return d, r.render(d, state.EventCancelled, "comment.cancelled.md.tmpl", "", map[string]any{"actor": cmd.User}, v)

	case "answers":
		if !r.Type.QA {
			return r.reject("This request type has no clarifying questions.", cmd)
		}
		if !isRequestor {
			return r.reject("Only the requestor can answer clarifying questions.", cmd)
		}
		if state.IsTerminal(phase) || phase == state.PhaseExecuting {
			return r.reject("The request is "+phase+"; answers can no longer change it.", cmd)
		}
		d := r.decision("answers", r.Phase())
		v := r.View()
		if r.QA != nil && r.QA.Complete && r.Valid() {
			sp := agent.BuildSpec(r.RC, r.QA, s.CurrentDigest, r.Issue.HTMLURL)
			v.Result = sp
		}
		return d, r.render(d, state.EventQAStatus, "comment.qa-status.md.tmpl", state.Prefix(state.EventQAStatus), map[string]any{"complete": r.QA != nil && r.QA.Complete}, v)

	case "submit":
		if !isRequestor {
			return r.reject("Only the requestor (@"+r.Issue.User.Login+") can submit this request.", cmd)
		}
		if state.IsTerminal(phase) {
			return r.reject("The request is already "+phase+".", cmd)
		}
		if phase == state.PhaseExecuting {
			return r.reject("The request is executing.", cmd)
		}
		if s.SubmitValid() && (phase == state.PhaseSubmitted || phase == state.PhaseApproved) {
			return r.reject("This exact request is already submitted; use `.status` to see approval progress.", cmd)
		}
		if !r.Valid() {
			return r.reject("The request is not valid yet. Fix the errors listed in the summary comment by editing the issue.", cmd)
		}
		if r.Type.QA && (r.QA == nil || !r.QA.Complete) {
			return r.reject("Answer all required clarifying questions with `.answers` before submitting.", cmd)
		}
		if ok, why, err := r.CheckRequestor(ctx); err != nil {
			return nil, err
		} else if !ok {
			return r.reject(why, cmd)
		}
		v := r.View()
		v.Actor = cmd.User
		if r.Type.QA {
			v.Result = agent.BuildSpec(r.RC, r.QA, s.CurrentDigest, r.Issue.HTMLURL)
		}
		data := map[string]any{"actor": cmd.User, "environment": r.Plan.Environment, "escalations": r.Plan.Escalations}
		if len(r.Plan.Rules) == 0 {
			d := r.decision("approved", state.PhaseApproved)
			d.Execute, d.Environment = true, r.Plan.Environment
			data["auto"] = true
			return d, r.render(d, state.EventApproved, "comment.approved.md.tmpl", "", data, v)
		}
		d := r.decision("submitted", state.PhaseSubmitted)
		return d, r.render(d, state.EventSubmitted, "comment.submitted.md.tmpl", "", data, v)

	case "approve", "deny", "retry":
		if r.Env.Dir == nil {
			return nil, fmt.Errorf("engine: authorization directory required")
		}
		if state.IsTerminal(phase) {
			return r.reject("The request is already "+phase+".", cmd)
		}
		if !s.SubmitValid() {
			msg := "There is no submission to act on. The requestor must comment `.submit` first."
			if s.Stale {
				msg = "The request changed after it was submitted, so earlier approvals no longer apply. The requestor must comment `.submit` again."
			}
			return r.reject(msg, cmd)
		}
		ok, why, err := state.VoterEligible(ctx, r.Env.Dir, r.Plan.Rules, cmd.User, r.Subject, r.ApprovalOptions())
		if err != nil {
			return nil, err
		}
		if !ok {
			return r.reject("@"+cmd.User+": "+why+". Eligible approvers: "+describeRules(r)+".", cmd)
		}
		ar, err := r.Approvals(ctx)
		if err != nil {
			return nil, err
		}
		v := r.View()
		v.Approval, v.Actor, v.Note = ar, cmd.User, cmd.Args
		if cmd.Name == "deny" || ar.Denied {
			d := r.decision("denied", state.PhaseDenied)
			d.Close = "not_planned"
			by := cmd.User
			if ar.Denied {
				by = ar.DeniedBy
			}
			return d, r.render(d, state.EventDenied, "comment.denied.md.tmpl", "", map[string]any{"actor": by, "reason": cmd.Args}, v)
		}
		if cmd.Name == "retry" {
			if phase != state.PhaseFailed {
				return r.reject("`.retry` is only available after a failed execution.", cmd)
			}
			if !ar.Satisfied {
				return r.reject("Approvals are no longer sufficient to retry.", cmd)
			}
			d := r.decision("approved", state.PhaseApproved)
			d.Execute, d.Environment = true, r.Plan.Environment
			return d, r.render(d, state.EventApproved, "comment.approved.md.tmpl", "", map[string]any{"actor": cmd.User, "retry": true, "approvers": approverList(ar)}, v)
		}
		if phase == state.PhaseApproved || phase == state.PhaseExecuting || phase == state.PhaseFailed {
			return r.reject("The request is already approved ("+phase+").", cmd)
		}
		if ar.Satisfied {
			d := r.decision("approved", state.PhaseApproved)
			d.Execute, d.Environment = true, r.Plan.Environment
			return d, r.render(d, state.EventApproved, "comment.approved.md.tmpl", "", map[string]any{"actor": cmd.User, "approvers": approverList(ar)}, v)
		}
		d := r.decision("approval-recorded", state.PhaseSubmitted)
		return d, r.render(d, state.EventStatus, "comment.approval-status.md.tmpl", state.Prefix(state.EventStatus), map[string]any{"actor": cmd.User}, v)
	}
	return r.reject("Unknown command.", cmd)
}

func describeRules(r *Request) string {
	var parts []string
	for _, rule := range r.Plan.Rules {
		parts = append(parts, state.Describe(r.Env.Registry.Organization, rule.AnyOf))
	}
	if len(parts) == 0 {
		return "none required"
	}
	return strings.Join(parts, "; ")
}

func approverList(ar *state.ApprovalResult) []string {
	var out []string
	for _, rs := range ar.Rules {
		for _, a := range rs.Approvers {
			out = append(out, a.User)
		}
	}
	sort.Strings(out)
	return out
}

// GateError is returned when execution must not proceed.
type GateError struct{ Reason string }

func (g *GateError) Error() string { return g.Reason }

// Gate re-verifies, from the timeline, that execution is authorized for the
// current content, then returns the "executing" decision. It is called by
// every execution workflow regardless of how it was triggered.
func (r *Request) Gate(ctx context.Context, force bool) (*Decision, error) {
	s := r.Snap
	if !r.Valid() {
		return nil, &GateError{"request no longer passes validation: " + strings.Join(r.Errors, "; ")}
	}
	if !s.ApprovedValid() {
		return nil, &GateError{fmt.Sprintf("no valid approval for the current request content (phase %s, stale=%v)", s.Phase, s.Stale)}
	}
	if s.Phase == state.PhaseExecuting && !force {
		return nil, &GateError{"an execution is already in progress or ended without reporting; use workflow_dispatch with force after checking the previous run"}
	}
	if s.Phase == state.PhaseCompleted {
		return nil, &GateError{"request already completed"}
	}
	auto := s.LastApproved != nil && s.LastApproved.Data["auto"] == true
	if !auto && len(r.Plan.Rules) > 0 {
		ar, err := r.Approvals(ctx)
		if err != nil {
			return nil, err
		}
		if !ar.Satisfied {
			return nil, &GateError{"recomputed approvals are not satisfied (an approver may have lost eligibility)"}
		}
	}
	if auto && len(r.Plan.Rules) > 0 {
		return nil, &GateError{"request was auto-approved but policy now requires approvals; the requestor must .submit again"}
	}
	d := r.decision("executing", state.PhaseExecuting)
	d.Execute, d.Environment = true, r.Plan.Environment
	return d, r.render(d, state.EventExecuting, "comment.executing.md.tmpl", "", map[string]any{"run": r.Env.RunURL}, nil)
}

// Finish records the execution outcome.
func (r *Request) Finish(status string, result any, errMsg string) (*Decision, error) {
	v := r.View()
	v.Result, v.Reason = result, errMsg
	switch status {
	case "completed":
		d := r.decision("completed", state.PhaseCompleted)
		if r.Type.CloseOnComplete {
			d.Close = "completed"
		}
		return d, r.render(d, state.EventCompleted, "comment.completed."+r.Type.ID+".md.tmpl", "", map[string]any{"run": r.Env.RunURL}, v)
	case "needs_input":
		res, _ := result.(*agent.Result)
		var dyn []qa.Question
		if res != nil {
			dyn = qa.Renumber(r.Questions, res.Questions)
		}
		d := r.decision("needs-input", state.PhaseAwaiting)
		v.Result = map[string]any{"agent": res, "questions": dyn}
		return d, r.render(d, state.EventQuestions, "comment.agent-followup.md.tmpl", "", map[string]any{"dynamic": dyn}, v)
	default:
		d := r.decision("failed", state.PhaseFailed)
		return d, r.render(d, state.EventFailed, "comment.failed.md.tmpl", "", map[string]any{"run": r.Env.RunURL, "error": truncate(errMsg, 500)}, v)
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
