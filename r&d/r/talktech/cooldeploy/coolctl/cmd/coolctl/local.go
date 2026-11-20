package main

// Commands that need no cool-api: scaffolding, validation, rendering and the
// small helpers that workflow jobs and cron pods use.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/CoolEngOrg/coolctl/internal/api"
	"github.com/CoolEngOrg/coolctl/internal/argo"
	"github.com/CoolEngOrg/coolctl/internal/chart"
	"github.com/CoolEngOrg/coolctl/internal/gitops"
	"github.com/CoolEngOrg/coolctl/internal/issueform"
	"github.com/CoolEngOrg/coolctl/internal/manifest"
	"github.com/CoolEngOrg/coolctl/internal/release"
	"github.com/CoolEngOrg/coolctl/internal/tmpl"
)

const defaultRegistry = "prdeus2coolopsacr001.azurecr.io"

func cmdInit(_ context.Context, args []string) error {
	fs := flags("init")
	team := fs.String("team", "", "team slug")
	app := fs.String("app", "", "app slug")
	preset := fs.String("preset", "go-server", "one of "+strings.Join(manifest.Presets, ", "))
	dir := fs.String("dir", ".", "directory to write into")
	fs.Parse(args)

	m := manifest.Default(*team, *app, *preset)
	if err := m.Validate(); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(m, "", "  ")
	var wf bytes.Buffer
	if err := tmpl.Text(&wf, "workflow.tmpl", nil); err != nil {
		return err
	}
	files := map[string][]byte{
		"cool.json":                  append(b, '\n'),
		".github/workflows/cool.yml": wf.Bytes(),
	}
	for name, content := range files {
		path := filepath.Join(*dir, name)
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("%s exists; remove it first", path)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, content, 0o644); err != nil {
			return err
		}
		fmt.Println("wrote", path)
	}
	return nil
}

func cmdValidate(_ context.Context, args []string) error {
	fs := flags("validate")
	file := fs.String("f", "cool.json", "manifest to check")
	fs.Parse(args)
	m, err := manifest.Load(*file)
	if err != nil {
		return err
	}
	fmt.Printf("%s: ok (%s/%s, preset %s)\n", *file, m.Team, m.App, m.Preset)
	return nil
}

func cmdRender(ctx context.Context, args []string) error {
	if len(args) == 0 {
		return errors.New("render what? dockerfile, workflow, dashboard, portal or chart")
	}
	switch what, rest := args[0], args[1:]; what {
	case "dockerfile":
		return renderDockerfile(rest)
	case "workflow":
		fs := flags("render workflow")
		out := fs.String("o", "-", "output file")
		fs.Parse(rest)
		var b bytes.Buffer
		if err := tmpl.Text(&b, "workflow.tmpl", nil); err != nil {
			return err
		}
		return output(*out, b.Bytes())
	case "dashboard":
		return renderDashboard(rest)
	case "portal":
		return renderPortal(ctx, rest)
	case "chart":
		return renderChart(rest)
	default:
		return fmt.Errorf("render %q: want dockerfile, workflow, dashboard, portal or chart", what)
	}
}

func renderDockerfile(args []string) error {
	fs := flags("render dockerfile")
	file := fs.String("f", "cool.json", "manifest")
	registry := fs.String("registry", defaultRegistry, "registry that mirrors the base images")
	pkg := fs.String("main", ".", "Go package to build")
	out := fs.String("o", "-", "output file")
	fs.Parse(args)
	m, err := manifest.Load(*file)
	if err != nil {
		return err
	}
	name := map[string]string{"go-server": "dockerfile.go.tmpl", "go-htmx": "dockerfile.go.tmpl", "static": "dockerfile.static.tmpl"}[m.Preset]
	if name == "" {
		return fmt.Errorf("preset %q brings its own Dockerfile", m.Preset)
	}
	var b bytes.Buffer
	if err := tmpl.Text(&b, name, tmpl.Docker{Preset: m.Preset, Registry: *registry, Port: m.Port, Main: *pkg}); err != nil {
		return err
	}
	return output(*out, b.Bytes())
}

func renderDashboard(args []string) error {
	fs := flags("render dashboard")
	app := fs.String("app", "", "team/app")
	env := fs.String("env", "prd", "environment")
	out := fs.String("o", "-", "output file")
	fs.Parse(args)
	team, name, err := splitApp(*app)
	if err != nil {
		return err
	}
	var b bytes.Buffer
	d := tmpl.Dashboard{Team: team, App: name, Environment: *env, Namespace: team + "-" + name}
	if err := tmpl.Text(&b, "dashboard.tmpl", d); err != nil {
		return err
	}
	if !json.Valid(b.Bytes()) {
		return errors.New("the dashboard template produced invalid JSON")
	}
	return output(*out, b.Bytes())
}

