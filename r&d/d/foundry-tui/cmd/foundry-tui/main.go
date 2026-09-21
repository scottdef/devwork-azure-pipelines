// Command foundry-tui manages, audits and monitors Microsoft Foundry
// model deployments from a terminal.
//
//	foundry-tui                 the screen
//	foundry-tui report          write an HTML + JSON record
//	foundry-tui test            probe deployments (API, Copilot CLI)
//	foundry-tui dispatch        render a template and run a workflow
//	foundry-tui audit           print or verify the local audit chain
//	foundry-tui perms           show what Foundry Owner covers
//	foundry-tui scaffold        copy the workflows into a repository
//
// The screen is one face of these commands, not a separate program.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	foundrytui "github.com/CoolGitOrg/foundry-tui"
	"github.com/CoolGitOrg/foundry-tui/internal/audit"
	"github.com/CoolGitOrg/foundry-tui/internal/azure"
	"github.com/CoolGitOrg/foundry-tui/internal/config"
	"github.com/CoolGitOrg/foundry-tui/internal/dispatch"
	"github.com/CoolGitOrg/foundry-tui/internal/github"
	"github.com/CoolGitOrg/foundry-tui/internal/perms"
	"github.com/CoolGitOrg/foundry-tui/internal/probe"
	"github.com/CoolGitOrg/foundry-tui/internal/report"
	"github.com/CoolGitOrg/foundry-tui/internal/run"
	"github.com/CoolGitOrg/foundry-tui/internal/tui"
)

var version = "dev" // set by -ldflags "-X main.version=..."

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := dispatchCmd(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "foundry-tui:", err)
		os.Exit(1)
	}
}

func dispatchCmd(ctx context.Context, args []string) error {
	cmd := "tui"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd, args = args[0], args[1:]
	}
	switch cmd {
	case "tui":
		return cmdTUI(ctx, args)
	case "report":
		return cmdReport(ctx, args)
	case "test":
		return cmdTest(ctx, args)
	case "dispatch":
		return cmdDispatch(ctx, args)
	case "audit":
		return cmdAudit(args)
	case "perms":
		return cmdPerms(ctx, args)
	case "scaffold":
		return cmdScaffold(args)
	case "version":
		fmt.Println("foundry-tui", version)
		return nil
	case "help":
		fmt.Println("usage: foundry-tui [tui|report|test|dispatch|audit|perms|scaffold|version] [flags]\nRun a command with -h for its flags.")
		return nil
	}
	return fmt.Errorf("unknown command %q; try 'foundry-tui help'", cmd)
}

// env is what every command needs, built once.
type env struct {
	cfg     config.Config
	r       run.Runner
	az      azure.Client
	gh      *github.Client
	ghErr   error
	perms   perms.Set
	log     *audit.Log
	history probe.History
	tfs     fs.FS
	actor   string
}

// flags returns a FlagSet carrying the flags all commands share.
func flags(name string) (*flag.FlagSet, *string) {
	fl := flag.NewFlagSet(name, flag.ExitOnError)
	return fl, fl.String("config", "", "config file (default: $FOUNDRY_TUI_CONFIG, ./foundry-tui.json, user config dir)")
}

// setup builds what every command shares. preview relaxes the az login
// check for commands that only render.
func setup(ctx context.Context, cfgPath string, needGitHub, preview bool) (*env, error) {
	cfg, err := config.Load(config.Path(cfgPath))
	if err != nil {
		return nil, err
	}
	e := &env{cfg: cfg, r: run.Exec{}, history: probe.NewHistory(cfg.StateDir)}
	e.az = azure.Client{R: e.r, Cfg: cfg}
	if e.tfs, err = dispatch.TemplateFS(foundrytui.Templates, cfg.TemplateDir); err != nil {
		return nil, err
	}
	if e.log, err = audit.Open(cfg.StateDir); err != nil {
		return nil, err
	}
	id, err := e.az.Whoami(ctx)
	switch {
	case err == nil:
		e.actor = id.User.Name
		e.perms = perms.Load(ctx, e.az)
	case preview:
		// Rendering a payload reads nothing from Azure; do not demand a login.
		e.actor, e.perms = os.Getenv("USER"), perms.Snapshot()
	default:
		return nil, fmt.Errorf("az is not signed in (run 'az login'): %w", err)
	}
	e.gh, e.ghErr = github.New(ctx, e.r, cfg.GitHubAPI, cfg.Repo)
	if e.ghErr != nil && needGitHub {
		return nil, e.ghErr
	}
	return e, nil
}

