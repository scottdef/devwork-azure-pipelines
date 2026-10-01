// Command adoclone duplicates an Azure DevOps project into a new project in
// the same organization.
//
// Exit codes: 0 ok, 1 component error (safe to rerun; it resumes from the
// checkpoint), 2 bad invocation, 3 finished with item failures (see
// todo.json), 4 verify found problems.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/coolado/adoclone/internal/client"
	"github.com/coolado/adoclone/internal/clone"
	"github.com/coolado/adoclone/internal/config"
	"github.com/coolado/adoclone/internal/metrics"
	"github.com/coolado/adoclone/internal/report"
	"github.com/coolado/adoclone/internal/state"
	"github.com/coolado/adoclone/internal/verify"
)

var version = "dev"

func main() { os.Exit(run(os.Args[1:])) }

func run(args []string) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		fmt.Fprintln(os.Stderr, config.Usage)
		return 2
	}
	cmd := args[0]
	switch cmd {
	case "version":
		fmt.Println(version)
		return 0
	case "components":
		for _, c := range clone.Components {
			fmt.Printf("%d  %-17s %s\n", c.Phase, c.Name, c.About)
		}
		return 0
	case "plan", "clone", "verify", "report", "cleanup":
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s\n", cmd, config.Usage)
		return 2
	}
	cfg, err := config.Parse(cmd, args[1:], os.Getenv, clone.Names())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	log := newLogger(cfg.LogLevel)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	m := metrics.New()
	m.Set("adoclone_info", 1, "version", version, "command", cmd, "source", cfg.Source, "target", cfg.Target, "dry_run", fmt.Sprint(cfg.DryRun))
	if cfg.MetricsAddr != "" {
		shutdown, err := m.Serve(cfg.MetricsAddr, log)
		if err != nil {
			log.Error("metrics", "err", err)
			return 2
		}
		defer func() {
			if cfg.MetricsHold > 0 {
				log.Info("holding metrics endpoint open", "for", cfg.MetricsHold.String())
				time.Sleep(cfg.MetricsHold)
			}
			sctx, c := context.WithTimeout(context.Background(), 5*time.Second)
			defer c()
			_ = shutdown(sctx)
		}()
	}

	if err := os.MkdirAll(cfg.WorkDir, 0o700); err != nil {
		log.Error("work dir", "path", cfg.WorkDir, "err", err)
		return 2
	}
	c := client.New(cfg.Org, cfg.PAT, log, cfg.DryRun || cmd == "plan")
	c.Metrics = m
	st, err := state.Load(cfg.StatePath)
	if err != nil {
		log.Error("checkpoint", "path", cfg.StatePath, "err", err)
		return 1
	}
	defer st.Close()
	if c.DryRun && cmd == "clone" {
		st.SetTodoFile("todo-dryrun.json") // keep dry-run findings out of the real run's list
	}

	x := &clone.Ctx{
		C: c, S: st, M: m, Log: log, Src: cfg.Source, Tgt: cfg.Target, Getenv: os.Getenv,
		Opt: clone.Options{
			BypassRules: cfg.BypassRules, Visibility: cfg.Visibility, RepoMode: cfg.RepoMode,
			VarGroupMode: cfg.VarGroupMode, SecureFilesDir: cfg.SecureFilesDir, Git: cfg.Git,
			WorkDir: cfg.WorkDir, PAT: cfg.PAT, AdoptExisting: cfg.AdoptExisting,
		},
	}
	log.Info("start", "command", cmd, "org", cfg.Org, "source", cfg.Source, "target", cfg.Target, "dryRun", c.DryRun, "version", version)

	switch cmd {
	case "plan":
		res := verify.Result{Source: cfg.Source, Counts: verify.Counts(ctx, c, cfg.Source, ""), Todos: len(st.Todos())}
		os.Stdout.Write(res.JSON())
		return 0

	case "clone":
		if err := clone.Run(ctx, x, cfg.Components); err != nil {
			log.Error("clone stopped; fix the cause and rerun to resume", "err", err)
			return 1
		}
		if n := x.Failures(); n > 0 {
			log.Warn("clone finished with item failures; see todo.json", "failures", n, "todo", st.TodoPath())
			return 3
		}
		log.Info("clone finished", "todos", len(st.Todos()), "todo", st.TodoPath())
		return 0

	case "verify", "report":
		res := verify.Result{Source: cfg.Source, Target: cfg.Target, Counts: verify.Counts(ctx, c, cfg.Source, cfg.Target), Todos: len(st.Todos())}
		if fc, err := verify.Fields(ctx, c, st, cfg.Source, cfg.Target, cfg.SamplePct); err != nil {
			log.Error("work item sample", "err", err)
		} else {
			res.Fields = fc
		}
		os.Stdout.Write(res.JSON())
		for _, r := range res.Counts {
			if r.Missing() {
				log.Warn("missing in target", "component", r.Component, "source", r.Source, "target", r.Target, "note", r.Note)
			}
		}
		if cmd == "report" {
			if err := report.Write(cfg.Out, res, st.Todos()); err != nil {
				log.Error("report", "err", err)
				return 1
			}
			log.Info("report written", "path", cfg.Out)
			return 0
		}
		if res.Problems() > 0 {
			return 4
		}
		return 0

	case "cleanup":
		if os.Getenv("CONFIRM") != cfg.Target {
			log.Error("cleanup deletes the target project; set CONFIRM to the target project's name")
			return 2
		}
		if err := clone.Cleanup(ctx, x); err != nil {
			log.Error("cleanup", "err", err)
			return 1
		}
		if c.DryRun {
			log.Info("dry run: nothing was deleted; rerun with -dry-run=false")
		}
		return 0
	}
	return 2
}

func newLogger(level string) *slog.Logger {
	var l slog.Level
	if err := l.UnmarshalText([]byte(strings.ToUpper(level))); err != nil || level == "" {
		l = slog.LevelInfo
	}
	// Logs go to stderr so stdout stays clean JSON for plan/verify.
	return slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: l}))
}
