package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/CoolEngOrg/issueops/internal/agent"
	"github.com/CoolEngOrg/issueops/internal/budget"
	"github.com/CoolEngOrg/issueops/internal/copilot"
	"github.com/CoolEngOrg/issueops/internal/engine"
	"github.com/CoolEngOrg/issueops/internal/foundry"
	"github.com/CoolEngOrg/issueops/internal/gha"
	"github.com/CoolEngOrg/issueops/internal/ghapi"
)

// ---- outcome files (written by execution steps, read by `finish`) -----------

type budgetOutcome struct {
	Result *budget.Result `json:"result"`
	Plan   *budget.Plan   `json:"plan"`
}

type reportOutcome struct {
	Report        *copilot.Report    `json:"report"`
	Artifacts     *copilot.Artifacts `json:"artifacts,omitempty"`
	ArtifactURL   string             `json:"artifact_url,omitempty"`
	RetentionDays int                `json:"retention_days,omitempty"`
	Grafana       bool               `json:"grafana,omitempty"`
}

type foundryOutcome struct {
	Plan              *foundry.Plan            `json:"plan"`
	Preflight         *foundry.PreflightResult `json:"preflight"`
	Endpoint          string                   `json:"endpoint"`
	ProvisioningState string                   `json:"provisioning_state"`
}

type agentOutcome struct {
	Dispatch *agent.DispatchResult `json:"dispatch,omitempty"`
	Agent    *agent.Result         `json:"agent,omitempty"`
	Spec     *agent.Spec           `json:"spec,omitempty"`
}

// loadResult converts an outcome file into template data for `finish`.
func loadResult(typeID, path, status string) (any, error) {
	extra := extras()
	switch typeID {
	case "copilot-budget-request":
		var o budgetOutcome
		if err := readJSON(path, &o); err != nil {
			return nil, err
		}
		return map[string]any{"result": o.Result, "plan": o.Plan}, nil
	case "copilot-usage-report":
		var o reportOutcome
		if err := readJSON(path, &o); err != nil {
			return nil, err
		}
		if v := extra["artifact_url"]; v != "" {
			o.ArtifactURL = v
		}
		ret := o.RetentionDays
		fmt.Sscanf(extra["retention_days"], "%d", &ret)
		return map[string]any{"report": o.Report, "artifact_url": o.ArtifactURL, "retention_days": ret, "grafana": o.Grafana || extra["grafana"] == "true"}, nil
	case "foundry-model-deployment":
		var o foundryOutcome
		if err := readJSON(path, &o); err != nil {
			return nil, err
		}
		return map[string]any{"plan": o.Plan, "preflight": o.Preflight, "endpoint": o.Endpoint, "provisioning_state": o.ProvisioningState}, nil
	case "agentic-task-request":
		var o agentOutcome
		if err := readJSON(path, &o); err != nil {
			return nil, err
		}
		if status == "needs_input" {
			return o.Agent, nil
		}
		return map[string]any{"dispatch": o.Dispatch, "agent": o.Agent}, nil
	}
	return nil, fmt.Errorf("no outcome loader for %s", typeID)
}

// extras parses ISSUEOPS_EXTRA ("k=v" lines), set by workflows after artifact upload.
func extras() map[string]string {
	out := map[string]string{}
	for _, l := range strings.Split(os.Getenv("ISSUEOPS_EXTRA"), "\n") {
		if k, v, ok := strings.Cut(strings.TrimSpace(l), "="); ok {
			out[k] = v
		}
	}
	return out
}

// verified loads the request for execution and enforces the gate's digest,
// closing the window between the gate job and the execution job.
func verified(ctx context.Context, a *app, number int, parsed, expectDigest string) (*engine.Request, error) {
	r, err := execContext(ctx, a, number, parsed)
	if err != nil {
		return nil, err
	}
	if expectDigest == "" {
		return nil, errors.New("--expect-digest is required (output of the gate job)")
	}
	if r.Snap.CurrentDigest != expectDigest {
		return nil, &engine.GateError{Reason: "request content changed after the gate approved it; aborting"}
	}
	if !r.Valid() {
		return nil, &engine.GateError{Reason: "request no longer validates: " + strings.Join(r.Errors, "; ")}
	}
	return r, nil
}

