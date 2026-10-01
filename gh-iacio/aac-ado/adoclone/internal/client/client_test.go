package client

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/coolado/adoclone/internal/metrics"
)

func testClient(t *testing.T, h http.HandlerFunc, dry bool) (*Client, *[]time.Duration) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	c := New("CoolADO", "pat", slog.New(slog.NewTextHandler(io.Discard, nil)), dry)
	c.Rewrite = func(x *url.URL) { x.Scheme, x.Host = "http", u.Host }
	var slept []time.Duration
	c.Sleep = func(_ context.Context, d time.Duration) error { slept = append(slept, d); return nil }
	c.Metrics = metrics.New()
	return c, &slept
}

func TestRetryAfterThenSuccess(t *testing.T) {
	n := 0
	c, slept := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Basic OnBhdA==" {
			t.Errorf("auth header = %q", r.Header.Get("Authorization"))
		}
		n++
		if n == 1 {
			w.Header().Set("Retry-After", "3")
			w.WriteHeader(429)
			return
		}
		io.WriteString(w, `{"count":7}`)
	}, false)
	var r struct{ Count int }
	if err := c.Get(context.Background(), c.URL("", "x"), &r); err != nil || r.Count != 7 || n != 2 {
		t.Fatalf("err=%v count=%d calls=%d", err, r.Count, n)
	}
	if len(*slept) != 1 || (*slept)[0] != 3*time.Second {
		t.Fatalf("slept %v, want [3s]", *slept)
	}
	if got := c.Metrics.Value("adoclone_throttle_seconds_total"); got != 3 {
		t.Fatalf("throttle seconds = %v", got)
	}
}

func TestPostNotRetriedOn500(t *testing.T) {
	n := 0
	c, _ := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		n++
		w.WriteHeader(500)
		io.WriteString(w, "boom")
	}, false)
	err := c.Post(context.Background(), c.URL("", "x"), map[string]int{"a": 1}, nil)
	if StatusOf(err) != 500 || n != 1 {
		t.Fatalf("status=%d calls=%d err=%v", StatusOf(err), n, err)
	}
	if c.Metrics.Value("adoclone_errors_total") != 1 {
		t.Fatal("error not counted")
	}
}

func TestGetRetriedOn500(t *testing.T) {
	n := 0
	c, _ := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		n++
		if n < 3 {
			w.WriteHeader(500)
			return
		}
		io.WriteString(w, `{}`)
	}, false)
	if err := c.Get(context.Background(), c.URL("", "x"), nil); err != nil || n != 3 {
		t.Fatalf("err=%v calls=%d", err, n)
	}
}

func TestNotFoundIsTyped(t *testing.T) {
	c, _ := testClient(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(404) }, false)
	if err := c.Get(context.Background(), c.URL("", "x"), nil); !IsNotFound(err) {
		t.Fatalf("want 404, got %v", err)
	}
}

func TestDryRunBlocksWritesButAllowsReads(t *testing.T) {
	var seen []string
	c, _ := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Method+" "+r.URL.Path)
		io.WriteString(w, `{"workItems":[]}`)
	}, true)
	ctx := context.Background()
	_ = c.Post(ctx, c.URL("", "P/_apis/wit/workitems/$Bug"), []int{1}, nil)
	_ = c.Patch(ctx, c.URL("", "_apis/x"), nil, nil)
	_ = c.Post(ctx, c.URL("", "P/_apis/wit/wiql?api-version=7.1"), map[string]string{"query": "q"}, nil)
	_ = c.Post(ctx, c.URL("", "_apis/wit/workitemsbatch"), nil, nil)
	_ = c.Get(ctx, c.URL("", "_apis/projects"), nil)
	want := "POST /CoolADO/P/_apis/wit/wiql,POST /CoolADO/_apis/wit/workitemsbatch,GET /CoolADO/_apis/projects"
	if got := strings.Join(seen, ","); got != want {
		t.Fatalf("sent %s\nwant %s", got, want)
	}
}

func TestListFollowsHeaderAndBodyTokens(t *testing.T) {
	c, _ := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Query().Get("continuationToken") {
		case "":
			w.Header().Set("X-Ms-Continuationtoken", "h2")
			io.WriteString(w, `{"value":[1,2]}`)
		case "h2":
			io.WriteString(w, `{"value":[3],"continuationToken":"b3"}`)
		case "b3":
			io.WriteString(w, `{"value":[4]}`)
		}
	}, false)
	items, err := c.List(context.Background(), c.URL("", "_apis/x?api-version=7.1"))
	if err != nil || len(items) != 4 {
		t.Fatalf("items=%d err=%v", len(items), err)
	}
}

func TestWorkItemIDsPagesPastTheCap(t *testing.T) {
	const total, page = 45000, 19000
	re := regexp.MustCompile(`\[System\.Id\] > (\d+)`)
	c, _ := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("$top") != "19000" {
			t.Errorf("$top = %q", r.URL.Query().Get("$top"))
		}
		var q struct{ Query string }
		json.NewDecoder(r.Body).Decode(&q)
		if !strings.Contains(q.Query, "'O''Brien'") {
			t.Errorf("project not escaped: %s", q.Query)
		}
		last, _ := strconv.Atoi(re.FindStringSubmatch(q.Query)[1])
		var b bytes.Buffer
		b.WriteString(`{"workItems":[`)
		for i, id := 0, last+1; i < page && id <= total; i, id = i+1, id+1 {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(`{"id":` + strconv.Itoa(id) + `}`)
		}
		b.WriteString(`]}`)
		w.Write(b.Bytes())
	}, false)
	ids, err := c.WorkItemIDs(context.Background(), "O'Brien", "")
	if err != nil || len(ids) != total || ids[total-1] != total {
		t.Fatalf("got %d ids, err=%v", len(ids), err)
	}
}

func TestDownloadStreams(t *testing.T) {
	c, _ := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept") != "application/octet-stream" {
			t.Errorf("accept = %q", r.Header.Get("Accept"))
		}
		io.WriteString(w, "hello")
	}, true)
	var b bytes.Buffer
	n, err := c.Download(context.Background(), c.URL("", "_apis/wit/attachments/x"), &b)
	if err != nil || n != 5 || b.String() != "hello" {
		t.Fatalf("n=%d err=%v body=%q", n, err, b.String())
	}
}

func TestWithHeader(t *testing.T) {
	c, _ := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Range") != "bytes 0-1/2" || r.Header.Get("Content-Type") != "application/octet-stream" {
			t.Errorf("headers %v", r.Header)
		}
	}, false)
	if _, err := c.Do(context.Background(), http.MethodPut, c.URL("", "x"), "application/octet-stream", []byte("ab"), nil, WithHeader("Content-Range", "bytes 0-1/2")); err != nil {
		t.Fatal(err)
	}
}

func TestPostNotRetriedOn502(t *testing.T) {
	n := 0
	c, _ := testClient(t, func(w http.ResponseWriter, r *http.Request) { n++; w.WriteHeader(502) }, false)
	if err := c.Post(context.Background(), c.URL("", "x"), nil, nil); StatusOf(err) != 502 || n != 1 {
		t.Fatalf("calls=%d err=%v", n, err)
	}
}
