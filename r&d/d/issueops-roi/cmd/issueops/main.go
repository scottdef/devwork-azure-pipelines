// Command issueops is the CoolEngOrg IssueOps platform CLI. It is invoked by
// the GitHub Actions workflows in .github/workflows and can be run locally
// (offline with --offline, or against the API with GH_TOKEN).
//
//	issueops <command> [subcommand] [flags]
//
// Lifecycle: resolve, parse, check-forms, intake, command classify|handle,
// gate, finish, issue fetch|close, render, simulate.
// Request types: budget plan|apply, report build|push, foundry
// plan|preflight|result, agent spec|dispatch|render-k8s|result.
// Operations: ops-metrics, labels, types, version.
//
// Only the Go standard library is used, plus go-echarts for report charts.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/CoolEngOrg/issueops/internal/authz"
	"github.com/CoolEngOrg/issueops/internal/config"
	"github.com/CoolEngOrg/issueops/internal/engine"
	"github.com/CoolEngOrg/issueops/internal/gha"
	"github.com/CoolEngOrg/issueops/internal/ghapi"
	"github.com/CoolEngOrg/issueops/internal/issueform"
	"github.com/CoolEngOrg/issueops/internal/state"
	"github.com/CoolEngOrg/issueops/internal/tmpl"
)

// Version is set with -ldflags "-X main.Version=...".
var Version = "dev"

type command struct {
	name  string
	usage string
	run   func(ctx context.Context, args []string) error
}

func commands() []command {
	return []command{
		{"version", "print the version", func(context.Context, []string) error { fmt.Println(Version); return nil }},
		{"types", "list request types from the registry", cmdTypes},
		{"resolve", "resolve the request type from issue labels", cmdResolve},
		{"parse", "parse an issue body with the Go parser (mirrors issue-ops/parser)", cmdParse},
		{"check-forms", "verify issue forms (JSON) match the registry", cmdCheckForms},
		{"intake", "validate an opened/edited/reopened issue and decide the summary comment", cmdIntake},
		{"command", "classify|handle a command comment", cmdCommand},
		{"gate", "re-verify approval before execution and emit the executing marker", cmdGate},
		{"finish", "record the execution outcome", cmdFinish},
		{"issue", "fetch|close an issue", cmdIssue},
		{"budget", "plan|apply a Copilot budget", cmdBudget},
		{"report", "build|push a Copilot usage and ROI report", cmdReport},
		{"foundry", "plan|preflight|result for a Foundry model deployment", cmdFoundry},
		{"agent", "spec|dispatch|render-k8s|result for an agentic task", cmdAgent},
		{"ops-metrics", "export IssueOps operational metrics (Prometheus text)", cmdOpsMetrics},
		{"labels", "create or update the platform labels from config/labels.json", cmdLabels},
		{"render", "render a template with JSON data (development aid)", cmdRender},
		{"simulate", "run a request lifecycle offline from a scenario file", cmdSimulate},
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: issueops <command> [flags]\n\ncommands:")
	for _, c := range commands() {
		fmt.Fprintf(os.Stderr, "  %-12s %s\n", c.name, c.usage)
	}
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	for _, c := range commands() {
		if c.name == os.Args[1] {
			if err := c.run(ctx, os.Args[2:]); err != nil {
				var ge *engine.GateError
				if errors.As(err, &ge) {
					gha.Error("execution gate refused: " + ge.Reason)
					reportRefusal(ge.Reason)
					os.Exit(3)
				}
				gha.Error(err.Error())
				os.Exit(1)
			}
			return
		}
	}
	usage()
	os.Exit(2)
}

// reportRefusal writes a notice comment for a refused gate so the requestor
// sees why nothing ran (the workflow posts it; exit code 3 = refused).
func reportRefusal(reason string) {
	dir := defaultOut()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return
	}
	body := state.Marker{Event: state.EventNotice, Data: map[string]any{"gate": "refused"}}.Format() +
		"\n> [!WARNING]\n> **Execution gate refused:** " + tmpl.Safe(reason) + "\n>\n> Nothing was changed."
	if u := defaultRunURL(); u != "" {
		body += " See the [workflow run](" + u + ")."
	}
	p := filepath.Join(dir, "gate-refused.md")
	if os.WriteFile(p, []byte(body+"\n"), 0o644) == nil {
		_ = gha.SetOutput("refused", "true")
		_ = gha.SetOutput("comment_path", p)
	}
}

