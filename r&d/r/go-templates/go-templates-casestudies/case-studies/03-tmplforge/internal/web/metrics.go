package web

import (
	"io"
	"sort"
	"strconv"
	"sync"
	"text/template"
	"time"
)

// Buckets are the render latency histogram bounds in seconds. 0.15 is
// there because the SLO is p99 < 150ms (docs/LOADTEST.md).
var Buckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.15, 0.25, 0.5, 1, 2.5}

// Sample is one render latency observation for the chart's ring buffer.
type Sample struct {
	At  time.Time
	Dur time.Duration
}

// Metrics holds the server's counters, a latency histogram and a ring
// buffer of recent samples. One mutex: contention is irrelevant next to a
// process spawn per render.
type Metrics struct {
	mu       sync.Mutex
	start    time.Time
	requests map[[2]string]uint64 // route, code
	renders  map[string]uint64    // outcome
	limited  uint64
	csrf     uint64
	authFail uint64
	inflight int
	counts   []uint64 // per bucket, non-cumulative; last is +Inf
	sum      float64
	n        uint64
	ring     []Sample
	next     int
	full     bool
}

// RingSize is how many recent render samples the chart can draw from.
const RingSize = 4096

// NewMetrics returns empty metrics.
func NewMetrics() *Metrics {
	return &Metrics{
		start:    time.Now(),
		requests: map[[2]string]uint64{},
		renders:  map[string]uint64{},
		counts:   make([]uint64, len(Buckets)+1),
		ring:     make([]Sample, RingSize),
	}
}

// Request counts one HTTP response.
func (m *Metrics) Request(route string, code int) {
	m.mu.Lock()
	m.requests[[2]string{route, strconv.Itoa(code)}]++
	m.mu.Unlock()
}

// Render records a sandboxed render: its outcome and latency.
func (m *Metrics) Render(outcome string, d time.Duration, at time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.renders[outcome]++
	s := d.Seconds()
	i := sort.SearchFloat64s(Buckets, s)
	m.counts[i]++
	m.sum += s
	m.n++
	m.ring[m.next] = Sample{At: at, Dur: d}
	m.next = (m.next + 1) % len(m.ring)
	if m.next == 0 {
		m.full = true
	}
}

// Inflight adjusts the concurrent sandbox gauge.
func (m *Metrics) Inflight(delta int) {
	m.mu.Lock()
	m.inflight += delta
	m.mu.Unlock()
}

// Limited counts a request rejected by the rate limiter.
func (m *Metrics) Limited() { m.mu.Lock(); m.limited++; m.mu.Unlock() }

// CSRF counts a POST rejected by the cross-site check.
func (m *Metrics) CSRF() { m.mu.Lock(); m.csrf++; m.mu.Unlock() }

// AuthFail counts a request rejected for a missing or wrong token.
func (m *Metrics) AuthFail() { m.mu.Lock(); m.authFail++; m.mu.Unlock() }

// Samples returns the ring buffer contents, oldest first.
func (m *Metrics) Samples() []Sample {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.full {
		return append([]Sample(nil), m.ring[:m.next]...)
	}
	return append(append([]Sample(nil), m.ring[m.next:]...), m.ring[:m.next]...)
}

type kv struct {
	Labels string
	Value  uint64
}

type bucketLine struct {
	Le    string
	Count uint64
}

