// Package testutil provides in-process fakes of the GitHub Actions REST
// API and the Dynatrace log ingest endpoint for tests.
package testutil

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/CoolGitOrg/gh-log-forwarder/internal/github"
)

// GitHub is a fake GitHub API. Mutate it between calls under Lock.
type GitHub struct {
	sync.Mutex
	Runs  map[string][]github.Run // repo -> runs
	Jobs  map[int64][]github.Job  // run ID -> jobs
	Logs  map[int64]string        // job ID -> log; absent = 404
	Repos []string                // what /installation/repositories returns
	Token string                  // expected bearer token ("" = any)
	Calls map[string]int
	srv   *httptest.Server
	blobs *httptest.Server // a different host:port, like GitHub's blob storage
}

// NewGitHub starts a fake GitHub API.
func NewGitHub() *GitHub {
	g := &GitHub{Runs: map[string][]github.Run{}, Jobs: map[int64][]github.Job{}, Logs: map[int64]string{}, Calls: map[string]int{}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /repos/{owner}/{repo}/actions/runs", g.listRuns)
	mux.HandleFunc("GET /repos/{owner}/{repo}/actions/runs/{id}", g.getRun)
	mux.HandleFunc("GET /repos/{owner}/{repo}/actions/runs/{id}/jobs", g.listJobs)
	mux.HandleFunc("GET /repos/{owner}/{repo}/actions/jobs/{id}/logs", g.jobLog)
	mux.HandleFunc("GET /installation/repositories", g.installationRepos)
	g.srv = httptest.NewServer(g.auth(mux))
	bmux := http.NewServeMux()
	bmux.HandleFunc("GET /blob/{id}", g.blob)
	g.blobs = httptest.NewServer(g.auth(bmux))
	return g
}

// URL is the API base URL.
func (g *GitHub) URL() string { return g.srv.URL }

// Close stops the server.
func (g *GitHub) Close() { g.srv.Close(); g.blobs.Close() }

func (g *GitHub) auth(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		g.Lock()
		g.Calls[r.Method+" "+routeKey(r.URL.Path)]++
		want := g.Token
		g.Unlock()
		if !strings.HasPrefix(r.URL.Path, "/blob/") && want != "" && r.Header.Get("Authorization") != "Bearer "+want {
			writeJSON(w, 401, map[string]string{"message": "Bad credentials"})
			return
		}
		if strings.HasPrefix(r.URL.Path, "/blob/") && r.Header.Get("Authorization") != "" {
			writeJSON(w, 400, map[string]string{"message": "token leaked to blob host"})
			return
		}
		w.Header().Set("X-RateLimit-Remaining", "14999")
		h.ServeHTTP(w, r)
	})
}

