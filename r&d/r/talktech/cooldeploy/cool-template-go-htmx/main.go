// A server-rendered page with HTMX on CoolDeploy: the starting point for
// the go-htmx preset.
//
// The server renders HTML. The browser asks for fragments and swaps them in;
// there is no build step and no client-side framework. The page here is a
// list you can add to. Replace it, and keep the platform's four:
//
//	PORT           listen on it
//	GET /healthz   answer 200 when ready to serve
//	GET /metrics   Prometheus text format
//	SIGTERM        finish in-flight requests, then exit
package main

import (
	"context"
	"embed"
	"errors"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/CoolEngOrg/cool-template-go-htmx/internal/metrics"
)

//go:embed web
var files embed.FS

type server struct {
	tmpl    *template.Template
	metrics metrics.Registry

	// notes live in memory: each replica has its own, and a restart forgets
	// them. A real app keeps state in an add-on, not in the process.
	mu    sync.Mutex
	notes []string
}

type view struct {
	Notes   []string
	Release string
	Error   string
}

func newServer() (*server, error) {
	t, err := template.ParseFS(files, "web/templates/*.html")
	return &server{tmpl: t, notes: []string{"Deployed with CoolDeploy", "Edit web/templates/page.html"}}, err
}

func (s *server) view() view {
	s.mu.Lock()
	defer s.mu.Unlock()
	return view{Notes: append([]string(nil), s.notes...), Release: os.Getenv("COOL_RELEASE")}
}

func (s *server) page(w http.ResponseWriter, r *http.Request) {
	s.tmpl.ExecuteTemplate(w, "page", s.view())
}

// addNote answers an HTMX form post with the list fragment, not a whole page.
func (s *server) addNote(w http.ResponseWriter, r *http.Request) {
	text := strings.TrimSpace(r.FormValue("text"))
	v := view{}
	switch {
	case text == "":
		v.Error = "Write something first."
	case len(text) > 200:
		v.Error = "Keep it under 200 characters."
	default:
		s.mu.Lock()
		s.notes = append(s.notes, text)
		if len(s.notes) > 50 {
			s.notes = s.notes[len(s.notes)-50:]
		}
		s.mu.Unlock()
	}
	msg := v.Error
	v = s.view()
	v.Error = msg
	s.tmpl.ExecuteTemplate(w, "notes", v)
}

type status struct {
	http.ResponseWriter
	code int
}

func (s *status) WriteHeader(code int) {
	s.code = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *server) routes() http.Handler {
	static, _ := fs.Sub(files, "web")
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok\n")) })
	mux.Handle("GET /metrics", &s.metrics)
	mux.Handle("GET /static/", http.FileServerFS(static))
	mux.HandleFunc("GET /{$}", s.page)
	mux.HandleFunc("POST /notes", s.addNote)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &status{ResponseWriter: w, code: http.StatusOK}
		w.Header().Set("Content-Security-Policy", "default-src 'self'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		mux.ServeHTTP(sw, r)
		route := r.Pattern
		if route == "" {
			route = "unmatched"
		}
		s.metrics.Inc("http_requests_total", "route", route, "code", strconv.Itoa(sw.code))
		s.metrics.Observe("http_request_seconds", time.Since(start).Seconds(), "route", route)
	})
}

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	s, err := newServer()
	if err != nil {
		log.Error("start", "err", err)
		os.Exit(1)
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	srv := &http.Server{Addr: ":" + port, Handler: s.routes(), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			log.Error("serve", "err", err)
			os.Exit(1)
		}
	}()
	log.Info("listening", "port", port, "release", os.Getenv("COOL_RELEASE"), "environment", os.Getenv("COOL_ENV"))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()
	<-ctx.Done()
	drain, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	srv.Shutdown(drain)
}
