package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/CoolGitOrg/gh-log-forwarder/internal/config"
	"github.com/CoolGitOrg/gh-log-forwarder/internal/state"
)

const checkpointUsage = `usage:
  ghlog2dt checkpoint show   --repo owner/name [--state dir]
  ghlog2dt checkpoint merge  [-o file] file1.json file2.json ...
  ghlog2dt checkpoint prune  --repo owner/name [--state dir]
  ghlog2dt checkpoint mark   --repo owner/name --job ID [--job ID ...]
  ghlog2dt checkpoint rewind --repo owner/name --to 6h|2026-10-27T08:00:00Z [--forget]
`

func cmdCheckpoint(ctx context.Context, e *env, args []string) error {
	if len(args) == 0 {
		fmt.Fprint(e.stderr, checkpointUsage)
		return &exitError{code: exitUsage}
	}
	sub, args := args[0], args[1:]
	if sub == "merge" {
		return checkpointMerge(e, args)
	}
	cfg, fs, _, err := setup("checkpoint "+sub, e, args, config.ScopeState)
	if err != nil {
		return err
	}
	var repo, to string
	var forget bool
	var jobs []int64
	fs.StringVar(&repo, "repo", "", "repository `owner/name`")
	if sub == "mark" {
		fs.Func("job", "job `ID` to mark shipped; repeatable", func(s string) error {
			n, err := strconv.ParseInt(s, 10, 64)
			if err != nil || n <= 0 {
				return fmt.Errorf("bad job id %q", s)
			}
			jobs = append(jobs, n)
			return nil
		})
	}
	if sub == "rewind" {
		fs.StringVar(&to, "to", "", "new high-water mark: a duration ago (6h) or an RFC 3339 time")
		fs.BoolVar(&forget, "forget", false, "also forget jobs shipped since then, so they are sent again (replay)")
	}
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if !config.ValidRepo(repo) {
		return usageErr("--repo owner/name is required")
	}
	st, err := state.Load(cfg.StateDir, repo)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	switch sub {
	case "show":
		b, err := st.Marshal()
		if err != nil {
			return err
		}
		_, err = e.stdout.Write(b)
		return err
	case "prune":
		before := len(st.Shipped)
		dropped := st.Prune(now, cfg.ShippedTTL.D(), cfg.PendingTTL.D())
		fmt.Fprintf(e.stderr, "abandoned %d pending runs; %d shipped jobs remain\n", len(dropped), len(st.Shipped))
		if len(dropped) == 0 && len(st.Shipped) == before {
			return nil // unchanged: do not rewrite (keeps the state branch quiet)
		}
	case "mark":
		if len(jobs) == 0 {
			return usageErr("mark needs at least one --job")
		}
		for _, j := range jobs {
			st.MarkShipped(j, now)
		}
	case "rewind":
		t, err := parseWhen(to, now)
		if err != nil {
			return usageErr("--to: %v", err)
		}
		if now.Sub(t) > config.DTMaxLogAge {
			return usageErr("--to %s is more than 24h ago; Dynatrace would refuse those logs", t.Format(time.RFC3339))
		}
		st.HighWater = t
		if forget {
			n := 0
			for id, at := range st.Shipped {
				if !at.Before(t) {
					delete(st.Shipped, id)
					n++
				}
			}
			fmt.Fprintf(e.stderr, "forgot %d shipped jobs; they will be sent again\n", n)
		}
	default:
		fmt.Fprint(e.stderr, checkpointUsage)
		return &exitError{code: exitUsage}
	}
	return st.Save(cfg.StateDir, now)
}

func parseWhen(s string, now time.Time) (time.Time, error) {
	if d, err := time.ParseDuration(s); err == nil {
		return now.Add(-d), nil
	}
	return time.Parse(time.RFC3339, s)
}

// checkpointMerge merges any number of files for the same repo. The
// commit-state script uses it to reconcile concurrent writers.
func checkpointMerge(e *env, args []string) error {
	fs := flag.NewFlagSet("ghlog2dt checkpoint merge", flag.ContinueOnError)
	fs.SetOutput(e.stderr)
	out := fs.String("o", "-", "write merged state to `file`")
	if err := fs.Parse(args); err != nil {
		return &exitError{code: exitUsage}
	}
	if fs.NArg() < 1 {
		return usageErr("merge needs at least one file")
	}
	var merged *state.State
	for _, p := range fs.Args() {
		s, err := state.ReadFile(p)
		if err != nil {
			return fmt.Errorf("%w: %w", state.ErrIO, err)
		}
		if merged == nil {
			merged = s
			continue
		}
		if merged, err = state.Merge(merged, s); err != nil {
			return usageErr("%v", err)
		}
	}
	b, err := merged.Marshal()
	if err != nil {
		return err
	}
	if *out == "-" {
		_, err = e.stdout.Write(b)
		return err
	}
	tmp := *out + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return fmt.Errorf("%w: %w", state.ErrIO, err)
	}
	if err := os.Rename(tmp, *out); err != nil {
		return fmt.Errorf("%w: %w", state.ErrIO, err)
	}
	return nil
}
