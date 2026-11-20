// Appsim is a simulated app. It answers requests with a configurable delay,
// error rate and CPU burn, so the sandbox cluster can rehearse a bad canary,
// a noisy neighbour or a mesh upgrade without touching a real app.
//
// Knobs come from the environment and can be changed while it runs:
//
//	SIM_LATENCY_MS   mean response delay in milliseconds, default 20
//	SIM_ERROR_RATE   share of requests answered with 500, 0 to 1, default 0
//	SIM_CPU_MS       milliseconds of CPU to burn per request, default 0
//
//	curl -X POST 'http://appsim/sim?error_rate=0.3'    # break it
//	curl -X POST 'http://appsim/sim?error_rate=0'      # heal it
//
// For rehearsals with no Prometheus at hand, appsim also answers the two
// instant queries that `coolctl gate` asks, from its own last 200 requests:
//
//	coolctl gate -endpoint http://localhost:8080 -namespace sim -workload appsim
//
// That endpoint is a stand-in, not a query engine: it looks at which of the
// two questions was asked and ignores the rest of the query.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/CoolEngOrg/coolctl/internal/metrics"
)

// knobs is the behaviour of the simulator.
type knobs struct {
	LatencyMS float64 `json:"latency_ms"`
	ErrorRate float64 `json:"error_rate"`
	CPUMS     float64 `json:"cpu_ms"`
}

type sim struct {
	mu sync.RWMutex
	k  knobs
	m  metrics.Registry

	// recent is a ring of the last requests, for the stand-in query endpoint.
	recent [200]sample
	seen   int
}

// sample is one answered request.
type sample struct {
	failed bool
	ms     float64
}

// record adds a request to the ring.
func (s *sim) record(failed bool, d time.Duration) {
	s.mu.Lock()
	s.recent[s.seen%len(s.recent)] = sample{failed, float64(d) / float64(time.Millisecond)}
	s.seen++
	s.mu.Unlock()
}

// query answers `coolctl gate` in the shape of the Prometheus HTTP API: the
// share of failed requests, or the 95th percentile duration in milliseconds.
func (s *sim) query(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	n := min(s.seen, len(s.recent))
	window := slices.Clone(s.recent[:n])
	s.mu.RUnlock()

	result := []any{}
	if n > 0 {
		var value float64
		if strings.Contains(r.FormValue("query"), "histogram_quantile") {
			ms := make([]float64, n)
			for i, x := range window {
				ms[i] = x.ms
			}
			slices.Sort(ms)
			value = ms[min(n-1, n*95/100)]
		} else {
			failed := 0
			for _, x := range window {
				if x.failed {
					failed++
				}
			}
			value = float64(failed) / float64(n)
		}
		result = append(result, map[string]any{
			"metric": map[string]string{},
			"value":  []any{float64(time.Now().Unix()), strconv.FormatFloat(value, 'f', -1, 64)},
		})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"status": "success",
		"data":   map[string]any{"resultType": "vector", "result": result},
	})
}

func envFloat(key string, def float64) float64 {
	if f, err := strconv.ParseFloat(os.Getenv(key), 64); err == nil {
		return f
	}
	return def
}

func (s *sim) work(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	s.mu.RLock()
	k := s.k
	s.mu.RUnlock()

	// Burn CPU first, then sleep the rest of an exponentially distributed delay.
	for burn := time.Now(); time.Since(burn) < time.Duration(k.CPUMS*float64(time.Millisecond)); {
	}
	time.Sleep(time.Duration(rand.ExpFloat64() * k.LatencyMS * float64(time.Millisecond)))

	code := http.StatusOK
	if rand.Float64() < k.ErrorRate {
		code = http.StatusInternalServerError
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]any{
		"app": os.Getenv("COOL_APP"), "release": os.Getenv("COOL_RELEASE"), "status": code,
	})
	s.m.Inc("appsim_requests_total", "code", strconv.Itoa(code))
	s.m.Observe("appsim_request_seconds", time.Since(start).Seconds())
	s.record(code >= 500, time.Since(start))
}

// tune reads or changes the knobs.
func (s *sim) tune(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		s.mu.Lock()
		for name, dst := range map[string]*float64{"latency_ms": &s.k.LatencyMS, "error_rate": &s.k.ErrorRate, "cpu_ms": &s.k.CPUMS} {
			if v := r.URL.Query().Get(name); v != "" {
				f, err := strconv.ParseFloat(v, 64)
				if err != nil || f < 0 {
					s.mu.Unlock()
					http.Error(w, fmt.Sprintf("%s: want a non-negative number", name), http.StatusBadRequest)
					return
				}
				*dst = f
			}
		}
		s.mu.Unlock()
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(s.k)
}

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	s := &sim{k: knobs{
		LatencyMS: envFloat("SIM_LATENCY_MS", 20),
		ErrorRate: envFloat("SIM_ERROR_RATE", 0),
		CPUMS:     envFloat("SIM_CPU_MS", 0),
	}}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok\n")) })
	mux.Handle("GET /metrics", &s.m)
	mux.HandleFunc("/sim", s.tune)
	mux.HandleFunc("/api/v1/query", s.query)
	mux.HandleFunc("/", s.work)

	srv := &http.Server{Addr: ":" + port, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			log.Error("serve", "err", err)
			os.Exit(1)
		}
	}()
	log.Info("listening", "port", port, "release", os.Getenv("COOL_RELEASE"), "knobs", s.k)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()
	<-ctx.Done()
	drain, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	srv.Shutdown(drain)
}
