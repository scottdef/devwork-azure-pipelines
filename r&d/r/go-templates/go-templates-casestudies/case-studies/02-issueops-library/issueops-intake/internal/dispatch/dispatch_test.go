package dispatch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

var meta = Meta{Issue: 42, Repo: "CoolGitOrg/issueops-intake", Requester: "alice"}

func publishReq() map[string]any {
	return map[string]any{
		"source-id": "payments-api", "template-id": "caddyfile", "phase": "1",
		"payload": "{\n  \"name\": \"payments-api\"\n}", "notes": "ignored",
	}
}

func TestBuildPublish(t *testing.T) {
	p, err := Build(EventPublish, publishReq(), meta)
	if err != nil {
		t.Fatal(err)
	}
	if len(p) > MaxKeys {
		t.Fatalf("%d keys", len(p))
	}
	want := "issue payload phase repo requester sha256 source_id template_id"
	if got := keys(p); got != want {
		t.Errorf("keys = %s", got)
	}
	if p["payload"] != `{"name":"payments-api"}` {
		t.Errorf("payload not compacted: %q", p["payload"])
	}
	sum := sha256.Sum256([]byte(`{"name":"payments-api"}`))
	if p["sha256"] != hex.EncodeToString(sum[:]) {
		t.Errorf("sha256 = %v", p["sha256"])
	}
}

func TestBuildErrors(t *testing.T) {
	big := publishReq()
	big["payload"] = `{"a":"` + strings.Repeat("x", MaxPayload) + `"}`
	bad := publishReq()
	bad["payload"] = "{"
	noSrc := publishReq()
	delete(noSrc, "source-id")
	for name, c := range map[string]struct {
		ev  string
		req map[string]any
		m   Meta
	}{
		"big":      {EventPublish, big, meta},
		"badjson":  {EventPublish, bad, meta},
		"nosource": {EventPublish, noSrc, meta},
		"event":    {"library-burn", publishReq(), meta},
		"issue":    {EventPublish, publishReq(), Meta{Repo: "a/b", Requester: "x"}},
		"repo":     {EventPublish, publishReq(), Meta{Issue: 1, Repo: "ab", Requester: "x"}},
		"borrow":   {EventBorrow, map[string]any{"item-id": "a/b/1"}, meta},
		"return":   {EventReturn, map[string]any{}, meta},
	} {
		if _, err := Build(c.ev, c.req, c.m); err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}

func TestBuildBorrowReturn(t *testing.T) {
	p, err := Build(EventBorrow, map[string]any{"item-id": "a/caddyfile/1", "days": "14", "reason": strings.Repeat("é", 1000)}, meta)
	if err != nil {
		t.Fatal(err)
	}
	if r := p["reason"].(string); len(r) > 1024 || !strings.HasPrefix(r, "é") || strings.ContainsRune(r, '�') {
		t.Errorf("reason clipped badly: %d bytes", len(r))
	}
	if keys(p) != "days issue item_id reason repo requester" {
		t.Errorf("borrow keys = %s", keys(p))
	}
	p, err = Build(EventReturn, map[string]any{"item-id": "a/caddyfile/1"}, meta)
	if err != nil || keys(p) != "issue item_id repo requester" {
		t.Errorf("return: %v %v", keys(p), err)
	}
}

func keys(m map[string]any) string {
	var k []string
	for x := range m {
		k = append(k, x)
	}
	// small, so insertion sort keeps imports down
	for i := 1; i < len(k); i++ {
		for j := i; j > 0 && k[j] < k[j-1]; j-- {
			k[j], k[j-1] = k[j-1], k[j]
		}
	}
	return strings.Join(k, " ")
}

func TestSend(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if r.Method != http.MethodPost || r.URL.Path != "/repos/CoolGitOrg/issueops-library/dispatches" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer tok123" {
			t.Errorf("Authorization = %q", got)
		}
		if r.Header.Get("X-GitHub-Api-Version") == "" || r.Header.Get("Accept") != "application/vnd.github+json" {
			t.Error("missing GitHub headers")
		}
		var body struct {
			EventType     string         `json:"event_type"`
			ClientPayload map[string]any `json:"client_payload"`
		}
		b, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(b, &body); err != nil {
			t.Error(err)
		}
		if body.EventType != EventPublish {
			t.Errorf("event_type = %q", body.EventType)
		}
		if len(body.ClientPayload) > MaxKeys {
			t.Errorf("client_payload has %d keys", len(body.ClientPayload))
		}
		if n < 3 {
			w.WriteHeader(http.StatusBadGateway) // two transient failures
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	var slept []time.Duration
	c := New(srv.URL, "tok123")
	c.Sleep = func(d time.Duration) { slept = append(slept, d) }
	p, _ := Build(EventPublish, publishReq(), meta)
	if err := c.Send(context.Background(), "CoolGitOrg/issueops-library", EventPublish, p); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 3 {
		t.Errorf("calls = %d, want 3", calls.Load())
	}
	// Backoff 2s then 4s; pacing sleeps (<=1s) may be interleaved.
	var backoff []time.Duration
	for _, d := range slept {
		if d >= 2*time.Second {
			backoff = append(backoff, d)
		}
	}
	if len(backoff) != 2 || backoff[0] != 2*time.Second || backoff[1] != 4*time.Second {
		t.Errorf("backoff sleeps = %v", slept)
	}
}

func TestSendPacing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	defer srv.Close()
	var slept []time.Duration
	c := New(srv.URL, "t")
	c.Sleep = func(d time.Duration) { slept = append(slept, d) }
	p, _ := Build(EventReturn, map[string]any{"item-id": "a/readme/1"}, meta)
	for i := 0; i < 3; i++ {
		if err := c.Send(context.Background(), "o/r", EventReturn, p); err != nil {
			t.Fatal(err)
		}
	}
	if len(slept) != 2 {
		t.Fatalf("want 2 pacing sleeps between 3 writes, got %v", slept)
	}
	for _, d := range slept {
		if d <= 0 || d > time.Second {
			t.Errorf("pacing sleep %v", d)
		}
	}
}

func TestSendClientError(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
	}))
	defer srv.Close()
	c := New(srv.URL, "t")
	c.Sleep = func(time.Duration) {}
	err := c.Send(context.Background(), "o/r", EventReturn, map[string]any{})
	se, ok := err.(*StatusError)
	if !ok || se.Code != 404 || calls.Load() != 1 {
		t.Errorf("err = %v, calls = %d", err, calls.Load())
	}
	c.Token = ""
	if err := c.Send(context.Background(), "o/r", EventReturn, nil); err == nil {
		t.Error("no token: want error")
	}
}

func TestSendGivesUp(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "7")
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	var slept []time.Duration
	c := New(srv.URL, "t")
	c.Retries = 2
	c.Sleep = func(d time.Duration) { slept = append(slept, d) }
	err := c.Send(context.Background(), "o/r", EventReturn, map[string]any{})
	if err == nil || !strings.Contains(err.Error(), "gave up after 3") {
		t.Fatalf("err = %v", err)
	}
	found := false
	for _, d := range slept {
		found = found || d == 7*time.Second
	}
	if !found {
		t.Errorf("Retry-After ignored: %v", slept)
	}
}
