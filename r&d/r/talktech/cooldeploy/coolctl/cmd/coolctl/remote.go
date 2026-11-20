package main

// Commands that call cool-api, plus the two that look at a running release:
// gate asks Prometheus, logs asks kubectl.

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/CoolEngOrg/coolctl/internal/api"
	"github.com/CoolEngOrg/coolctl/internal/client"
	"github.com/CoolEngOrg/coolctl/internal/gitops"
	"github.com/CoolEngOrg/coolctl/internal/manifest"
	"github.com/CoolEngOrg/coolctl/internal/prom"
	"github.com/CoolEngOrg/coolctl/internal/release"
)

// remote holds the flags every cool-api command shares.
type remote struct {
	api, app, issue *string
}

func remoteFlags(name string) (*remote, *flag.FlagSet) {
	fs := flags(name)
	return &remote{
		api:   fs.String("api", os.Getenv("COOL_API_URL"), "cool-api base URL; default $COOL_API_URL"),
		app:   fs.String("app", "", "team/app; default: read cool.json"),
		issue: fs.String("issue", os.Getenv("COOL_ISSUE_URL"), "URL of the IssueOps issue behind this change"),
	}, fs
}

func (r *remote) client() (*client.Client, error) {
	if *r.api == "" {
		return nil, errors.New("no cool-api URL: pass -api or set COOL_API_URL")
	}
	return &client.Client{BaseURL: *r.api}, nil
}

// path returns /v1/apps/<team>/<app>, taking the app from cool.json when the
// flag is unset.
func (r *remote) path() (string, error) {
	if *r.app == "" {
		m, err := manifest.Load("cool.json")
		if err != nil {
			return "", fmt.Errorf("no -app given and no usable cool.json: %w", err)
		}
		*r.app = m.Team + "/" + m.App
	}
	team, app, err := splitApp(*r.app)
	if err != nil {
		return "", err
	}
	return "/v1/apps/" + team + "/" + app, nil
}

// post sends one write and prints its result.
func (r *remote) post(ctx context.Context, method, suffix string, in map[string]any) (api.Result, error) {
	var res api.Result
	c, err := r.client()
	if err != nil {
		return res, err
	}
	base, err := r.path()
	if err != nil {
		return res, err
	}
	if *r.issue != "" && in != nil {
		in["issue"] = *r.issue
	}
	if err := c.Do(ctx, method, base+suffix, in, &res); err != nil {
		return res, err
	}
	fmt.Println(res.Message)
	if res.URL != "" {
		fmt.Println(res.URL)
	}
	writeOutputs(map[string]string{"release": res.Release, "url": res.URL, "application": res.Application})
	return res, nil
}

