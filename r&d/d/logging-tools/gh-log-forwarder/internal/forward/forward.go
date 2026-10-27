// Package forward is ghlog2dt's engine: discover completed jobs, fetch
// and parse their logs, hand records to a sink, and keep the checkpoint
// honest. The rules that make it correct:
//
//  1. job_id is the dedupe key; every attempt has its own job IDs.
//  2. Only completed jobs ship. Unfinished runs wait in the pending set
//     and are re-polled until done or pending_ttl passes.
//  3. A job is marked shipped only after the request carrying its last
//     record has completed; state is saved after that. Delivery is
//     therefore at-least-once; DQL can dedup on job ID + line number.
//  4. Lines older than max_age are dropped (or re-stamped), because
//     Dynatrace refuses logs older than 24 hours.
package forward

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"time"

	"github.com/CoolGitOrg/gh-log-forwarder/internal/config"
	"github.com/CoolGitOrg/gh-log-forwarder/internal/dynatrace"
	"github.com/CoolGitOrg/gh-log-forwarder/internal/github"
	"github.com/CoolGitOrg/gh-log-forwarder/internal/parse"
	"github.com/CoolGitOrg/gh-log-forwarder/internal/record"
	"github.com/CoolGitOrg/gh-log-forwarder/internal/state"
)

// JobRef is one unit of work, and the NDJSON line `list` prints and
// `fetch` reads.
type JobRef struct {
	Repo  string     `json:"repo"`
	Run   github.Run `json:"run"`
	Job   github.Job `json:"job"`
	Empty bool       `json:"empty,omitempty"` // never ran: nothing to download
}

// RunWork is a completed run and its unshipped jobs.
type RunWork struct {
	Run        github.Run
	Refs       []JobRef
	Incomplete bool // some job not completed yet: keep the run pending
}

// RepoSummary is the per-repository result.
type RepoSummary struct {
	Repo               string    `json:"repo"`
	Mode               string    `json:"mode"`
	WindowFrom         time.Time `json:"window_from,omitzero"`
	WindowTo           time.Time `json:"window_to,omitzero"`
	Runs               int       `json:"runs"`
	RunsPending        int       `json:"runs_pending"`
	PendingAbandoned   int       `json:"pending_abandoned"`
	Jobs               int       `json:"jobs"`
	JobsAlreadyShipped int       `json:"jobs_already_shipped"`
	JobsNotReady       int       `json:"jobs_not_ready"`
	LogBytes           int64     `json:"log_bytes"`
	Lines              int       `json:"lines"`
	Records            int       `json:"records"`
	StaleDropped       int       `json:"stale_dropped"`
	StaleRewritten     int       `json:"stale_rewritten"`
	Redactions         int       `json:"redactions"`
	Truncated          int       `json:"truncated"`
	Error              string    `json:"error,omitempty"`
}

// Forwarder holds the collaborators.
type Forwarder struct {
	GH     *github.Client
	Cfg    *config.Config
	Redact []*regexp.Regexp
	Log    *slog.Logger
	Now    func() time.Time
	DryRun bool // never save state
}

var errNotReady = errors.New("job log not available yet")

func (f *Forwarder) now() time.Time {
	if f.Now != nil {
		return f.Now().UTC()
	}
	return time.Now().UTC()
}

// Window returns the created-at window for a sweep.
func (f *Forwarder) Window(st *state.State, now time.Time) (time.Time, time.Time) {
	from := now.Add(-f.Cfg.Lookback.D())
	if !st.HighWater.IsZero() {
		from = st.HighWater.Add(-f.Cfg.Lookback.D())
	}
	if floor := now.Add(-f.Cfg.MaxAge.D()); from.Before(floor) {
		from = floor
	}
	return from, now
}