func execFlags(fs *flag.FlagSet) (*int, *string, *string) {
	return fs.Int("issue", 0, "issue number"),
		fs.String("parsed", "", "issue-ops/parser JSON (default PARSED_JSON env)"),
		fs.String("expect-digest", os.Getenv("ISSUEOPS_DIGEST"), "digest approved by the gate job")
}

// ---- budget --------------------------------------------------------------------

func cmdBudget(ctx context.Context, args []string) error {
	if len(args) < 1 {
		return errors.New("usage: issueops budget plan|apply [flags]")
	}
	sub := args[0]
	fs := flag.NewFlagSet("budget "+sub, flag.ExitOnError)
	var c common
	c.register(fs)
	number, parsed, digest := execFlags(fs)
	dry := fs.Bool("dry-run", false, "show the API call without changing anything")
	_ = fs.Parse(args[1:])
	a, err := c.load()
	if err != nil {
		return err
	}
	r, err := verified(ctx, a, *number, *parsed, *digest)
	if err != nil {
		return err
	}
	plan, errs := budget.BuildPlan(r.RC)
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	if sub == "plan" {
		return writeJSON(filepath.Join(a.outDir, "plan.json"), map[string]any{"plan": plan, "create_payload": plan.CreatePayload(), "update_payload": plan.UpdatePayload()})
	}
	if sub != "apply" {
		return fmt.Errorf("unknown subcommand %q", sub)
	}
	gh := a.gh
	if plan.OwnerKind == "enterprise" {
		// Enterprise budget endpoints do not accept GitHub App installation or
		// fine-grained tokens; a classic PAT of an enterprise billing owner is used.
		tok := os.Getenv("GH_ENTERPRISE_TOKEN")
		if tok == "" {
			return errors.New("enterprise budgets need GH_ENTERPRISE_TOKEN (classic PAT, enterprise billing manager/owner)")
		}
		gha.Mask(tok)
		gh = ghapi.New(tok)
	}
	res, err := budget.Apply(ctx, gh, plan, *dry)
	if err != nil {
		return err
	}
	fmt.Printf("budget %s: %s %s %s $%d\n", res.Action, plan.OwnerKind, plan.Scope, plan.SKU, plan.Amount)
	return writeJSON(filepath.Join(a.outDir, "outcome.json"), budgetOutcome{Result: res, Plan: plan})
}

// ---- report --------------------------------------------------------------------

