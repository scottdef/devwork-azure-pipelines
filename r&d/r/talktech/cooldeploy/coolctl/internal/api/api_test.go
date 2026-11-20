package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/CoolEngOrg/coolctl/internal/argo"
	"github.com/CoolEngOrg/coolctl/internal/gitops"
	"github.com/CoolEngOrg/coolctl/internal/manifest"
	"github.com/CoolEngOrg/coolctl/internal/metrics"
	"github.com/CoolEngOrg/coolctl/internal/oidc"
	"github.com/CoolEngOrg/coolctl/internal/oidctest"
	"github.com/CoolEngOrg/coolctl/internal/release"
)

const (
	ciRef  = "CoolEngOrg/cool-actions/.github/workflows/cool-deploy.yml@refs/tags/v1"
	opsRef = "CoolEngOrg/issueops/.github/workflows/execute-cool-change.yml@refs/heads/main"
	repo   = "CoolEngOrg/checkout"
	issue  = "https://github.com/CoolEngOrg/issueops/issues/7"
)

// world is a cool-api with a directory store and a fake OIDC issuer.
type world struct {
	t     *testing.T
	srv   *httptest.Server
	iss   *oidctest.Issuer
	store *gitops.DirStore
}

func newWorld(t *testing.T) *world {
	t.Helper()
	iss, err := oidctest.New()
	if err != nil {
		t.Fatal(err)
	}
	isrv := httptest.NewServer(iss.Handler())
	t.Cleanup(isrv.Close)
	iss.URL = isrv.URL

	store := gitops.NewDirStore(t.TempDir())
	s := &Server{
		Store:    store,
		Verifier: &oidc.Verifier{Issuer: iss.URL, Audience: "cool-api"},
		Policy:   oidc.DefaultPolicy,
		Argo:     argo.None{},
		Domain:   "coolapps.example",
		Chart:    "cool-app-1.0.0",
		Log:      slog.New(slog.NewTextHandler(io.Discard, nil)),
		Metrics:  &metrics.Registry{},
		Now:      func() time.Time { return time.Date(2026, 11, 12, 9, 0, 0, 0, time.UTC) },
	}
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	return &world{t, srv, iss, store}
}

func (w *world) ci(repository string) string {
	tok, _ := w.iss.Token(map[string]any{"aud": "cool-api", "repository": repository, "job_workflow_ref": ciRef, "actor": "dev1"}, time.Minute)
	return tok
}

func (w *world) ops(environment string) string {
	tok, _ := w.iss.Token(map[string]any{
		"aud": "cool-api", "repository": "CoolEngOrg/issueops", "job_workflow_ref": opsRef,
		"environment": environment, "actor": "approver1", "run_id": "99",
	}, time.Minute)
	return tok
}

// call sends a request and decodes the reply into out. It returns the status.
func (w *world) call(method, path, token string, in, out any) int {
	w.t.Helper()
	var body io.Reader
	if in != nil {
		b, _ := json.Marshal(in)
		body = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, w.srv.URL+path, body)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		w.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if out != nil && resp.StatusCode < 300 {
		// Zero the target first: Unmarshal keeps fields the JSON omits, and a
		// reused Result would carry an old "standby" into the next comparison.
		if r, ok := out.(*Result); ok {
			*r = Result{}
		}
		if err := json.Unmarshal(b, out); err != nil {
			w.t.Fatalf("%s %s: %v: %s", method, path, err, b)
		}
	}
	if resp.StatusCode >= 300 {
		w.t.Logf("%s %s -> %d %s", method, path, resp.StatusCode, bytes.TrimSpace(b))
	}
	return resp.StatusCode
}

func digest(c string) string { return "sha256:" + strings.Repeat(c, 64) }

// show renders a release list as "id:weight id:sweight", s marking a standby.
func show(rs []release.Release) string {
	var parts []string
	for _, r := range rs {
		s := ""
		if r.Standby {
			s = "s"
		}
		parts = append(parts, fmt.Sprintf("%s:%s%d", r.ID, s, r.Weight))
	}
	return strings.Join(parts, " ")
}

const base = "/v1/apps/payments/checkout"

