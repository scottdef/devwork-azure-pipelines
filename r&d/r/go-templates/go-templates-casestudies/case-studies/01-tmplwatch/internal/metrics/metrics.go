// Package metrics keeps tmplwatch's counters and writes them in the
// Prometheus text exposition format through a text/template.
package metrics

import (
	_ "embed"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"
)

// ringSize is how many recent render durations feed the quantiles.
const ringSize = 1024

// Ring is a fixed-size buffer of recent observations.
type Ring struct {
	buf  [ringSize]float64
	n    int // total observations ever
	sum  float64
	full bool
}

// Observe adds v.
func (r *Ring) Observe(v float64) {
	r.buf[r.n%ringSize] = v
	r.n++
	r.sum += v
}

// Quantile returns the q-quantile (0..1) of the retained window using
// the nearest-rank method, or NaN when empty.
func (r *Ring) Quantile(q float64) float64 {
	k := min(r.n, ringSize)
	if k == 0 {
		return math.NaN()
	}
	s := make([]float64, k)
	copy(s, r.buf[:k])
	sort.Float64s(s)
	i := int(math.Ceil(q*float64(k))) - 1
	return s[max(0, min(i, k-1))]
}

// Metrics is the set of tmplwatch counters. It is safe for concurrent use.
type Metrics struct {
	mu          sync.Mutex
	version     string
	start       time.Time
	renders     map[string]float64
	renderErrs  map[string]float64
	writes      map[string]float64
	incomplete  map[string]bool
	commands    map[[2]string]float64 // template, result
	polls       map[string]float64
	lastPoll    time.Time
	execRestart float64
	durations   Ring
}

// New returns zeroed metrics. Poll results are pre-seeded so that rate()
// works from the first scrape.
func New(version string) *Metrics {
	return &Metrics{
		version: version, start: time.Now(),
		renders: map[string]float64{}, renderErrs: map[string]float64{},
		writes: map[string]float64{}, incomplete: map[string]bool{},
		commands: map[[2]string]float64{},
		polls:    map[string]float64{"changed": 0, "unchanged": 0, "error": 0},
	}
}

// Template registers a template name so that its series exist at zero.
func (m *Metrics) Template(name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.renders[name] += 0
	m.renderErrs[name] += 0
	m.writes[name] += 0
	m.incomplete[name] = m.incomplete[name] || false
}

// Render records one render of name.
func (m *Metrics) Render(name string, d time.Duration, err error, incomplete bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.renders[name]++
	if err != nil {
		m.renderErrs[name]++
	}
	m.incomplete[name] = incomplete || err != nil
	m.durations.Observe(d.Seconds())
}

// Write records that name's destination was (or in -dry would be) written.
func (m *Metrics) Write(name string) {
	m.mu.Lock()
	m.writes[name]++
	m.mu.Unlock()
}

// Command records a command run for name with result "ok" or "error".
func (m *Metrics) Command(name, result string) {
	m.mu.Lock()
	m.commands[[2]string{name, result}]++
	m.mu.Unlock()
}

// Poll records one source poll with result changed, unchanged or error.
func (m *Metrics) Poll(result string) {
	m.mu.Lock()
	m.polls[result]++
	m.lastPoll = time.Now()
	m.mu.Unlock()
}

// ExecRestart records a restart of the supervised child.
func (m *Metrics) ExecRestart() {
	m.mu.Lock()
	m.execRestart++
	m.mu.Unlock()
}

// Incomplete returns the sorted names of templates whose last render was
// incomplete or failed.
func (m *Metrics) Incomplete() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []string
	for n, bad := range m.incomplete {
		if bad {
			out = append(out, n)
		}
	}
	sort.Strings(out)
	return out
}

// Sample is one labelled value.
type Sample struct {
	Labels string // rendered {k="v",...} or ""
	Value  string
}

// Family is one metric with its samples.
type Family struct {
	Name, Help, Type string
	Samples          []Sample
}

//go:embed metrics.tmpl
var metricsTmpl string

var tmpl = template.Must(template.New("metrics").Parse(metricsTmpl))