func cmdReport(ctx context.Context, args []string) error {
	if len(args) < 1 {
		return errors.New("usage: issueops report build|push [flags]")
	}
	sub := args[0]
	fs := flag.NewFlagSet("report "+sub, flag.ExitOnError)
	var c common
	c.register(fs)
	number, parsed, digest := execFlags(fs)
	fixtures := fs.String("fixtures", "", "read report data from a fixture directory instead of the API")
	pace := fs.Duration("search-pace", 2100*time.Millisecond, "delay between search API calls (30/min limit)")
	reviews := fs.Bool("reviews", true, "count pull-request reviews per team member (teams scope)")
	file := fs.String("file", "", "metrics.prom to push")
	gateway := fs.String("pushgateway", os.Getenv("PUSHGATEWAY_URL"), "Prometheus Pushgateway base URL")
	job := fs.String("job", "issueops_copilot_roi", "Pushgateway job name")
	grouping := fs.String("grouping", "", "extra grouping labels, e.g. org=CoolEngOrg,scope=teams")
	_ = fs.Parse(args[1:])
	if sub == "push" {
		return pushMetrics(ctx, *gateway, *job, *grouping, *file)
	}
	if sub != "build" {
		return fmt.Errorf("unknown subcommand %q", sub)
	}
	if *fixtures != "" {
		c.offline = true
	}
	a, err := c.load()
	if err != nil {
		return err
	}
	var r *engine.Request
	if a.gh != nil {
		if r, err = verified(ctx, a, *number, *parsed, *digest); err != nil {
			return err
		}
	} else {
		if r, err = offlineRequest(ctx, a, "copilot-usage-report", *parsed, *number); err != nil {
			return err
		}
	}
	sp, settings, errs, _ := copilot.BuildSpec(r.RC)
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	var src copilot.Source
	if *fixtures != "" {
		src = &copilot.FixtureSource{Dir: *fixtures}
	} else {
		src = &copilot.APISource{GH: a.gh, Org: a.reg.Organization, SearchPace: *pace, Log: func(s string) { fmt.Println(s) }}
	}
	rep, err := copilot.Build(ctx, src, sp, copilot.Options{Org: a.reg.Organization, Pricing: a.reg.Pricing, Settings: *settings, Now: a.now, WithReviews: *reviews, Log: func(s string) { fmt.Println(s) }})
	if err != nil {
		return err
	}
	arts, err := copilot.WriteAll(rep, filepath.Join(a.outDir, "report"), settings.AssetsHost)
	if err != nil {
		return err
	}
	for k, v := range map[string]string{"report_dir": filepath.Join(a.outDir, "report"), "html": arts.HTML, "prom": arts.Prom, "grafana": fmt.Sprint(sp.Grafana)} {
		if err := gha.SetOutput(k, v); err != nil {
			return err
		}
	}
	fmt.Printf("report: %d days, %d teams, %d repositories, total cost $%.2f\n", rep.DaysFound, len(rep.Teams), len(rep.Repos), rep.Totals.TotalCostUSD)
	return writeJSON(filepath.Join(a.outDir, "outcome.json"), reportOutcome{Report: rep, Artifacts: arts, RetentionDays: 30, Grafana: sp.Grafana})
}

func pushMetrics(ctx context.Context, gateway, job, grouping, file string) error {
	if gateway == "" {
		return errors.New("--pushgateway (or PUSHGATEWAY_URL) is required")
	}
	body, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	u := strings.TrimRight(gateway, "/") + "/metrics/job/" + url.PathEscape(job)
	for _, kv := range strings.Split(grouping, ",") {
		if k, v, ok := strings.Cut(strings.TrimSpace(kv), "="); ok {
			u += "/" + url.PathEscape(k) + "/" + url.PathEscape(v)
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, u, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "text/plain; version=0.0.4")
	// Pushgateway has no authentication of its own; CoolEngOrg exposes it
	// through an internal ingress with basic auth (PUSHGATEWAY_AUTH=user:password).
	if auth := os.Getenv("PUSHGATEWAY_AUTH"); auth != "" {
		user, pass, _ := strings.Cut(auth, ":")
		gha.Mask(pass)
		req.SetBasicAuth(user, pass)
	}
	resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("pushgateway: HTTP %d: %s", resp.StatusCode, b)
	}
	fmt.Println("pushed", file, "to", u)
	return nil
}

// ---- foundry -------------------------------------------------------------------