func (w *world) register() {
	w.t.Helper()
	in := map[string]string{"team": "payments", "app": "checkout", "repo": repo, "issue": issue}
	if code := w.call("POST", "/v1/apps", w.ops("cool-dev"), in, nil); code != 200 {
		w.t.Fatalf("register: %d", code)
	}
}

func TestLifecycle(t *testing.T) {
	w := newWorld(t)
	w.register()
	if code := w.call("POST", "/v1/apps", w.ops("cool-dev"), map[string]string{"team": "payments", "app": "checkout", "repo": repo}, nil); code != 409 {
		t.Fatalf("second register: %d, want 409", code)
	}

	m := manifest.Default("payments", "checkout", "go-htmx")
	m.Env = map[string]string{"LOG_LEVEL": "debug"}
	m.Environments = map[string]manifest.Override{"prd": {Scale: &manifest.Scale{Min: 3, Max: 12, CPU: 60}}}

	// Two pushes to main.
	var res Result
	for _, d := range []struct{ sha, digest string }{{"aaaaaaa1", digest("a")}, {"bbbbbbb2", digest("b")}} {
		in := map[string]any{"environment": "dev", "sha": d.sha, "digest": d.digest, "manifest": m}
		if code := w.call("POST", base+"/deployments", w.ci(repo), in, &res); code != 200 {
			t.Fatalf("deploy: %d", code)
		}
	}
	if got := show(res.Releases); got != "rbbbbbbb:100 raaaaaaa:s0" {
		t.Fatalf("dev after two deploys: %s", got)
	}
	if res.URL != "https://checkout.dev.coolapps.example" || res.Application != "payments-checkout-dev" {
		t.Errorf("result = %+v", res)
	}

	// Promote dev to tst, then to prd, by issue.
	if code := w.call("POST", base+"/promote", w.ops("cool-tst"), map[string]string{"from": "dev", "to": "tst", "issue": issue}, &res); code != 200 {
		t.Fatalf("promote to tst: %d", code)
	}
	if code := w.call("POST", base+"/promote", w.ops("cool-prd"), map[string]string{"from": "tst", "to": "prd", "issue": issue}, &res); code != 200 {
		t.Fatalf("promote to prd: %d", code)
	}
	var prd release.Values
	if _, err := gitops.ReadJSON(context.Background(), w.store, gitops.ValuesPath("payments", "checkout", "prd"), &prd); err != nil {
		t.Fatal(err)
	}
	if prd.Scale.Min != 3 {
		t.Errorf("prd scale = %+v: the manifest's prd override did not travel with the release", prd.Scale)
	}
	if prd.Releases[0].Digest != digest("b") {
		t.Errorf("prd runs %s, want the digest built for dev", prd.Releases[0].Digest)
	}

	// A third build rolls out in prd by stages.
	in := map[string]any{"environment": "dev", "sha": "ccccccc3", "digest": digest("c"), "manifest": m}
	w.call("POST", base+"/deployments", w.ci(repo), in, nil)
	if code := w.call("POST", base+"/weights", w.ops("cool-prd"), map[string]any{"environment": "prd", "from": "dev", "weight": 10, "issue": issue}, &res); code != 200 {
		t.Fatalf("stage 10: %d", code)
	}
	if got := show(res.Releases); got != "rccccccc:10 rbbbbbbb:90" {
		t.Fatalf("prd at stage 10: %s", got)
	}
	if code := w.call("POST", base+"/weights", w.ops("cool-prd"), map[string]any{"environment": "prd", "weight": 50}, &res); code != 200 {
		t.Fatalf("stage 50: %d", code)
	}
	if code := w.call("POST", base+"/weights", w.ops("cool-prd"), map[string]any{"environment": "prd", "weight": 100}, &res); code != 200 {
		t.Fatalf("stage 100: %d", code)
	}
	if got := show(res.Releases); got != "rccccccc:100 rbbbbbbb:s0" {
		t.Fatalf("prd after the rollout: %s", got)
	}

	// Instant rollback.
	if code := w.call("POST", base+"/rollback", w.ops("cool-prd"), map[string]string{"environment": "prd", "reason": "error spike", "issue": issue}, &res); code != 200 {
		t.Fatalf("rollback: %d", code)
	}
	if got := show(res.Releases); got != "rccccccc:s0 rbbbbbbb:100" || res.Release != "rbbbbbbb" {
		t.Fatalf("prd after rollback: %s (%s)", got, res.Release)
	}

	// Overrides and a custom domain.
	cfg := map[string]any{"environment": "prd", "set": map[string]string{"FEATURE_X": "on"}, "add_domains": []string{"pay.example.com"}, "issue": issue}
	if code := w.call("POST", base+"/config", w.ops("cool-prd"), cfg, nil); code != 200 {
		t.Fatalf("config: %d", code)
	}

	// Status sees all three environments.
	var st AppStatus
	if code := w.call("GET", base, "", nil, &st); code != 200 || len(st.Environments) != 3 {
		t.Fatalf("status: %d, %d environments", code, len(st.Environments))
	}
	resp, _ := http.Get(w.srv.URL + base + "/status")
	page, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(page), "rbbbbbbb") || !strings.Contains(resp.Header.Get("Content-Type"), "text/html") {
		t.Errorf("status fragment: %s", page)
	}

	// Retire removes every file.
	if code := w.call("DELETE", base, w.ops("cool-prd"), map[string]string{"confirm": "payments/checkout", "issue": issue}, nil); code != 200 {
		t.Fatalf("retire: %d", code)
	}
	if apps, _ := gitops.Apps(context.Background(), w.store); len(apps) != 0 {
		t.Errorf("apps after retire: %+v", apps)
	}
}

