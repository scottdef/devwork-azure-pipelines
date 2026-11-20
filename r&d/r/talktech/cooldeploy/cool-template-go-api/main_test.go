package main

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRoutes(t *testing.T) {
	s, err := newServer(slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(s.routes())
	defer srv.Close()

	for path, want := range map[string]struct {
		code int
		body string
	}{
		"/healthz":          {200, "ok"},
		"/api/items":        {200, `"name":"First item"`},
		"/api/items/item-2": {200, `"id":"item-2"`},
		"/api/items/nope":   {404, `"error"`},
		"/nothing":          {404, ""},
	} {
		resp, err := http.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != want.code || !strings.Contains(string(b), want.body) {
			t.Errorf("GET %s = %d %q, want %d containing %q", path, resp.StatusCode, b, want.code, want.body)
		}
	}

	// The platform scrapes /metrics; the requests above must be counted.
	resp, err := http.Get(srv.URL + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(b), `http_requests_total{code="200",route="GET /api/items"} 1`) {
		t.Errorf("/metrics does not count requests by route:\n%s", b)
	}
}
