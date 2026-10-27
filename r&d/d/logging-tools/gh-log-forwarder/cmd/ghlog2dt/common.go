package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime"
	"sort"
	"strings"

	"github.com/CoolGitOrg/gh-log-forwarder/internal/config"
	"github.com/CoolGitOrg/gh-log-forwarder/internal/dynatrace"
	"github.com/CoolGitOrg/gh-log-forwarder/internal/github"
	"github.com/CoolGitOrg/gh-log-forwarder/internal/state"
)

// codeFor maps an error to an exit status.
func codeFor(err error) int {
	switch {
	case err == nil:
		return exitOK
	case errors.Is(err, github.ErrAuth), errors.Is(err, dynatrace.ErrAuth):
		return exitAuth
	case errors.Is(err, github.ErrRateLimited), errors.Is(err, dynatrace.ErrBudget):
		return exitBudget
	case errors.Is(err, state.ErrIO):
		return exitState
	}
	var x *exitError
	if errors.As(err, &x) {
		return x.code
	}
	return exitFail
}

// global errors stop a multi-repo run: every repo would hit them.
func global(err error) bool {
	c := codeFor(err)
	return c == exitAuth || c == exitBudget || errors.Is(err, context.Canceled)
}

type logFlags struct {
	format string
	level  string
}

func (l *logFlags) bind(fs *flag.FlagSet) {
	fs.StringVar(&l.format, "log-format", "json", "stderr log format: json | text")
	fs.StringVar(&l.level, "log-level", "info", "debug | info | warn | error")
}

func (l *logFlags) logger(w io.Writer) (*slog.Logger, error) {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(l.level)); err != nil {
		return nil, usageErr("--log-level: %v", err)
	}
	o := &slog.HandlerOptions{Level: lvl}
	switch l.format {
	case "json":
		return slog.New(slog.NewJSONHandler(w, o)).With("tool", "ghlog2dt"), nil
	case "text":
		return slog.New(slog.NewTextHandler(w, o)), nil
	}
	return nil, usageErr("--log-format must be json or text")
}

// setup loads config (file, env), binds flags, parses args.
func setup(name string, e *env, args []string, scope config.Scope) (*config.Config, *flag.FlagSet, *logFlags, error) {
	path, explicit := config.ConfigPath(args, e.getenv)
	cfg, err := config.Load(path, explicit)
	if err != nil {
		return nil, nil, nil, usageErr("%v", err)
	}
	if err := cfg.ApplyEnv(e.getenv); err != nil {
		return nil, nil, nil, usageErr("%v", err)
	}
	fs := flag.NewFlagSet("ghlog2dt "+name, flag.ContinueOnError)
	fs.SetOutput(e.stderr)
	cfg.BindFlags(fs, scope)
	lf := &logFlags{}
	lf.bind(fs)
	return cfg, fs, lf, nil
}

func parseFlags(fs *flag.FlagSet, args []string) error {
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return &exitError{code: exitOK}
		}
		return &exitError{code: exitUsage}
	}
	if fs.NArg() > 0 {
		return usageErr("unexpected arguments: %s", strings.Join(fs.Args(), " "))
	}
	return nil
}

func newGitHub(cfg *config.Config, log *slog.Logger) *github.Client {
	gh := github.New(cfg.GitHubAPI, cfg.GitHubToken)
	gh.APIVersion = cfg.APIVersion
	gh.UserAgent = "ghlog2dt/" + version
	gh.Log = log
	return gh
}

func newShipper(cfg *config.Config, log *slog.Logger, dry io.Writer) *dynatrace.Shipper {
	s := &dynatrace.Shipper{
		MaxBytes:   cfg.DT.BatchBytes,
		MaxRecords: cfg.DT.BatchRecords,
		DryRun:     dry,
		Log:        log,
	}
	if dry == nil {
		c := dynatrace.NewClient(cfg.DT.URL, cfg.DT.Token, cfg.DT.AuthScheme, cfg.DT.Timeout.D())
		c.Gzip = cfg.DT.Gzip
		c.MaxRetries = cfg.DT.MaxRetries
		c.Log = log
		s.Poster = c
	}
	return s
}

// resolveRepos turns --repo/--all/--include/--exclude into a list.
func resolveRepos(ctx context.Context, gh *github.Client, cfg *config.Config) ([]string, error) {
	set := map[string]bool{}
	for _, r := range cfg.Targets {
		if cfg.Selected(r) {
			set[r] = true
		}
	}
	if cfg.All {
		all, err := gh.InstallationRepos(ctx)
		if err != nil {
			return nil, fmt.Errorf("listing installation repositories: %w", err)
		}
		for _, r := range all {
			if cfg.Selected(r) {
				set[r] = true
			}
		}
	}
	if len(set) == 0 {
		return nil, usageErr("no target repositories: use --repo, --all or GHLOG_TARGET_REPOS")
	}
	out := make([]string, 0, len(set))
	for r := range set {
		out = append(out, r)
	}
	sort.Strings(out)
	return out, nil
}

// output opens "-" as w, anything else as a file to create.
func output(path string, w io.Writer) (io.Writer, func() error, error) {
	if path == "" || path == "-" {
		return w, func() error { return nil }, nil
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, nil, err
	}
	bw := bufio.NewWriter(f)
	return bw, func() error {
		if err := bw.Flush(); err != nil {
			f.Close()
			return err
		}
		return f.Close()
	}, nil
}

// appendStepSummary writes Markdown to $GITHUB_STEP_SUMMARY if set.
func appendStepSummary(e *env, write func(io.Writer) error) {
	p := e.getenv("GITHUB_STEP_SUMMARY")
	if p == "" {
		return
	}
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_ = write(f)
}

// readLines calls fn for each non-empty line of r; lines may be long.
func readLines(r io.Reader, fn func([]byte) error) error {
	br := bufio.NewReaderSize(r, 1<<20)
	for {
		line, err := br.ReadBytes('\n')
		if len(line) > 0 {
			line = []byte(strings.TrimRight(string(line), "\r\n"))
			if len(line) > 0 {
				if ferr := fn(line); ferr != nil {
					return ferr
				}
			}
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func cmdVersion(_ context.Context, e *env, args []string) error {
	fs := flag.NewFlagSet("ghlog2dt version", flag.ContinueOnError)
	fs.SetOutput(e.stderr)
	asJSON := fs.Bool("json", false, "print JSON")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if *asJSON {
		fmt.Fprintf(e.stdout, `{"version":%q,"commit":%q,"go":%q,"platform":"%s/%s"}`+"\n",
			version, commit, runtime.Version(), runtime.GOOS, runtime.GOARCH)
		return nil
	}
	fmt.Fprintf(e.stdout, "ghlog2dt %s (%s) %s %s/%s\n", version, commit, runtime.Version(), runtime.GOOS, runtime.GOARCH)
	return nil
}