// writeOutputs appends key=value lines to $GITHUB_OUTPUT, when a workflow
// step is listening.
func writeOutputs(kv map[string]string) {
	path := os.Getenv("GITHUB_OUTPUT")
	if path == "" {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	for k, v := range kv {
		if !strings.ContainsAny(v, "\r\n") {
			fmt.Fprintf(f, "%s=%s\n", k, v)
		}
	}
}

func cmdDeploy(ctx context.Context, args []string) error {
	r, fs := remoteFlags("deploy")
	env := fs.String("env", "dev", "environment")
	digest := fs.String("digest", "", "image digest, sha256:...")
	sha := fs.String("sha", os.Getenv("GITHUB_SHA"), "commit the image was built from; default $GITHUB_SHA")
	pr := fs.Int("pr", 0, "pull-request number, to deploy a preview")
	file := fs.String("f", "cool.json", "manifest to send with the release")
	wait := fs.Duration("wait", 0, "wait this long for Argo CD to report the release healthy")
	fs.Parse(args)

	m, err := manifest.Load(*file)
	if err != nil {
		return err
	}
	if *r.app == "" {
		*r.app = m.Team + "/" + m.App
	}
	in := map[string]any{"environment": *env, "digest": *digest, "sha": *sha, "manifest": m}
	if *pr > 0 {
		in["preview"] = *pr
	}
	res, err := r.post(ctx, http.MethodPost, "/deployments", in)
	if err != nil || *wait == 0 {
		return err
	}
	return waitHealthy(ctx, r, *env, res.Release, *wait)
}

func cmdPreview(ctx context.Context, args []string) error {
	if len(args) == 0 || args[0] != "down" {
		return errors.New("usage: coolctl preview down -pr N   (coolctl deploy -pr N brings one up)")
	}
	r, fs := remoteFlags("preview down")
	pr := fs.Int("pr", 0, "pull-request number")
	fs.Parse(args[1:])
	if *pr <= 0 {
		return errors.New("preview down: -pr is required")
	}
	_, err := r.post(ctx, http.MethodDelete, fmt.Sprintf("/previews/%d", *pr), nil)
	return err
}

func cmdPromote(ctx context.Context, args []string) error {
	r, fs := remoteFlags("promote")
	from := fs.String("from", "dev", "source environment")
	to := fs.String("to", "tst", "target environment")
	rel := fs.String("release", "", "release to promote; default: the active one in -from")
	wait := fs.Duration("wait", 0, "wait this long for Argo CD to report the release healthy")
	fs.Parse(args)
	res, err := r.post(ctx, http.MethodPost, "/promote", map[string]any{"from": *from, "to": *to, "release": *rel})
	if err != nil || *wait == 0 {
		return err
	}
	return waitHealthy(ctx, r, *to, res.Release, *wait)
}

func cmdRollback(ctx context.Context, args []string) error {
	r, fs := remoteFlags("rollback")
	env := fs.String("env", "", "environment")
	reason := fs.String("reason", "", "why; recorded in the commit")
	fs.Parse(args)
	_, err := r.post(ctx, http.MethodPost, "/rollback", map[string]any{"environment": *env, "reason": *reason})
	return err
}

func cmdWeights(ctx context.Context, args []string) error {
	r, fs := remoteFlags("weights")
	env := fs.String("env", "", "environment")
	weight := fs.Int("weight", -1, "percent of traffic for the release, 0 to 100")
	rel := fs.String("release", "", "release; default: the one being rolled out")
	from := fs.String("from", "", "bring the release from this environment, to start a rolling release")
	fs.Parse(args)
	if *weight < 0 {
		return errors.New("weights: -weight is required")
	}
	_, err := r.post(ctx, http.MethodPost, "/weights",
		map[string]any{"environment": *env, "weight": *weight, "release": *rel, "from": *from})
	return err
}

func cmdConfig(ctx context.Context, args []string) error {
	r, fs := remoteFlags("config")
	env := fs.String("env", "", "environment")
	identity := fs.String("identity", "", "client ID of the app's managed identity")
	var set, unset, add, remove list
	fs.Var(&set, "set", "KEY=VALUE override; repeatable")
	fs.Var(&unset, "unset", "override to remove; repeatable")
	fs.Var(&add, "add-domain", "custom domain to add; repeatable")
	fs.Var(&remove, "remove-domain", "custom domain to remove; repeatable")
	fs.Parse(args)

	in := map[string]any{"environment": *env}
	if len(set) > 0 {
		kv := map[string]string{}
		for _, s := range set {
			k, v, ok := strings.Cut(s, "=")
			if !ok {
				return fmt.Errorf("-set %q: want KEY=VALUE", s)
			}
			kv[k] = v
		}
		in["set"] = kv
	}
	if len(unset) > 0 {
		in["unset"] = []string(unset)
	}
	if len(add) > 0 {
		in["add_domains"] = []string(add)
	}
	if len(remove) > 0 {
		in["remove_domains"] = []string(remove)
	}
	if *identity != "" {
		in["identity"] = *identity
	}
	_, err := r.post(ctx, http.MethodPost, "/config", in)
	return err
}

func cmdRegister(ctx context.Context, args []string) error {
	r, fs := remoteFlags("register")
	repo := fs.String("repo", "", "source repository, owner/name")
	group := fs.String("group", "", "Entra ID group of the team; default: from teams/<team>/team.json")
	env := fs.String("env", "dev", "environment whose approval this job runs under")
	var ids list
	fs.Var(&ids, "identity", "ENV=CLIENT-ID of the app's managed identity in an environment; repeatable")
	fs.Parse(args)
	c, err := r.client()
	if err != nil {
		return err
	}
	team, app, err := splitApp(*r.app)
	if err != nil {
		return err
	}
	identities := map[string]string{}
	for _, kv := range ids {
		env, id, ok := strings.Cut(kv, "=")
		if !ok {
			return fmt.Errorf("-identity %q: want ENV=CLIENT-ID", kv)
		}
		identities[env] = id
	}
	in := map[string]any{"team": team, "app": app, "repo": *repo, "group": *group, "issue": *r.issue, "identities": identities, "environment": *env}
	var a gitops.App
	if err := c.Do(ctx, http.MethodPost, "/v1/apps", in, &a); err != nil {
		return err
	}
	fmt.Printf("registered %s/%s from %s on chart %s\n", a.Team, a.App, a.Repo, a.Chart)
	return nil
}

func cmdRetire(ctx context.Context, args []string) error {
	r, fs := remoteFlags("retire")
	confirm := fs.String("confirm", "", "type team/app again to confirm")
	fs.Parse(args)
	c, err := r.client()
	if err != nil {
		return err
	}
	base, err := r.path()
	if err != nil {
		return err
	}
	var out struct{ Removed int }
	if err := c.Do(ctx, http.MethodDelete, base, map[string]any{"confirm": *confirm, "issue": *r.issue}, &out); err != nil {
		return err
	}
	fmt.Printf("retired %s: removed %d files from cool-gitops\n", *r.app, out.Removed)
	return nil
}

func getStatus(ctx context.Context, r *remote) (api.AppStatus, error) {
	var st api.AppStatus
	c, err := r.client()
	if err != nil {
		return st, err
	}
	base, err := r.path()
	if err != nil {
		return st, err
	}
	return st, c.Do(ctx, http.MethodGet, base, nil, &st)
}

func cmdStatus(ctx context.Context, args []string) error {
	r, fs := remoteFlags("status")
	asJSON := fs.Bool("json", false, "print JSON")
	fs.Parse(args)
	st, err := getStatus(ctx, r)
	if err != nil {
		return err
	}
	if *asJSON {
		return printJSON(st)
	}
	fmt.Printf("%s/%s  %s  chart %s\n", st.Team, st.App.App, st.Repo, st.Chart)
	for _, e := range st.Environments {
		fmt.Printf("\n%s  %s  %s/%s\n", e.Environment, e.URL, e.Sync, e.Health)
		for _, rel := range e.Releases {
			note := ""
			if rel.Standby {
				note = "  standby"
			}
			fmt.Printf("  %s  %3d%%  %.19s  %s%s\n", rel.ID, rel.Weight, rel.Digest, rel.At, note)
		}
	}
	for _, p := range st.Previews {
		fmt.Printf("\npreview of pull request %d  %s  %s\n", p.PR, p.URL, p.Release)
	}
	return nil
}

func cmdOpen(ctx context.Context, args []string) error {
	r, fs := remoteFlags("open")
	env := fs.String("env", "dev", "environment")
	fs.Parse(args)
	st, err := getStatus(ctx, r)
	if err != nil {
		return err
	}
	for _, e := range st.Environments {
		if e.Environment == *env {
			fmt.Println(e.URL)
			return nil
		}
	}
	return fmt.Errorf("%s is not deployed in %s", *r.app, *env)
}

func cmdWait(ctx context.Context, args []string) error {
	r, fs := remoteFlags("wait")
	env := fs.String("env", "dev", "environment")
	rel := fs.String("release", "", "release that must be active; default: any")
	timeout := fs.Duration("timeout", 10*time.Minute, "give up after this long")
	fs.Parse(args)
	return waitHealthy(ctx, r, *env, *rel, *timeout)
}

// waitHealthy polls until Argo CD reports the Application synced and healthy
// with the wanted release active.
func waitHealthy(ctx context.Context, r *remote, env, rel string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	last := ""
	for {
		st, err := getStatus(ctx, r)
		if err != nil {
			return err
		}
		for _, e := range st.Environments {
			if e.Environment != env {
				continue
			}
			if e.Sync == "Unknown" && e.Health == "Unknown" {
				fmt.Println("no Argo CD status is available here; not waiting")
				return nil
			}
			active := ""
			if i := release.Active(e.Releases); i >= 0 {
				active = e.Releases[i].ID
			}
			if e.Ready() && (rel == "" || rel == active) {
				fmt.Printf("%s in %s: %s is synced and healthy\n", *r.app, env, active)
				return nil
			}
			if now := e.Sync + "/" + e.Health; now != last {
				fmt.Printf("%s in %s: %s %s\n", *r.app, env, now, e.Message)
				last = now
			}
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("%s in %s was not healthy after %s", *r.app, env, timeout)
		case <-time.After(5 * time.Second):
		}
	}
}

func cmdGate(ctx context.Context, args []string) error {
	fs := flags("gate")
	endpoint := fs.String("endpoint", os.Getenv("COOL_PROM_URL"), "Prometheus query endpoint; default $COOL_PROM_URL")
	tokenEnv := fs.String("token-env", "COOL_PROM_TOKEN", "environment variable that holds the bearer token")
	ns := fs.String("namespace", "", "namespace of the release")
	workload := fs.String("workload", "", "Deployment name: <app>-<release>")
	window := fs.String("window", "5m", "PromQL range to judge")
	maxErr := fs.Float64("max-error-rate", 0.01, "error budget as a fraction: 0.01 is one percent")
	maxP95 := fs.Float64("max-p95-ms", 0, "latency budget in milliseconds; 0 disables it")
	allowEmpty := fs.Bool("allow-no-data", false, "pass when the release has had no traffic")
	fs.Parse(args)
	if *endpoint == "" || *ns == "" || *workload == "" {
		return errors.New("gate: -endpoint, -namespace and -workload are required")
	}
	c := prom.Client{Endpoint: *endpoint, Token: os.Getenv(*tokenEnv)}
	g := prom.Gate{MaxErrorRate: *maxErr, MaxP95ms: *maxP95, AllowNoData: *allowEmpty}
	v, err := g.Check(ctx, c, *ns, *workload, *window)
	if err != nil {
		return err
	}
	printJSON(v)
	writeOutputs(map[string]string{"pass": fmt.Sprint(v.Pass), "reason": v.Reason})
	if !v.Pass {
		return errors.New("gate failed: " + v.Reason)
	}
	return nil
}

func cmdLogs(ctx context.Context, args []string) error {
	fs := flags("logs")
	app := fs.String("app", "", "team/app")
	env := fs.String("env", "dev", "environment")
	rel := fs.String("release", "", "only this release")
	kctx := fs.String("context", "", "kubectl context; default <env>-eus2-appdev-aks-001")
	follow := fs.Bool("f", false, "follow")
	tail := fs.Int("tail", 200, "lines to start with")
	fs.Parse(args)
	team, name, err := splitApp(*app)
	if err != nil {
		return err
	}
	if *kctx == "" {
		*kctx = *env + "-eus2-appdev-aks-001"
	}
	selector := "cool/app=" + name
	if *rel != "" {
		selector += ",cool/release=" + *rel
	}
	kargs := []string{"--context", *kctx, "--namespace", team + "-" + name, "logs",
		"--selector", selector, "--container", "app", "--prefix", fmt.Sprintf("--tail=%d", *tail)}
	if *follow {
		kargs = append(kargs, "--follow")
	}
	cmd := exec.CommandContext(ctx, "kubectl", kargs...)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
}