// ---- shared flags and application context -------------------------------

type common struct {
	configPath string
	formsDir   string
	botLogin   string
	runURL     string
	outDir     string
	offline    bool
	templates  string
	nowFlag    string
}

func (c *common) register(fs *flag.FlagSet) {
	fs.StringVar(&c.configPath, "config", envOr("ISSUEOPS_CONFIG", "config/issueops.json"), "registry file")
	fs.StringVar(&c.formsDir, "forms-dir", envOr("ISSUEOPS_FORMS_DIR", "config/forms"), "issue forms converted to JSON")
	fs.StringVar(&c.botLogin, "bot-login", os.Getenv("ISSUEOPS_BOT_LOGIN"), "login of the IssueOps GitHub App bot (app-slug[bot])")
	fs.StringVar(&c.runURL, "run-url", defaultRunURL(), "URL of the current workflow run")
	fs.StringVar(&c.outDir, "out", defaultOut(), "output directory")
	fs.BoolVar(&c.offline, "offline", false, "do not call the GitHub API")
	fs.StringVar(&c.templates, "templates", os.Getenv("ISSUEOPS_TEMPLATES"), "template override directory")
	fs.StringVar(&c.nowFlag, "now", os.Getenv("ISSUEOPS_NOW"), "override the current time (RFC3339), for reproducible runs")
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func defaultRunURL() string {
	s, r, id := os.Getenv("GITHUB_SERVER_URL"), os.Getenv("GITHUB_REPOSITORY"), os.Getenv("GITHUB_RUN_ID")
	if s == "" || r == "" || id == "" {
		return ""
	}
	return fmt.Sprintf("%s/%s/actions/runs/%s", s, r, id)
}

func defaultOut() string {
	if t := os.Getenv("RUNNER_TEMP"); t != "" {
		return filepath.Join(t, "issueops")
	}
	return "out"
}

type app struct {
	common
	reg *config.Registry
	set *tmpl.Set
	gh  *ghapi.Client
	dir authz.Directory
	now time.Time
}

func (c *common) load() (*app, error) {
	reg, err := config.Load(c.configPath)
	if err != nil {
		return nil, err
	}
	set, err := tmpl.Load(c.templates)
	if err != nil {
		return nil, err
	}
	a := &app{common: *c, reg: reg, set: set, now: time.Now().UTC()}
	if c.nowFlag != "" {
		t, err := time.Parse(time.RFC3339, c.nowFlag)
		if err != nil {
			return nil, fmt.Errorf("--now: %w", err)
		}
		a.now = t.UTC()
	}
	if !c.offline {
		gh, err := ghapi.NewFromEnv()
		if err != nil {
			return nil, fmt.Errorf("%w (use --offline for local runs)", err)
		}
		a.gh = gh
		a.dir = authz.NewGitHub(gh)
	}
	if err := os.MkdirAll(c.outDir, 0o755); err != nil {
		return nil, err
	}
	return a, nil
}

func (a *app) env() *engine.Env {
	return &engine.Env{Registry: a.reg, GH: a.gh, Dir: a.dir, BotLogin: a.botLogin, Templates: a.set,
		Now: func() time.Time { return a.now }, RunURL: a.runURL, Handlers: engine.DefaultHandlers()}
}

// ---- GitHub event payloads ------------------------------------------------

type eventPayload struct {
	Action  string          `json:"action"`
	Issue   *ghapi.Issue    `json:"issue"`
	Comment *ghapi.Comment  `json:"comment"`
	Changes json.RawMessage `json:"changes"`
}

func readEvent(path string) (*eventPayload, error) {
	if path == "" {
		path = os.Getenv("GITHUB_EVENT_PATH")
	}
	if path == "" {
		return nil, errors.New("no event payload (--event or GITHUB_EVENT_PATH)")
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var ev eventPayload
	if err := json.Unmarshal(b, &ev); err != nil {
		return nil, fmt.Errorf("decode event: %w", err)
	}
	if ev.Issue == nil {
		return nil, errors.New("event has no issue")
	}
	return &ev, nil
}

// ---- parsed body and validator results -------------------------------------

// loadValues reads issue-ops/parser JSON from --parsed (path) or the
// PARSED_JSON environment variable; with neither, it parses the body with the
// Go parser using the forms directory.
func (a *app) loadValues(parsedPath string, issue *ghapi.Issue) (issueform.Values, error) {
	if parsedPath != "" {
		return issueform.LoadValues(parsedPath)
	}
	if env := os.Getenv("PARSED_JSON"); strings.TrimSpace(env) != "" {
		return issueform.DecodeValues([]byte(env))
	}
	rt, err := a.reg.ResolveLabels(issue.LabelNames())
	if err != nil {
		return nil, err
	}
	form, err := issueform.LoadFormForTemplate(a.formsDir, rt.Template)
	if err != nil {
		return nil, err
	}
	return issueform.Parse(issue.Body, form)
}

// formErrors reads issue-ops/validator results (FORM_RESULT, FORM_ERRORS).
func formErrors() []string {
	if os.Getenv("FORM_RESULT") != "failure" {
		return nil
	}
	raw := strings.TrimSpace(os.Getenv("FORM_ERRORS"))
	var list []string
	if json.Unmarshal([]byte(raw), &list) == nil && len(list) > 0 {
		return list
	}
	for _, l := range strings.Split(raw, "\n") {
		if l = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(l), "-")); l != "" {
			list = append(list, l)
		}
	}
	if len(list) == 0 {
		list = []string{"The issue form did not pass template validation."}
	}
	return list
}

