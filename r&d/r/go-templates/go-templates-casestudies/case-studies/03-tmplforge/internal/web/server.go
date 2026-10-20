// Package web is tmplforge's HTML GUI: html/template pages, htmx for
// interaction, no client framework. Every render of user-supplied template
// text goes through forge.Sandbox in a child process; the server itself
// only parses (lint, vars) and never executes untrusted templates.
package web

import (
	"context"
	"crypto/subtle"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/CoolGitOrg/tmplforge/internal/forge"
	"github.com/CoolGitOrg/tmplforge/internal/funcs"
)

//go:embed tmpl/*.html
var tmplFS embed.FS

//go:embed static
var staticFS embed.FS

// Config configures a Server.
type Config struct {
	Addr          string         // listen address, for the loopback check
	Token         string         // required bearer token; "" disables auth
	Rate          float64        // requests/second per client IP; 0 disables limiting
	Sandbox       *forge.Sandbox // renderer; default &forge.Sandbox{}
	MaxConcurrent int            // concurrent sandbox processes; default 2*NumCPU
	MaxBody       int64          // request body cap; default 256 KiB
	AssetsDir     string         // optional on-disk overlay for /static/
	Version       string
	Log           *log.Logger // nil discards
}

// Server is the GUI http.Handler.
type Server struct {
	cfg     Config
	mux     *http.ServeMux
	pages   map[string]*template.Template
	parts   *template.Template
	metrics *Metrics
	limiter *Limiter
	sem     chan struct{}
	catalog *forge.Catalog
	static  http.Handler
}

// DefaultMaxBody bounds every request body: a 64 KiB template, two
// fixtures and form overhead fit with room to spare.
const DefaultMaxBody = 256 << 10

// IsLoopback reports whether addr ("host:port") binds only loopback.
func IsLoopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// CheckConfig enforces the exposure rule: off loopback, a token is
// mandatory and rate limiting is on.
func CheckConfig(c *Config) error {
	if !IsLoopback(c.Addr) {
		if c.Token == "" {
			return fmt.Errorf("serve: %s is not loopback; -token is required", c.Addr)
		}
		if len(c.Token) < 16 {
			return errors.New("serve: -token must be at least 16 characters")
		}
		if c.Rate <= 0 {
			return errors.New("serve: -rate must be positive when not on loopback")
		}
	}
	return nil
}

// New builds a Server. It parses every page template up front so a broken
// template fails at start, not on first request.
func New(cfg Config) (*Server, error) {
	if cfg.Sandbox == nil {
		cfg.Sandbox = &forge.Sandbox{}
	}
	if cfg.MaxConcurrent <= 0 {
		cfg.MaxConcurrent = 2 * runtime.NumCPU()
	}
	if cfg.MaxBody <= 0 {
		cfg.MaxBody = DefaultMaxBody
	}
	if cfg.Version == "" {
		cfg.Version = "dev"
	}
	if cfg.Log == nil {
		cfg.Log = log.New(io.Discard, "", 0)
	}
	cat, err := forge.LoadCatalog()
	if err != nil {
		return nil, err
	}
	s := &Server{
		cfg:     cfg,
		mux:     http.NewServeMux(),
		pages:   map[string]*template.Template{},
		metrics: NewMetrics(),
		sem:     make(chan struct{}, cfg.MaxConcurrent),
		catalog: cat,
	}
	if cfg.Rate > 0 {
		s.limiter = NewLimiter(cfg.Rate)
	}
	if err := s.parseTemplates(); err != nil {
		return nil, err
	}
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		return nil, err
	}
	var sfs fs.FS = sub
	if cfg.AssetsDir != "" {
		sfs = overlay{os.DirFS(cfg.AssetsDir), sub}
	}
	s.static = http.StripPrefix("/static/", http.FileServerFS(sfs))
	s.routes()
	return s, nil
}

// overlay serves from top first, then base: operators drop
// echarts/echarts.min.js into -assets DIR without rebuilding.
type overlay struct{ top, base fs.FS }

func (o overlay) Open(name string) (fs.File, error) {
	if f, err := o.top.Open(name); err == nil {
		return f, nil
	}
	return o.base.Open(name)
}

