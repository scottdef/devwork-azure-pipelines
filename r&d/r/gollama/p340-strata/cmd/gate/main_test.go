package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func post(t *testing.T, url, key, body string) (*http.Response, string) {
	t.Helper()
	req, _ := http.NewRequest("POST", url+"/v1/chat/completions", strings.NewReader(body))
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	return resp, string(b)
}

func TestFailoverRewriteAuth(t *testing.T) {
	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "loading", http.StatusServiceUnavailable)
	}))
	defer down.Close()
	var seenModel, seenKey string
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var m struct{ Model string }
		json.NewDecoder(r.Body).Decode(&m)
		seenModel, seenKey = m.Model, r.Header.Get("Authorization")
		io.WriteString(w, "data: one\n\n")
		w.(http.Flusher).Flush()
		io.WriteString(w, "data: two\n\n")
	}))
	defer up.Close()
	dead := httptest.NewServer(nil)
	dead.Close() // connection refused

	t.Setenv("UP_KEY", "sk-upstream")
	g := &Gate{Key: "sk-gate", Client: http.DefaultClient, Routes: map[string][]Upstream{
		"brain": {{URL: dead.URL}, {URL: down.URL}, {URL: up.URL, Model: "qwen36", KeyEnv: "UP_KEY"}},
		"gone":  {{URL: dead.URL}},
	}}
	srv := httptest.NewServer(g)
	defer srv.Close()

	if resp, _ := post(t, srv.URL, "wrong", `{"model":"brain"}`); resp.StatusCode != 401 {
		t.Fatalf("bad key: got %d, want 401", resp.StatusCode)
	}
	resp, body := post(t, srv.URL, "sk-gate", `{"model":"brain","stream":true}`)
	if resp.StatusCode != 200 || !strings.Contains(body, "one") || !strings.Contains(body, "two") {
		t.Fatalf("failover: got %d %q", resp.StatusCode, body)
	}
	if seenModel != "qwen36" || seenKey != "Bearer sk-upstream" {
		t.Fatalf("rewrite: model %q key %q", seenModel, seenKey)
	}
	if got := resp.Header.Get("X-Gate-Upstream"); got != up.URL {
		t.Fatalf("upstream header %q, want %q", got, up.URL)
	}
	if resp, _ := post(t, srv.URL, "sk-gate", `{"model":"nope"}`); resp.StatusCode != 404 {
		t.Fatalf("unknown alias: got %d, want 404", resp.StatusCode)
	}
	if resp, _ := post(t, srv.URL, "sk-gate", `{"model":"gone"}`); resp.StatusCode != 502 {
		t.Fatalf("all down: got %d, want 502", resp.StatusCode)
	}
}

func TestModelsList(t *testing.T) {
	g := &Gate{Client: http.DefaultClient, Routes: map[string][]Upstream{"fast": nil, "brain": nil}}
	srv := httptest.NewServer(g)
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/v1/models")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(b), `"brain"`) || strings.Index(string(b), "brain") > strings.Index(string(b), "fast") {
		t.Fatalf("models list: %s", b)
	}
}
