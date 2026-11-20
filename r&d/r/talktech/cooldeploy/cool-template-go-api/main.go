// A JSON API on CoolDeploy: the starting point for the go-server preset.
//
// It does what the platform asks of every app and nothing else:
//
//	PORT           listen on it
//	GET /healthz   answer 200 when ready to serve
//	GET /metrics   Prometheus text format
//	SIGTERM        finish in-flight requests, then exit
//
// Replace the items below with your own routes. Keep the four above.
package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/CoolEngOrg/cool-template-go-api/internal/metrics"
)

//go:embed data/items.json
var files embed.FS

// Item is the sample resource. Replace it.
type Item struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type server struct {
	items   []Item
	metrics metrics.Registry
	log     *slog.Logger
}

func newServer(log *slog.Logger) (*server, error) {
	s := &server{log: log}
	b, err := files.ReadFile("data/items.json")
	if err != nil {
		return nil, err
	}
	return s, json.Unmarshal(b, &s.items)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func (s *server) listItems(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"items": s.items})
}

func (s *server) getItem(w http.ResponseWriter, r *http.Request) {
	for _, it := range s.items {
		if it.ID == r.PathValue("id") {
			writeJSON(w, http.StatusOK, it)
			return
		}
	}
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "no such item"})
}

// status remembers the response code for the metrics.
type status struct {
	http.ResponseWriter
	code int
}

func (s *status) WriteHeader(code int) {
	s.code = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok\n")) })
	mux.Handle("GET /metrics", &s.metrics)
	mux.HandleFunc("GET /api/items", s.listItems)
	mux.HandleFunc("GET /api/items/{id}", s.getItem)

	// Count and time every request, labelled by the route it matched.
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &status{ResponseWriter: w, code: http.StatusOK}
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
	s, err := newServer(log)
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
