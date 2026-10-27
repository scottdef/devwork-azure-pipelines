package github

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func newTestClient(srv *httptest.Server) *Client {
	c := New(srv.URL, "tok")
	c.Sleep = func(context.Context, time.Duration) error { return nil }
	return c
}

func TestPaginationFollowsLink(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" || r.Header.Get("X-GitHub-Api-Version") == "" {
			t.Errorf("missing headers: %v", r.Header)
		}
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page == 0 {
			page = 1
		}
		if page < 3 {
			w.Header().Set("Link", fmt.Sprintf(`<%s/repos/o/a/actions/runs/1/jobs?filter=all&page=%d>; rel="next", <x>; rel="last"`, srv.URL, page+1))
		}
		fmt.Fprintf(w, `{"total_count":3,"jobs":[{"id":%d,"run_attempt":%d,"status":"completed"}]}`, 10-page, 4-page)
	}))
	defer srv.Close()
	jobs, err := newTestClient(srv).ListJobs(context.Background(), "o/a", 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 3 || jobs[0].RunAttempt != 1 || jobs[2].RunAttempt != 3 {
		t.Fatalf("jobs %+v (want 3, sorted by attempt)", jobs)
	}
}

func TestJobLogRedirectDropsToken(t *testing.T) {
	blob := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if a := r.Header.Get("Authorization"); a != "" {
			t.Errorf("blob host received Authorization %q", a)
		}
		io.WriteString(w, "2026-10-27T10:00:00.0000000Z hello\n")
	}))
	defer blob.Close()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, blob.URL+"/signed?sig=x", http.StatusFound)
	}))
	defer api.Close()
	rc, err := newTestClient(api).JobLog(context.Background(), "o/a", 5)
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	b, _ := io.ReadAll(rc)
	if !strings.Contains(string(b), "hello") {
		t.Fatalf("body %q", b)
	}
}

func TestJobLog404RetriedThenNotFound(t *testing.T) {
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.Add(1)
		http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
	}))
	defer srv.Close()
	_, err := newTestClient(srv).JobLog(context.Background(), "o/a", 5)
	if !IsNotFound(err) || n.Load() != 4 {
		t.Fatalf("err=%v calls=%d (want 404 after 1+3 tries)", err, n.Load())
	}
}

func TestRetriesServerErrors(t *testing.T) {
	var n atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if n.Add(1) < 3 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		io.WriteString(w, `{"id":42,"status":"completed"}`)
	}))
	defer srv.Close()
	r, err := newTestClient(srv).GetRun(context.Background(), "o/a", 42)
	if err != nil || r.ID != 42 || n.Load() != 3 {
		t.Fatalf("run=%+v err=%v calls=%d", r, err, n.Load())
	}
}

func TestRateLimits(t *testing.T) {
	t.Run("secondary Retry-After", func(t *testing.T) {
		var n atomic.Int32
		var waited time.Duration
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if n.Add(1) == 1 {
				w.Header().Set("Retry-After", "7")
				http.Error(w, `{"message":"You have exceeded a secondary rate limit"}`, http.StatusForbidden)
				return
			}
			w.Header().Set("X-RateLimit-Remaining", "4321")
			io.WriteString(w, `{"id":1}`)
		}))
		defer srv.Close()
		c := newTestClient(srv)
		c.Sleep = func(_ context.Context, d time.Duration) error { waited += d; return nil }
		if _, err := c.GetRun(context.Background(), "o/a", 1); err != nil {
			t.Fatal(err)
		}
		if waited < 7*time.Second || waited > 8*time.Second {
			t.Errorf("waited %s, want ~7s", waited)
		}
		if c.RateRemaining() != 4321 {
			t.Errorf("remaining %d", c.RateRemaining())
		}
	})
	t.Run("primary resets too late", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-RateLimit-Remaining", "0")
			w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(time.Now().Add(50*time.Minute).Unix(), 10))
			http.Error(w, `{"message":"API rate limit exceeded"}`, http.StatusForbidden)
		}))
		defer srv.Close()
		_, err := newTestClient(srv).GetRun(context.Background(), "o/a", 1)
		if !errors.Is(err, ErrRateLimited) {
			t.Fatalf("err=%v", err)
		}
	})
	t.Run("401 is ErrAuth", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, `{"message":"Bad credentials"}`, http.StatusUnauthorized)
		}))
		defer srv.Close()
		_, err := newTestClient(srv).GetRun(context.Background(), "o/a", 1)
		if !errors.Is(err, ErrAuth) {
			t.Fatalf("err=%v", err)
		}
	})
	t.Run("403 permission is not retried", func(t *testing.T) {
		var n atomic.Int32
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			n.Add(1)
			http.Error(w, `{"message":"Resource not accessible by integration"}`, http.StatusForbidden)
		}))
		defer srv.Close()
		_, err := newTestClient(srv).GetRun(context.Background(), "o/a", 1)
		var se *StatusError
		if !errors.As(err, &se) || se.Code != 403 || n.Load() != 1 {
			t.Fatalf("err=%v calls=%d", err, n.Load())
		}
	})
}

