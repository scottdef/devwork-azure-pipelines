// Cool-static is the file server behind the static build preset. It serves a
// directory with the headers a static site wants and nothing else.
//
//	COOL_STATIC_DIR   directory to serve, default /site
//	PORT              listen port, default 8080
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/CoolEngOrg/coolctl/internal/metrics"
)

// headers sets caching and a few safe defaults. HTML is revalidated on every
// load; everything else may be cached for a day.
func headers(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Dotfiles and the manifest are not part of the site.
		for _, seg := range strings.Split(r.URL.Path, "/") {
			if strings.HasPrefix(seg, ".") || seg == "cool.json" || seg == "Dockerfile" {
				http.NotFound(w, r)
				return
			}
		}
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		if ext := path.Ext(r.URL.Path); ext == "" || ext == ".html" {
			h.Set("Cache-Control", "no-cache")
		} else {
			h.Set("Cache-Control", "public, max-age=86400")
		}
		next.ServeHTTP(w, r)
	})
}

// code remembers the status of a response.
type code struct {
	http.ResponseWriter
	status int
}

func (c *code) WriteHeader(status int) {
	c.status = status
	c.ResponseWriter.WriteHeader(status)
}

// counted counts requests by status and times them. There is no path label:
// a site has as many paths as files, and a metric should not.
func counted(m *metrics.Registry, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		c := &code{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(c, r)
		m.Inc("http_requests_total", "code", strconv.Itoa(c.status))
		m.Observe("http_request_seconds", time.Since(start).Seconds())
	})
}

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	dir, port := os.Getenv("COOL_STATIC_DIR"), os.Getenv("PORT")
	if dir == "" {
		dir = "/site"
	}
	if port == "" {
		port = "8080"
	}
	var m metrics.Registry
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok\n")) })
	mux.Handle("GET /metrics", &m)
	mux.Handle("/", counted(&m, headers(http.FileServer(http.Dir(dir)))))

	srv := &http.Server{Addr: ":" + port, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			log.Error("serve", "err", err)
			os.Exit(1)
		}
	}()
	log.Info("listening", "port", port, "dir", dir, "release", os.Getenv("COOL_RELEASE"))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()
	<-ctx.Done()
	drain, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	srv.Shutdown(drain)
}
