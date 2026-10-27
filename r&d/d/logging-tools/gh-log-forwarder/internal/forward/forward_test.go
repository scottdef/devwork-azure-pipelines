package forward

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/CoolGitOrg/gh-log-forwarder/internal/config"
	"github.com/CoolGitOrg/gh-log-forwarder/internal/dynatrace"
	"github.com/CoolGitOrg/gh-log-forwarder/internal/github"
	"github.com/CoolGitOrg/gh-log-forwarder/internal/state"
	"github.com/CoolGitOrg/gh-log-forwarder/internal/testutil"
)

const repo = "CoolGitOrg/app-a"

var t0 = time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)

func tp(t time.Time) *time.Time { return &t }

type rig struct {
	gh    *testutil.GitHub
	dt    *testutil.Dynatrace
	cfg   *config.Config
	now   time.Time
	f     *Forwarder
	ghc   *github.Client
	sleep func(context.Context, time.Duration) error
}

func newRig(t *testing.T) *rig {
	t.Helper()
	r := &rig{gh: testutil.NewGitHub(), dt: testutil.NewDynatrace(), now: t0}
	t.Cleanup(r.gh.Close)
	t.Cleanup(r.dt.Close)
	r.gh.Token = "ghs_test"
	cfg := config.Defaults()
	cfg.StateDir = t.TempDir()
	cfg.Targets = []string{repo}
	cfg.DT.URL = r.dt.URL()
	cfg.DT.Token = "dt0c01.test"
	cfg.Attributes["dt.security_context"] = "ci"
	r.cfg = cfg
	r.ghc = github.New(r.gh.URL(), "ghs_test")
	r.ghc.Sleep = func(context.Context, time.Duration) error { return nil }
	redact, _ := cfg.Redactors()
	r.f = &Forwarder{
		GH: r.ghc, Cfg: cfg, Redact: redact,
		Log: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Now: func() time.Time { return r.now },
	}
	return r
}

func (r *rig) shipper() *dynatrace.Shipper {
	c := dynatrace.NewClient(r.cfg.DT.URL, r.cfg.DT.Token, "Api-Token", 5*time.Second)
	c.Sleep = func(context.Context, time.Duration) error { return nil }
	c.MaxRetries = 1
	return &dynatrace.Shipper{Poster: c, MaxBytes: r.cfg.DT.BatchBytes, MaxRecords: 3}
}

func run(id int64, created time.Time, status, conclusion string) github.Run {
	return github.Run{
		ID: id, Name: "CI", Path: ".github/workflows/ci.yml", HeadBranch: "main", HeadSHA: "abc123",
		Event: "push", Status: status, Conclusion: conclusion, RunAttempt: 1,
		CreatedAt: created, UpdatedAt: created, Actor: github.Actor{Login: "octocat"},
	}
}

func job(id, runID int64, attempt int, status, conclusion string, start time.Time) github.Job {
	return github.Job{
		ID: id, RunID: runID, RunAttempt: attempt, Name: "build", Status: status, Conclusion: conclusion,
		StartedAt: tp(start), CompletedAt: tp(start.Add(time.Minute)), RunnerName: "GitHub Actions 7",
		HTMLURL: "https://github.com/" + repo + "/actions/runs/1/job/1",
		Steps: []github.Step{
			{Name: "Set up job", Number: 1, StartedAt: tp(start)},
			{Name: "Run make", Number: 2, StartedAt: tp(start.Add(time.Second))},
		},
	}
}

func byJob(recs []map[string]any) map[string]int {
	m := map[string]int{}
	for _, r := range recs {
		m[r["cicd.pipeline.task.run.id"].(string)]++
	}
	return m
}

