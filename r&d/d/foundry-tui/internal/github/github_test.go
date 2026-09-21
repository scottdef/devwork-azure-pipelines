package github

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Server is a tiny GitHub: two workflows, one of them agentic.
func Server(t *testing.T) *Client {
	t.Helper()
	now := time.Now().UTC()
	ts := func(d time.Duration) string { return now.Add(d).Format(time.RFC3339) }
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/CoolGitOrg/foundry-ops/actions/workflows", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			http.Error(w, `{"message":"Bad credentials"}`, http.StatusUnauthorized)
			return
		}
		fmt.Fprint(w, `{"workflows":[
		 {"id":1,"name":"foundry-deploy","path":".github/workflows/foundry-deploy.yml","state":"active"},
		 {"id":2,"name":"Foundry record triage","path":".github/workflows/foundry-record-triage.lock.yml","state":"active"},
		 {"id":3,"name":"Silent agent","path":".github/workflows/silent.lock.yml","state":"disabled_manually"}]}`)
	})
	mux.HandleFunc("/repos/CoolGitOrg/foundry-ops/actions/runs", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `{"workflow_runs":[
		 {"id":42,"name":"foundry-deploy","display_title":"foundry-deploy create gpt-4o-eu [ftui-20260921T141503Z-9f2c1a]","path":".github/workflows/foundry-deploy.yml","event":"workflow_dispatch","status":"completed","conclusion":"success","run_number":7,"html_url":"https://github.example/runs/42","created_at":%q,"run_started_at":%q,"updated_at":%q,"triggering_actor":{"login":"crafty"}},
		 {"id":41,"name":"Foundry record triage","display_title":"Foundry record triage","path":".github/workflows/foundry-record-triage.lock.yml","event":"schedule","status":"completed","conclusion":"failure","run_number":3,"created_at":%q,"run_started_at":%q,"updated_at":%q,"triggering_actor":{"login":"github-actions[bot]"}},
		 {"id":40,"name":"Foundry record triage","display_title":"Foundry record triage","path":".github/workflows/foundry-record-triage.lock.yml","event":"schedule","status":"completed","conclusion":"success","run_number":2,"created_at":%q,"run_started_at":%q,"updated_at":%q,"triggering_actor":{"login":"github-actions[bot]"}},
		 {"id":39,"name":"Foundry record triage","display_title":"Foundry record triage","path":".github/workflows/foundry-record-triage.lock.yml","event":"schedule","status":"in_progress","conclusion":null,"run_number":4,"created_at":%q,"run_started_at":%q,"updated_at":%q,"triggering_actor":{"login":"github-actions[bot]"}}]}`,
			ts(-10*time.Minute), ts(-10*time.Minute), ts(-8*time.Minute),
			ts(-3*time.Hour), ts(-3*time.Hour), ts(-3*time.Hour+4*time.Minute),
			ts(-27*time.Hour), ts(-27*time.Hour), ts(-27*time.Hour+2*time.Minute),
			ts(-time.Minute), ts(-time.Minute), ts(-time.Minute))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return &Client{Base: srv.URL, Repo: "CoolGitOrg/foundry-ops", Token: "test-token", HTTP: srv.Client()}
}

func TestSummarize(t *testing.T) {
	c := Server(t)
	ctx := context.Background()
	wfs, err := c.Workflows(ctx)
	if err != nil {
		t.Fatal(err)
	}
	runs, err := c.Runs(ctx, "", 100)
	if err != nil {
		t.Fatal(err)
	}
	stats := Summarize(wfs, runs)
	if len(stats) != 3 {
		t.Fatalf("want 3 rows (a silent agent still gets one), got %d", len(stats))
	}
	var triage Stats
	for _, s := range stats {
		if s.Workflow == "Foundry record triage" {
			triage = s
		}
	}
	if !triage.Agentic || triage.Runs != 3 || triage.Succeeded != 1 || triage.Failed != 1 || triage.Active != 1 {
		t.Errorf("triage = %+v", triage)
	}
	if triage.SuccessRate != 50 || triage.AvgDuration != 3*time.Minute {
		t.Errorf("rate %.0f%%, mean %s", triage.SuccessRate, triage.AvgDuration)
	}
	if triage.Last.ID != 39 {
		t.Errorf("last run = %d, want the newest", triage.Last.ID)
	}
	if r, ok := FindRun(runs, "ftui-20260921T141503Z-9f2c1a"); !ok || r.ID != 42 {
		t.Error("FindRun did not match the request id in the run title")
	}
	if _, ok := FindRun(runs, ""); ok {
		t.Error("an empty request id matched a run")
	}
}

func TestErrorsCarryGitHubsMessage(t *testing.T) {
	c := Server(t)
	c.Token = "wrong"
	if _, err := c.Workflows(context.Background()); err == nil || !strings.Contains(err.Error(), "Bad credentials") {
		t.Errorf("err = %v", err)
	}
}