func (a *app) comments(ctx context.Context, number int) ([]ghapi.Comment, error) {
	if a.gh == nil {
		return nil, nil
	}
	return a.gh.ListComments(ctx, a.reg.Organization, a.reg.Repository, number)
}

func (a *app) issue(ctx context.Context, number int) (*ghapi.Issue, error) {
	if a.gh == nil {
		return nil, errors.New("fetching an issue requires the GitHub API (omit --offline)")
	}
	return a.gh.GetIssue(ctx, a.reg.Organization, a.reg.Repository, number)
}

// ---- decision output ------------------------------------------------------

// emit writes the decision to files and step outputs.
func (a *app) emit(d *engine.Decision) error {
	b, _ := json.MarshalIndent(d, "", "  ")
	if err := os.WriteFile(filepath.Join(a.outDir, "decision.json"), b, 0o644); err != nil {
		return err
	}
	out := map[string]string{
		"action":        d.Action,
		"phase":         d.Phase,
		"execute":       fmt.Sprint(d.Execute),
		"environment":   d.Environment,
		"workflow":      d.Workflow,
		"request_type":  d.RequestType,
		"close":         d.Close,
		"rejected":      fmt.Sprint(d.Rejected),
		"reason":        d.Reason,
		"allowlist":     d.Allowlist,
		"digest":        d.Digest,
		"escalated":     fmt.Sprint(d.Escalated),
		"labels_add":    strings.Join(d.LabelsAdd, "\n"),
		"labels_remove": strings.Join(d.LabelsRemove, "\n"),
	}
	if d.Comment != nil {
		p := filepath.Join(a.outDir, "comment.md")
		if err := os.WriteFile(p, []byte(d.Comment.Body), 0o644); err != nil {
			return err
		}
		out["comment_path"] = p
		out["comment_event"] = d.Comment.Event
		out["comment_update_prefix"] = d.Comment.UpdatePrefix
	}
	keys := make([]string, 0, len(out))
	for k := range out {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if err := gha.SetOutput(k, out[k]); err != nil {
			return err
		}
	}
	return gha.AppendSummary(fmt.Sprintf("### IssueOps decision\n\n| key | value |\n|---|---|\n| request type | `%s` |\n| action | `%s` |\n| phase | `%s` |\n| execute | `%v` |\n| environment | `%s` |\n| digest | `%s` |\n", d.RequestType, d.Action, d.Phase, d.Execute, d.Environment, d.Digest))
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}