var tmplFuncs = template.FuncMap{
	"dict":   funcs.All()["dict"],
	"ms":     func(d time.Duration) string { return strconv.FormatFloat(ms(d), 'f', 1, 64) },
	"hhmmss": func(t time.Time) string { return t.UTC().Format("15:04:05") },
}

func (s *Server) parseTemplates() error {
	parts, err := template.New("items.html").Funcs(tmplFuncs).ParseFS(tmplFS, "tmpl/items.html")
	if err != nil {
		return err
	}
	s.parts = parts
	for _, p := range []string{"index.html", "builder.html", "test.html", "new.html", "chart.html"} {
		t, err := template.New("layout.html").Funcs(tmplFuncs).ParseFS(tmplFS, "tmpl/layout.html", "tmpl/items.html", "tmpl/"+p)
		if err != nil {
			return fmt.Errorf("web: %s: %w", p, err)
		}
		s.pages[p] = t
	}
	return nil
}

func (s *Server) routes() {
	m := s.mux
	m.HandleFunc("GET /{$}", s.handleIndex)
	m.HandleFunc("GET /builder", s.handleBuilder)
	m.HandleFunc("POST /builder/var", s.handleVar)
	m.HandleFunc("POST /render", s.handleRender)
	m.HandleFunc("POST /lint", s.handleLint)
	m.HandleFunc("POST /vars", s.handleVars)
	m.HandleFunc("GET /test", s.handleTestPage)
	m.HandleFunc("POST /test", s.handleTest)
	m.HandleFunc("POST /export", s.handleExport)
	m.HandleFunc("GET /new", s.handleNewPage)
	m.HandleFunc("POST /new/issueops", s.handleNewIssueOps)
	m.HandleFunc("POST /new/cicd", s.handleNewCICD)
	m.HandleFunc("GET /metrics", s.handleMetrics)
	m.HandleFunc("GET /chart", s.handleChart)
	m.HandleFunc("GET /chart.js", s.handleChartJS)
	m.HandleFunc("GET /healthz", s.handleHealthz)
	m.Handle("GET /static/", s.static)
}

// Metrics exposes the server's metrics, for tests and the CLI.
func (s *Server) Metrics() *Metrics { return s.metrics }

// ServeHTTP applies, outermost first: metrics, security headers, auth,
// rate limit, cross-site check, body cap; then routes.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	sw := &statusWriter{ResponseWriter: w, code: http.StatusOK}
	s.serve(sw, r)
	route := r.Pattern
	if route == "" {
		route = "unmatched"
	}
	s.metrics.Request(route, sw.code)
	s.cfg.Log.Printf("%s %s %d %v", r.Method, r.URL.Path, sw.code, time.Since(start).Round(time.Microsecond))
}

// CSP: scripts only from this origin (htmx, /chart.js, a locally served
// echarts.min.js); no inline script, no eval. /chart relaxes style-src for
// ECharts' inline tooltip styles; nothing relaxes script-src.
const (
	cspBase  = "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; object-src 'none'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'"
	cspChart = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; object-src 'none'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'"
)

