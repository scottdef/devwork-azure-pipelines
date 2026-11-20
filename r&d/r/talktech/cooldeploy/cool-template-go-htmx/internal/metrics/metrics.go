// Package metrics writes counters and histograms in the Prometheus text
// format. It exists so that Go services on the platform need no client
// library. Copy the file into an app; it has no dependencies.
package metrics

import (
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// DefaultBuckets suits request durations in seconds.
var DefaultBuckets = []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10}

// Registry holds every metric of a process. The zero value is ready to use.
type Registry struct {
	mu       sync.Mutex
	counters map[string]map[string]float64 // name -> label set -> value
	hists    map[string]map[string]*hist   // name -> label set -> histogram
	buckets  map[string][]float64
}

type hist struct {
	counts []uint64 // one per bucket, cumulative on output
	sum    float64
	n      uint64
}

// Buckets sets the bucket bounds of a histogram before its first observation.
func (r *Registry) Buckets(name string, bounds ...float64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.buckets == nil {
		r.buckets = map[string][]float64{}
	}
	r.buckets[name] = bounds
}

// Add adds v to a counter. Labels are key, value pairs.
func (r *Registry) Add(name string, v float64, labels ...string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.counters == nil {
		r.counters = map[string]map[string]float64{}
	}
	if r.counters[name] == nil {
		r.counters[name] = map[string]float64{}
	}
	r.counters[name][labelSet(labels)] += v
}

// Inc adds one to a counter.
func (r *Registry) Inc(name string, labels ...string) { r.Add(name, 1, labels...) }

// Observe records v in a histogram.
func (r *Registry) Observe(name string, v float64, labels ...string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.hists == nil {
		r.hists = map[string]map[string]*hist{}
	}
	if r.hists[name] == nil {
		r.hists[name] = map[string]*hist{}
	}
	bounds := r.buckets[name]
	if bounds == nil {
		bounds = DefaultBuckets
	}
	ls := labelSet(labels)
	h := r.hists[name][ls]
	if h == nil {
		h = &hist{counts: make([]uint64, len(bounds))}
		r.hists[name][ls] = h
	}
	for i, b := range bounds {
		if v <= b {
			h.counts[i]++
			break
		}
	}
	h.sum += v
	h.n++
}

// ServeHTTP writes every metric, sorted, in the text exposition format.
func (r *Registry) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	r.WriteTo(w)
}

// WriteTo writes every metric to w.
func (r *Registry) WriteTo(w io.Writer) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var b strings.Builder
	for _, name := range sortedKeys(r.counters) {
		fmt.Fprintf(&b, "# TYPE %s counter\n", name)
		for _, ls := range sortedKeys(r.counters[name]) {
			fmt.Fprintf(&b, "%s%s %s\n", name, braces(ls), num(r.counters[name][ls]))
		}
	}
	for _, name := range sortedKeys(r.hists) {
		bounds := r.buckets[name]
		if bounds == nil {
			bounds = DefaultBuckets
		}
		fmt.Fprintf(&b, "# TYPE %s histogram\n", name)
		for _, ls := range sortedKeys(r.hists[name]) {
			h := r.hists[name][ls]
			var cum uint64
			for i, bound := range bounds {
				cum += h.counts[i]
				fmt.Fprintf(&b, "%s_bucket%s %d\n", name, braces(join(ls, `le="`+num(bound)+`"`)), cum)
			}
			fmt.Fprintf(&b, "%s_bucket%s %d\n", name, braces(join(ls, `le="+Inf"`)), h.n)
			fmt.Fprintf(&b, "%s_sum%s %s\n", name, braces(ls), num(h.sum))
			fmt.Fprintf(&b, "%s_count%s %d\n", name, braces(ls), h.n)
		}
	}
	n, err := io.WriteString(w, b.String())
	return int64(n), err
}

// labelSet turns k, v pairs into `k="v",k2="v2"`, sorted by key.
func labelSet(kv []string) string {
	if len(kv)%2 != 0 {
		kv = append(kv, "")
	}
	pairs := make([]string, 0, len(kv)/2)
	for i := 0; i < len(kv); i += 2 {
		pairs = append(pairs, kv[i]+`="`+escape(kv[i+1])+`"`)
	}
	sort.Strings(pairs)
	return strings.Join(pairs, ",")
}

var escaper = strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`)

func escape(s string) string { return escaper.Replace(s) }

func join(a, b string) string {
	if a == "" {
		return b
	}
	return a + "," + b
}

func braces(ls string) string {
	if ls == "" {
		return ""
	}
	return "{" + ls + "}"
}

func num(f float64) string {
	if math.IsInf(f, 1) {
		return "+Inf"
	}
	return strconv.FormatFloat(f, 'g', -1, 64)
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
