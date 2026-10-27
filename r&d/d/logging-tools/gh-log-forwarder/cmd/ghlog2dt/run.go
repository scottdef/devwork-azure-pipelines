package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/CoolGitOrg/gh-log-forwarder/internal/config"
	"github.com/CoolGitOrg/gh-log-forwarder/internal/dynatrace"
	"github.com/CoolGitOrg/gh-log-forwarder/internal/forward"
	"github.com/CoolGitOrg/gh-log-forwarder/internal/obs"
	"github.com/CoolGitOrg/gh-log-forwarder/internal/state"
)

const allScopes = config.ScopeGitHub | config.ScopeDynatrace | config.ScopeParse | config.ScopeState

func cmdRun(ctx context.Context, e *env, args []string) error {
	cfg, fs, lf, err := setup("run", e, args, allScopes)
	if err != nil {
		return err
	}
	var (
		runID       int64
		attempt     int
		dryRun      bool
		summaryPath string
		recordsPath string
		failOnStale bool
		printConfig bool
	)
	fs.Int64Var(&runID, "run-id", 0, "ship only this workflow run (event-driven mode; needs exactly one --repo)")
	fs.IntVar(&attempt, "run-attempt", 0, "with --run-id: only this attempt (0 = all attempts)")
	fs.BoolVar(&dryRun, "dry-run", false, "write records as NDJSON instead of posting; never save state")
	fs.StringVar(&summaryPath, "summary", "-", "write the JSON summary to `file` (- = stdout, or stderr when dry-run records use stdout)")
	fs.StringVar(&recordsPath, "records-out", "-", "with --dry-run: write records to `file`")
	fs.BoolVar(&failOnStale, "fail-on-stale", false, "exit 3 if any line was too old to ship")
	fs.BoolVar(&printConfig, "print-config", false, "print the effective configuration (secrets elided) and exit")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	log, err := lf.logger(e.stderr)
	if err != nil {
		return err
	}
	if printConfig {
		fmt.Fprint(e.stdout, cfg.String())
		return nil
	}
	if err := cfg.Validate(config.Need{GitHub: true, Dynatrace: !dryRun}); err != nil {
		return usageErr("%v", err)
	}
	if attempt != 0 && runID == 0 {
		return usageErr("--run-attempt needs --run-id")
	}
	redact, _ := cfg.Redactors()
	gh := newGitHub(cfg, log)

	repos, err := resolveRepos(ctx, gh, cfg)
	if err != nil {
		return err
	}
	if runID != 0 && len(repos) != 1 {
		return usageErr("--run-id needs exactly one repository, got %d", len(repos))
	}

	var dry io.Writer
	var closeRecords func() error = func() error { return nil }
	if dryRun {
		if dry, closeRecords, err = output(recordsPath, e.stdout); err != nil {
			return err
		}
		if recordsPath == "" || recordsPath == "-" {
			if summaryPath == "" || summaryPath == "-" {
				summaryPath = "/dev/stderr"
			}
		}
	}
	ship := newShipper(cfg, log, dry)
	f := &forward.Forwarder{GH: gh, Cfg: cfg, Redact: redact, Log: log, DryRun: dryRun}

	sum := &obs.Summary{Tool: "ghlog2dt", Version: version, Command: "run", StartedAt: time.Now().UTC(), DryRun: dryRun}
	var stop error
	for _, repo := range repos {
		var rs forward.RepoSummary
		var err error
		if runID != 0 {
			rs, err = f.ShipRun(ctx, repo, runID, attempt, ship)
		} else {
			rs, err = f.Sweep(ctx, repo, ship)
		}
		if err != nil {
			rs.Error = err.Error()
			log.Error("repository failed", "repo", repo, "err", err)
			if global(err) || errors.Is(err, state.ErrIO) {
				stop = err
			}
		} else {
			log.Info("repository done", "repo", repo, "jobs", rs.Jobs, "records", rs.Records, "stale_dropped", rs.StaleDropped, "pending", rs.RunsPending)
		}
		sum.Add(rs)
		if stop != nil {
			break
		}
	}
	finish(ctx, e, cfg, log, sum, ship, gh.Calls(), gh.RateRemaining(), dryRun)
	if err := closeRecords(); err != nil {
		return err
	}
	if err := writeSummary(e, summaryPath, sum); err != nil {
		return err
	}
	switch {
	case stop != nil:
		return &exitError{codeFor(stop), nil}
	case sum.Errors > 0:
		return &exitError{exitFail, nil}
	case sum.Dynatrace.Partial > 0 || sum.Dynatrace.Rejected > 0 || sum.Dynatrace.Stale > 0:
		return &exitError{exitPartial, nil}
	case failOnStale && sum.Totals.StaleDropped > 0:
		return &exitError{exitPartial, nil}
	}
	return nil
}

// finish stamps timings and ships the self-monitoring record.
func finish(ctx context.Context, e *env, cfg *config.Config, log *slog.Logger, sum *obs.Summary, ship *dynatrace.Shipper, calls, remaining int, dry bool) {
	sum.GitHubCalls, sum.GitHubRemaining = calls, remaining
	sum.DurationMS = time.Since(sum.StartedAt).Milliseconds()
	if cfg.SelfLog && !dry && ship.Poster != nil && ctx.Err() == nil {
		if err := ship.Add(ctx, sum.SelfRecord(time.Now(), cfg.Attributes)); err == nil {
			if err := ship.Flush(ctx); err != nil {
				log.Warn("self-monitoring record not shipped", "err", err)
			}
		}
	}
	sum.Dynatrace = ship.Stats
	appendStepSummary(e, sum.WriteMarkdown)
}

