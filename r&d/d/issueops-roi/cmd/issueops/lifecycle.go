package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/CoolEngOrg/issueops/internal/config"
	"github.com/CoolEngOrg/issueops/internal/engine"
	"github.com/CoolEngOrg/issueops/internal/gha"
	"github.com/CoolEngOrg/issueops/internal/ghapi"
	"github.com/CoolEngOrg/issueops/internal/issueform"
	"github.com/CoolEngOrg/issueops/internal/state"
)

func cmdTypes(_ context.Context, args []string) error {
	fs := flag.NewFlagSet("types", flag.ExitOnError)
	var c common
	c.register(fs)
	_ = fs.Parse(args)
	c.offline = true
	a, err := c.load()
	if err != nil {
		return err
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tLABEL\tTEMPLATE\tWORKFLOW\tENVIRONMENT")
	for _, t := range a.reg.RequestTypes {
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", t.ID, t.Label, t.Template, t.Execute.Workflow, t.Execute.Environment)
	}
	return w.Flush()
}

func cmdResolve(_ context.Context, args []string) error {
	fs := flag.NewFlagSet("resolve", flag.ExitOnError)
	var c common
	c.register(fs)
	labels := fs.String("labels", "", "comma-separated labels (default: from the event payload)")
	event := fs.String("event", "", "event payload path (default GITHUB_EVENT_PATH)")
	_ = fs.Parse(args)
	c.offline = true
	a, err := c.load()
	if err != nil {
		return err
	}
	var names []string
	if *labels != "" {
		names = issueform.SplitCSV(*labels)
	} else {
		ev, err := readEvent(*event)
		if err != nil {
			return err
		}
		names = ev.Issue.LabelNames()
	}
	rt, err := a.reg.ResolveLabels(names)
	if err != nil {
		return err
	}
	for k, v := range map[string]string{"request_type": rt.ID, "template": rt.Template, "label": rt.Label, "workflow": rt.Execute.Workflow, "qa": strconv.FormatBool(rt.QA)} {
		if err := gha.SetOutput(k, v); err != nil {
			return err
		}
	}
	fmt.Println(rt.ID)
	return nil
}

func cmdParse(_ context.Context, args []string) error {
	fs := flag.NewFlagSet("parse", flag.ExitOnError)
	var c common
	c.register(fs)
	typ := fs.String("type", "", "request type id")
	bodyFile := fs.String("body-file", "", "issue body markdown")
	_ = fs.Parse(args)
	c.offline = true
	a, err := c.load()
	if err != nil {
		return err
	}
	rt, err := a.reg.ByID(*typ)
	if err != nil {
		return err
	}
	form, err := issueform.LoadFormForTemplate(a.formsDir, rt.Template)
	if err != nil {
		return err
	}
	body, err := os.ReadFile(*bodyFile)
	if err != nil {
		return err
	}
	v, err := issueform.Parse(string(body), form)
	if err != nil {
		return err
	}
	b, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(b))
	return nil
}

// fieldRefs lists form fields referenced by the registry (conditions,
// field_users and target-repository fields), which must exist in the template.
func fieldRefs(a *app, id string) []string {
	rt, _ := a.reg.ByID(id)
	set := map[string]bool{}
	add := func(f string) {
		if f != "" && !strings.HasPrefix(f, "$") {
			set[f] = true
		}
	}
	addCond := func(c config.Condition) {
		for _, p := range append(append([]config.Predicate{}, c.All...), c.Any...) {
			add(p.Field)
		}
	}
	for _, e := range rt.Approval.Escalations {
		addCond(e.When)
		for _, r := range e.Rules {
			for _, f := range r.AnyOf.FieldUsers {
				add(f)
			}
		}
	}
	for _, r := range rt.Approval.Rules {
		for _, f := range r.AnyOf.FieldUsers {
			add(f)
		}
	}
	if rt.Approval.AutoApprove != nil {
		addCond(*rt.Approval.AutoApprove)
	}
	add(rt.Approval.TargetRepoField)
	if rt.Requestors != nil {
		add(rt.Requestors.TargetRepoField)
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func cmdCheckForms(_ context.Context, args []string) error {
	fs := flag.NewFlagSet("check-forms", flag.ExitOnError)
	var c common
	c.register(fs)
	_ = fs.Parse(args)
	c.offline = true
	a, err := c.load()
	if err != nil {
		return err
	}
	var errs []error
	for _, rt := range a.reg.RequestTypes {
		form, err := issueform.LoadFormForTemplate(a.formsDir, rt.Template)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w (run `make forms`)", rt.ID, err))
			continue
		}
		hasType, hasPlatform := false, false
		for _, l := range form.Labels {
			hasType = hasType || l == rt.Label
			hasPlatform = hasPlatform || l == a.reg.PlatformLabel
		}
		if !hasType || !hasPlatform {
			errs = append(errs, fmt.Errorf("%s: form labels must include %q and %q", rt.ID, a.reg.PlatformLabel, rt.Label))
		}
		if err := issueform.CheckDrift(form, fieldRefs(a, rt.ID)); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", rt.ID, err))
		}
		fields, err := form.Fields()
		if err != nil {
			errs = append(errs, err)
		}
		for _, f := range fields {
			for _, o := range f.Options {
				if strings.Contains(o, ", ") {
					errs = append(errs, fmt.Errorf("%s: dropdown %s option %q contains \", \" which issue-ops/parser splits on", rt.ID, f.Key, o))
				}
			}
			if f.HasRender {
				errs = append(errs, fmt.Errorf("%s: textarea %s uses render:, which wraps the value in a code fence; remove it", rt.ID, f.Key))
			}
		}
		fmt.Printf("ok  %-26s %d fields, labels %v\n", rt.ID, len(fields), form.Labels)
	}
	return errors.Join(errs...)
}