// An identity given at registration reaches the values of its environment
// on the first deploy there, and a later config change may replace it.
func TestIdentities(t *testing.T) {
	w := newWorld(t)
	const devID, tstID = "99999999-aaaa-4bbb-8ccc-dddddddddddd", "88888888-aaaa-4bbb-8ccc-dddddddddddd"
	in := map[string]any{"team": "payments", "app": "checkout", "repo": repo, "issue": issue,
		"identities": map[string]string{"dev": devID, "tst": tstID}}
	if code := w.call("POST", "/v1/apps", w.ops("cool-dev"), in, nil); code != 200 {
		t.Fatalf("register: %d", code)
	}
	bad := map[string]any{"team": "payments", "app": "other", "repo": repo, "identities": map[string]string{"qa": devID}}
	if code := w.call("POST", "/v1/apps", w.ops("cool-dev"), bad, nil); code != 400 {
		t.Errorf("register with an unknown environment: %d, want 400", code)
	}
	dep := map[string]any{"sha": "aaaaaaa1", "digest": digest("a")}
	if code := w.call("POST", base+"/deployments", w.ci(repo), dep, nil); code != 200 {
		t.Fatalf("deploy: %d", code)
	}
	if code := w.call("POST", base+"/promote", w.ops("cool-tst"), map[string]string{"from": "dev", "to": "tst"}, nil); code != 200 {
		t.Fatalf("promote: %d", code)
	}
	for env, want := range map[string]string{"dev": devID, "tst": tstID} {
		var v release.Values
		if _, err := gitops.ReadJSON(context.Background(), w.store, gitops.ValuesPath("payments", "checkout", env), &v); err != nil {
			t.Fatal(err)
		}
		if v.Identity != want {
			t.Errorf("%s identity = %q, want %q", env, v.Identity, want)
		}
	}
}

func TestPreview(t *testing.T) {
	w := newWorld(t)
	w.register()
	var res Result
	in := map[string]any{"sha": "ddddddd4", "digest": digest("d"), "preview": 42}
	if code := w.call("POST", base+"/deployments", w.ci(repo), in, &res); code != 200 {
		t.Fatalf("preview up: %d", code)
	}
	if res.URL != "https://checkout-pr-42.preview.dev.coolapps.example" || res.Application != "payments-checkout-pr-42" {
		t.Errorf("result = %+v", res)
	}
	var st AppStatus
	w.call("GET", base, "", nil, &st)
	if len(st.Previews) != 1 || st.Previews[0].PR != 42 {
		t.Errorf("previews = %+v", st.Previews)
	}
	if code := w.call("DELETE", base+"/previews/42", w.ci(repo), nil, nil); code != 200 {
		t.Fatalf("preview down: %d", code)
	}
	if code := w.call("DELETE", base+"/previews/42", w.ci(repo), nil, nil); code != 200 {
		t.Fatalf("second preview down should be a no-op: %d", code)
	}
}