// Expose writes the exposition text to w.
func (m *Metrics) Expose(w io.Writer) error {
	return tmpl.Execute(w, m.Families())
}

// Families snapshots the metrics as sorted families.
func (m *Metrics) Families() []Family {
	m.mu.Lock()
	defer m.mu.Unlock()
	byTemplate := func(src map[string]float64) []Sample {
		var s []Sample
		for _, n := range sortedKeys(src) {
			s = append(s, Sample{labels("template", n), num(src[n])})
		}
		return s
	}
	var inc []Sample
	for _, n := range sortedKeys(m.renders) {
		v := 0.0
		if m.incomplete[n] {
			v = 1
		}
		inc = append(inc, Sample{labels("template", n), num(v)})
	}
	var cmds []Sample
	ck := make([][2]string, 0, len(m.commands))
	for k := range m.commands {
		ck = append(ck, k)
	}
	sort.Slice(ck, func(i, j int) bool {
		return ck[i][0]+"\x00"+ck[i][1] < ck[j][0]+"\x00"+ck[j][1]
	})
	for _, k := range ck {
		cmds = append(cmds, Sample{labels("template", k[0], "result", k[1]), num(m.commands[k])})
	}
	var polls []Sample
	for _, r := range sortedKeys(m.polls) {
		polls = append(polls, Sample{labels("result", r), num(m.polls[r])})
	}
	var errsTotal float64
	for _, v := range m.renderErrs {
		errsTotal += v
	}
	d := &m.durations
	var dur []Sample
	for _, q := range []string{"0.5", "0.95", "0.99"} {
		f, _ := strconv.ParseFloat(q, 64)
		dur = append(dur, Sample{labels("quantile", q), num(d.Quantile(f))})
	}
	last := 0.0
	if !m.lastPoll.IsZero() {
		last = float64(m.lastPoll.UnixMilli()) / 1000
	}
	return []Family{
		{"tmplwatch_build_info", "Build information; value is always 1.", "gauge",
			[]Sample{{labels("version", m.version), "1"}}},
		{"tmplwatch_start_time_seconds", "Unix time the process started.", "gauge",
			[]Sample{{"", num(float64(m.start.UnixMilli()) / 1000)}}},
		{"tmplwatch_renders_total", "Template renders attempted.", "counter", byTemplate(m.renders)},
		{"tmplwatch_render_errors_total", "Template renders that failed with an error.", "counter",
			append([]Sample{{"", num(errsTotal)}}, byTemplate(m.renderErrs)...)},
		{"tmplwatch_render_duration_seconds", "Render duration over the last 1024 renders.", "summary",
			append(dur, Sample{"_sum", num(d.sum)}, Sample{"_count", num(float64(d.n))})},
		{"tmplwatch_template_incomplete", "1 if the template's last render was incomplete or failed.", "gauge", inc},
		{"tmplwatch_writes_total", "Destination files written.", "counter", byTemplate(m.writes)},
		{"tmplwatch_source_polls_total", "Source polls by result.", "counter", polls},
		{"tmplwatch_last_poll_timestamp_seconds", "Unix time of the last source poll.", "gauge",
			[]Sample{{"", num(last)}}},
		{"tmplwatch_commands_total", "Template commands run, by result.", "counter", cmds},
		{"tmplwatch_exec_restarts_total", "Restarts of the supervised exec child.", "counter",
			[]Sample{{"", num(m.execRestart)}}},
	}
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func num(v float64) string {
	switch {
	case math.IsNaN(v):
		return "NaN"
	case math.IsInf(v, 1):
		return "+Inf"
	case math.IsInf(v, -1):
		return "-Inf"
	}
	return strconv.FormatFloat(v, 'g', -1, 64)
}

var labelEscaper = strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`)

// labels renders k1, v1, k2, v2 ... as {k1="v1",k2="v2"}.
func labels(kv ...string) string {
	var b strings.Builder
	b.WriteByte('{')
	for i := 0; i+1 < len(kv); i += 2 {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(kv[i])
		b.WriteString(`="`)
		b.WriteString(labelEscaper.Replace(kv[i+1]))
		b.WriteByte('"')
	}
	b.WriteByte('}')
	return b.String()
}
