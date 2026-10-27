package dynatrace

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func newTestClient(url string) *Client {
	c := NewClient(url, "dt0c01.secret", "Api-Token", 5*time.Second)
	c.Sleep = func(context.Context, time.Duration) error { return nil }
	return c
}

func TestPostOutcomes(t *testing.T) {
	cases := []struct {
		status  int
		body    string
		outcome Outcome
		err     error
	}{
		{204, "", Accepted, nil},
		{200, `{"details":{"message":"1 invalid"}}`, Partial, nil},
		{400, `{"error":{"code":400,"message":"All logs are out of correct time range."}}`, Stale, nil},
		{401, `{}`, 0, ErrAuth},
		{403, `{}`, 0, ErrAuth},
		{402, `{}`, 0, ErrBudget},
		{413, `{}`, 0, ErrTooLarge},
	}
	for _, tc := range cases {
		t.Run(fmt.Sprint(tc.status), func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != IngestPath || r.Header.Get("Authorization") != "Api-Token dt0c01.secret" ||
					!strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
					t.Errorf("bad request %s %v", r.URL.Path, r.Header)
				}
				w.WriteHeader(tc.status)
				io.WriteString(w, tc.body)
			}))
			defer srv.Close()
			resp, err := newTestClient(srv.URL).Post(context.Background(), []byte(`[{"content":"x"}]`), "")
			if tc.err != nil {
				if !errors.Is(err, tc.err) {
					t.Fatalf("err=%v want %v", err, tc.err)
				}
				return
			}
			if err != nil || resp.Outcome != tc.outcome {
				t.Fatalf("resp=%+v err=%v", resp, err)
			}
		})
	}
}

func TestPostRetriesThrottlingWithRetryAfter(t *testing.T) {
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch n.Add(1) {
		case 1:
			w.Header().Set("Retry-After", "3")
			w.WriteHeader(429)
		case 2:
			w.WriteHeader(503)
		default:
			w.WriteHeader(204)
		}
	}))
	defer srv.Close()
	c := newTestClient(srv.URL)
	var waits []time.Duration
	c.Sleep = func(_ context.Context, d time.Duration) error { waits = append(waits, d); return nil }
	resp, err := c.Post(context.Background(), []byte(`[]`), "")
	if err != nil || resp.Outcome != Accepted || resp.Retries != 2 {
		t.Fatalf("resp=%+v err=%v", resp, err)
	}
	if len(waits) != 2 || waits[0] != 3*time.Second {
		t.Fatalf("waits %v", waits)
	}
}

func TestPostGivesUp(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }))
	defer srv.Close()
	c := newTestClient(srv.URL)
	c.MaxRetries = 2
	if _, err := c.Post(context.Background(), []byte(`[]`), ""); err == nil {
		t.Fatal("expected failure after retries")
	}
}

func TestGzipAndHeaderAttrs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Encoding") != "gzip" {
			t.Error("missing Content-Encoding")
		}
		if r.Header.Get("X-Dynatrace-Attr") != "log.source=github-actions" {
			t.Errorf("attr header %q", r.Header.Get("X-Dynatrace-Attr"))
		}
		zr, err := gzip.NewReader(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(zr)
		if string(b) != `[{"content":"x"}]` {
			t.Errorf("body %s", b)
		}
		w.WriteHeader(204)
	}))
	defer srv.Close()
	c := newTestClient(srv.URL)
	c.Gzip = true
	if _, err := c.Post(context.Background(), []byte(`[{"content":"x"}]`), "log.source=github-actions"); err != nil {
		t.Fatal(err)
	}
}

// fakePoster records requests and answers by rule.
type fakePoster struct {
	mu       sync.Mutex
	batches  [][]json.RawMessage
	maxBytes int
	reject   string // a record containing this is "invalid" (400)
}

func (f *fakePoster) Post(_ context.Context, payload []byte, _ string) (Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.maxBytes > 0 && len(payload) > f.maxBytes {
		return Response{Code: 413}, ErrTooLarge
	}
	if f.reject != "" && bytes.Contains(payload, []byte(f.reject)) {
		return Response{Code: 400}, &RejectedError{Code: 400, Message: "invalid"}
	}
	var recs []json.RawMessage
	if err := json.Unmarshal(payload, &recs); err != nil {
		return Response{}, err
	}
	f.batches = append(f.batches, recs)
	return Response{Outcome: Accepted, Code: 204}, nil
}

func rec(i int) []byte { return []byte(fmt.Sprintf(`{"content":"line %04d"}`, i)) }

func TestShipperRespectsLimitsAndOrder(t *testing.T) {
	fp := &fakePoster{}
	s := &Shipper{Poster: fp, MaxBytes: 200, MaxRecords: 5}
	ctx := context.Background()
	for i := 0; i < 23; i++ {
		if err := s.Add(ctx, rec(i)); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, b := range fp.batches {
		size := 2
		for _, r := range b {
			if string(r) != string(rec(n)) {
				t.Fatalf("order broken at %d: %s", n, r)
			}
			size += len(r) + 1
			n++
		}
		if len(b) > 5 || size > 200 {
			t.Fatalf("batch of %d records / %d bytes exceeds limits", len(b), size)
		}
	}
	if n != 23 || s.Added() != 23 || s.Flushed() != 23 || s.Stats.Records != 23 {
		t.Fatalf("n=%d added=%d flushed=%d stats=%+v", n, s.Added(), s.Flushed(), s.Stats)
	}
}

func TestShipperSplitsOn413AndIsolatesBadRecord(t *testing.T) {
	fp := &fakePoster{maxBytes: 90, reject: "line 0007"}
	s := &Shipper{Poster: fp, MaxBytes: 10 << 20, MaxRecords: 1000}
	ctx := context.Background()
	for i := 0; i < 16; i++ {
		s.Add(ctx, rec(i))
	}
	if err := s.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	got := 0
	for _, b := range fp.batches {
		got += len(b)
	}
	if got != 15 || s.Stats.Rejected != 1 || s.Stats.Splits == 0 || s.Flushed() != 16 {
		t.Fatalf("shipped %d, stats %+v, flushed %d", got, s.Stats, s.Flushed())
	}
}

func TestShipperHeaderChangeFlushes(t *testing.T) {
	fp := &fakePoster{}
	s := &Shipper{Poster: fp, MaxBytes: 1 << 20, MaxRecords: 100}
	ctx := context.Background()
	s.SetHeader(ctx, "a=1")
	s.Add(ctx, rec(1))
	s.SetHeader(ctx, "a=1")
	s.Add(ctx, rec(2))
	s.SetHeader(ctx, "a=2")
	s.Add(ctx, rec(3))
	s.Flush(ctx)
	if len(fp.batches) != 2 || len(fp.batches[0]) != 2 {
		t.Fatalf("batches %v", fp.batches)
	}
}

func TestShipperDryRun(t *testing.T) {
	var out bytes.Buffer
	s := &Shipper{DryRun: &out, MaxBytes: 1 << 20, MaxRecords: 2}
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		s.Add(ctx, rec(i))
	}
	s.Flush(ctx)
	if strings.Count(out.String(), "\n") != 3 || s.Flushed() != 3 {
		t.Fatalf("%q", out.String())
	}
}
