package report

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	foundrytui "github.com/CoolGitOrg/foundry-tui"
	"github.com/CoolGitOrg/foundry-tui/internal/audit"
	"github.com/CoolGitOrg/foundry-tui/internal/azure"
	"github.com/CoolGitOrg/foundry-tui/internal/azure/azuretest"
	"github.com/CoolGitOrg/foundry-tui/internal/config"
	"github.com/CoolGitOrg/foundry-tui/internal/dispatch"
	"github.com/CoolGitOrg/foundry-tui/internal/github"
	"github.com/CoolGitOrg/foundry-tui/internal/perms"
	"github.com/CoolGitOrg/foundry-tui/internal/probe"
)

func sources(t *testing.T) (Sources, *httptest.Server) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/CoolGitOrg/foundry-ops/actions/workflows", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"workflows":[{"id":1,"name":"foundry-deploy","path":".github/workflows/foundry-deploy.yml","state":"active"},
		  {"id":2,"name":"Record <triage>","path":".github/workflows/foundry-record-triage.lock.yml","state":"active"}]}`)
	})
	mux.HandleFunc("/repos/CoolGitOrg/foundry-ops/actions/runs", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprintf(w, `{"workflow_runs":[{"id":42,"name":"foundry-deploy","display_title":"<script>alert(1)</script>","path":".github/workflows/foundry-deploy.yml",
		  "event":"workflow_dispatch","status":"completed","conclusion":"failure","run_number":7,"html_url":"https://github.example/runs/42",
		  "created_at":%q,"run_started_at":%q,"updated_at":%q,"triggering_actor":{"login":"crafty"}}]}`, now, now, now)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	state := t.TempDir()
	log, _ := audit.Open(state)
	log.Append("crafty", "dispatch.request", "gpt-4o", map[string]string{"request_id": "ftui-1"})
	hist := probe.NewHistory(state)
	hist.Append(probe.Result{Time: time.Now(), Deployment: "gpt-4o", Kind: "api", OK: false, Detail: "429 Too Many Requests"})

	cfg := config.Default()
	return Sources{
		Cfg: cfg, Azure: azure.Client{R: azuretest.Fake(), Cfg: cfg},
		GitHub: &github.Client{Base: srv.URL, Repo: cfg.Repo, Token: "t", HTTP: srv.Client()},
		Perms:  perms.Snapshot(), Log: log, History: hist, Version: "test", Actor: "crafty",
	}, srv
}

func TestCollectStaysInsideTheRole(t *testing.T) {
	src, _ := sources(t)
	s := Collect(context.Background(), src)

	if len(s.Deployments) != 3 || len(s.Usage) != 2 || len(s.Access) != 2 || len(s.Alerts) != 1 {
		t.Fatalf("sections missing: %d deployments, %d usage, %d access, %d alerts; skipped=%v",
			len(s.Deployments), len(s.Usage), len(s.Access), len(s.Alerts), s.Skipped)
	}
	if len(s.Models) != 2 {
		t.Errorf("want catalog rows for deployed models only, got %d", len(s.Models))
	}
	for _, key := range []string{"metrics", "activity"} {
		if !strings.Contains(s.Skipped[key], "outside Foundry Owner") {
			t.Errorf("%s should be skipped with a reason, got %q", key, s.Skipped[key])
		}
	}
	if s.AuditHead == "" || s.AuditErr != "" {
		t.Errorf("audit head %q err %q", s.AuditHead, s.AuditErr)
	}

	want := map[string]string{ // subject → fragment of the finding
		"embed-small":                            "provisioning state is Failed",
		"gpt-4o":                                 "retires in",
		"OpenAI.Standard.text-embedding-3-small": "quota is 86% used",
		"foundry-429-rate":                       "alert is firing",
		"foundry-deploy":                         "last run failed",
	}
	for subj, frag := range want {
		found := false
		for _, f := range s.Findings {
			if f.Subject == subj && strings.Contains(f.Text, frag) {
				found = true
			}
		}
		if !found {
			t.Errorf("no finding for %s containing %q in %+v", subj, frag, s.Findings)
		}
	}
	if s.Findings[0].Severity != "high" {
		t.Error("findings are not sorted by severity")
	}
}

func TestMetricsAreNeverRequestedWithoutTheAction(t *testing.T) {
	src, _ := sources(t)
	Collect(context.Background(), src)
	for _, c := range azuretest.Lines(src.Azure.R) {
		if strings.Contains(c, "monitor metrics") {
			t.Errorf("metrics were read although the role lacks the action: %s", c)
		}
	}
}

func TestWrite(t *testing.T) {
	src, _ := sources(t)
	s := Collect(context.Background(), src)
	tfs, _ := dispatch.TemplateFS(foundrytui.Templates, "")
	dir := t.TempDir()
	if d := os.Getenv("FOUNDRY_SAMPLE_DIR"); d != "" { // make sample: keep the fixture report
		dir = d
	}
	files, err := Write(tfs, dir, s)
	if err != nil {
		t.Fatal(err)
	}
	html, _ := os.ReadFile(files.HTML)
	page := string(html)
	for _, want := range []string{"ais-coolgit-copilot-prod", "Foundry Owner", "gpt-4o", "uami-foundry-deploy", "chain verified",
		"Microsoft.Insights/metrics/read", "foundry-record-triage.lock.yml", "width: 86%"} {
		if !strings.Contains(page, want) {
			t.Errorf("report lacks %q", want)
		}
	}
	for _, bad := range []string{"<script>alert", "ZgotmplZ", "<no value>", "SECRET-TOKEN", "http://", "<script"} {
		if strings.Contains(page, bad) {
			t.Errorf("report contains %q", bad)
		}
	}
	// The checksum file must agree with what is on disk, in sha256sum's format.
	sums, _ := os.ReadFile(files.Sum)
	js, _ := os.ReadFile(files.JSON)
	if !strings.Contains(string(sums), fmt.Sprintf("%x  %s", sha256.Sum256(js), filepath.Base(files.JSON))) {
		t.Errorf("checksum file does not match the JSON:\n%s", sums)
	}
	idx, _ := os.ReadFile(filepath.Join(dir, "index.html"))
	if !strings.Contains(string(idx), filepath.Base(files.HTML)) {
		t.Error("index.html does not list the new record")
	}
}