// renderPortal writes the portal's static fragments from a checkout of
// cool-gitops: the app list, one fragment per app, and the data as JSON.
func renderPortal(ctx context.Context, args []string) error {
	fs := flags("render portal")
	dir := fs.String("gitops", ".", "checkout of cool-gitops")
	out := fs.String("out", "site", "portal directory to write fragments into")
	domain := fs.String("domain", "coolapps.example", "DNS zone of app hosts")
	issues := fs.String("issues", "https://github.com/CoolEngOrg/issueops/issues", "base URL of IssueOps issues")
	fs.Parse(args)

	store := gitops.NewDirStore(*dir)
	apps, err := gitops.Apps(ctx, store)
	if err != nil {
		return err
	}
	view := struct {
		Apps    []api.AppStatus
		Columns []string
		Active  map[string]string // "team/app/env" -> active release
		Issues  string
		Built   string
	}{Columns: []string{"dev", "tst", "prd"}, Active: map[string]string{}, Issues: *issues,
		Built: time.Now().UTC().Format("2006-01-02 15:04 UTC")}

	if err := os.MkdirAll(filepath.Join(*out, "fragments", "app"), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(*out, "data"), 0o755); err != nil {
		return err
	}
	for _, a := range apps {
		st, err := api.AppStatusOf(ctx, store, argo.None{}, *domain, a, false)
		if err != nil {
			return err
		}
		view.Apps = append(view.Apps, st)
		for _, e := range st.Environments {
			if i := release.Active(e.Releases); i >= 0 {
				view.Active[a.Team+"/"+a.App+"/"+e.Environment] = e.Releases[i].ID
			}
		}
		var b bytes.Buffer
		if err := tmpl.HTML(&b, "status.html", st); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(*out, "fragments", "app", a.Team+"-"+a.App+".html"), b.Bytes(), 0o644); err != nil {
			return err
		}
	}
	var b bytes.Buffer
	if err := tmpl.HTML(&b, "apps.html", view); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(*out, "fragments", "apps.html"), b.Bytes(), 0o644); err != nil {
		return err
	}
	data, _ := json.MarshalIndent(view.Apps, "", "  ")
	if err := os.WriteFile(filepath.Join(*out, "data", "apps.json"), append(data, '\n'), 0o644); err != nil {
		return err
	}
	fmt.Printf("rendered %d apps into %s\n", len(apps), *out)
	return nil
}

func renderChart(args []string) error {
	fs := flags("render chart")
	dir := fs.String("chart", "", "chart directory")
	name := fs.String("release", "", "release name; default: the chart name")
	ns := fs.String("namespace", "default", "namespace")
	out := fs.String("o", "-", "output file")
	var files list
	fs.Var(&files, "f", "values file in JSON; repeat to layer, last wins")
	fs.Parse(args)
	if *dir == "" {
		return errors.New("render chart: -chart is required")
	}
	c, err := chart.Load(*dir)
	if err != nil {
		return err
	}
	var layers []map[string]any
	for _, f := range files {
		v, err := chart.LoadValues(f)
		if err != nil {
			return err
		}
		layers = append(layers, v)
	}
	if *name == "" {
		*name = c.Name
	}
	s, err := c.Render(*name, *ns, layers...)
	if err != nil {
		return err
	}
	return output(*out, []byte(s))
}

func cmdIssue(_ context.Context, args []string) error {
	if len(args) == 0 || args[0] != "fields" {
		return errors.New("usage: coolctl issue fields [-f body.md] [-field name]")
	}
	fs := flags("issue fields")
	file := fs.String("f", "-", "file with the issue body; - reads standard input or $ISSUE_BODY")
	field := fs.String("field", "", "print this one field instead of all as JSON")
	fs.Parse(args[1:])

	var body []byte
	var err error
	switch {
	case *file != "-":
		body, err = os.ReadFile(*file)
	case os.Getenv("ISSUE_BODY") != "":
		body = []byte(os.Getenv("ISSUE_BODY"))
	default:
		body, err = io.ReadAll(io.LimitReader(os.Stdin, 1<<20))
	}
	if err != nil {
		return err
	}
	fields := issueform.Parse(string(body))
	if *field != "" {
		fmt.Println(fields[*field])
		return nil
	}
	return printJSON(fields)
}

// cmdCall is what a cron pod runs: one HTTP call, with the shared secret as
// a bearer token, that fails the Job when the app answers with an error.
func cmdCall(ctx context.Context, args []string) error {
	fs := flags("call")
	url := fs.String("url", "", "URL to call")
	method := fs.String("method", http.MethodPost, "HTTP method")
	bearer := fs.String("bearer-env", "CRON_SECRET", "environment variable that holds the bearer token")
	timeout := fs.Duration("timeout", 60*time.Second, "give up after this long")
	fs.Parse(args)
	if *url == "" {
		return errors.New("call: -url is required")
	}
	ctx, cancel := context.WithTimeout(ctx, *timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, *method, *url, nil)
	if err != nil {
		return err
	}
	if tok := os.Getenv(*bearer); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	req.Header.Set("User-Agent", "coolctl-cron/"+version)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	fmt.Printf("%s %s -> %s\n", *method, *url, resp.Status)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("the app answered %s: %s", resp.Status, bytes.TrimSpace(b))
	}
	return nil
}

func cmdServe(ctx context.Context, args []string) error {
	fs := flags("serve")
	dir := fs.String("dir", ".", "directory to serve")
	addr := fs.String("addr", "127.0.0.1:8000", "listen address")
	fs.Parse(args)
	srv := &http.Server{Addr: *addr, Handler: http.FileServer(http.Dir(*dir)), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		srv.Close()
	}()
	fmt.Printf("serving %s on http://%s\n", *dir, *addr)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