func TestListRunsSplitsWindowAt1000(t *testing.T) {
	from := time.Date(2026, 10, 27, 0, 0, 0, 0, time.UTC)
	to := from.Add(4 * time.Hour)
	// 1500 runs spread evenly: any window wider than 2h reports >= 1000.
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		a, b, _ := strings.Cut(r.URL.Query().Get("created"), "..")
		lo, _ := time.Parse(createdLayout, a)
		hi, _ := time.Parse(createdLayout, b)
		var runs []Run
		for i := 0; i < 1500; i++ {
			ct := from.Add(time.Duration(i) * 4 * time.Hour / 1500).Truncate(time.Second)
			if !ct.Before(lo) && !ct.After(hi) {
				runs = append(runs, Run{ID: int64(i + 1), CreatedAt: ct, Status: "completed"})
			}
		}
		total := len(runs)
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page == 0 {
			page = 1
		}
		start, end := (page-1)*100, min(page*100, total, 1000)
		if start > end {
			start = end
		}
		if end < min(total, 1000) {
			q := r.URL.Query()
			q.Set("page", strconv.Itoa(page+1))
			w.Header().Set("Link", fmt.Sprintf(`<http://%s%s?%s>; rel="next"`, r.Host, r.URL.Path, q.Encode()))
		}
		json.NewEncoder(w).Encode(runsPage{TotalCount: total, WorkflowRuns: runs[start:end]})
	}))
	defer srv.Close()
	runs, err := newTestClient(srv).ListRuns(context.Background(), "o/a", from, to)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 1500 {
		t.Fatalf("got %d runs, want 1500 (window split missing?)", len(runs))
	}
	for i := 1; i < len(runs); i++ {
		if runs[i].CreatedAt.Before(runs[i-1].CreatedAt) {
			t.Fatal("runs not sorted by created_at")
		}
	}
}

func TestAppJWTAndInstallationToken(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	pemPKCS1 := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	p8, _ := x509.MarshalPKCS8PrivateKey(key)
	pemPKCS8 := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: p8})
	for _, p := range [][]byte{pemPKCS1, pemPKCS8} {
		if _, err := ParsePrivateKey(p); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := ParsePrivateKey([]byte("nope")); err == nil {
		t.Fatal("garbage key accepted")
	}

	now := time.Unix(1_800_000_000, 0)
	jwt, err := AppJWT("123456", key, now)
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		t.Fatalf("jwt %q", jwt)
	}
	sig, _ := base64.RawURLEncoding.DecodeString(parts[2])
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(&key.PublicKey, crypto.SHA256, sum[:], sig); err != nil {
		t.Fatalf("signature: %v", err)
	}
	claimsJSON, _ := base64.RawURLEncoding.DecodeString(parts[1])
	var claims map[string]any
	json.Unmarshal(claimsJSON, &claims)
	if claims["iss"] != float64(123456) || claims["iat"] != float64(now.Unix()-60) || claims["exp"] != float64(now.Unix()+540) {
		t.Fatalf("claims %v", claims)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+jwt {
			t.Errorf("app endpoints must use the JWT")
		}
		switch {
		case r.Method == "GET" && r.URL.Path == "/orgs/CoolGitOrg/installation":
			io.WriteString(w, `{"id":777}`)
		case r.Method == "POST" && r.URL.Path == "/app/installations/777/access_tokens":
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			if body["permissions"].(map[string]any)["actions"] != "read" {
				t.Errorf("body %v", body)
			}
			w.WriteHeader(201)
			io.WriteString(w, `{"token":"ghs_x","expires_at":"2026-10-27T13:00:00Z"}`)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	c := newTestClient(srv)
	c.Token = "must-not-be-used"
	id, err := c.OrgInstallationID(context.Background(), jwt, "CoolGitOrg")
	if err != nil || id != 777 {
		t.Fatalf("id=%d err=%v", id, err)
	}
	tok, err := c.NewInstallationToken(context.Background(), jwt, id, nil, map[string]string{"actions": "read"})
	if err != nil || tok.Token != "ghs_x" {
		t.Fatalf("tok=%+v err=%v", tok, err)
	}
}

func TestNextLink(t *testing.T) {
	h := http.Header{}
	h.Add("Link", `<https://api.github.com/x?page=1>; rel="prev", <https://api.github.com/x?page=3>; rel="next"`)
	if got := NextLink(h); got != "https://api.github.com/x?page=3" {
		t.Fatalf("%q", got)
	}
	if NextLink(http.Header{}) != "" {
		t.Fatal("no link should be empty")
	}
}