func TestDenials(t *testing.T) {
	w := newWorld(t)
	w.register()
	dep := map[string]any{"environment": "dev", "sha": "aaaaaaa1", "digest": digest("a")}
	w.call("POST", base+"/deployments", w.ci(repo), dep, nil)

	tests := []struct {
		name         string
		method, path string
		token        string
		in           any
		want         int
	}{
		{"no token", "POST", base + "/deployments", "", dep, 401},
		{"garbage token", "POST", base + "/deployments", "abc.def.ghi", dep, 401},
		{"another repo deploys this app", "POST", base + "/deployments", w.ci("CoolEngOrg/other"), dep, 403},
		{"ci deploys to prd", "POST", base + "/deployments", w.ci(repo), map[string]any{"environment": "prd", "sha": "aaaaaaa1", "digest": digest("a")}, 403},
		{"ci promotes", "POST", base + "/promote", w.ci(repo), map[string]string{"from": "dev", "to": "tst"}, 403},
		{"tst approval used for prd", "POST", base + "/promote", w.ops("cool-tst"), map[string]string{"from": "dev", "to": "prd"}, 403},
		{"ci rolls back", "POST", base + "/rollback", w.ci(repo), map[string]string{"environment": "dev"}, 403},
		{"rollback with no standby", "POST", base + "/rollback", w.ops("cool-dev"), map[string]string{"environment": "dev"}, 409},
		{"bad digest", "POST", base + "/deployments", w.ci(repo), map[string]any{"sha": "aaaaaaa1", "digest": "latest"}, 400},
		{"unknown field", "POST", base + "/deployments", w.ci(repo), map[string]any{"sha": "aaaaaaa1", "digest": digest("a"), "replicas": 99}, 400},
		{"manifest for another app", "POST", base + "/deployments", w.ci(repo), map[string]any{"sha": "aaaaaaa1", "digest": digest("a"), "manifest": manifest.Default("payments", "other", "static")}, 400},
		{"unregistered app", "POST", "/v1/apps/payments/ghost/deployments", w.ci(repo), dep, 404},
		{"reserved variable", "POST", base + "/config", w.ops("cool-dev"), map[string]any{"environment": "dev", "set": map[string]string{"COOL_ENV": "x"}}, 400},
		{"retire without confirmation", "DELETE", base, w.ops("cool-prd"), map[string]string{"confirm": "yes"}, 400},
		{"issue is not a URL", "POST", base + "/rollback", w.ops("cool-dev"), map[string]string{"environment": "dev", "issue": "see chat"}, 400},
		{"a name that is not a slug", "GET", "/v1/apps/payments/Bad_Name", "", nil, 400},
		{"a preview-like app name", "GET", "/v1/apps/payments/site-pr-4", "", nil, 400},
	}
	for _, tt := range tests {
		if got := w.call(tt.method, tt.path, tt.token, tt.in, nil); got != tt.want {
			t.Errorf("%s: status %d, want %d", tt.name, got, tt.want)
		}
	}
}

func TestCommitMessageNamesTheIssue(t *testing.T) {
	w := newWorld(t)
	w.register()
	b, _, err := w.store.Read(context.Background(), gitops.AppPath("payments", "checkout"))
	if err != nil || !strings.Contains(string(b), `"repo": "CoolEngOrg/checkout"`) {
		t.Fatalf("app.json = %s, %v", b, err)
	}
	c := oidc.Claims{Actor: "approver1", JobWorkflowRef: opsRef, Repository: "CoolEngOrg/issueops", RunID: "99"}
	msg := message("promote: payments/checkout r1234567 from tst to prd", c, issue)
	for _, want := range []string{"Actor: approver1", "Issue: " + issue, "Run: https://github.com/CoolEngOrg/issueops/actions/runs/99"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message lacks %q:\n%s", want, msg)
		}
	}
}
