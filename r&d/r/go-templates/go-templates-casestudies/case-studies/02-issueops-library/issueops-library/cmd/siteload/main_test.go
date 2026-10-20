package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunAgainstHTTPTest(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path == "/lib/missing" {
			http.NotFound(w, r)
			return
		}
		w.Write([]byte("<html>ok</html>"))
	}))
	defer srv.Close()
	c := Config{Base: srv.URL + "/lib/", Paths: []string{"", "ledger/open.html"}, Rate: 200, Duration: 300 * time.Millisecond, MaxP99: time.Second, MaxErr: 0.01}
	r := Run(context.Background(), c, srv.Client())
	if r.Requests < 20 || r.Errors != 0 || r.Status[200] != r.Requests {
		t.Fatalf("report %+v", r)
	}
	if int(hits.Load()) != r.Requests {
		t.Errorf("server saw %d, report %d", hits.Load(), r.Requests)
	}
	if f := r.Check(c); f != "" {
		t.Errorf("check: %s", f)
	}
	var b strings.Builder
	r.Print(&b, c)
	if !strings.Contains(b.String(), "p99") || !strings.Contains(b.String(), "/ledger/open.html") {
		t.Errorf("report:\n%s", b.String())
	}

	c.Paths = []string{"missing"}
	r = Run(context.Background(), c, srv.Client())
	if r.Errors != r.Requests || !strings.Contains(r.Check(c), "error rate") {
		t.Errorf("404s not counted: %+v", r)
	}
}

func TestPct(t *testing.T) {
	var d []time.Duration
	for i := 1; i <= 100; i++ {
		d = append(d, time.Duration(i)*time.Millisecond)
	}
	if pct(d, 50) != 50*time.Millisecond || pct(d, 99) != 99*time.Millisecond || pct(d, 100) != 100*time.Millisecond {
		t.Errorf("pct wrong: %v %v %v", pct(d, 50), pct(d, 99), pct(d, 100))
	}
	if pct(nil, 99) != 0 {
		t.Error("empty")
	}
	if (Report{}).Check(Config{}) == "" {
		t.Error("zero requests should fail")
	}
}