func (e *env) dispatcher() dispatch.Dispatcher {
	return dispatch.Dispatcher{R: e.r, Cfg: e.cfg, FS: e.tfs, Log: e.log, Actor: e.actor}
}

func cmdTUI(ctx context.Context, args []string) error {
	fl, cfgPath := flags("tui")
	theme := fl.String("theme", "term", "term (your terminal's colours) or acme")
	fl.Parse(args)
	e, err := setup(ctx, *cfgPath, false, false)
	if err != nil {
		return err
	}
	return tui.New(ctx, tui.Deps{
		Cfg: e.cfg, Azure: e.az, GitHub: e.gh, Perms: e.perms, Disp: e.dispatcher(), Log: e.log,
		History: e.history, TFS: e.tfs, Runner: e.r, Actor: e.actor, Version: version, Theme: *theme,
	}, nil).Run()
}

func cmdReport(ctx context.Context, args []string) error {
	fl, cfgPath := flags("report")
	out := fl.String("out", "", "directory to write into (default: report_dir from config)")
	failOn := fl.String("fail-on", "", "exit 2 if a finding of this severity exists: high or warn")
	fl.Parse(args)
	e, err := setup(ctx, *cfgPath, false, false)
	if err != nil {
		return err
	}
	dir := e.cfg.ReportDir
	if *out != "" {
		dir = *out
	}
	snap := report.Collect(ctx, report.Sources{Cfg: e.cfg, Azure: e.az, GitHub: e.gh, Perms: e.perms,
		Log: e.log, History: e.history, Version: version, Actor: e.actor})
	files, err := report.Write(e.tfs, dir, snap)
	if err != nil {
		return err
	}
	_, _ = e.log.Append(e.actor, "report.write", files.HTML, map[string]string{
		"findings_high": strconv.Itoa(snap.Count("high")), "findings_warn": strconv.Itoa(snap.Count("warn"))})
	fmt.Printf("%s\n%s\n%s\n", files.HTML, files.JSON, files.Sum)
	for _, f := range snap.Findings {
		fmt.Fprintf(os.Stderr, "%-4s %s: %s\n", f.Severity, f.Subject, f.Text)
	}
	if *failOn == "high" && snap.Count("high") > 0 || *failOn == "warn" && snap.Count("high")+snap.Count("warn") > 0 {
		os.Exit(2)
	}
	return nil
}

func cmdTest(ctx context.Context, args []string) error {
	fl, cfgPath := flags("test")
	name := fl.String("deployment", "", "deployment to probe (default: every chat deployment)")
	mode := fl.String("mode", "api", "api, copilot or both")
	asJSON := fl.Bool("json", false, "print results as JSON lines")
	fl.Parse(args)
	e, err := setup(ctx, *cfgPath, false, false)
	if err != nil {
		return err
	}
	acct, err := e.az.Account(ctx)
	if err != nil {
		return err
	}
	ds, err := e.az.Deployments(ctx)
	if err != nil {
		return err
	}
	tok, err := e.az.Token(ctx)
	if err != nil {
		return err
	}
	hc := &http.Client{Timeout: 90 * time.Second}
	failed, matched := 0, 0
	for _, d := range ds {
		if *name != "" && d.Name != *name {
			continue
		}
		matched++
		t := probe.NewTarget(acct, d)
		var results []probe.Result
		if *mode == "api" || *mode == "both" {
			results = append(results, probe.API(ctx, hc, t, tok))
		}
		if *mode == "copilot" || *mode == "both" {
			results = append(results, probe.Copilot(ctx, e.r, e.cfg.Copilot, e.tfs, t, tok))
		}
		for _, res := range results {
			_ = e.history.Append(res)
			_, _ = e.log.Append(e.actor, "probe."+res.Kind, d.Name, map[string]string{
				"ok": strconv.FormatBool(res.OK), "latency_ms": strconv.FormatInt(res.LatencyMS, 10), "detail": res.Detail})
			if *asJSON {
				json.NewEncoder(os.Stdout).Encode(res)
			} else {
				fmt.Printf("%-5v %-8s %-32s %6d ms  %s\n", res.OK, res.Kind, res.Deployment, res.LatencyMS, res.Detail)
			}
			if !res.OK && !strings.HasPrefix(res.Detail, "skipped") {
				failed++
			}
		}
	}
	if matched == 0 {
		return fmt.Errorf("no deployment named %q", *name)
	}
	if failed > 0 {
		return fmt.Errorf("%d probe(s) failed", failed)
	}
	return nil
}