// The exposition format is text, so it is a text/template.
var promTmpl = template.Must(template.New("metrics").Parse(`# HELP tmplforge_build_info Build information.
# TYPE tmplforge_build_info gauge
tmplforge_build_info{version="{{.Version}}",goversion="{{.GoVersion}}"} 1
# HELP tmplforge_start_time_seconds Unix time the server started.
# TYPE tmplforge_start_time_seconds gauge
tmplforge_start_time_seconds {{.Start}}
# HELP tmplforge_http_requests_total HTTP responses by route and status code.
# TYPE tmplforge_http_requests_total counter
{{- range .Requests}}
tmplforge_http_requests_total{{"{"}}{{.Labels}}{{"}"}} {{.Value}}
{{- end}}
# HELP tmplforge_renders_total Sandboxed renders by outcome.
# TYPE tmplforge_renders_total counter
{{- range .Renders}}
tmplforge_renders_total{{"{"}}{{.Labels}}{{"}"}} {{.Value}}
{{- end}}
# HELP tmplforge_render_seconds Sandboxed render latency, including process spawn.
# TYPE tmplforge_render_seconds histogram
{{- range .Buckets}}
tmplforge_render_seconds_bucket{le="{{.Le}}"} {{.Count}}
{{- end}}
tmplforge_render_seconds_sum {{.Sum}}
tmplforge_render_seconds_count {{.N}}
# HELP tmplforge_sandbox_inflight Sandboxed renders currently running.
# TYPE tmplforge_sandbox_inflight gauge
tmplforge_sandbox_inflight {{.Inflight}}
# HELP tmplforge_rate_limited_total Requests rejected by the per-IP token bucket.
# TYPE tmplforge_rate_limited_total counter
tmplforge_rate_limited_total {{.Limited}}
# HELP tmplforge_csrf_rejected_total POSTs rejected by the cross-site check.
# TYPE tmplforge_csrf_rejected_total counter
tmplforge_csrf_rejected_total {{.CSRF}}
# HELP tmplforge_auth_failed_total Requests rejected for a missing or wrong token.
# TYPE tmplforge_auth_failed_total counter
tmplforge_auth_failed_total {{.AuthFail}}
`))

// WriteProm writes the Prometheus text exposition format.
func (m *Metrics) WriteProm(w io.Writer, version, goversion string) error {
	m.mu.Lock()
	d := struct {
		Version, GoVersion      string
		Start                   int64
		Requests, Renders       []kv
		Buckets                 []bucketLine
		Sum                     string
		N                       uint64
		Inflight                int
		Limited, CSRF, AuthFail uint64
	}{
		Version: version, GoVersion: goversion, Start: m.start.Unix(),
		Sum: strconv.FormatFloat(m.sum, 'g', -1, 64), N: m.n, Inflight: m.inflight,
		Limited: m.limited, CSRF: m.csrf, AuthFail: m.authFail,
	}
	for k, v := range m.requests {
		d.Requests = append(d.Requests, kv{`route="` + k[0] + `",code="` + k[1] + `"`, v})
	}
	for k, v := range m.renders {
		d.Renders = append(d.Renders, kv{`outcome="` + k + `"`, v})
	}
	var cum uint64
	for i, c := range m.counts {
		cum += c
		le := "+Inf"
		if i < len(Buckets) {
			le = strconv.FormatFloat(Buckets[i], 'g', -1, 64)
		}
		d.Buckets = append(d.Buckets, bucketLine{le, cum})
	}
	m.mu.Unlock()
	sort.Slice(d.Requests, func(i, j int) bool { return d.Requests[i].Labels < d.Requests[j].Labels })
	sort.Slice(d.Renders, func(i, j int) bool { return d.Renders[i].Labels < d.Renders[j].Labels })
	return promTmpl.Execute(w, d)
}

// Point is one chart point: latency percentiles over a time window.
type Point struct {
	At       time.Time
	N        int
	P50, P95 time.Duration
}

// Series buckets samples into windows of width w and computes p50/p95 of
// each (nearest-rank). Empty windows are skipped.
func Series(samples []Sample, w time.Duration) []Point {
	if len(samples) == 0 || w <= 0 {
		return nil
	}
	var out []Point
	var cur []time.Duration
	var start time.Time
	flush := func() {
		if len(cur) == 0 {
			return
		}
		sort.Slice(cur, func(i, j int) bool { return cur[i] < cur[j] })
		out = append(out, Point{At: start, N: len(cur), P50: rank(cur, 0.50), P95: rank(cur, 0.95)})
		cur = cur[:0]
	}
	for _, s := range samples {
		t := s.At.Truncate(w)
		if !t.Equal(start) {
			flush()
			start = t
		}
		cur = append(cur, s.Dur)
	}
	flush()
	return out
}

func rank(sorted []time.Duration, q float64) time.Duration {
	i := int(q*float64(len(sorted))+0.999999) - 1
	return sorted[max(0, min(i, len(sorted)-1))]
}
