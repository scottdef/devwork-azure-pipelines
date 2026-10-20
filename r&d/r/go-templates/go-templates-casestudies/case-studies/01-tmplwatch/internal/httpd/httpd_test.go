package httpd

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/CoolGitOrg/tmplwatch/internal/metrics"
	"github.com/CoolGitOrg/tmplwatch/internal/runner"
)

type fake struct {
	complete bool
}

func (f *fake) Status() runner.Status {
	return runner.Status{Healthy: f.complete, Templates: []runner.TemplateStatus{
		{Name: "Caddyfile", Complete: true}, {Name: "nginx", Complete: f.complete},
	}}
}

func (f *fake) Current(n string) ([]byte, bool) {
	if n == "Caddyfile" {
		return []byte(":80 {\n}\n"), true
	}
	return nil, false
}

func (f *fake) Names() []string { return []string{"Caddyfile", "nginx"} }

func get(t *testing.T, srv *httptest.Server, path string) (int, string, http.Header) {
	t.Helper()
	resp, err := http.Get(srv.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b), resp.Header
}

var sample = regexp.MustCompile(`^[a-zA-Z_:][a-zA-Z0-9_:]*(\{[^}]*\})? (\S+)$`)

func TestEndpoints(t *testing.T) {
	f := &fake{}
	m := metrics.New("test")
	m.Template("Caddyfile")
	m.Render("Caddyfile", time.Millisecond, nil, false)
	m.Poll("unchanged")
	srv := httptest.NewServer(Handler(Options{State: f, Metrics: m}))
	defer srv.Close()

	if code, body, _ := get(t, srv, "/healthz"); code != 503 || !strings.Contains(body, "nginx") {
		t.Errorf("healthz while incomplete: %d %q", code, body)
	}
	f.complete = true
	if code, body, _ := get(t, srv, "/healthz"); code != 200 || body != "ok\n" {
		t.Errorf("healthz: %d %q", code, body)
	}

	code, body, hdr := get(t, srv, "/metrics")
	if code != 200 || !strings.HasPrefix(hdr.Get("Content-Type"), "text/plain; version=0.0.4") {
		t.Fatalf("metrics: %d %s", code, hdr.Get("Content-Type"))
	}
	want := map[string]bool{
		"tmplwatch_renders_total": false, "tmplwatch_render_errors_total": false,
		"tmplwatch_render_duration_seconds": false, "tmplwatch_source_polls_total": false,
		"tmplwatch_last_poll_timestamp_seconds": false, "tmplwatch_commands_total": false,
	}
	sc := bufio.NewScanner(strings.NewReader(body))
	n := 0
	for sc.Scan() {
		l := sc.Text()
		n++
		if strings.HasPrefix(l, "# TYPE ") {
			name := strings.Fields(l)[2]
			if _, ok := want[name]; ok {
				want[name] = true
			}
			continue
		}
		if strings.HasPrefix(l, "# HELP ") {
			continue
		}
		if !sample.MatchString(l) {
			t.Errorf("line %d not a valid sample: %q", n, l)
		}
	}
	for k, ok := range want {
		if !ok {
			t.Errorf("metrics missing family %s", k)
		}
	}
	if !strings.Contains(body, `tmplwatch_renders_total{template="Caddyfile"} 1`) {
		t.Error("renders_total sample missing")
	}

	if code, body, hdr := get(t, srv, "/render?template=Caddyfile"); code != 200 || body != ":80 {\n}\n" || hdr.Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("render: %d %q", code, body)
	}
	if code, _, _ := get(t, srv, "/render?template=nginx"); code != 404 {
		t.Errorf("render incomplete: %d", code)
	}
	if _, body, _ := get(t, srv, "/render"); body != "Caddyfile\nnginx\n" {
		t.Errorf("render list %q", body)
	}

	_, body, _ = get(t, srv, "/status")
	var st runner.Status
	if err := json.Unmarshal([]byte(body), &st); err != nil || len(st.Templates) != 2 {
		t.Errorf("status: %v %s", err, body)
	}
	if _, body, _ := get(t, srv, "/debug/vars"); !strings.Contains(body, `"tmplwatch"`) {
		t.Error("expvar missing tmplwatch")
	}
	if code, _, _ := get(t, srv, "/debug/pprof/"); code != 404 {
		t.Errorf("pprof served without -pprof: %d", code)
	}
	resp, _ := http.Post(srv.URL+"/healthz", "text/plain", nil)
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("POST healthz: %d", resp.StatusCode)
	}
}

func TestPprof(t *testing.T) {
	srv := httptest.NewServer(Handler(Options{State: &fake{}, Metrics: metrics.New("t"), Pprof: true}))
	defer srv.Close()
	if code, _, _ := get(t, srv, "/debug/pprof/"); code != 200 {
		t.Errorf("pprof: %d", code)
	}
}

func TestIsLoopback(t *testing.T) {
	for addr, want := range map[string]bool{
		"127.0.0.1:9180": true, "localhost:1": true, "[::1]:9180": true, "127.0.0.2:1": true,
		":9180": false, "0.0.0.0:9180": false, "10.0.0.1:80": false,
	} {
		if IsLoopback(addr) != want {
			t.Errorf("IsLoopback(%q) != %v", addr, want)
		}
	}
}