func routeKey(p string) string {
	parts := strings.Split(p, "/")
	for i, s := range parts {
		if _, err := strconv.Atoi(s); err == nil {
			parts[i] = "{id}"
		}
	}
	return strings.Join(parts, "/")
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func repoOf(r *http.Request) string { return r.PathValue("owner") + "/" + r.PathValue("repo") }

func (g *GitHub) listRuns(w http.ResponseWriter, r *http.Request) {
	g.Lock()
	defer g.Unlock()
	var lo, hi time.Time
	if c := r.URL.Query().Get("created"); c != "" {
		a, b, _ := strings.Cut(c, "..")
		lo, _ = time.Parse("2006-01-02T15:04:05Z", a)
		hi, _ = time.Parse("2006-01-02T15:04:05Z", b)
	}
	runs := []github.Run{}
	for _, run := range g.Runs[repoOf(r)] {
		ct := run.CreatedAt.Truncate(time.Second)
		if !lo.IsZero() && (ct.Before(lo) || ct.After(hi)) {
			continue
		}
		runs = append(runs, run)
	}
	sort.Slice(runs, func(i, j int) bool { return runs[i].CreatedAt.After(runs[j].CreatedAt) }) // newest first, like GitHub
	writeJSON(w, 200, map[string]any{"total_count": len(runs), "workflow_runs": runs})
}

func (g *GitHub) findRun(repo string, id int64) (github.Run, bool) {
	for _, run := range g.Runs[repo] {
		if run.ID == id {
			return run, true
		}
	}
	return github.Run{}, false
}

func (g *GitHub) getRun(w http.ResponseWriter, r *http.Request) {
	g.Lock()
	defer g.Unlock()
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	run, ok := g.findRun(repoOf(r), id)
	if !ok {
		writeJSON(w, 404, map[string]string{"message": "Not Found"})
		return
	}
	writeJSON(w, 200, run)
}

func (g *GitHub) listJobs(w http.ResponseWriter, r *http.Request) {
	g.Lock()
	defer g.Unlock()
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	jobs := g.Jobs[id]
	if r.URL.Query().Get("filter") != "all" {
		writeJSON(w, 400, map[string]string{"message": "test fake expects filter=all"})
		return
	}
	if jobs == nil {
		jobs = []github.Job{}
	}
	writeJSON(w, 200, map[string]any{"total_count": len(jobs), "jobs": jobs})
}

func (g *GitHub) jobLog(w http.ResponseWriter, r *http.Request) {
	g.Lock()
	defer g.Unlock()
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if _, ok := g.Logs[id]; !ok {
		writeJSON(w, 404, map[string]string{"message": "Not Found"})
		return
	}
	// Mimic GitHub: redirect to signed storage on another host.
	http.Redirect(w, r, g.blobs.URL+"/blob/"+strconv.FormatInt(id, 10), http.StatusFound)
}

func (g *GitHub) blob(w http.ResponseWriter, r *http.Request) {
	g.Lock()
	defer g.Unlock()
	id, _ := strconv.ParseInt(r.PathValue("id"), 10, 64)
	w.Header().Set("Content-Type", "text/plain")
	io.WriteString(w, g.Logs[id])
}

func (g *GitHub) installationRepos(w http.ResponseWriter, r *http.Request) {
	g.Lock()
	defer g.Unlock()
	var rs []map[string]any
	for _, n := range g.Repos {
		rs = append(rs, map[string]any{"full_name": n, "archived": strings.HasSuffix(n, "-archived")})
	}
	writeJSON(w, 200, map[string]any{"total_count": len(rs), "repositories": rs})
}

// Log renders lines as a GitHub job log starting at t, one line per
// 100ms, with a BOM like the real thing.
func Log(t time.Time, lines ...string) string {
	var b strings.Builder
	b.WriteString("\xef\xbb\xbf")
	for i, l := range lines {
		fmt.Fprintf(&b, "%s %s\n", t.Add(time.Duration(i)*100*time.Millisecond).UTC().Format("2006-01-02T15:04:05.0000000Z"), l)
	}
	return b.String()
}

// Dynatrace is a fake log ingest endpoint.
type Dynatrace struct {
	sync.Mutex
	Records []map[string]any
	Headers []string // X-Dynatrace-Attr per request
	Status  int      // if non-zero, answer every request with this
	Token   string
	srv     *httptest.Server
}

// NewDynatrace starts a fake ingest endpoint.
func NewDynatrace() *Dynatrace {
	d := &Dynatrace{Token: "dt0c01.test"}
	d.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		d.Lock()
		defer d.Unlock()
		if r.Method != "POST" || r.URL.Path != "/api/v2/logs/ingest" {
			w.WriteHeader(404)
			return
		}
		if r.Header.Get("Authorization") != "Api-Token "+d.Token {
			w.WriteHeader(401)
			return
		}
		if d.Status != 0 {
			w.WriteHeader(d.Status)
			return
		}
		var recs []map[string]any
		if err := json.NewDecoder(r.Body).Decode(&recs); err != nil {
			w.WriteHeader(400)
			return
		}
		d.Records = append(d.Records, recs...)
		d.Headers = append(d.Headers, r.Header.Get("X-Dynatrace-Attr"))
		w.WriteHeader(204)
	}))
	return d
}

// URL is the environment URL.
func (d *Dynatrace) URL() string { return d.srv.URL }

// Close stops the server.
func (d *Dynatrace) Close() { d.srv.Close() }

// Snapshot returns a copy of the records received so far.
func (d *Dynatrace) Snapshot() []map[string]any {
	d.Lock()
	defer d.Unlock()
	return append([]map[string]any(nil), d.Records...)
}