func TestSweepLifecycle(t *testing.T) {
	r := newRig(t)
	g := r.gh
	start := t0.Add(-30 * time.Minute)
	g.Runs[repo] = []github.Run{
		run(99, t0.Add(-2*time.Hour), "completed", "success"),     // before the first window
		run(100, start, "completed", "failure"),                   // ships now
		run(101, t0.Add(-10*time.Minute), "in_progress", ""),      // pending
		run(102, t0.Add(-20*time.Minute), "completed", "success"), // log not uploaded yet
	}
	g.Jobs[99] = []github.Job{job(990, 99, 1, "completed", "success", t0.Add(-2*time.Hour))}
	g.Logs[990] = testutil.Log(t0.Add(-2*time.Hour), "old")
	g.Jobs[100] = []github.Job{
		job(1001, 100, 1, "completed", "failure", start),
		{ID: 1002, RunID: 100, RunAttempt: 1, Name: "deploy", Status: "completed", Conclusion: "skipped"},
	}
	g.Logs[1001] = testutil.Log(start,
		"##[group]Run make",
		"go build ./...",
		"token ghp_abcdefghijklmnopqrstuvwxyz0123456789 printed",
		"##[endgroup]",
		"##[error]Process completed with exit code 2.")
	g.Jobs[101] = []github.Job{job(1011, 101, 1, "in_progress", "", t0.Add(-10*time.Minute))}
	g.Jobs[102] = []github.Job{job(1021, 102, 1, "completed", "success", t0.Add(-20*time.Minute))}

	ctx := context.Background()

	// Sweep 1.
	sum, err := r.f.Sweep(ctx, repo, r.shipper())
	if err != nil {
		t.Fatal(err)
	}
	recs := r.dt.Snapshot()
	if got := byJob(recs); got["1001"] != 4 || len(got) != 1 {
		t.Fatalf("sweep 1 shipped %v (want 4 lines of job 1001: endgroup dropped)", got)
	}
	first := recs[0]
	for k, want := range map[string]any{
		"content": "Run make", "severity": "INFO", "gh.log.marker": "group", "gh.line": float64(1),
		"gh.step.number": float64(1), "log.source": "github-actions", "dt.security_context": "ci",
		"vcs.repository.name": repo, "vcs.ref.head.name": "main", "vcs.ref.head.revision": "abc123",
		"cicd.pipeline.name": "CI", "cicd.pipeline.run.id": "100", "cicd.pipeline.task.name": "build",
		"cicd.pipeline.result": "failure", "gh.run_attempt": float64(1), "gh.actor": "octocat",
	} {
		if first[k] != want {
			t.Errorf("record[%q] = %v, want %v", k, first[k], want)
		}
	}
	if !strings.Contains(recs[2]["content"].(string), "[REDACTED]") || strings.Contains(recs[2]["content"].(string), "ghp_") {
		t.Errorf("token not redacted: %v", recs[2]["content"])
	}
	if recs[3]["severity"] != "ERROR" {
		t.Errorf("error line severity %v", recs[3]["severity"])
	}
	if sum.Jobs != 2 || sum.JobsNotReady != 1 || sum.Records != 4 || sum.RunsPending != 2 {
		t.Errorf("summary %+v", sum)
	}
	st, _ := state.Load(r.cfg.StateDir, repo)
	if !st.IsShipped(1001) || !st.IsShipped(1002) || st.IsShipped(1021) {
		t.Errorf("shipped %v", st.Shipped)
	}
	if _, ok := st.Pending[101]; !ok {
		t.Error("in-progress run 101 not pending")
	}
	if _, ok := st.Pending[102]; !ok {
		t.Error("run 102 (log 404) not pending")
	}
	if !st.HighWater.Equal(t0) {
		t.Errorf("high water %s", st.HighWater)
	}

	// Sweep 2, 15 minutes later: 101 finished, 102's log uploaded.
	r.now = t0.Add(15 * time.Minute)
	g.Lock()
	g.Runs[repo][2] = run(101, t0.Add(-10*time.Minute), "completed", "success")
	g.Jobs[101] = []github.Job{job(1011, 101, 1, "completed", "success", t0.Add(-10*time.Minute))}
	g.Logs[1011] = testutil.Log(t0.Add(-10*time.Minute), "a", "b")
	g.Logs[1021] = testutil.Log(t0.Add(-20*time.Minute), "c")
	g.Unlock()
	if _, err := r.f.Sweep(ctx, repo, r.shipper()); err != nil {
		t.Fatal(err)
	}
	got := byJob(r.dt.Snapshot())
	if got["1001"] != 4 || got["1011"] != 2 || got["1021"] != 1 || len(got) != 3 {
		t.Fatalf("after sweep 2: %v", got)
	}
	st, _ = state.Load(r.cfg.StateDir, repo)
	if len(st.Pending) != 0 {
		t.Errorf("pending should be empty: %v", st.Pending)
	}

	// Sweep 3: nothing new, nothing re-shipped.
	r.now = t0.Add(30 * time.Minute)
	if _, err := r.f.Sweep(ctx, repo, r.shipper()); err != nil {
		t.Fatal(err)
	}
	if n := len(r.dt.Snapshot()); n != 7 {
		t.Fatalf("sweep 3 shipped duplicates: %d records", n)
	}
}