func cmdFoundry(ctx context.Context, args []string) error {
	if len(args) < 1 {
		return errors.New("usage: issueops foundry plan|preflight|result [flags]")
	}
	sub := args[0]
	fs := flag.NewFlagSet("foundry "+sub, flag.ExitOnError)
	var c common
	c.register(fs)
	number, parsed, digest := execFlags(fs)
	planPath := fs.String("plan", "", "plan.json from `foundry plan`")
	models := fs.String("models", "", "az cognitiveservices account list-models -o json")
	usage := fs.String("usage", "", "az cognitiveservices usage list -o json")
	deps := fs.String("deployments", "", "az cognitiveservices account deployment list -o json")
	pre := fs.String("preflight", "", "preflight.json")
	dep := fs.String("deployment", "", "az cognitiveservices account deployment show -o json")
	acct := fs.String("account", "", "az cognitiveservices account show -o json")
	_ = fs.Parse(args[1:])
	switch sub {
	case "plan":
		a, err := c.load()
		if err != nil {
			return err
		}
		r, err := verified(ctx, a, *number, *parsed, *digest)
		if err != nil {
			return err
		}
		plan, errs := foundry.BuildPlan(r.RC)
		if len(errs) > 0 {
			return errors.New(strings.Join(errs, "; "))
		}
		params, err := plan.BicepParameters()
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(a.outDir, "parameters.json"), params, 0o644); err != nil {
			return err
		}
		if err := writeJSON(filepath.Join(a.outDir, "plan.json"), plan); err != nil {
			return err
		}
		for k, v := range map[string]string{"account": plan.Account, "resource_group": plan.ResourceGroup, "region": plan.Region,
			"deployment_name": plan.DeploymentName, "environment": plan.Environment, "plan": filepath.Join(a.outDir, "plan.json"),
			"parameters": filepath.Join(a.outDir, "parameters.json")} {
			if err := gha.SetOutput(k, v); err != nil {
				return err
			}
		}
		return nil
	case "preflight":
		c.offline = true
		a, err := c.load()
		if err != nil {
			return err
		}
		var plan foundry.Plan
		var ms []foundry.AzModel
		var us []foundry.AzUsage
		var ds []foundry.AzDeployment
		for p, v := range map[string]any{*planPath: &plan, *models: &ms, *usage: &us, *deps: &ds} {
			if err := readJSON(p, v); err != nil {
				return fmt.Errorf("preflight input %s: %w", p, err)
			}
		}
		strict := true
		if rt, err := a.reg.ByID("foundry-model-deployment"); err == nil {
			var s foundry.Settings
			if rt.DecodeSettings(&s) == nil {
				strict = s.QuotaStrict
			}
		}
		res := foundry.Preflight(&plan, ms, us, ds, strict)
		if err := writeJSON(filepath.Join(a.outDir, "preflight.json"), res); err != nil {
			return err
		}
		for _, w := range res.Warnings {
			gha.Warning(w)
		}
		_ = gha.SetOutput("mode", res.Mode)
		if !res.OK {
			return errors.New("pre-flight failed: " + strings.Join(res.Errors, " "))
		}
		fmt.Printf("pre-flight ok: mode=%s quota %s available=%v\n", res.Mode, res.QuotaName, res.QuotaAvailable)
		return nil
	case "result":
		c.offline = true
		a, err := c.load()
		if err != nil {
			return err
		}
		var o foundryOutcome
		o.Plan, o.Preflight = &foundry.Plan{}, &foundry.PreflightResult{}
		if err := readJSON(*planPath, o.Plan); err != nil {
			return err
		}
		if err := readJSON(*pre, o.Preflight); err != nil {
			return err
		}
		var d foundry.AzDeployment
		if err := readJSON(*dep, &d); err != nil {
			return err
		}
		var acc struct {
			Properties struct {
				Endpoint  string            `json:"endpoint"`
				Endpoints map[string]string `json:"endpoints"`
			} `json:"properties"`
		}
		if err := readJSON(*acct, &acc); err != nil {
			return err
		}
		o.ProvisioningState = d.Properties.ProvisioningState
		o.Endpoint = acc.Properties.Endpoint
		if ep := acc.Properties.Endpoints["OpenAI Language Model Instance API"]; ep != "" {
			o.Endpoint = ep
		}
		if err := writeJSON(filepath.Join(a.outDir, "outcome.json"), o); err != nil {
			return err
		}
		if !strings.EqualFold(o.ProvisioningState, "Succeeded") {
			return fmt.Errorf("deployment provisioning state is %q", o.ProvisioningState)
		}
		return nil
	}
	return fmt.Errorf("unknown subcommand %q", sub)
}

// ---- agent ---------------------------------------------------------------------

