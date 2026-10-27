package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CoolGitOrg/gh-log-forwarder/internal/github"
	"github.com/CoolGitOrg/gh-log-forwarder/internal/state"
	"github.com/CoolGitOrg/gh-log-forwarder/internal/testutil"
)

type result struct {
	code   int
	stdout string
	stderr string
}

func invoke(t *testing.T, vars map[string]string, stdin string, args ...string) result {
	t.Helper()
	var out, errb bytes.Buffer
	e := &env{strings.NewReader(stdin), &out, &errb, func(k string) string { return vars[k] }}
	code := realMain(context.Background(), args, e)
	return result{code, out.String(), errb.String()}
}

const repo = "CoolGitOrg/app-a"

// fixture builds a fake GitHub with one completed run created "now".
func fixture(t *testing.T) (*testutil.GitHub, *testutil.Dynatrace, map[string]string) {
	t.Helper()
	gh, dt := testutil.NewGitHub(), testutil.NewDynatrace()
	t.Cleanup(gh.Close)
	t.Cleanup(dt.Close)
	gh.Token = "ghs_test"
	now := time.Now().UTC().Add(-5 * time.Minute)
	gh.Runs[repo] = []github.Run{{ID: 1, Name: "CI", Status: "completed", Conclusion: "success", CreatedAt: now, RunAttempt: 1, HeadBranch: "main"}}
	gh.Jobs[1] = []github.Job{{ID: 11, RunID: 1, RunAttempt: 1, Name: "build", Status: "completed", Conclusion: "success",
		StartedAt: &now, RunnerName: "r1", Steps: []github.Step{{Name: "Set up job", Number: 1, StartedAt: &now}}}}
	gh.Logs[11] = testutil.Log(now, "hello", "##[error]boom")
	gh.Repos = []string{repo, "CoolGitOrg/old-archived"}
	vars := map[string]string{
		"GH_TOKEN":        "ghs_test",
		"GITHUB_API_URL":  gh.URL(),
		"DT_URL":          dt.URL(),
		"DT_LOGS_TOKEN":   "dt0c01.test",
		"GHLOG_STATE_DIR": t.TempDir(),
	}
	return gh, dt, vars
}

func TestRunLive(t *testing.T) {
	_, dt, vars := fixture(t)
	vars["GITHUB_STEP_SUMMARY"] = filepath.Join(t.TempDir(), "summary.md")
	vars["GHLOG_CONFIG"] = filepath.Join(t.TempDir(), "ghlog2dt.json")
	os.WriteFile(vars["GHLOG_CONFIG"], []byte(`{"lookback":"30m","exclude":["*/*-archived"]}`), 0o644)
	r := invoke(t, vars, "", "run", "--all", "--log-format", "text")
	if r.code != 0 {
		t.Fatalf("exit %d\n%s", r.code, r.stderr)
	}
	var sum struct {
		Totals struct{ Jobs, Records int }
		Repos  []struct{ Repo string }
	}
	if err := json.Unmarshal([]byte(r.stdout), &sum); err != nil {
		t.Fatalf("summary %q: %v", r.stdout, err)
	}
	if sum.Totals.Jobs != 1 || sum.Totals.Records != 2 || len(sum.Repos) != 1 {
		t.Fatalf("summary %s", r.stdout)
	}
	recs := dt.Snapshot()
	if len(recs) != 3 || recs[2]["log.source"] != "ghlog2dt.self" {
		t.Fatalf("want 2 job records + 1 self record, got %v", recs)
	}
	md, _ := os.ReadFile(vars["GITHUB_STEP_SUMMARY"])
	if !strings.Contains(string(md), "| `CoolGitOrg/app-a` | 1 | 1 | 2 |") {
		t.Fatalf("step summary:\n%s", md)
	}
	// Second run ships nothing new.
	r = invoke(t, vars, "", "run", "--repo", repo)
	if r.code != 0 || len(dt.Snapshot()) != 4 { // + one more self record
		t.Fatalf("rerun: exit %d, %d records", r.code, len(dt.Snapshot()))
	}
}