// loadRequest loads issue, comments and parsed values for intake/command.
func (a *app) loadRequest(ctx context.Context, issue *ghapi.Issue, parsedPath string) (*engine.Request, error) {
	values, err := a.loadValues(parsedPath, issue)
	if err != nil {
		return nil, err
	}
	comments, err := a.comments(ctx, issue.Number)
	if err != nil {
		return nil, err
	}
	return a.env().Load(ctx, issue, comments, values, formErrors())
}

func cmdIntake(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("intake", flag.ExitOnError)
	var c common
	c.register(fs)
	event := fs.String("event", "", "event payload (default GITHUB_EVENT_PATH)")
	parsed := fs.String("parsed", "", "issue-ops/parser JSON (default PARSED_JSON env)")
	_ = fs.Parse(args)
	a, err := c.load()
	if err != nil {
		return err
	}
	ev, err := readEvent(*event)
	if err != nil {
		return err
	}
	r, err := a.loadRequest(ctx, ev.Issue, *parsed)
	if err != nil {
		return err
	}
	d, err := r.Intake(ctx, ev.Action)
	if err != nil {
		return err
	}
	return a.emit(d)
}

func cmdCommand(ctx context.Context, args []string) error {
	if len(args) < 1 {
		return errors.New("usage: issueops command classify|handle [flags]")
	}
	sub := args[0]
	fs := flag.NewFlagSet("command "+sub, flag.ExitOnError)
	var c common
	c.register(fs)
	event := fs.String("event", "", "event payload (default GITHUB_EVENT_PATH)")
	parsed := fs.String("parsed", "", "issue-ops/parser JSON (default PARSED_JSON env)")
	_ = fs.Parse(args[1:])
	ev, err := readEvent(*event)
	if err != nil {
		return err
	}
	if ev.Comment == nil {
		return errors.New("event has no comment")
	}
	switch sub {
	case "classify":
		cmd := state.ParseCommand(ev.Comment.Body)
		name, trigger, priv := "", "", false
		if cmd != nil && !strings.EqualFold(ev.Comment.User.Type, "Bot") {
			name, trigger, priv = cmd.Name, cmd.Trigger(), state.PrivilegedCommands[cmd.Name]
		}
		for k, v := range map[string]string{"command": name, "trigger": trigger, "privileged": strconv.FormatBool(priv)} {
			if err := gha.SetOutput(k, v); err != nil {
				return err
			}
		}
		fmt.Printf("command=%q privileged=%v\n", name, priv)
		return nil
	case "handle":
		a, err := c.load()
		if err != nil {
			return err
		}
		cmd := state.ParseCommand(ev.Comment.Body)
		if cmd == nil {
			return errors.New("comment is not a command")
		}
		r, err := a.loadRequest(ctx, ev.Issue, *parsed)
		if err != nil {
			return err
		}
		d, err := r.Handle(ctx, &state.CommandEvent{Command: cmd, User: ev.Comment.User.Login, CommentID: ev.Comment.ID, At: ev.Comment.CreatedAt})
		if err != nil {
			return err
		}
		return a.emit(d)
	case "allowlist":
		a, err := c.load()
		if err != nil {
			return err
		}
		r, err := a.loadRequest(ctx, ev.Issue, *parsed)
		if err != nil {
			return err
		}
		fmt.Println(r.Allowlist())
		return gha.SetOutput("allowlist", r.Allowlist())
	}
	return fmt.Errorf("unknown subcommand %q", sub)
}

// execContext loads a request for execution from the API (issue number).
func execContext(ctx context.Context, a *app, number int, parsedPath string) (*engine.Request, error) {
	is, err := a.issue(ctx, number)
	if err != nil {
		return nil, err
	}
	if is.State != "open" {
		return nil, &engine.GateError{Reason: "issue is " + is.State}
	}
	return a.loadRequest(ctx, is, parsedPath)
}