func TestStaleLinesDroppedOrRewritten(t *testing.T) {
	for _, mode := range []string{"drop", "rewrite"} {
		t.Run(mode, func(t *testing.T) {
			r := newRig(t)
			r.cfg.Stale = mode
			r.gh.Runs[repo] = []github.Run{run(1, t0.Add(-time.Hour), "completed", "success")}
			r.gh.Jobs[1] = []github.Job{job(11, 1, 1, "completed", "success", t0.Add(-time.Hour))}
			// A long job: first line 25h old, last line fresh.
			r.gh.Logs[11] = testutil.Log(t0.Add(-25*time.Hour), "ancient") + testutil.Log(t0.Add(-time.Minute), "fresh")[3:]
			sum, err := r.f.Sweep(context.Background(), repo, r.shipper())
			if err != nil {
				t.Fatal(err)
			}
			recs := r.dt.Snapshot()
			if mode == "drop" {
				if len(recs) != 1 || recs[0]["content"] != "fresh" || sum.StaleDropped != 1 {
					t.Fatalf("recs %v sum %+v", recs, sum)
				}
				return
			}
			if len(recs) != 2 || sum.StaleRewritten != 1 || recs[0]["gh.log.timestamp_original"] == nil {
				t.Fatalf("recs %v sum %+v", recs, sum)
			}
		})
	}
}

func TestAuthFailureKeepsCheckpoint(t *testing.T) {
	r := newRig(t)
	r.gh.Runs[repo] = []github.Run{run(1, t0.Add(-time.Hour), "completed", "success")}
	r.gh.Jobs[1] = []github.Job{job(11, 1, 1, "completed", "success", t0.Add(-time.Hour))}
	r.gh.Logs[11] = testutil.Log(t0.Add(-time.Hour), "x")
	r.dt.Status = 401
	_, err := r.f.Sweep(context.Background(), repo, r.shipper())
	if !errors.Is(err, dynatrace.ErrAuth) {
		t.Fatalf("err=%v", err)
	}
	st, _ := state.Load(r.cfg.StateDir, repo)
	if st.IsShipped(11) || !st.HighWater.IsZero() {
		t.Fatalf("checkpoint advanced past a failed ship: %+v", st)
	}
	r.dt.Status = 0
	if _, err := r.f.Sweep(context.Background(), repo, r.shipper()); err != nil {
		t.Fatal(err)
	}
	if len(r.dt.Snapshot()) != 1 {
		t.Fatal("retry after recovery should ship the job")
	}
}

func TestGitHubAuthFailure(t *testing.T) {
	r := newRig(t)
	r.gh.Token = "something-else"
	if _, err := r.f.Sweep(context.Background(), repo, r.shipper()); !errors.Is(err, github.ErrAuth) {
		t.Fatalf("err=%v", err)
	}
}

func TestHeaderAttrsMode(t *testing.T) {
	r := newRig(t)
	r.cfg.DT.HeaderAttrs = true
	r.gh.Runs[repo] = []github.Run{run(1, t0.Add(-time.Hour), "completed", "success")}
	r.gh.Jobs[1] = []github.Job{
		job(11, 1, 1, "completed", "success", t0.Add(-time.Hour)),
		job(12, 1, 1, "completed", "success", t0.Add(-time.Hour)),
	}
	r.gh.Logs[11] = testutil.Log(t0.Add(-time.Hour), "a")
	r.gh.Logs[12] = testutil.Log(t0.Add(-time.Hour), "b")
	if _, err := r.f.Sweep(context.Background(), repo, r.shipper()); err != nil {
		t.Fatal(err)
	}
	recs := r.dt.Snapshot()
	if len(recs) != 2 || recs[0]["vcs.repository.name"] != nil {
		t.Fatalf("job attrs should be in the header, not the body: %v", recs)
	}
	if len(r.dt.Headers) != 2 {
		t.Fatalf("each job should get its own request: %v", r.dt.Headers)
	}
	q, _ := url.ParseQuery(r.dt.Headers[1])
	if q.Get("cicd.pipeline.task.run.id") != "12" || q.Get("log.source") != "github-actions" {
		t.Fatalf("header %q", r.dt.Headers[1])
	}
}

