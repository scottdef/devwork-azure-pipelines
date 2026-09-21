package verify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CoolGitOrg/foundry-byok/internal/foundry"
)

const testKey = "unit-test-key-0123456789"

// fakeAzure emulates the parts of an Azure resource that verify touches.
func fakeAzure(t *testing.T, deployments map[string]foundry.WireAPI, modelIDs []string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	auth := func(w http.ResponseWriter, r *http.Request) bool {
		if r.Header.Get("api-key") == testKey || r.Header.Get("Authorization") == "Bearer tok" {
			return true
		}
		http.Error(w, `{"error":{"code":"401","message":"Access denied"}}`, http.StatusUnauthorized)
		return false
	}
	mux.HandleFunc("/openai/v1/models", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		type m struct {
			ID string `json:"id"`
		}
		var out struct {
			Data []m `json:"data"`
		}
		for _, id := range modelIDs {
			out.Data = append(out.Data, m{id})
		}
		json.NewEncoder(w).Encode(out)
	})
	infer := func(api foundry.WireAPI) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if !auth(w, r) {
				return
			}
			raw, _ := io.ReadAll(r.Body)
			var body struct {
				Model string `json:"model"`
			}
			json.Unmarshal(raw, &body)
			want, ok := deployments[body.Model]
			switch {
			case !ok:
				http.Error(w, `{"error":{"code":"DeploymentNotFound"}}`, http.StatusNotFound)
			case want != api:
				http.Error(w, `{"error":{"message":"unsupported operation for this model"}}`, http.StatusBadRequest)
			default:
				w.Write([]byte(`{"ok":true}`))
			}
		}
	}
	mux.HandleFunc("/openai/v1/responses", infer(foundry.WireResponses))
	mux.HandleFunc("/openai/v1/chat/completions", infer(foundry.WireCompletions))
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func resolve(t *testing.T, s foundry.Spec) foundry.Resolved {
	t.Helper()
	r, err := foundry.Resolve(s)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func finding(t *testing.T, res Result, check string) Finding {
	t.Helper()
	for _, f := range res.Findings {
		if f.Check == check {
			return f
		}
	}
	t.Fatalf("no finding %q in %+v", check, res.Findings)
	return Finding{}
}

func TestCheckHappyPath(t *testing.T) {
	srv := fakeAzure(t, map[string]foundry.WireAPI{"gpt-5.4": foundry.WireResponses}, []string{"gpt-4o", "gpt-5.4-2026-03-05"})
	r := resolve(t, foundry.Spec{Endpoint: srv.URL, Model: "gpt-5.4", ModelVersion: "2026-03-05", Deployment: "gpt-5.4"})

	res := Check(context.Background(), r, Credentials{APIKey: testKey}, Options{Probe: true})
	if !res.OK() {
		t.Fatalf("expected OK, got %+v", res.Findings)
	}
	if f := finding(t, res, "deployment-probe"); f.Status != StatusOK {
		t.Errorf("probe = %+v", f)
	}
	// Bearer tokens work too, and win over a key.
	res = Check(context.Background(), r, Credentials{APIKey: "wrong", BearerToken: "tok"}, Options{})
	if !res.OK() {
		t.Errorf("bearer auth failed: %+v", res.Findings)
	}
}

func TestModelIDNotListedSuggestsClosest(t *testing.T) {
	srv := fakeAzure(t, nil, []string{"gpt-4o", "gpt-5.4-2026-03-05"})
	// model_version omitted: the classic cause of gh-aw "model not found".
	r := resolve(t, foundry.Spec{Endpoint: srv.URL, Model: "gpt-5.4", Deployment: "gpt-5.4"})

	res := Check(context.Background(), r, Credentials{APIKey: testKey}, Options{})
	f := finding(t, res, "model-id-listed")
	if f.Status != StatusFail || !strings.Contains(f.Hint, "gpt-5.4-2026-03-05") {
		t.Errorf("finding = %+v, want a failure that suggests gpt-5.4-2026-03-05", f)
	}
	if res.OK() {
		t.Error("Result.OK() = true despite a failed check")
	}
}

func TestProbeDiagnosesWrongWireAPIAndMissingDeployment(t *testing.T) {
	srv := fakeAzure(t, map[string]foundry.WireAPI{"o4-mini-aw": foundry.WireResponses}, []string{"o4-mini"})

	// Forced completions against a responses-only deployment -> 400 + hint.
	r := resolve(t, foundry.Spec{Endpoint: srv.URL, Model: "o4-mini", Deployment: "o4-mini-aw", WireAPI: foundry.WireCompletions})
	f := finding(t, Check(context.Background(), r, Credentials{APIKey: testKey}, Options{Probe: true}), "deployment-probe")
	if f.Status != StatusFail || !strings.Contains(f.Hint, "wire_api=responses") {
		t.Errorf("wrong-wire finding = %+v", f)
	}

	// Unknown deployment -> 404 + hint.
	r = resolve(t, foundry.Spec{Endpoint: srv.URL, Model: "o4-mini", Deployment: "typo"})
	f = finding(t, Check(context.Background(), r, Credentials{APIKey: testKey}, Options{Probe: true}), "deployment-probe")
	if f.Status != StatusFail || !strings.Contains(f.Hint, "deployment name not found") {
		t.Errorf("missing-deployment finding = %+v", f)
	}
}

func TestBadCredentialNeverLeaks(t *testing.T) {
	srv := fakeAzure(t, nil, []string{"gpt-4o"})
	r := resolve(t, foundry.Spec{Endpoint: srv.URL, Model: "gpt-4o", Deployment: "d"})
	const secret = "super-secret-wrong-key"

	res := Check(context.Background(), r, Credentials{APIKey: secret}, Options{Probe: true})
	f := finding(t, res, "model-id-listed")
	if f.Status != StatusFail || !strings.Contains(f.Hint, "credential rejected") {
		t.Errorf("finding = %+v", f)
	}
	blob, _ := json.Marshal(res)
	if strings.Contains(string(blob), secret) {
		t.Error("credential leaked into the result")
	}
}

func TestNoCredentials(t *testing.T) {
	r := resolve(t, foundry.Spec{Resource: "svc", Model: "gpt-4o", Deployment: "d"})
	res := Check(context.Background(), r, Credentials{}, Options{})
	if res.OK() || finding(t, res, "credentials").Status != StatusFail {
		t.Errorf("findings = %+v", res.Findings)
	}
}

func TestRetriesOn429ThenSucceeds(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Write([]byte(`{"data":[{"id":"gpt-4o"}]}`))
	}))
	defer srv.Close()
	r := resolve(t, foundry.Spec{Endpoint: srv.URL, Model: "gpt-4o", Deployment: "d"})

	res := Check(context.Background(), r, Credentials{APIKey: testKey}, Options{MaxRetryWait: time.Millisecond})
	if !res.OK() || atomic.LoadInt32(&calls) != 2 {
		t.Errorf("OK=%v calls=%d findings=%+v", res.OK(), calls, res.Findings)
	}
}