// Discover lists runs in the window plus pending runs, records
// unfinished runs as pending, and returns the work for completed runs.
func (f *Forwarder) Discover(ctx context.Context, repo string, st *state.State, now time.Time, sum *RepoSummary) ([]RunWork, error) {
	from, to := f.Window(st, now)
	sum.WindowFrom, sum.WindowTo = from, to
	runs, err := f.GH.ListRuns(ctx, repo, from, to)
	if err != nil {
		return nil, err
	}
	seen := map[int64]bool{}
	var done []github.Run
	for _, r := range runs {
		seen[r.ID] = true
		if !f.Cfg.WorkflowSelected(r.Name, r.Path) {
			continue
		}
		sum.Runs++
		if r.Status != "completed" {
			st.AddPending(r.ID, now)
			continue
		}
		done = append(done, r)
	}
	pending := make([]int64, 0, len(st.Pending))
	for id := range st.Pending {
		if !seen[id] {
			pending = append(pending, id)
		}
	}
	slices.Sort(pending)
	for _, id := range pending {
		r, err := f.GH.GetRun(ctx, repo, id)
		if github.IsNotFound(err) {
			f.Log.Warn("pending run vanished", "repo", repo, "run", id)
			st.RemovePending(id)
			continue
		}
		if err != nil {
			return nil, err
		}
		if r.Status == "completed" {
			done = append(done, r)
		}
	}
	var work []RunWork
	for _, r := range done {
		jobs, err := f.GH.ListJobs(ctx, repo, r.ID)
		if err != nil {
			return nil, err
		}
		work = append(work, f.runWork(repo, r, jobs, st, sum))
	}
	return work, nil
}

func (f *Forwarder) runWork(repo string, r github.Run, jobs []github.Job, st *state.State, sum *RepoSummary) RunWork {
	rw := RunWork{Run: r}
	for _, j := range jobs {
		if st.IsShipped(j.ID) {
			sum.JobsAlreadyShipped++
			continue
		}
		if j.Status != "completed" {
			rw.Incomplete = true
			sum.JobsNotReady++
			continue
		}
		empty := j.Conclusion == "skipped" || (j.StartedAt == nil) || (len(j.Steps) == 0 && j.RunnerName == "")
		rw.Refs = append(rw.Refs, JobRef{Repo: repo, Run: r, Job: j, Empty: empty})
	}
	return rw
}

// Sweep processes one repository end to end and saves its checkpoint.
func (f *Forwarder) Sweep(ctx context.Context, repo string, ship *dynatrace.Shipper) (RepoSummary, error) {
	sum := RepoSummary{Repo: repo, Mode: "sweep"}
	st, err := state.Load(f.Cfg.StateDir, repo)
	if err != nil {
		return sum, err
	}
	now := f.now()
	if dropped := st.Prune(now, f.Cfg.ShippedTTL.D(), f.Cfg.PendingTTL.D()); len(dropped) > 0 {
		sum.PendingAbandoned = len(dropped)
		f.Log.Warn("abandoned pending runs past pending_ttl", "repo", repo, "runs", dropped)
	}
	work, err := f.Discover(ctx, repo, st, now, &sum)
	if err != nil {
		return sum, err
	}
	perr := f.process(ctx, st, work, ship, &sum)
	if perr == nil {
		st.HighWater = sum.WindowTo
	}
	sum.RunsPending = len(st.Pending)
	if !f.DryRun {
		if err := st.Save(f.Cfg.StateDir, f.now()); err != nil {
			return sum, errors.Join(perr, err)
		}
	}
	return sum, perr
}

// ShipRun processes a single run (event-driven mode). attempt > 0
// restricts it to that attempt. The high-water mark is not touched.
func (f *Forwarder) ShipRun(ctx context.Context, repo string, runID int64, attempt int, ship *dynatrace.Shipper) (RepoSummary, error) {
	sum := RepoSummary{Repo: repo, Mode: "run"}
	st, err := state.Load(f.Cfg.StateDir, repo)
	if err != nil {
		return sum, err
	}
	now := f.now()
	st.Prune(now, f.Cfg.ShippedTTL.D(), f.Cfg.PendingTTL.D())
	r, err := f.GH.GetRun(ctx, repo, runID)
	if err != nil {
		return sum, err
	}
	sum.Runs = 1
	var perr error
	if r.Status != "completed" {
		st.AddPending(r.ID, now)
	} else {
		jobs, err := f.GH.ListJobs(ctx, repo, runID)
		if err != nil {
			return sum, err
		}
		if attempt > 0 {
			jobs = slices.DeleteFunc(jobs, func(j github.Job) bool { return j.RunAttempt != attempt })
		}
		perr = f.process(ctx, st, []RunWork{f.runWork(repo, r, jobs, st, &sum)}, ship, &sum)
	}
	sum.RunsPending = len(st.Pending)
	if !f.DryRun {
		if err := st.Save(f.Cfg.StateDir, f.now()); err != nil {
			return sum, errors.Join(perr, err)
		}
	}
	return sum, perr
}

type doneJob struct {
	id  int64
	seq int64
}