func TestRunDryRun(t *testing.T) {
	_, dt, vars := fixture(t)
	delete(vars, "DT_URL")
	delete(vars, "DT_LOGS_TOKEN")
	r := invoke(t, vars, "", "run", "--repo", repo, "--dry-run")
	if r.code != 0 {
		t.Fatalf("exit %d\n%s", r.code, r.stderr)
	}
	lines := strings.Split(strings.TrimSpace(r.stdout), "\n")
	if len(lines) != 2 || !strings.Contains(lines[1], `"severity":"ERROR"`) {
		t.Fatalf("records:\n%s", r.stdout)
	}
	if !strings.Contains(r.stderr, `"dry_run":true`) {
		t.Fatalf("summary should go to stderr in dry-run: %s", r.stderr)
	}
	if len(dt.Snapshot()) != 0 {
		t.Fatal("dry run posted")
	}
	st, _ := state.Load(vars["GHLOG_STATE_DIR"], repo)
	if st.IsShipped(11) {
		t.Fatal("dry run saved state")
	}
}

func TestExitCodes(t *testing.T) {
	gh, dt, vars := fixture(t)
	if r := invoke(t, vars, "", "run"); r.code != exitUsage {
		t.Errorf("no targets: exit %d", r.code)
	}
	if r := invoke(t, vars, "", "run", "--repo", "bad"); r.code != exitUsage {
		t.Errorf("bad repo: exit %d", r.code)
	}
	if r := invoke(t, vars, "", "nope"); r.code != exitUsage {
		t.Errorf("unknown command: exit %d", r.code)
	}
	dt.Lock()
	dt.Status = 401
	dt.Unlock()
	if r := invoke(t, vars, "", "run", "--repo", repo); r.code != exitAuth {
		t.Errorf("dynatrace 401: exit %d\n%s", r.code, r.stderr)
	}
	dt.Lock()
	dt.Status = 402
	dt.Unlock()
	if r := invoke(t, vars, "", "run", "--repo", repo); r.code != exitBudget {
		t.Errorf("dynatrace 402: exit %d", r.code)
	}
	gh.Lock()
	gh.Token = "other"
	gh.Unlock()
	if r := invoke(t, vars, "", "run", "--repo", repo, "--dry-run"); r.code != exitAuth {
		t.Errorf("github 401: exit %d", r.code)
	}
	if r := invoke(t, vars, "", "run", "--repo", repo, "--run-attempt", "2"); r.code != exitUsage {
		t.Errorf("--run-attempt without --run-id: exit %d", r.code)
	}
}

func TestRunIDMode(t *testing.T) {
	_, dt, vars := fixture(t)
	r := invoke(t, vars, "", "run", "--repo", repo, "--run-id", "1", "--self-log=false")
	if r.code != 0 || len(dt.Snapshot()) != 2 {
		t.Fatalf("exit %d, %d records\n%s", r.code, len(dt.Snapshot()), r.stderr)
	}
	if r := invoke(t, vars, "", "run", "--repo", repo, "--repo", "CoolGitOrg/b", "--run-id", "1"); r.code != exitUsage {
		t.Fatalf("--run-id with two repos: exit %d", r.code)
	}
}

func TestListFetchShipPipeline(t *testing.T) {
	_, dt, vars := fixture(t)
	l := invoke(t, vars, "", "list", "--repo", repo)
	if l.code != 0 || strings.Count(l.stdout, "\n") != 1 || !strings.Contains(l.stdout, `"id":11`) {
		t.Fatalf("list: exit %d\n%s%s", l.code, l.stdout, l.stderr)
	}
	f := invoke(t, vars, l.stdout, "fetch")
	if f.code != 0 || strings.Count(f.stdout, "\n") != 2 {
		t.Fatalf("fetch: exit %d\n%s%s", f.code, f.stdout, f.stderr)
	}
	s := invoke(t, vars, f.stdout, "ship")
	if s.code != 0 || len(dt.Snapshot()) != 2 {
		t.Fatalf("ship: exit %d, %d records\n%s", s.code, len(dt.Snapshot()), s.stderr)
	}
	if r := invoke(t, vars, "not json\n", "ship"); r.code != exitUsage {
		t.Fatalf("ship garbage: exit %d", r.code)
	}
	// list never writes state.
	if st, _ := state.Load(vars["GHLOG_STATE_DIR"], repo); len(st.Shipped) != 0 {
		t.Fatal("list/fetch/ship must not write the checkpoint")
	}
}