func cmdAgent(ctx context.Context, args []string) error {
	if len(args) < 1 {
		return errors.New("usage: issueops agent spec|dispatch|render-k8s|result [flags]")
	}
	sub := args[0]
	fs := flag.NewFlagSet("agent "+sub, flag.ExitOnError)
	var c common
	c.register(fs)
	number, parsed, digest := execFlags(fs)
	logPath := fs.String("log", "", "runner log (for result)")
	_ = fs.Parse(args[1:])
	if sub == "result" {
		c.offline = true
		a, err := c.load()
		if err != nil {
			return err
		}
		b, err := os.ReadFile(*logPath)
		if err != nil {
			return err
		}
		res, err := agent.ParseResult(string(b))
		if err != nil {
			res = &agent.Result{Status: "failed", Error: err.Error()}
		}
		if err := writeJSON(filepath.Join(a.outDir, "outcome.json"), agentOutcome{Agent: res}); err != nil {
			return err
		}
		_ = gha.SetOutput("status", res.Status)
		_ = gha.SetOutput("error", res.Error)
		fmt.Println("agent status:", res.Status)
		return nil
	}
	a, err := c.load()
	if err != nil {
		return err
	}
	r, err := verified(ctx, a, *number, *parsed, *digest)
	if err != nil {
		return err
	}
	s, err := agent.LoadSettings(r.RC)
	if err != nil {
		return err
	}
	if r.QA == nil || !r.QA.Complete {
		return &engine.GateError{Reason: "clarifying questions are not complete"}
	}
	sp := agent.BuildSpec(r.RC, r.QA, r.Snap.CurrentDigest, r.Issue.HTMLURL)
	be := s.Backends[sp.Backend]
	switch sub {
	case "spec":
		for k, v := range map[string]string{"backend": sp.Backend, "repo": sp.Repo, "catalog": sp.Catalog, "namespace": be.Namespace} {
			if err := gha.SetOutput(k, v); err != nil {
				return err
			}
		}
		return writeJSON(filepath.Join(a.outDir, "spec.json"), sp)
	case "dispatch":
		var res *agent.DispatchResult
		switch sp.Backend {
		case agent.BackendCopilot:
			tok := os.Getenv("COPILOT_AGENT_TOKEN")
			if tok == "" {
				return errors.New("COPILOT_AGENT_TOKEN (user-to-server token of a Copilot-licensed machine user) is required")
			}
			gha.Mask(tok)
			res, err = agent.DispatchCopilot(ctx, ghapi.New(tok), a.set, sp, be)
		case agent.BackendCatalog:
			res, err = agent.DispatchCatalog(ctx, a.gh, s, sp, r.QA)
		default:
			return fmt.Errorf("backend %s is not dispatched through the API (use render-k8s)", sp.Backend)
		}
		if err != nil {
			return err
		}
		return writeJSON(filepath.Join(a.outDir, "outcome.json"), agentOutcome{Dispatch: res, Spec: sp})
	case "render-k8s":
		if sp.Backend != agent.BackendAKS {
			return fmt.Errorf("backend %s does not use AKS", sp.Backend)
		}
		manifest, jd, err := agent.RenderAKS(a.set, sp, be, r.Issue.Number)
		if err != nil {
			return err
		}
		p := filepath.Join(a.outDir, "agent-job.yaml")
		if err := os.WriteFile(p, []byte(manifest), 0o644); err != nil {
			return err
		}
		for k, v := range map[string]string{"manifest": p, "job_name": jd.JobName, "namespace": jd.Namespace} {
			if err := gha.SetOutput(k, v); err != nil {
				return err
			}
		}
		return nil
	}
	return fmt.Errorf("unknown subcommand %q", sub)
}

// offlineRequest builds a request from a parsed-values file without the API
// (used for local dry runs and fixtures).
func offlineRequest(ctx context.Context, a *app, typeID, parsedPath string, number int) (*engine.Request, error) {
	rt, err := a.reg.ByID(typeID)
	if err != nil {
		return nil, err
	}
	if parsedPath == "" {
		return nil, errors.New("--parsed is required offline")
	}
	values, err := loadValuesFile(parsedPath)
	if err != nil {
		return nil, err
	}
	is := &ghapi.Issue{Number: number, User: ghapi.User{Login: "local-user"}, Labels: []ghapi.Label{{Name: a.reg.PlatformLabel}, {Name: rt.Label}}, State: "open"}
	return a.env().Load(ctx, is, nil, values, nil)
}

func loadValuesFile(p string) (map[string]any, error) {
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	var v map[string]any
	return v, json.Unmarshal(b, &v)
}
