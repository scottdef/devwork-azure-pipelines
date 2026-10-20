package web

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/CoolGitOrg/tmplforge/internal/forge"
)

func TestMain(m *testing.M) {
	forge.SandboxChildMain()
	os.Exit(m.Run())
}

func newServer(t *testing.T, cfg Config) *Server {
	t.Helper()
	if cfg.Sandbox == nil {
		cfg.Sandbox = &forge.Sandbox{Exe: os.Args[0], Timeout: 2 * time.Second}
	}
	if cfg.Addr == "" {
		cfg.Addr = "127.0.0.1:0"
	}
	s, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

type resp struct {
	code int
	body string
	h    http.Header
}

func do(t *testing.T, h http.Handler, method, target string, form url.Values, hdr map[string]string) resp {
	t.Helper()
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	r := httptest.NewRequest(method, target, body)
	if form != nil {
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if method == http.MethodPost {
		r.Header.Set("HX-Request", "true")
	}
	for k, v := range hdr {
		if v == "" {
			r.Header.Del(k)
		} else {
			r.Header.Set(k, v)
		}
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return resp{w.Code, w.Body.String(), w.Header()}
}

func TestPages(t *testing.T) {
	s := newServer(t, Config{})
	for _, tc := range []struct {
		path string
		want []string
	}{
		{"/", []string{"Catalog", `href="/builder?name=Dockerfile"`, "Corefile", "25 starter templates"}},
		{"/builder", []string{"Builder", `hx-post="/render"`, `hx-trigger="keyup changed delay:300ms"`, `hx-post="/builder/var"`}},
		{"/builder?name=email.html", []string{"email.html", "&lt;!doctype html&gt;", `<option value="html" selected>`, `name="var_name" value="subject"`}},
		{"/builder?name=workflow.yml", []string{`name="delims" value="[[,]]"`}},
		{"/test?name=motd.txt", []string{"Fixture A", "== {{ upper .host }} =="}},
		{"/new", []string{"IssueOps kit", "CI/CD workflow", "repository_dispatch"}},
		{"/healthz", []string{"ok"}},
		{"/static/htmx.min.js", []string{"htmx"}},
		{"/static/app.js", []string{"htmx:afterSwap"}},
		{"/static/style.css", []string{":root"}},
	} {
		r := do(t, s, "GET", tc.path, nil, nil)
		if r.code != 200 {
			t.Errorf("GET %s: %d", tc.path, r.code)
			continue
		}
		for _, w := range tc.want {
			if !strings.Contains(r.body, w) {
				t.Errorf("GET %s: missing %q", tc.path, w)
			}
		}
	}
	if r := do(t, s, "GET", "/builder?name=nope", nil, nil); r.code != 404 {
		t.Errorf("unknown entry: %d", r.code)
	}
	if r := do(t, s, "GET", "/nope", nil, nil); r.code != 404 {
		t.Errorf("unknown path: %d", r.code)
	}
	if r := do(t, s, "DELETE", "/", nil, nil); r.code != 405 && r.code != 403 {
		t.Errorf("DELETE /: %d", r.code)
	}
}

func TestSecurityHeaders(t *testing.T) {
	s := newServer(t, Config{})
	r := do(t, s, "GET", "/", nil, nil)
	csp := r.h.Get("Content-Security-Policy")
	if !strings.Contains(csp, "script-src 'self'") || strings.Contains(csp, "unsafe-inline") || strings.Contains(csp, "unsafe-eval") {
		t.Errorf("CSP %q", csp)
	}
	if r.h.Get("X-Content-Type-Options") != "nosniff" || r.h.Get("X-Frame-Options") != "DENY" {
		t.Errorf("headers %v", r.h)
	}
	if strings.Contains(r.body, "<script>") || strings.Contains(r.body, " onclick=") {
		t.Error("inline script in page")
	}
	r = do(t, s, "GET", "/chart", nil, nil)
	if c := r.h.Get("Content-Security-Policy"); !strings.Contains(c, "script-src 'self';") {
		t.Errorf("chart CSP relaxes script-src: %q", c)
	}
}

func form(kv ...string) url.Values {
	v := url.Values{}
	for i := 0; i+1 < len(kv); i += 2 {
		v.Add(kv[i], kv[i+1])
	}
	return v
}

func TestRender(t *testing.T) {
	s := newServer(t, Config{})
	r := do(t, s, "POST", "/render", form("template", "Hello {{.name}}!", "fixture", `{"name":"<b>rob</b>"}`, "format", "txt"), nil)
	if r.code != 200 || !strings.Contains(r.body, "<pre class=\"out\">Hello &lt;b&gt;rob&lt;/b&gt;!</pre>") {
		t.Errorf("%d %s", r.code, r.body)
	}
	// declared defaults fill the data
	r = do(t, s, "POST", "/render", form("template", "{{.n | add 1}}", "var_name", "n", "var_type", "int", "var_default", "41"), nil)
	if !strings.Contains(r.body, ">42</pre>") {
		t.Errorf("var default: %s", r.body)
	}
	// html format: output is html/template-escaped by the sandbox, then
	// escaped again for display inside <pre>
	r = do(t, s, "POST", "/render", form("template", "<p>{{.x}}</p>", "fixture", `{"x":"<script>"}`, "format", "html"), nil)
	if !strings.Contains(r.body, "&lt;p&gt;&amp;lt;script&amp;gt;&lt;/p&gt;") {
		t.Errorf("html escaping: %s", r.body)
	}
	// error line highlighting
	r = do(t, s, "POST", "/render", form("template", "a\nb\n{{.x | nofunc}}"), nil)
	if !strings.Contains(r.body, `class="err"`) || !strings.Contains(r.body, `<span class="bad"><span class="ln">3</span>`) {
		t.Errorf("error highlight: %s", r.body)
	}
	// unsafe funcs are absent in the GUI
	r = do(t, s, "POST", "/render", form("template", `{{env "HOME"}}`), nil)
	if !strings.Contains(r.body, "not defined") {
		t.Errorf("env reachable: %s", r.body)
	}
	// bad fixture
	r = do(t, s, "POST", "/render", form("template", "x", "fixture", "[1]"), nil)
	if !strings.Contains(r.body, "want JSON object") {
		t.Errorf("bad fixture: %s", r.body)
	}
	// delimiters
	r = do(t, s, "POST", "/render", form("template", "[[.a]] ${{ x }}", "fixture", `{"a":1}`, "delims", "[[,]]"), nil)
	if !strings.Contains(r.body, ">1 ${{ x }}</pre>") {
		t.Errorf("delims: %s", r.body)
	}
}

func TestRenderLimits(t *testing.T) {
	s := newServer(t, Config{Sandbox: &forge.Sandbox{Exe: os.Args[0], Timeout: 300 * time.Millisecond}})
	start := time.Now()
	r := do(t, s, "POST", "/render", form("template", "{{range seq 100000000}}{{end}}"), nil)
	if !strings.Contains(r.body, "timed out") {
		t.Errorf("timeout: %s", r.body)
	}
	if time.Since(start) > 3*time.Second {
		t.Error("timeout too slow")
	}
	r = do(t, s, "POST", "/render", form("template", strings.Repeat("x", forge.MaxTemplate+1)), nil)
	if !strings.Contains(r.body, "template too large") {
		t.Errorf("template cap: %.200s", r.body)
	}
	r = do(t, s, "POST", "/render", form("template", `{{range seq 2000}}{{repeat 1000 "x"}}{{end}}`), nil)
	if !strings.Contains(r.body, "output limit exceeded") {
		t.Errorf("output cap: %.200s", r.body)
	}
	r = do(t, s, "POST", "/render", form("template", strings.Repeat("x", DefaultMaxBody+1)), nil)
	if r.code != http.StatusRequestEntityTooLarge {
		t.Errorf("body cap: %d", r.code)
	}
}

// TestCrashingChildDoesNotKillServer: a template that kills its renderer
// with a runtime fatal error (stack overflow) leaves the server serving.
func TestCrashingChildDoesNotKillServer(t *testing.T) {
	s := newServer(t, Config{})
	r := do(t, s, "POST", "/render", form("template", `{{define "r"}}{{template "r" .}}{{end}}{{template "r" .}}`), nil)
	if r.code != 200 || !strings.Contains(r.body, "crashed") {
		t.Errorf("crash: %d %s", r.code, r.body)
	}
	r = do(t, s, "POST", "/render", form("template", "still {{ \"here\" }}"), nil)
	if !strings.Contains(r.body, "still here") {
		t.Errorf("after crash: %s", r.body)
	}
	m := do(t, s, "GET", "/metrics", nil, nil)
	if !strings.Contains(m.body, `tmplforge_renders_total{outcome="crash"} 1`) {
		t.Errorf("crash not counted:\n%s", m.body)
	}
}

func TestLintVarsTest(t *testing.T) {
	s := newServer(t, Config{})
	r := do(t, s, "POST", "/lint", form("template", "{{.a}}\n{{.b}", "var_name", "a"), nil)
	if !strings.Contains(r.body, `<li class="err">untitled:2: bad character`) {
		t.Errorf("lint error: %s", r.body)
	}
	r = do(t, s, "POST", "/lint", form("template", "{{.a}}\n{{.b}}", "var_name", "a", "var_type", "string"), nil)
	if !strings.Contains(r.body, `<li class="warn">untitled:2: warning: .b referenced but not declared`) {
		t.Errorf("lint warn: %s", r.body)
	}
	r = do(t, s, "POST", "/lint", form("template", "{{.a}}"), nil)
	if !strings.Contains(r.body, "lint: ok") {
		t.Errorf("lint ok: %s", r.body)
	}
	r = do(t, s, "POST", "/vars", form("template", "{{range .users}}{{.name}}{{end}}"), nil)
	if !strings.Contains(r.body, "&#34;users&#34;: [") || !strings.Contains(r.body, "<code>.users[].name</code>") {
		t.Errorf("vars: %s", r.body)
	}
	r = do(t, s, "POST", "/vars?raw=1", form("template", "{{.a.b}}"), nil)
	if r.h.Get("Content-Type") != "application/json" || !strings.Contains(r.body, `"b": ""`) {
		t.Errorf("vars raw: %s", r.body)
	}
	r = do(t, s, "POST", "/test", form("template", "{{.n}}\nsame", "fixture_a", `{"n":1}`, "fixture_b", `{"n":2}`), nil)
	if !strings.Contains(r.body, `<span class="del">- 1`) || !strings.Contains(r.body, `<span class="add">&#43; 2`) || !strings.Contains(r.body, "+1 -1") {
		t.Errorf("test diff: %d %q", r.code, r.body)
	}
	r = do(t, s, "POST", "/test", form("template", "x", "fixture_a", `{}`, "fixture_b", `{}`), nil)
	if !strings.Contains(r.body, "identical") {
		t.Errorf("test same: %s", r.body)
	}
}

func TestVarRow(t *testing.T) {
	s := newServer(t, Config{})
	r := do(t, s, "POST", "/builder/var", form("new_name", "port", "new_type", "int", "new_default", "8080"), nil)
	if !strings.Contains(r.body, `name="var_name" value="port"`) || !strings.Contains(r.body, "<option selected>int</option>") {
		t.Errorf("var row: %s", r.body)
	}
	r = do(t, s, "POST", "/builder/var", form("new_name", "port", "new_type", "int", "new_default", "x"), nil)
	if !strings.Contains(r.body, `class="err"`) {
		t.Errorf("var error: %s", r.body)
	}
	r = do(t, s, "POST", "/builder/var", form("new_name", `"><script>`, "new_type", "string"), nil)
	if strings.Contains(r.body, "<script>") {
		t.Errorf("unescaped: %s", r.body)
	}
}

func TestExport(t *testing.T) {
	s := newServer(t, Config{})
	r := do(t, s, "POST", "/export", form("name", "../evil name", "template", "hi {{.who}}", "fixture", `{"who":"glenda"}`), nil)
	if r.code != 200 || r.h.Get("Content-Type") != "application/gzip" || !strings.Contains(r.h.Get("Content-Disposition"), `filename="evil-name.tar.gz"`) {
		t.Fatalf("%d %v", r.code, r.h)
	}
	zr, err := gzip.NewReader(strings.NewReader(r.body))
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(zr)
	got := map[string]string{}
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(tr)
		got[h.Name] = string(b)
	}
	if got["evil-name/evil-name.txt"] != "hi glenda" || got["evil-name/SHA256SUMS"] == "" || got["evil-name/run.go"] == "" {
		t.Errorf("bundle: %v", keys(got))
	}
}

func keys(m map[string]string) []string {
	var k []string
	for x := range m {
		k = append(k, x)
	}
	return k
}

func TestGenerate(t *testing.T) {
	s := newServer(t, Config{})
	r := do(t, s, "POST", "/new/issueops", form("name", "repo-request", "fields", "repo:string!,vis:dropdown=a|b"), nil)
	for _, w := range []string{".github/ISSUE_TEMPLATE/repo-request.yml", ".github/workflows/repo-request-parse.yml", "cmd/repo-request-parse/main.go", "${{ github.event.issue.body }}"} {
		if !strings.Contains(r.body, w) {
			t.Errorf("issueops missing %q", w)
		}
	}
	r = do(t, s, "POST", "/new/issueops", form("name", "Bad Name", "fields", "a"), nil)
	if !strings.Contains(r.body, `class="err"`) {
		t.Errorf("bad issueops: %s", r.body)
	}
	r = do(t, s, "POST", "/new/cicd", form("kind", "dispatch", "binary", "tmplforge", "inputs", "env:choice=beta|prod"), nil)
	if !strings.Contains(r.body, "INPUT_ENV: ${{ inputs.env }}") {
		t.Errorf("cicd: %s", r.body)
	}
	r = do(t, s, "POST", "/new/cicd", form("kind", "repository_dispatch", "types", "a, b"), nil)
	if !strings.Contains(r.body, "types: [&#34;a&#34;,&#34;b&#34;]") {
		t.Errorf("cicd types: %s", r.body)
	}
	r = do(t, s, "POST", "/new/cicd", form("kind", "bogus"), nil)
	if !strings.Contains(r.body, "unknown kind") {
		t.Errorf("bad kind: %s", r.body)
	}
}

func TestMetricsAndChart(t *testing.T) {
	s := newServer(t, Config{Version: "v9.9.9"})
	do(t, s, "POST", "/render", form("template", "x"), nil)
	do(t, s, "POST", "/render", form("template", "{{"), nil)
	r := do(t, s, "GET", "/metrics", nil, nil)
	for _, w := range []string{
		`tmplforge_build_info{version="v9.9.9"`,
		`tmplforge_renders_total{outcome="ok"} 1`,
		`tmplforge_renders_total{outcome="template_error"} 1`,
		`tmplforge_render_seconds_bucket{le="+Inf"} 2`,
		`tmplforge_render_seconds_count 2`,
		`tmplforge_http_requests_total{route="POST /render",code="200"} 2`,
		"# TYPE tmplforge_render_seconds histogram",
	} {
		if !strings.Contains(r.body, w) {
			t.Errorf("metrics missing %q:\n%s", w, r.body)
		}
	}
	if !strings.HasPrefix(r.h.Get("Content-Type"), "text/plain; version=0.0.4") {
		t.Errorf("metrics content type %q", r.h.Get("Content-Type"))
	}
	r = do(t, s, "GET", "/chart", nil, nil)
	for _, w := range []string{`<script src="/static/echarts/echarts.min.js" defer></script>`, `<script src="/chart.js?w=10s" defer></script>`, `id="latency"`, "<th>p95 ms</th>", `id="chart-missing"`} {
		if !strings.Contains(r.body, w) {
			t.Errorf("chart missing %q", w)
		}
	}
	if strings.Contains(r.body, "go-echarts.github.io") {
		t.Error("chart references the public CDN")
	}
	if strings.Count(r.body, "<tr><td>") < 1 {
		t.Error("no data rows in chart table")
	}
	r = do(t, s, "GET", "/chart.js", nil, nil)
	for _, w := range []string{`typeof echarts === "undefined"`, `echarts.init(document.getElementById('latency')`, `"name":"p95"`} {
		if !strings.Contains(r.body, w) {
			t.Errorf("chart.js missing %q:\n%s", w, r.body)
		}
	}
	if strings.Contains(r.body, "<script") {
		t.Error("chart.js still wrapped in <script>")
	}
}

func TestSeries(t *testing.T) {
	t0 := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	var ss []Sample
	for i := 1; i <= 20; i++ {
		ss = append(ss, Sample{At: t0.Add(time.Duration(i%2) * 10 * time.Second), Dur: time.Duration(i) * time.Millisecond})
	}
	// out of order windows are fine; Series groups consecutive runs
	pts := Series([]Sample{{t0, 5 * time.Millisecond}, {t0, 1 * time.Millisecond}, {t0.Add(15 * time.Second), 9 * time.Millisecond}}, 10*time.Second)
	if len(pts) != 2 || pts[0].N != 2 || pts[0].P50 != time.Millisecond || pts[0].P95 != 5*time.Millisecond || pts[1].P50 != 9*time.Millisecond {
		t.Errorf("%+v", pts)
	}
	if Series(nil, time.Second) != nil {
		t.Error("empty")
	}
	_ = ss
}

func TestCSRF(t *testing.T) {
	s := newServer(t, Config{})
	f := form("template", "x")
	for _, tc := range []struct {
		hdr  map[string]string
		want int
	}{
		{map[string]string{"HX-Request": ""}, 403},                                                      // no evidence at all
		{map[string]string{"HX-Request": "", "Sec-Fetch-Site": "cross-site"}, 403},                      // browser says cross-site
		{map[string]string{"HX-Request": "true", "Sec-Fetch-Site": "cross-site"}, 403},                  // header cannot override browser
		{map[string]string{"HX-Request": "", "Origin": "https://evil.example"}, 403},                    // foreign origin
		{map[string]string{"HX-Request": "", "Sec-Fetch-Site": "same-origin"}, 200},                     // modern browser, plain form
		{map[string]string{"HX-Request": "", "Origin": "http://example.com"}, 200},                      // httptest Host is example.com
		{map[string]string{"HX-Request": "true"}, 200},                                                  // htmx, old browser
		{map[string]string{"HX-Request": "", "Sec-Fetch-Site": "same-site", "Origin": "http://x"}, 403}, // sibling subdomain
	} {
		r := do(t, s, "POST", "/render", f, tc.hdr)
		if r.code != tc.want {
			t.Errorf("%v: %d, want %d", tc.hdr, r.code, tc.want)
		}
	}
	// GETs are never blocked
	if r := do(t, s, "GET", "/", nil, map[string]string{"Sec-Fetch-Site": "cross-site"}); r.code != 200 {
		t.Errorf("cross-site GET: %d", r.code)
	}
	m := do(t, s, "GET", "/metrics", nil, nil)
	if !strings.Contains(m.body, "tmplforge_csrf_rejected_total 5") {
		t.Errorf("csrf metric:\n%s", m.body)
	}
}

func TestRateLimit(t *testing.T) {
	s := newServer(t, Config{Rate: 2}) // burst 4
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s.limiter.Now = func() time.Time { return now }
	codes := []int{}
	for i := 0; i < 6; i++ {
		codes = append(codes, do(t, s, "GET", "/", nil, nil).code)
	}
	want := []int{200, 200, 200, 200, 429, 429}
	for i := range want {
		if codes[i] != want[i] {
			t.Fatalf("codes %v, want %v", codes, want)
		}
	}
	r := do(t, s, "GET", "/", nil, nil)
	if r.h.Get("Retry-After") != "1" {
		t.Errorf("Retry-After %q", r.h.Get("Retry-After"))
	}
	now = now.Add(500 * time.Millisecond) // one token back
	if c := do(t, s, "GET", "/", nil, nil).code; c != 200 {
		t.Errorf("after refill: %d", c)
	}
	// a different client has its own bucket
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "10.9.9.9:1234"
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Errorf("other client: %d", w.Code)
	}
	// healthz is exempt
	if c := do(t, s, "GET", "/healthz", nil, nil).code; c != 200 {
		t.Errorf("healthz limited: %d", c)
	}
}

func TestLimiterSweep(t *testing.T) {
	l := NewLimiter(1)
	now := time.Unix(0, 0)
	l.Now = func() time.Time { return now }
	for i := 0; i < maxBuckets+10; i++ {
		l.Allow(net.IPv4(10, byte(i>>16), byte(i>>8), byte(i)).String())
	}
	if len(l.buckets) > maxBuckets {
		t.Errorf("%d buckets", len(l.buckets))
	}
}

func TestAuth(t *testing.T) {
	tok := "0123456789abcdef0123"
	s := newServer(t, Config{Token: tok, Addr: "0.0.0.0:8088", Rate: 100})
	if r := do(t, s, "GET", "/", nil, nil); r.code != 401 || r.h.Get("WWW-Authenticate") == "" {
		t.Errorf("no token: %d", r.code)
	}
	if r := do(t, s, "GET", "/", nil, map[string]string{"Authorization": "Bearer wrong"}); r.code != 401 {
		t.Errorf("wrong token: %d", r.code)
	}
	if r := do(t, s, "GET", "/", nil, map[string]string{"Authorization": "Bearer " + tok}); r.code != 200 {
		t.Errorf("bearer: %d", r.code)
	}
	r := do(t, s, "GET", "/?token="+tok, nil, nil)
	if r.code != 200 || !strings.Contains(r.h.Get("Set-Cookie"), tokenCookie+"="+tok) || !strings.Contains(r.h.Get("Set-Cookie"), "HttpOnly") {
		t.Errorf("query token: %d %q", r.code, r.h.Get("Set-Cookie"))
	}
	if r := do(t, s, "POST", "/render", form("template", "x"), map[string]string{"Cookie": tokenCookie + "=" + tok}); r.code != 200 {
		t.Errorf("cookie: %d", r.code)
	}
	if r := do(t, s, "GET", "/healthz", nil, nil); r.code != 200 {
		t.Errorf("healthz needs no token: %d", r.code)
	}
}

func TestCheckConfig(t *testing.T) {
	for _, tc := range []struct {
		c  Config
		ok bool
	}{
		{Config{Addr: "127.0.0.1:8088"}, true},
		{Config{Addr: "[::1]:8088"}, true},
		{Config{Addr: "localhost:8088"}, true},
		{Config{Addr: "0.0.0.0:8088"}, false},
		{Config{Addr: ":8088"}, false},
		{Config{Addr: "0.0.0.0:8088", Token: "short", Rate: 10}, false},
		{Config{Addr: "0.0.0.0:8088", Token: "0123456789abcdef", Rate: 0}, false},
		{Config{Addr: "0.0.0.0:8088", Token: "0123456789abcdef", Rate: 10}, true},
	} {
		c := tc.c
		if err := CheckConfig(&c); (err == nil) != tc.ok {
			t.Errorf("%+v: %v", tc.c, err)
		}
	}
}

func TestConcurrencyLimit(t *testing.T) {
	s := newServer(t, Config{MaxConcurrent: 1, Sandbox: &forge.Sandbox{Exe: os.Args[0], Timeout: 2 * time.Second}})
	s.sem <- struct{}{} // occupy the only slot
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	req := httptest.NewRequest("POST", "/render", strings.NewReader("template=x")).WithContext(ctx)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	w := httptest.NewRecorder()
	s.ServeHTTP(w, req)
	if w.Code != 503 || w.Header().Get("Retry-After") == "" {
		t.Errorf("busy: %d", w.Code)
	}
	<-s.sem
}

func TestServeShutdown(t *testing.T) {
	s := newServer(t, Config{})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error)
	go func() { done <- Serve(ctx, ln, s) }()
	res, err := http.Get("http://" + ln.Addr().String() + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Error(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no shutdown")
	}
}

func TestAssetsOverlay(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(dir+"/echarts", 0o755)
	os.WriteFile(dir+"/echarts/echarts.min.js", []byte("var echarts={};"), 0o644)
	s := newServer(t, Config{AssetsDir: dir})
	if r := do(t, s, "GET", "/static/echarts/echarts.min.js", nil, nil); r.code != 200 || r.body != "var echarts={};" {
		t.Errorf("overlay: %d %q", r.code, r.body)
	}
	if r := do(t, s, "GET", "/static/htmx.min.js", nil, nil); r.code != 200 {
		t.Errorf("base through overlay: %d", r.code)
	}
}
