// Package metrics is a small Prometheus text-format registry built on the
// standard library. It exposes the counters the guide asks for
// (adoclone_items_total, adoclone_errors_total, adoclone_throttle_seconds_total)
// plus request counts and a per-component status gauge for the Grafana dashboard.
package metrics

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Component status values for adoclone_phase_status.
const (
	Pending = 0
	Running = 1
	Done    = 2
	Failed  = 3
)

type family struct {
	help   string
	typ    string
	values map[string]float64
}

// Registry holds metric families. A nil *Registry is valid and records nothing.
type Registry struct {
	mu  sync.Mutex
	fam map[string]*family
}

// New returns a registry with every adoclone metric declared.
func New() *Registry {
	r := &Registry{fam: map[string]*family{}}
	r.declare("adoclone_items_total", "counter", "Objects read from the source (side=source) or written to the target (side=target), by component.")
	r.declare("adoclone_errors_total", "counter", "Requests and items that failed after retries.")
	r.declare("adoclone_throttle_seconds_total", "counter", "Seconds spent waiting on Retry-After, rate-limit headers and backoff.")
	r.declare("adoclone_requests_total", "counter", "HTTP requests sent to Azure DevOps, by method and status code.")
	r.declare("adoclone_phase_status", "gauge", "Component status: 0 pending, 1 running, 2 done, 3 failed.")
	r.declare("adoclone_info", "gauge", "Run information; always 1.")
	return r
}

func (r *Registry) declare(name, typ, help string) {
	r.fam[name] = &family{help: help, typ: typ, values: map[string]float64{}}
}

func labelKey(kv []string) string {
	if len(kv) < 2 {
		return ""
	}
	parts := make([]string, 0, len(kv)/2)
	for i := 0; i+1 < len(kv); i += 2 {
		parts = append(parts, fmt.Sprintf(`%s="%s"`, kv[i], escape(kv[i+1])))
	}
	sort.Strings(parts)
	return "{" + strings.Join(parts, ",") + "}"
}

var escaper = strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`)

func escape(s string) string { return escaper.Replace(s) }

// Add adds v to a counter. Labels are key, value pairs.
func (r *Registry) Add(name string, v float64, labels ...string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if f := r.fam[name]; f != nil {
		f.values[labelKey(labels)] += v
	}
}

// Inc adds 1 to a counter.
func (r *Registry) Inc(name string, labels ...string) { r.Add(name, 1, labels...) }

// Set sets a gauge.
func (r *Registry) Set(name string, v float64, labels ...string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if f := r.fam[name]; f != nil {
		f.values[labelKey(labels)] = v
	}
}

// Value returns the current value of a series (0 if absent).
func (r *Registry) Value(name string, labels ...string) float64 {
	if r == nil {
		return 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if f := r.fam[name]; f != nil {
		return f.values[labelKey(labels)]
	}
	return 0
}

// WriteText writes every family in Prometheus text exposition format 0.0.4.
func (r *Registry) WriteText(w io.Writer) error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	names := make([]string, 0, len(r.fam))
	for n := range r.fam {
		names = append(names, n)
	}
	sort.Strings(names)
	var b strings.Builder
	for _, n := range names {
		f := r.fam[n]
		fmt.Fprintf(&b, "# HELP %s %s\n# TYPE %s %s\n", n, f.help, n, f.typ)
		keys := make([]string, 0, len(f.values))
		for k := range f.values {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(&b, "%s%s %s\n", n, k, strconv.FormatFloat(f.values[k], 'g', -1, 64))
		}
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// Handler serves /metrics.
func (r *Registry) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		_ = r.WriteText(w)
	})
}

// Serve exposes /metrics and /healthz on addr in the background and returns a
// shutdown function.
func (r *Registry) Serve(addr string, log *slog.Logger) (func(context.Context) error, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	mux := http.NewServeMux()
	mux.Handle("/metrics", r.Handler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "ok\n") })
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("metrics server", "err", err)
		}
	}()
	log.Info("metrics listening", "addr", ln.Addr().String())
	return srv.Shutdown, nil
}