func writeSummary(e *env, path string, sum *obs.Summary) error {
	if path == "/dev/stderr" {
		return sum.WriteJSON(e.stderr)
	}
	w, closeFn, err := output(path, e.stdout)
	if err != nil {
		return err
	}
	if err := sum.WriteJSON(w); err != nil {
		closeFn()
		return err
	}
	return closeFn()
}

// cmdList prints unshipped completed jobs. It reads the checkpoint but
// never writes it.
func cmdList(ctx context.Context, e *env, args []string) error {
	cfg, fs, lf, err := setup("list", e, args, config.ScopeGitHub|config.ScopeState)
	if err != nil {
		return err
	}
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	log, err := lf.logger(e.stderr)
	if err != nil {
		return err
	}
	if err := cfg.Validate(config.Need{GitHub: true}); err != nil {
		return usageErr("%v", err)
	}
	gh := newGitHub(cfg, log)
	repos, err := resolveRepos(ctx, gh, cfg)
	if err != nil {
		return err
	}
	f := &forward.Forwarder{GH: gh, Cfg: cfg, Log: log, DryRun: true}
	enc := json.NewEncoder(e.stdout)
	enc.SetEscapeHTML(false)
	now := time.Now().UTC()
	for _, repo := range repos {
		st, err := state.Load(cfg.StateDir, repo)
		if err != nil {
			return err
		}
		var rs forward.RepoSummary
		work, err := f.Discover(ctx, repo, st, now, &rs)
		if err != nil {
			return fmt.Errorf("%s: %w", repo, err)
		}
		for _, w := range work {
			for _, ref := range w.Refs {
				if err := enc.Encode(ref); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// cmdFetch turns job refs on stdin into Dynatrace records on stdout.
func cmdFetch(ctx context.Context, e *env, args []string) error {
	cfg, fs, lf, err := setup("fetch", e, args, config.ScopeGitHub|config.ScopeParse)
	if err != nil {
		return err
	}
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	log, err := lf.logger(e.stderr)
	if err != nil {
		return err
	}
	if err := cfg.Validate(config.Need{GitHub: true}); err != nil {
		return usageErr("%v", err)
	}
	redact, _ := cfg.Redactors()
	f := &forward.Forwarder{GH: newGitHub(cfg, log), Cfg: cfg, Redact: redact, Log: log, DryRun: true}
	out, closeFn, _ := output("-", e.stdout)
	sum := &obs.Summary{Tool: "ghlog2dt", Version: version, Command: "fetch", StartedAt: time.Now().UTC()}
	byRepo := map[string]*forward.RepoSummary{}
	var order []string
	err = readLines(e.stdin, func(line []byte) error {
		var ref forward.JobRef
		if err := json.Unmarshal(line, &ref); err != nil {
			return fmt.Errorf("stdin: not a job ref: %w", err)
		}
		if !config.ValidRepo(ref.Repo) || ref.Job.ID == 0 {
			return fmt.Errorf("stdin: job ref needs repo and job.id")
		}
		rs := byRepo[ref.Repo]
		if rs == nil {
			rs = &forward.RepoSummary{Repo: ref.Repo, Mode: "fetch"}
			byRepo[ref.Repo] = rs
			order = append(order, ref.Repo)
		}
		before := rs.Records
		err := f.FetchJob(ctx, ref, rs, func(b []byte) error {
			_, err := out.Write(append(b, '\n'))
			return err
		})
		if err != nil {
			return fmt.Errorf("%s job %d: %w", ref.Repo, ref.Job.ID, err)
		}
		rs.Jobs++
		log.Debug("fetched", "repo", ref.Repo, "job", ref.Job.ID, "records", rs.Records-before)
		return nil
	})
	if cerr := closeFn(); err == nil {
		err = cerr
	}
	for _, r := range order {
		sum.Add(*byRepo[r])
	}
	sum.DurationMS = time.Since(sum.StartedAt).Milliseconds()
	_ = sum.WriteJSON(e.stderr)
	return err
}

// cmdShip posts records read from stdin.
func cmdShip(ctx context.Context, e *env, args []string) error {
	cfg, fs, lf, err := setup("ship", e, args, config.ScopeDynatrace)
	if err != nil {
		return err
	}
	var dryRun bool
	fs.BoolVar(&dryRun, "dry-run", false, "validate and count records, do not post")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	log, err := lf.logger(e.stderr)
	if err != nil {
		return err
	}
	if err := cfg.Validate(config.Need{Dynatrace: !dryRun}); err != nil {
		return usageErr("%v", err)
	}
	var dry io.Writer
	if dryRun {
		dry = io.Discard
	}
	ship := newShipper(cfg, log, dry)
	sum := &obs.Summary{Tool: "ghlog2dt", Version: version, Command: "ship", StartedAt: time.Now().UTC(), DryRun: dryRun}
	n := 0
	err = readLines(e.stdin, func(line []byte) error {
		n++
		if len(line) == 0 || line[0] != '{' || !json.Valid(line) {
			return usageErr("stdin line %d: not a JSON object", n)
		}
		return ship.Add(ctx, bytes.Clone(line))
	})
	if err == nil {
		err = ship.Flush(ctx)
	}
	sum.Add(forward.RepoSummary{Repo: "-", Mode: "ship", Records: n})
	finish(ctx, e, cfg, log, sum, ship, 0, -1, true)
	if werr := sum.WriteJSON(e.stdout); err == nil {
		err = werr
	}
	if err != nil {
		return err
	}
	if sum.Dynatrace.Partial > 0 || sum.Dynatrace.Rejected > 0 || sum.Dynatrace.Stale > 0 {
		return &exitError{exitPartial, nil}
	}
	return nil
}
