// Checkout is the payments team's sample app on CoolDeploy: a small shop
// with a server-rendered HTMX front end and a JSON API.
//
// It follows the platform's contract and nothing more:
//
//	PORT             the port to listen on
//	GET /healthz     answers 200 when the app can serve
//	GET /metrics     Prometheus text format
//	SIGTERM          finish in-flight requests, then exit
//
// The rest of its environment is described in README.md.
package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/CoolEngOrg/payments-checkout/internal/metrics"
)

//go:embed data/*.json web/templates/*.html web/static/*
var files embed.FS

// config is what the platform tells the app about where it runs.
type config struct {
	Port       string
	Env        string // COOL_ENV: dev, tst or prd
	Release    string // COOL_RELEASE: the release this process belongs to
	Currency   string
	CronSecret string
	Flags      map[string]any // from COOL_FLAGS_FILE
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// loadConfig reads the environment. A missing flags file means no flags.
func loadConfig() config {
	c := config{
		Port:       getenv("PORT", "8080"),
		Env:        getenv("COOL_ENV", "local"),
		Release:    getenv("COOL_RELEASE", "dev"),
		Currency:   getenv("CURRENCY", "USD"),
		CronSecret: os.Getenv("CRON_SECRET"),
		Flags:      map[string]any{},
	}
	if b, err := os.ReadFile(getenv("COOL_FLAGS_FILE", "/etc/cool/flags.json")); err == nil {
		json.Unmarshal(b, &c.Flags)
	}
	return c
}

// app is the server: its settings, its data and its templates.
type app struct {
	cfg     config
	shop    *shop
	tmpl    *template.Template
	metrics metrics.Registry
	log     *slog.Logger
}

func newApp(cfg config, log *slog.Logger) (*app, error) {
	s, err := newShop(files)
	if err != nil {
		return nil, err
	}
	a := &app{cfg: cfg, shop: s, log: log}
	a.tmpl, err = template.New("").Funcs(template.FuncMap{"money": a.money}).ParseFS(files, "web/templates/*.html")
	return a, err
}

// status remembers the response code for the access log and the metrics.
type status struct {
	http.ResponseWriter
	code int
}

func (s *status) WriteHeader(code int) {
	s.code = code
	s.ResponseWriter.WriteHeader(code)
}

// observe wraps the routes with security headers, metrics, an access log and
// a guard that turns a panic into a 500.
func (a *app) observe(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &status{ResponseWriter: w, code: http.StatusOK}
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("Content-Security-Policy", "default-src 'self'; frame-ancestors 'none'")
		h.Set("X-Cool-Release", a.cfg.Release)
		defer func() {
			if p := recover(); p != nil {
				a.log.Error("panic", "path", r.URL.Path, "panic", p)
				http.Error(sw, "internal error", http.StatusInternalServerError)
			}
			// The mux records the pattern it matched on the request. Using
			// it as the label keeps the number of series small.
			route := r.Pattern
			if route == "" {
				route = "unmatched"
			}
			took := time.Since(start)
			a.metrics.Inc("http_requests_total", "route", route, "code", strconv.Itoa(sw.code))
			a.metrics.Observe("http_request_seconds", took.Seconds(), "route", route)
			if r.URL.Path != "/healthz" && r.URL.Path != "/metrics" {
				a.log.Info("request", "method", r.Method, "path", r.URL.Path, "code", sw.code,
					"ms", took.Milliseconds(), "release", a.cfg.Release)
			}
		}()
		next.ServeHTTP(sw, r)
	})
}

func (a *app) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok\n")) })
	mux.Handle("GET /metrics", &a.metrics)
	mux.HandleFunc("GET /version", a.version)
	mux.Handle("GET /static/", http.FileServerFS(sub(files, "web")))

	// The HTMX front end: a page, and the fragments it swaps in.
	mux.HandleFunc("GET /{$}", a.page)
	mux.HandleFunc("GET /products", a.productsFragment)
	mux.HandleFunc("GET /cart", a.cartFragment)
	mux.HandleFunc("POST /cart", a.cartAdd)
	mux.HandleFunc("DELETE /cart", a.cartClear)
	mux.HandleFunc("POST /checkout", a.checkout)
	mux.HandleFunc("GET /orders", a.ordersFragment)
	mux.HandleFunc("POST /orders/{id}/pay", a.payFragment)

	// The JSON API, described in openapi.yaml and published through APIM.
	mux.HandleFunc("GET /api/products", a.apiProducts)
	mux.HandleFunc("GET /api/products/{sku}", a.apiProduct)
	mux.HandleFunc("GET /api/orders", a.apiOrders)
	mux.HandleFunc("POST /api/orders", a.apiCreateOrder)
	mux.HandleFunc("GET /api/orders/{id}", a.apiOrder)
	mux.HandleFunc("POST /api/orders/{id}/pay", a.apiPay)

	// Called by the platform's cron pod, with CRON_SECRET as a bearer token.
	mux.HandleFunc("POST /internal/cron/expire-orders", a.cronExpire)
	return a.observe(mux)
}

func (a *app) version(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"app": "checkout", "release": a.cfg.Release, "environment": a.cfg.Env, "flags": a.cfg.Flags,
	})
}

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	a, err := newApp(loadConfig(), log)
	if err != nil {
		log.Error("start", "err", err)
		os.Exit(1)
	}
	srv := &http.Server{
		Addr:              ":" + a.cfg.Port,
		Handler:           a.routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       90 * time.Second,
	}
	go func() {
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			log.Error("serve", "err", err)
			os.Exit(1)
		}
	}()
	log.Info("listening", "port", a.cfg.Port, "environment", a.cfg.Env, "release", a.cfg.Release,
		"products", len(a.shop.Products("")))

	// Kubernetes sends SIGTERM and then waits terminationGracePeriodSeconds.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()
	<-ctx.Done()
	drain, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	if err := srv.Shutdown(drain); err != nil {
		log.Error("shutdown", "err", err)
	}
	log.Info("stopped")
}
