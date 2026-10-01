package clone

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coolado/adoclone/internal/client"
	"github.com/coolado/adoclone/internal/jx"
	"github.com/coolado/adoclone/internal/metrics"
	"github.com/coolado/adoclone/internal/state"
)

const (
	srcID   = "11111111-1111-1111-1111-111111111111"
	tgtID   = "22222222-2222-2222-2222-222222222222"
	srcRepo = "33333333-3333-3333-3333-333333333333"
	tgtRepo = "44444444-4444-4444-4444-444444444444"
)

type handler func(w http.ResponseWriter, r *http.Request, m []string, body []byte)

type route struct {
	method string
	re     *regexp.Regexp
	h      handler
}

// fake is a tiny Azure DevOps stand-in: regexp routes on the decoded path.
type fake struct {
	mu     sync.Mutex
	routes []route
	calls  []string
}

func (f *fake) on(method, pattern string, h handler) {
	f.routes = append(f.routes, route{method, regexp.MustCompile("^" + pattern + "$"), h})
}

func (f *fake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	f.mu.Lock()
	f.calls = append(f.calls, r.Method+" "+r.URL.Path)
	f.mu.Unlock()
	for _, rt := range f.routes {
		if rt.method != r.Method {
			continue
		}
		if m := rt.re.FindStringSubmatch(r.URL.Path); m != nil {
			rt.h(w, r, m, body)
			return
		}
	}
	w.WriteHeader(http.StatusNotFound)
	io.WriteString(w, `{"message":"no fake route"}`)
}

func (f *fake) count(method, contains string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if strings.HasPrefix(c, method+" ") && strings.Contains(c, contains) {
			n++
		}
	}
	return n
}

func writeJSON(w http.ResponseWriter, v any) { json.NewEncoder(w).Encode(v) }

func projectRoutes(f *fake) {
	for name, id := range map[string]string{"Src": srcID, "Tgt": tgtID} {
		p := jx.M{"id": id, "name": name, "visibility": "private",
			"defaultTeam":  jx.M{"id": id, "name": name + " Team"},
			"capabilities": jx.M{"versioncontrol": jx.M{"sourceControlType": "Git"}, "processTemplate": jx.M{"templateTypeId": "proc"}}}
		f.on("GET", "/CoolADO/_apis/projects/"+name, func(w http.ResponseWriter, _ *http.Request, _ []string, _ []byte) { writeJSON(w, p) })
	}
}

func newTestCtx(t *testing.T, f *fake, dry bool, statePath string) *Ctx {
	t.Helper()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	u, _ := url.Parse(srv.URL)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	c := client.New("CoolADO", "pat", log, dry)
	c.Rewrite = func(x *url.URL) { x.Scheme, x.Host = "http", u.Host }
	c.Sleep = func(context.Context, time.Duration) error { return nil }
	m := metrics.New()
	c.Metrics = m
	if statePath == "" {
		statePath = filepath.Join(t.TempDir(), "state.json")
	}
	s, err := state.Load(statePath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return &Ctx{C: c, S: s, M: m, Log: log, Src: "Src", Tgt: "Tgt", Getenv: func(string) string { return "" },
		Opt: Options{BypassRules: true, Visibility: "private", RepoMode: "mirror", VarGroupMode: "copy", Git: "git", WorkDir: t.TempDir(), PAT: "pat"}}
}

// mapCtx returns a non-dry context with no server, for transform tests.
func mapCtx(t *testing.T, maps map[string]map[string]string) *Ctx {
	x := newTestCtx(t, &fake{}, false, "")
	x.S.SetProjects(srcID, tgtID)
	for kind, m := range maps {
		for s, d := range m {
			x.S.Put(kind, s, d)
		}
	}
	return x
}