func (s *Server) serve(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	if r.URL.Path == "/chart" {
		h.Set("Content-Security-Policy", cspChart)
	} else {
		h.Set("Content-Security-Policy", cspBase)
	}
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Cross-Origin-Opener-Policy", "same-origin")
	h.Set("Cross-Origin-Resource-Policy", "same-origin")

	if r.URL.Path == "/healthz" {
		s.mux.ServeHTTP(w, r)
		return
	}
	if !s.authorized(w, r) {
		s.metrics.AuthFail()
		w.Header().Set("WWW-Authenticate", `Bearer realm="tmplforge"`)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if s.limiter != nil {
		if ok, wait := s.limiter.Allow(clientIP(r)); !ok {
			s.metrics.Limited()
			w.Header().Set("Retry-After", strconv.Itoa(int(wait/time.Second)+1))
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		if !sameOrigin(r) {
			s.metrics.CSRF()
			http.Error(w, "cross-site request rejected", http.StatusForbidden)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, s.cfg.MaxBody)
	}
	s.mux.ServeHTTP(w, r)
}

// tokenCookie carries the token for browsers after /?token=... once.
const tokenCookie = "tmplforge_token"

// authorized checks the bearer token, from the Authorization header, the
// cookie, or a ?token= query (which then sets the cookie). Comparison is
// constant-time.
func (s *Server) authorized(w http.ResponseWriter, r *http.Request) bool {
	if s.cfg.Token == "" {
		return true
	}
	want := []byte(s.cfg.Token)
	eq := func(got string) bool { return subtle.ConstantTimeCompare([]byte(got), want) == 1 }
	if a := r.Header.Get("Authorization"); strings.HasPrefix(a, "Bearer ") && eq(strings.TrimPrefix(a, "Bearer ")) {
		return true
	}
	if c, err := r.Cookie(tokenCookie); err == nil && eq(c.Value) {
		return true
	}
	if q := r.URL.Query().Get("token"); q != "" && r.Method == http.MethodGet && eq(q) {
		http.SetCookie(w, &http.Cookie{Name: tokenCookie, Value: q, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: r.TLS != nil})
		return true
	}
	return false
}

// sameOrigin is the CSRF rule for state-changing requests. A browser
// sends Sec-Fetch-Site on every request (all current engines), and Origin
// on every cross-origin POST; either one settles it. Failing both, an
// HX-Request header proves same-origin script: a cross-site page cannot set
// a custom header without a CORS preflight, which this server never
// approves. Requests with none of the three (curl without -H) are refused.
func sameOrigin(r *http.Request) bool {
	switch r.Header.Get("Sec-Fetch-Site") {
	case "same-origin", "none":
		return true
	case "cross-site", "same-site":
		return false
	}
	if o := r.Header.Get("Origin"); o != "" {
		return o == "http://"+r.Host || o == "https://"+r.Host
	}
	return r.Header.Get("HX-Request") == "true"
}

type statusWriter struct {
	http.ResponseWriter
	code        int
	wroteHeader bool
}

func (w *statusWriter) WriteHeader(code int) {
	if !w.wroteHeader {
		w.code, w.wroteHeader = code, true
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	w.wroteHeader = true
	return w.ResponseWriter.Write(b)
}

// Unwrap lets http.ResponseController reach the real writer.
func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// render runs one sandboxed render under the concurrency semaphore and
// records its latency.
func (s *Server) render(ctx context.Context, src string, fixture []byte, o forge.Options) ([]byte, error) {
	select {
	case s.sem <- struct{}{}:
	case <-ctx.Done():
		return nil, errBusy
	}
	defer func() { <-s.sem }()
	s.metrics.Inflight(1)
	defer s.metrics.Inflight(-1)
	start := time.Now()
	out, err := s.cfg.Sandbox.Render(ctx, src, fixture, o)
	s.metrics.Render(outcome(err), time.Since(start), start)
	return out, err
}

var errBusy = errors.New("server busy: too many renders in flight")

func outcome(err error) string {
	var re *forge.RenderError
	switch {
	case err == nil:
		return "ok"
	case errors.As(err, &re):
		return "template_error"
	case errors.Is(err, forge.ErrTimeout):
		return "timeout"
	case errors.Is(err, forge.ErrOutputLimit):
		return "output_limit"
	case errors.Is(err, forge.ErrTemplateLimit):
		return "template_limit"
	case errors.Is(err, forge.ErrCrashed):
		return "crash"
	}
	return "error"
}

// ListenAndServe runs the server until ctx is done, then shuts down
// gracefully, giving in-flight renders time to finish.
func ListenAndServe(ctx context.Context, cfg Config) error {
	if err := CheckConfig(&cfg); err != nil {
		return err
	}
	s, err := New(cfg)
	if err != nil {
		return err
	}
	ln, err := net.Listen("tcp", cfg.Addr)
	if err != nil {
		return err
	}
	return Serve(ctx, ln, s)
}

// Serve serves h on ln until ctx is done.
func Serve(ctx context.Context, ln net.Listener, h http.Handler) error {
	srv := &http.Server{
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()
	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(sctx); err != nil {
		return err
	}
	if err := <-errc; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
