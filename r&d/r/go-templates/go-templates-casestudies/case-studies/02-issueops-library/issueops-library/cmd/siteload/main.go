// Command siteload is a small constant-rate HTTP load generator for the
// static library site, used when neither vegeta nor hey is installed.
//
// Usage:
//
//	siteload -base https://coolgitorg.github.io/issueops-library/ -rate 50 -d 2m \
//	    -p / -p items/payments-api/caddyfile/42/ -p ledger/open.html
//
// Requests rotate over the paths at a fixed rate (open model: a slow
// response does not slow the sender, so latency is not hidden). It prints
// a text report and exits 1 if the error rate or p99 exceeds the limits.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

var version = "dev"

type paths []string

func (p *paths) String() string     { return strings.Join(*p, ",") }
func (p *paths) Set(s string) error { *p = append(*p, s); return nil }

// Config is one run.
type Config struct {
	Base     string
	Paths    []string
	Rate     int
	Duration time.Duration
	Timeout  time.Duration
	MaxP99   time.Duration
	MaxErr   float64
}

// Sample is one request outcome.
type Sample struct {
	Path    string
	Status  int
	Latency time.Duration
	Bytes   int64
	Err     error
}

// Report summarises a run.
type Report struct {
	Requests      int
	Errors        int
	Status        map[int]int
	P50, P90, P99 time.Duration
	Max           time.Duration
	Bytes         int64
	Elapsed       time.Duration
	PerPath       map[string]time.Duration // p99 per path
}

func main() {
	var c Config
	var ps paths
	flag.StringVar(&c.Base, "base", "https://coolgitorg.github.io/issueops-library/", "site base `url`")
	flag.Var(&ps, "p", "`path` under base (repeatable; default: catalog, ledger fragment, library.json)")
	flag.IntVar(&c.Rate, "rate", 50, "requests per `second`")
	flag.DurationVar(&c.Duration, "d", 2*time.Minute, "run `duration`")
	flag.DurationVar(&c.Timeout, "timeout", 10*time.Second, "per-request `timeout`")
	flag.DurationVar(&c.MaxP99, "max-p99", 1500*time.Millisecond, "fail if p99 exceeds `d`")
	flag.Float64Var(&c.MaxErr, "max-err", 0.01, "fail if error `fraction` exceeds this")
	serve := flag.String("serve", "", "serve `dir` over HTTP instead of generating load")
	addr := flag.String("addr", "127.0.0.1:8765", "listen `address` for -serve")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}
	if *serve != "" {
		fmt.Fprintf(os.Stderr, "siteload: serving %s on http://%s/\n", *serve, *addr)
		srv := &http.Server{Addr: *addr, Handler: http.FileServer(http.Dir(*serve)), ReadHeaderTimeout: 5 * time.Second}
		if err := srv.ListenAndServe(); err != nil {
			fmt.Fprintln(os.Stderr, "siteload:", err)
			os.Exit(1)
		}
		return
	}
	c.Paths = ps
	if len(c.Paths) == 0 {
		c.Paths = []string{"", "ledger/open.html", "library.json"}
	}
	if c.Rate <= 0 || c.Duration <= 0 {
		fmt.Fprintln(os.Stderr, "siteload: -rate and -d must be positive")
		os.Exit(2)
	}
	rep := Run(context.Background(), c, &http.Client{Timeout: c.Timeout})
	rep.Print(os.Stdout, c)
	if fail := rep.Check(c); fail != "" {
		fmt.Fprintln(os.Stderr, "siteload: FAIL:", fail)
		os.Exit(1)
	}
}

// Run issues Rate requests per second for Duration.
func Run(ctx context.Context, c Config, client *http.Client) Report {
	base := strings.TrimRight(c.Base, "/") + "/"
	ctx, cancel := context.WithTimeout(ctx, c.Duration)
	defer cancel()
	tick := time.NewTicker(time.Second / time.Duration(c.Rate))
	defer tick.Stop()
	var (
		mu      sync.Mutex
		samples []Sample
		wg      sync.WaitGroup
	)
	start := time.Now()
	for i := 0; ; i++ {
		select {
		case <-ctx.Done():
			wg.Wait()
			return summarize(samples, time.Since(start))
		case <-tick.C:
		}
		p := c.Paths[i%len(c.Paths)]
		wg.Add(1)
		go func() {
			defer wg.Done()
			s := fetch(client, base+strings.TrimLeft(p, "/"))
			s.Path = p
			mu.Lock()
			samples = append(samples, s)
			mu.Unlock()
		}()
	}
}