// kv collects repeated -set key=value flags.
type kv map[string]string

func (m kv) String() string { return "" }
func (m kv) Set(s string) error {
	k, v, ok := strings.Cut(s, "=")
	if !ok {
		return fmt.Errorf("want key=value, got %q", s)
	}
	m[k] = v
	return nil
}

func cmdDispatch(ctx context.Context, args []string) error {
	fl, cfgPath := flags("dispatch")
	kind := fl.String("kind", "deploy", "request kind: "+strings.Join(dispatch.Kinds(), ", "))
	yes := fl.Bool("yes", false, "send it; without this flag the payload and command are only printed")
	params := kv{}
	fl.Var(params, "set", "parameter as key=value (repeatable)")
	fl.Parse(args)
	e, err := setup(ctx, *cfgPath, *yes, !*yes)
	if err != nil {
		return err
	}
	d := e.dispatcher()
	req, err := d.Render(*kind, params)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "# %s < payload   (sha256 %s)\n", req.Cmd(), req.SHA256)
	fmt.Println(string(req.Payload))
	if !*yes {
		fmt.Fprintln(os.Stderr, "# not sent: add -yes to dispatch")
		return nil
	}
	out, err := d.Send(ctx, req)
	if out != "" {
		fmt.Fprintln(os.Stderr, out)
	}
	return err
}

func cmdAudit(args []string) error {
	fl, cfgPath := flags("audit")
	verify := fl.Bool("verify", false, "check the hash chain and exit non-zero if it is broken")
	fl.Parse(args)
	cfg, err := config.Load(config.Path(*cfgPath))
	if err != nil {
		return err
	}
	log, err := audit.Open(cfg.StateDir)
	if err != nil {
		return err
	}
	recs, err := log.Read()
	if err != nil {
		return err
	}
	if *verify {
		n, err := audit.Verify(recs)
		if err != nil {
			return fmt.Errorf("%s: %d good records, then: %w", log.Path, n, err)
		}
		head := ""
		if n > 0 {
			head = recs[n-1].Hash
		}
		fmt.Printf("%s: %d records verified, head %s\n", log.Path, n, head)
		return nil
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 8, 2, ' ', 0)
	for _, r := range recs {
		fmt.Fprintf(w, "%d\t%s\t%s\t%s\t%s\t%s\n", r.Seq, r.Time.Format(time.RFC3339), r.Actor, r.Kind, r.Subject, r.Detail["request_id"])
	}
	return w.Flush()
}

func cmdPerms(ctx context.Context, args []string) error {
	fl, cfgPath := flags("perms")
	fl.Parse(args)
	cfg, err := config.Load(config.Path(*cfgPath))
	if err != nil {
		return err
	}
	set := perms.Load(ctx, azure.Client{R: run.Exec{}, Cfg: cfg})
	fmt.Printf("%s (%s)\n", set.Role, set.Source)
	w := tabwriter.NewWriter(os.Stdout, 0, 8, 2, ' ', 0)
	for _, c := range set.Matrix() {
		verdict := "allowed"
		if !c.Allowed {
			verdict = "NOT ALLOWED"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", verdict, c.What, c.Action)
	}
	return w.Flush()
}

func cmdScaffold(args []string) error {
	fl := flag.NewFlagSet("scaffold", flag.ExitOnError)
	dir := fl.String("dir", ".", "repository root to copy .github/workflows into")
	force := fl.Bool("force", false, "overwrite existing files")
	fl.Parse(args)
	names, err := foundrytui.Workflows.ReadDir(".github/workflows")
	if err != nil {
		return err
	}
	dst := filepath.Join(*dir, ".github", "workflows")
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	for _, n := range names {
		b, err := foundrytui.Workflows.ReadFile(".github/workflows/" + n.Name())
		if err != nil {
			return err
		}
		path := filepath.Join(dst, n.Name())
		if _, err := os.Stat(path); err == nil && !*force {
			fmt.Println("kept   ", path)
			continue
		}
		if err := os.WriteFile(path, b, 0o644); err != nil {
			return err
		}
		fmt.Println("wrote  ", path)
	}
	return nil
}
