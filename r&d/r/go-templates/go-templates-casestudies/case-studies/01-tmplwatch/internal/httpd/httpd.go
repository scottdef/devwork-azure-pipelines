// Package httpd serves tmplwatch's loopback control surface: health,
// Prometheus metrics, current renders, status, expvar, and optionally
// pprof.
package httpd

import (
	"encoding/json"
	"expvar"
	"fmt"
	"net/http"
	"net/http/pprof"
	"strings"
	"sync"
	"time"

	"github.com/CoolGitOrg/tmplwatch/internal/metrics"
	"github.com/CoolGitOrg/tmplwatch/internal/runner"
)

// State is what the handlers read. *runner.Runner implements it.
type State interface {
	Status() runner.Status
	Current(name string) ([]byte, bool)
	Names() []string
}

// Options configure the handler.
type Options struct {
	State   State
	Metrics *metrics.Metrics
	Pprof   bool
}

var publishOnce sync.Once

// Handler returns the mux.
func Handler(o Options) http.Handler {
	publishOnce.Do(func() {
		// expvar's /debug/vars gets a live status view; registered once
		// per process because expvar names are global.
		expvar.Publish("tmplwatch", expvar.Func(func() any { return current.Load() }))
	})
	current.Store(o.State)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		var bad []string
		for _, t := range o.State.Status().Templates {
			if !t.Complete {
				bad = append(bad, t.Name)
			}
		}
		if len(bad) > 0 {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprintf(w, "incomplete: %s\n", strings.Join(bad, " "))
			return
		}
		fmt.Fprintln(w, "ok")
	})
	mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		if err := o.Metrics.Expose(w); err != nil {
			// Headers are gone; the scrape will fail to parse, which is
			// the right signal.
			fmt.Fprintf(w, "\n# error: %v\n", err)
		}
	})
	mux.HandleFunc("GET /render", func(w http.ResponseWriter, r *http.Request) {
		name := r.URL.Query().Get("template")
		if name == "" {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			for _, n := range o.State.Names() {
				fmt.Fprintln(w, n)
			}
			return
		}
		b, ok := o.State.Current(name)
		if !ok {
			http.Error(w, "no complete render for template "+name, http.StatusNotFound)
			return
		}
		// Rendered output is config text, never HTML for a browser.
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Write(b)
	})
	mux.HandleFunc("GET /status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		enc.Encode(o.State.Status())
	})
	mux.Handle("GET /debug/vars", expvar.Handler())
	if o.Pprof {
		mux.HandleFunc("/debug/pprof/", pprof.Index)
		mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
		mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	}
	return mux
}

// Server returns an http.Server with conservative timeouts.
func Server(addr string, h http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      60 * time.Second, // pprof profile defaults to 30s
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
}

// IsLoopback reports whether addr's host is a loopback name or address;
// pprof is refused elsewhere.
func IsLoopback(addr string) bool {
	host := addr
	if i := strings.LastIndex(addr, ":"); i >= 0 {
		host = addr[:i]
	}
	host = strings.Trim(host, "[]")
	switch host {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	return strings.HasPrefix(host, "127.")
}