func cmdGate(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("gate", flag.ExitOnError)
	var c common
	c.register(fs)
	number := fs.Int("issue", 0, "issue number")
	parsed := fs.String("parsed", "", "issue-ops/parser JSON of the fetched body (default PARSED_JSON env)")
	expectType := fs.String("type", "", "fail unless the issue is this request type")
	force := fs.Bool("force", false, "allow re-execution while a previous run is marked executing")
	_ = fs.Parse(args)
	a, err := c.load()
	if err != nil {
		return err
	}
	r, err := execContext(ctx, a, *number, *parsed)
	if err != nil {
		return err
	}
	if *expectType != "" && r.Type.ID != *expectType {
		return &engine.GateError{Reason: fmt.Sprintf("issue #%d is %s, not %s", *number, r.Type.ID, *expectType)}
	}
	d, err := r.Gate(ctx, *force)
	if err != nil {
		return err
	}
	return a.emit(d)
}

func cmdFinish(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("finish", flag.ExitOnError)
	var c common
	c.register(fs)
	number := fs.Int("issue", 0, "issue number")
	parsed := fs.String("parsed", "", "issue-ops/parser JSON (default PARSED_JSON env)")
	status := fs.String("status", "completed", "completed | failed | needs_input")
	resultPath := fs.String("result", "", "result JSON written by the execution step")
	errMsg := fs.String("error", os.Getenv("ISSUEOPS_ERROR"), "error message for failed runs")
	_ = fs.Parse(args)
	a, err := c.load()
	if err != nil {
		return err
	}
	is, err := a.issue(ctx, *number)
	if err != nil {
		return err
	}
	r, err := a.loadRequest(ctx, is, *parsed)
	if err != nil {
		return err
	}
	var result any
	if *resultPath != "" {
		if _, statErr := os.Stat(*resultPath); statErr == nil {
			var err error
			if result, err = loadResult(r.Type.ID, *resultPath, *status); err != nil {
				return err
			}
		} else if *status == "completed" {
			*status, *errMsg = "failed", "result file missing: "+*resultPath
		}
	}
	d, err := r.Finish(*status, result, *errMsg)
	if err != nil {
		return err
	}
	return a.emit(d)
}

func cmdIssue(ctx context.Context, args []string) error {
	if len(args) < 1 {
		return errors.New("usage: issueops issue fetch|close [flags]")
	}
	sub := args[0]
	fs := flag.NewFlagSet("issue "+sub, flag.ExitOnError)
	var c common
	c.register(fs)
	number := fs.Int("issue", 0, "issue number")
	reason := fs.String("reason", "completed", "close reason: completed | not_planned")
	_ = fs.Parse(args[1:])
	a, err := c.load()
	if err != nil {
		return err
	}
	switch sub {
	case "fetch":
		is, err := a.issue(ctx, *number)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(a.outDir, "body.md"), []byte(is.Body), 0o644); err != nil {
			return err
		}
		if err := writeJSON(filepath.Join(a.outDir, "issue.json"), is); err != nil {
			return err
		}
		rt, err := a.reg.ResolveLabels(is.LabelNames())
		if err != nil {
			return err
		}
		for k, v := range map[string]string{"body": is.Body, "state": is.State, "template": rt.Template, "request_type": rt.ID, "requestor": is.User.Login} {
			if err := gha.SetOutput(k, v); err != nil {
				return err
			}
		}
		return nil
	case "close":
		if *reason != "completed" && *reason != "not_planned" {
			return fmt.Errorf("invalid reason %q", *reason)
		}
		if a.gh == nil {
			return errors.New("close requires the API")
		}
		return a.gh.CloseIssue(ctx, a.reg.Organization, a.reg.Repository, *number, *reason)
	}
	return fmt.Errorf("unknown subcommand %q", sub)
}

func cmdRender(_ context.Context, args []string) error {
	fs := flag.NewFlagSet("render", flag.ExitOnError)
	var c common
	c.register(fs)
	name := fs.String("template", "", "template name (see --list)")
	data := fs.String("data", "", "JSON data file")
	list := fs.Bool("list", false, "list templates")
	_ = fs.Parse(args)
	c.offline = true
	a, err := c.load()
	if err != nil {
		return err
	}
	if *list {
		for _, n := range a.set.Names() {
			fmt.Println(n)
		}
		return nil
	}
	var v any
	if *data != "" {
		if err := readJSON(*data, &v); err != nil {
			return err
		}
	}
	out, err := a.set.Render(*name, v)
	if err != nil {
		return err
	}
	fmt.Print(out)
	return nil
}