func TestCheckpointCommands(t *testing.T) {
	dir := t.TempDir()
	vars := map[string]string{"GHLOG_STATE_DIR": dir}
	if r := invoke(t, vars, "", "checkpoint", "mark", "--repo", repo, "--job", "5", "--job", "6"); r.code != 0 {
		t.Fatalf("mark: %d %s", r.code, r.stderr)
	}
	if r := invoke(t, vars, "", "checkpoint", "rewind", "--repo", repo, "--to", "2h"); r.code != 0 {
		t.Fatalf("rewind: %d %s", r.code, r.stderr)
	}
	if r := invoke(t, vars, "", "checkpoint", "rewind", "--repo", repo, "--to", "30h"); r.code != exitUsage {
		t.Fatalf("rewind beyond 24h: %d", r.code)
	}
	// replay = rewind --forget: jobs shipped after the mark are forgotten.
	other := state.New("CoolGitOrg/replay")
	other.MarkShipped(1, time.Now().Add(-3*time.Hour))
	other.MarkShipped(2, time.Now().Add(-1*time.Hour))
	other.Save(dir, time.Now())
	if r := invoke(t, vars, "", "checkpoint", "rewind", "--repo", "CoolGitOrg/replay", "--to", "2h", "--forget"); r.code != 0 {
		t.Fatalf("rewind --forget: %d %s", r.code, r.stderr)
	}
	if st, _ := state.Load(dir, "CoolGitOrg/replay"); !st.IsShipped(1) || st.IsShipped(2) {
		t.Fatalf("forget: %v", st.Shipped)
	}
	show := invoke(t, vars, "", "checkpoint", "show", "--repo", repo)
	if !strings.Contains(show.stdout, `"5"`) || !strings.Contains(show.stdout, `"high_water"`) {
		t.Fatalf("show: %s", show.stdout)
	}
	mergeFile := filepath.Join(t.TempDir(), "x.json")
	s := state.New(repo)
	s.MarkShipped(7, time.Now())
	b, _ := s.Marshal()
	os.WriteFile(mergeFile, b, 0o644)
	m := invoke(t, vars, "", "checkpoint", "merge", filepath.Join(dir, "CoolGitOrg__app-a.json"), mergeFile)
	if m.code != 0 || !strings.Contains(m.stdout, `"7"`) || !strings.Contains(m.stdout, `"5"`) {
		t.Fatalf("merge: %d %s %s", m.code, m.stdout, m.stderr)
	}
}

func TestSchedule(t *testing.T) {
	cases := map[int]string{5: "2,7,12,17,22,27,32,37,42,47,52,57 * * * *", 15: "7,22,37,52 * * * *", 30: "7,37 * * * *", 60: "7 * * * *"}
	for iv, want := range cases {
		s, err := ComputeSchedule(iv, 7)
		if err != nil || s.Cron != want {
			t.Errorf("interval %d: %q %v", iv, s.Cron, err)
		}
	}
	if s, _ := ComputeSchedule(15, 7); s.Lookback != "90m" {
		t.Errorf("lookback %s", s.Lookback)
	}
	if _, err := ComputeSchedule(7, 0); err == nil {
		t.Error("interval 7 accepted")
	}
	wf := filepath.Join(t.TempDir(), "forward.yml")
	os.WriteFile(wf, []byte("on:\n  schedule:\n    - cron: \"7,22,37,52 * * * *\"  # ghlog2dt:cron (managed)\n  workflow_dispatch:\n"), 0o644)
	if r := invoke(t, nil, "", "schedule", "--interval", "30", "--rewrite", wf); r.code != 0 {
		t.Fatalf("rewrite: %d %s", r.code, r.stderr)
	}
	b, _ := os.ReadFile(wf)
	if !strings.Contains(string(b), `- cron: "7,37 * * * *"  # ghlog2dt:cron (managed)`) {
		t.Fatalf("rewritten:\n%s", b)
	}
}

func TestVersionAndHelp(t *testing.T) {
	if r := invoke(t, nil, "", "version", "--json"); r.code != 0 || !strings.Contains(r.stdout, `"version":"dev"`) {
		t.Fatalf("%+v", r)
	}
	if r := invoke(t, nil, "", "help"); r.code != 0 || !strings.Contains(r.stderr, "checkpoint") {
		t.Fatalf("%+v", r)
	}
	if r := invoke(t, nil, "", "run", "-h"); r.code != 0 || !strings.Contains(r.stderr, "-dt-url") {
		t.Fatalf("run -h: %+v", r)
	}
}