// process ships the work in order and updates st. On a fatal error it
// stops, still marking every job whose records were flushed.
func (f *Forwarder) process(ctx context.Context, st *state.State, work []RunWork, ship *dynatrace.Shipper, sum *RepoSummary) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	sem := make(chan struct{}, max(1, f.Cfg.Concurrency))

	var queue []doneJob
	commit := func() {
		i := 0
		for ; i < len(queue) && queue[i].seq <= ship.Flushed(); i++ {
			st.MarkShipped(queue[i].id, f.now())
			sum.Jobs++
		}
		queue = queue[i:]
	}
	var resolve, keep []int64
	var fatal error
	for _, rw := range work {
		notReady := rw.Incomplete
		chs := f.prefetch(ctx, rw.Refs, sem)
		for i, ref := range rw.Refs {
			res := <-chs[i]
			err := res.err
			if err == nil {
				err = f.shipJob(ctx, ref, res, ship, sum)
			}
			if res.held {
				<-sem
			}
			switch {
			case errors.Is(err, errNotReady):
				notReady = true
				sum.JobsNotReady++
				f.Log.Info("job log not ready; run stays pending", "repo", ref.Repo, "run", ref.Run.ID, "job", ref.Job.ID)
				continue
			case err != nil:
				fatal = fmt.Errorf("%s job %d: %w", ref.Repo, ref.Job.ID, err)
			}
			if fatal != nil {
				break
			}
			queue = append(queue, doneJob{ref.Job.ID, ship.Added()})
			commit()
		}
		if fatal != nil {
			break
		}
		if notReady {
			keep = append(keep, rw.Run.ID)
		} else {
			resolve = append(resolve, rw.Run.ID)
		}
	}
	if fatal == nil {
		fatal = ship.Flush(ctx)
	}
	commit()
	if fatal != nil {
		return fatal
	}
	now := f.now()
	for _, id := range keep {
		st.AddPending(id, now)
	}
	for _, id := range resolve {
		st.RemovePending(id)
	}
	return nil
}

type fetched struct {
	data      []byte
	truncated bool
	held      bool
	err       error
}

// sem bounds concurrent downloads *and* downloaded-but-unprocessed logs.
// It is acquired in job order by one dispatcher, so the consumer, which
// also goes in order, can never wait on a job starved of a slot.
func (f *Forwarder) prefetch(ctx context.Context, refs []JobRef, sem chan struct{}) []chan fetched {
	chs := make([]chan fetched, len(refs))
	for i := range chs {
		chs[i] = make(chan fetched, 1)
	}
	go func() {
		for i, ref := range refs {
			if ref.Empty {
				chs[i] <- fetched{}
				continue
			}
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				for j := i; j < len(refs); j++ {
					chs[j] <- fetched{err: ctx.Err()}
				}
				return
			}
			go func(i int, ref JobRef) {
				res := f.download(ctx, ref)
				res.held = true
				chs[i] <- res
			}(i, ref)
		}
	}()
	return chs
}

func (f *Forwarder) download(ctx context.Context, ref JobRef) fetched {
	rc, err := f.GH.JobLog(ctx, ref.Repo, ref.Job.ID)
	if github.IsNotFound(err) {
		return fetched{err: errNotReady}
	}
	if err != nil {
		return fetched{err: err}
	}
	defer rc.Close()
	data, err := io.ReadAll(io.LimitReader(rc, f.Cfg.MaxLogBytes+1))
	if err != nil {
		return fetched{err: fmt.Errorf("reading log: %w", err)}
	}
	res := fetched{data: data}
	if int64(len(data)) > f.Cfg.MaxLogBytes {
		res.data, res.truncated = data[:f.Cfg.MaxLogBytes], true
	}
	return res
}

// shipJob emits one job's records into the shipper.
func (f *Forwarder) shipJob(ctx context.Context, ref JobRef, res fetched, ship *dynatrace.Shipper, sum *RepoSummary) error {
	attrs := f.JobAttrs(ref)
	inline := attrs
	if f.Cfg.DT.HeaderAttrs {
		if err := ship.SetHeader(ctx, record.HeaderValue(attrs)); err != nil {
			return err
		}
		inline = nil
	}
	return f.EmitJob(ref, res.data, res.truncated, inline, sum, func(b []byte) error { return ship.Add(ctx, b) })
}

// FetchJob downloads and emits one job (the `fetch` subcommand path).
func (f *Forwarder) FetchJob(ctx context.Context, ref JobRef, sum *RepoSummary, sink func([]byte) error) error {
	var res fetched
	if !ref.Empty {
		res = f.download(ctx, ref)
		if res.err != nil {
			return res.err
		}
	}
	return f.EmitJob(ref, res.data, res.truncated, f.JobAttrs(ref), sum, sink)
}