func TestShipRunAttemptFilter(t *testing.T) {
	r := newRig(t)
	r.gh.Runs[repo] = []github.Run{run(1, t0.Add(-time.Hour), "completed", "success")}
	r.gh.Jobs[1] = []github.Job{
		job(11, 1, 1, "completed", "failure", t0.Add(-time.Hour)),
		job(21, 1, 2, "completed", "success", t0.Add(-10*time.Minute)),
	}
	r.gh.Logs[11] = testutil.Log(t0.Add(-time.Hour), "attempt 1")
	r.gh.Logs[21] = testutil.Log(t0.Add(-10*time.Minute), "attempt 2")
	sum, err := r.f.ShipRun(context.Background(), repo, 1, 2, r.shipper())
	if err != nil {
		t.Fatal(err)
	}
	recs := r.dt.Snapshot()
	if len(recs) != 1 || recs[0]["content"] != "attempt 2" || recs[0]["gh.run_attempt"] != float64(2) || sum.Jobs != 1 {
		t.Fatalf("recs %v sum %+v", recs, sum)
	}
	st, _ := state.Load(r.cfg.StateDir, repo)
	if !st.HighWater.IsZero() {
		t.Error("ShipRun must not move the high-water mark")
	}
	// A later sweep ships attempt 1 but not attempt 2 again.
	if _, err := r.f.Sweep(context.Background(), repo, r.shipper()); err != nil {
		t.Fatal(err)
	}
	if got := byJob(r.dt.Snapshot()); got["11"] != 1 || got["21"] != 1 {
		t.Fatalf("%v", got)
	}
}

func TestWorkflowFilterAndConcurrency(t *testing.T) {
	r := newRig(t)
	r.cfg.Workflows = []string{"CI"}
	r.cfg.Concurrency = 2
	other := run(2, t0.Add(-time.Hour), "completed", "success")
	other.Name, other.Path = "Lint", ".github/workflows/lint.yml"
	r.gh.Runs[repo] = []github.Run{run(1, t0.Add(-time.Hour), "completed", "success"), other}
	for i := int64(0); i < 9; i++ {
		r.gh.Jobs[1] = append(r.gh.Jobs[1], job(100+i, 1, 1, "completed", "success", t0.Add(-time.Hour)))
		r.gh.Logs[100+i] = testutil.Log(t0.Add(-time.Hour), "x", "y")
	}
	r.gh.Jobs[2] = []github.Job{job(200, 2, 1, "completed", "success", t0.Add(-time.Hour))}
	r.gh.Logs[200] = testutil.Log(t0.Add(-time.Hour), "lint")
	if _, err := r.f.Sweep(context.Background(), repo, r.shipper()); err != nil {
		t.Fatal(err)
	}
	recs := r.dt.Snapshot()
	if len(recs) != 18 {
		t.Fatalf("got %d records, want 18 (Lint excluded)", len(recs))
	}
	// Order is preserved across concurrent downloads.
	for i := 0; i < 9; i++ {
		if want := 100 + i; recs[2*i]["cicd.pipeline.task.run.id"] != strconv.Itoa(want) {
			t.Fatalf("record %d from job %v, want %d", 2*i, recs[2*i]["cicd.pipeline.task.run.id"], want)
		}
	}
}

func TestDryRunDoesNotSave(t *testing.T) {
	r := newRig(t)
	r.f.DryRun = true
	r.gh.Runs[repo] = []github.Run{run(1, t0.Add(-time.Hour), "completed", "success")}
	r.gh.Jobs[1] = []github.Job{job(11, 1, 1, "completed", "success", t0.Add(-time.Hour))}
	r.gh.Logs[11] = testutil.Log(t0.Add(-time.Hour), "x")
	var out strings.Builder
	s := &dynatrace.Shipper{DryRun: &out, MaxBytes: 1 << 20, MaxRecords: 10}
	if _, err := r.f.Sweep(context.Background(), repo, s); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"content":"x"`) {
		t.Fatalf("dry-run output %q", out.String())
	}
	if _, err := state.Load(r.cfg.StateDir, repo); err != nil {
		t.Fatal(err)
	}
	if st, _ := state.Load(r.cfg.StateDir, repo); st.IsShipped(11) {
		t.Fatal("dry run saved state")
	}
}