func TestRedirectsAreNotFollowed(t *testing.T) {
	var leaked int32
	evil := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("api-key") != "" {
			atomic.StoreInt32(&leaked, 1)
		}
	}))
	defer evil.Close()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, evil.URL, http.StatusFound)
	}))
	defer srv.Close()
	r := resolve(t, foundry.Spec{Endpoint: srv.URL, Model: "gpt-4o", Deployment: "d"})

	res := Check(context.Background(), r, Credentials{APIKey: testKey}, Options{})
	if res.OK() {
		t.Error("a redirect must be reported as a failure")
	}
	if atomic.LoadInt32(&leaked) == 1 {
		t.Error("api-key header was replayed to the redirect target")
	}
}

func TestAnthropicProbeUsesXAPIKey(t *testing.T) {
	var gotKey, gotVersion string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/anthropic/v1/messages" {
			http.NotFound(w, r)
			return
		}
		gotKey, gotVersion = r.Header.Get("x-api-key"), r.Header.Get("anthropic-version")
		w.Write([]byte(`{"content":[{"type":"text","text":"pong"}]}`))
	}))
	defer srv.Close()
	r := resolve(t, foundry.Spec{Endpoint: srv.URL, Model: "claude-opus-4-5", Deployment: "claude-opus-4-5"})

	res := Check(context.Background(), r, Credentials{APIKey: testKey}, Options{Probe: true})
	if !res.OK() || gotKey != testKey || gotVersion == "" {
		t.Errorf("OK=%v x-api-key=%q anthropic-version=%q findings=%+v", res.OK(), gotKey, gotVersion, res.Findings)
	}
}

func TestContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	r := resolve(t, foundry.Spec{Endpoint: srv.URL, Model: "gpt-4o", Deployment: "d"})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	res := Check(ctx, r, Credentials{APIKey: testKey}, Options{MaxRetries: 5})
	if res.OK() {
		t.Error("expected failure")
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("cancellation took %v; retries must respect the context", elapsed)
	}
}