// JobAttrs are the attributes shared by every line of a job. Names
// follow OpenTelemetry CI/CD and VCS semantic conventions where one
// exists; GitHub-specific extras live under gh.*.
func (f *Forwarder) JobAttrs(ref JobRef) []record.KV {
	keys := make([]string, 0, len(f.Cfg.Attributes))
	for k := range f.Cfg.Attributes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	kv := make([]record.KV, 0, len(keys)+16)
	for _, k := range keys {
		kv = append(kv, record.KV{K: k, V: f.Cfg.Attributes[k]})
	}
	r, j := ref.Run, ref.Job
	add := func(k, v string) {
		if v != "" {
			kv = append(kv, record.KV{K: k, V: v})
		}
	}
	add("vcs.repository.name", ref.Repo)
	add("vcs.ref.head.name", r.HeadBranch)
	add("vcs.ref.head.revision", r.HeadSHA)
	add("cicd.pipeline.name", r.Name)
	add("cicd.pipeline.run.id", strconv.FormatInt(r.ID, 10))
	add("cicd.pipeline.task.name", j.Name)
	add("cicd.pipeline.task.run.id", strconv.FormatInt(j.ID, 10))
	add("cicd.pipeline.result", j.Conclusion)
	kv = append(kv, record.KV{K: "gh.run_attempt", V: j.RunAttempt})
	add("gh.event", r.Event)
	add("gh.actor", r.Actor.Login)
	add("gh.runner.name", j.RunnerName)
	add("gh.workflow.path", r.Path)
	add("gh.job.url", j.HTMLURL)
	return kv
}

// EmitJob parses one job log and calls sink with each encoded record.
func (f *Forwarder) EmitJob(ref JobRef, data []byte, truncated bool, attrs []record.KV, sum *RepoSummary, sink func([]byte) error) error {
	now := f.now()
	cutoff := now.Add(-f.Cfg.MaxAge.D())
	fallback := now
	if ref.Job.StartedAt != nil {
		fallback = ref.Job.StartedAt.UTC()
	}
	steps := make([]parse.Step, 0, len(ref.Job.Steps))
	for _, s := range ref.Job.Steps {
		if s.StartedAt != nil {
			steps = append(steps, parse.Step{Number: s.Number, Name: s.Name, Start: s.StartedAt.UTC()})
		}
	}
	sum.LogBytes += int64(len(data))

	var b record.Builder
	build := func(ts time.Time, orig time.Time, text, sev, marker string, n int, step *parse.Step, trunc bool) []byte {
		b.Reset()
		b.Str("timestamp", ts.Format(time.RFC3339Nano))
		b.Str("content", text)
		b.Str("severity", sev)
		b.Int("gh.line", int64(n))
		if step != nil {
			b.Int("gh.step.number", int64(step.Number))
			b.Str("gh.step.name", step.Name)
		}
		if marker != "" {
			b.Str("gh.log.marker", marker)
		}
		if trunc {
			b.Bool("gh.log.truncated", true)
		}
		if !orig.IsZero() {
			b.Str("gh.log.timestamp_original", orig.Format(time.RFC3339Nano))
		}
		for _, a := range attrs {
			b.Any(a)
		}
		return b.Bytes()
	}

	last := 0
	ps, err := parse.Parse(bytes.NewReader(data), parse.Options{
		Steps:     steps,
		Redact:    f.Redact,
		MaxLine:   f.Cfg.MaxLineBytes,
		DropNoise: f.Cfg.DropNoise,
		StripANSI: f.Cfg.StripANSI,
	}, func(l parse.Line) error {
		last = l.N
		ts := l.Time
		if ts.IsZero() {
			ts = fallback
		}
		var orig time.Time
		if ts.Before(cutoff) {
			if f.Cfg.Stale != "rewrite" {
				sum.StaleDropped++
				return nil
			}
			orig, ts = ts, now
			sum.StaleRewritten++
		}
		sum.Records++
		return sink(build(ts, orig, l.Text, l.Severity, l.Marker, l.N, l.Step, l.Truncated))
	})
	sum.Lines += ps.Lines
	sum.Redactions += ps.Redactions
	sum.Truncated += ps.Truncated
	if err != nil {
		return err
	}
	if truncated {
		sum.Records++
		msg := fmt.Sprintf("[ghlog2dt] job log truncated after %d bytes (max_log_bytes)", f.Cfg.MaxLogBytes)
		return sink(build(now, time.Time{}, msg, parse.Warn, "", last+1, nil, true))
	}
	return nil
}