func fetch(client *http.Client, url string) Sample {
	t0 := time.Now()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return Sample{Err: err}
	}
	req.Header.Set("User-Agent", "siteload/"+version)
	resp, err := client.Do(req)
	if err != nil {
		return Sample{Err: err, Latency: time.Since(t0)}
	}
	n, err := io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return Sample{Status: resp.StatusCode, Latency: time.Since(t0), Bytes: n, Err: err}
}

func summarize(samples []Sample, elapsed time.Duration) Report {
	r := Report{Status: map[int]int{}, PerPath: map[string]time.Duration{}, Elapsed: elapsed}
	var lat []time.Duration
	per := map[string][]time.Duration{}
	for _, s := range samples {
		r.Requests++
		r.Bytes += s.Bytes
		if s.Err != nil || s.Status >= 400 {
			r.Errors++
		}
		if s.Err == nil {
			r.Status[s.Status]++
		}
		lat = append(lat, s.Latency)
		per[s.Path] = append(per[s.Path], s.Latency)
	}
	r.P50, r.P90, r.P99, r.Max = pct(lat, 50), pct(lat, 90), pct(lat, 99), pct(lat, 100)
	for p, l := range per {
		r.PerPath[p] = pct(l, 99)
	}
	return r
}

// pct returns the q-th percentile (nearest rank) of d.
func pct(d []time.Duration, q int) time.Duration {
	if len(d) == 0 {
		return 0
	}
	s := append([]time.Duration(nil), d...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	i := (q*len(s)+99)/100 - 1
	if i < 0 {
		i = 0
	}
	if i >= len(s) {
		i = len(s) - 1
	}
	return s[i]
}

// Print writes the human report.
func (r Report) Print(w io.Writer, c Config) {
	rate := 0.0
	if r.Elapsed > 0 {
		rate = float64(r.Requests) / r.Elapsed.Seconds()
	}
	fmt.Fprintf(w, "target    %s (%d paths) at %d req/s for %s\n", c.Base, len(c.Paths), c.Rate, c.Duration)
	fmt.Fprintf(w, "requests  %d (%.1f/s achieved), %d errors, %d bytes\n", r.Requests, rate, r.Errors, r.Bytes)
	fmt.Fprintf(w, "latency   p50 %s  p90 %s  p99 %s  max %s\n", round(r.P50), round(r.P90), round(r.P99), round(r.Max))
	codes := make([]int, 0, len(r.Status))
	for k := range r.Status {
		codes = append(codes, k)
	}
	sort.Ints(codes)
	for _, k := range codes {
		fmt.Fprintf(w, "status    %d x %d\n", k, r.Status[k])
	}
	ps := make([]string, 0, len(r.PerPath))
	for p := range r.PerPath {
		ps = append(ps, p)
	}
	sort.Strings(ps)
	for _, p := range ps {
		fmt.Fprintf(w, "path p99  %-40s %s\n", "/"+p, round(r.PerPath[p]))
	}
}

// Check returns a failure reason, or "".
func (r Report) Check(c Config) string {
	if r.Requests == 0 {
		return "no requests completed"
	}
	if f := float64(r.Errors) / float64(r.Requests); f > c.MaxErr {
		return fmt.Sprintf("error rate %.2f%% > %.2f%%", 100*f, 100*c.MaxErr)
	}
	if c.MaxP99 > 0 && r.P99 > c.MaxP99 {
		return fmt.Sprintf("p99 %s > %s", round(r.P99), c.MaxP99)
	}
	return ""
}

func round(d time.Duration) time.Duration { return d.Round(100 * time.Microsecond) }
