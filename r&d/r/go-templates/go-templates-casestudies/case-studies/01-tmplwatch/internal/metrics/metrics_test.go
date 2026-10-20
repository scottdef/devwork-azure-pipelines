package metrics

import (
	"bufio"
	"bytes"
	"errors"
	"math"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestRingQuantile(t *testing.T) {
	var r Ring
	if !math.IsNaN(r.Quantile(0.5)) {
		t.Error("empty ring not NaN")
	}
	for i := 1; i <= 100; i++ {
		r.Observe(float64(i))
	}
	for q, want := range map[float64]float64{0.5: 50, 0.95: 95, 0.99: 99, 1: 100, 0: 1} {
		if got := r.Quantile(q); got != want {
			t.Errorf("q%v = %v, want %v", q, got, want)
		}
	}
	// Overflow: only the last ringSize observations count.
	for i := 0; i < ringSize; i++ {
		r.Observe(1000)
	}
	if r.Quantile(0.01) != 1000 {
		t.Error("ring did not wrap")
	}
}

// line matches a sample line: name{labels} value
var line = regexp.MustCompile(`^([a-zA-Z_:][a-zA-Z0-9_:]*)(\{[^}]*\})? (\S+)$`)

// Parse reads exposition text into "name{labels}" → value and checks
// that every sample's family was declared by HELP and TYPE lines first.
func Parse(text string) (map[string]string, error) {
	out := map[string]string{}
	typed := map[string]string{}
	sc := bufio.NewScanner(strings.NewReader(text))
	for sc.Scan() {
		l := sc.Text()
		switch {
		case l == "":
			return nil, errors.New("blank line")
		case strings.HasPrefix(l, "# HELP "):
		case strings.HasPrefix(l, "# TYPE "):
			f := strings.Fields(l)
			if len(f) != 4 {
				return nil, errors.New("bad TYPE: " + l)
			}
			typed[f[2]] = f[3]
		default:
			m := line.FindStringSubmatch(l)
			if m == nil {
				return nil, errors.New("bad sample: " + l)
			}
			fam := strings.TrimSuffix(strings.TrimSuffix(m[1], "_sum"), "_count")
			if typed[fam] == "" {
				return nil, errors.New("sample before TYPE: " + l)
			}
			out[m[1]+m[2]] = m[3]
		}
	}
	return out, nil
}

func TestExpose(t *testing.T) {
	m := New("v1.2.3")
	m.Template("Caddyfile")
	m.Template(`we"ird`)
	m.Render("Caddyfile", 2*time.Millisecond, nil, false)
	m.Render("Caddyfile", 4*time.Millisecond, errors.New("x"), false)
	m.Write("Caddyfile")
	m.Command("Caddyfile", "ok")
	m.Poll("changed")
	m.Poll("unchanged")
	m.Poll("unchanged")
	var buf bytes.Buffer
	if err := m.Expose(&buf); err != nil {
		t.Fatal(err)
	}
	text := buf.String()
	if !strings.HasSuffix(text, "\n") || strings.HasPrefix(text, "\n") {
		t.Errorf("framing: %q", text[:20])
	}
	got, err := Parse(text)
	if err != nil {
		t.Fatalf("%v\n%s", err, text)
	}
	for k, want := range map[string]string{
		`tmplwatch_build_info{version="v1.2.3"}`:                     "1",
		`tmplwatch_renders_total{template="Caddyfile"}`:              "2",
		`tmplwatch_renders_total{template="we\"ird"}`:                "0",
		`tmplwatch_render_errors_total`:                              "1",
		`tmplwatch_render_errors_total{template="Caddyfile"}`:        "1",
		`tmplwatch_render_duration_seconds{quantile="0.5"}`:          "0.002",
		`tmplwatch_render_duration_seconds{quantile="0.99"}`:         "0.004",
		`tmplwatch_render_duration_seconds_count`:                    "2",
		`tmplwatch_template_incomplete{template="Caddyfile"}`:        "1",
		`tmplwatch_source_polls_total{result="unchanged"}`:           "2",
		`tmplwatch_source_polls_total{result="error"}`:               "0",
		`tmplwatch_commands_total{result="ok",template="Caddyfile"}`: "",
		`tmplwatch_commands_total{template="Caddyfile",result="ok"}`: "1",
		`tmplwatch_exec_restarts_total`:                              "0",
	} {
		if want == "" {
			if _, ok := got[k]; ok {
				t.Errorf("unexpected %s", k)
			}
			continue
		}
		if got[k] != want {
			t.Errorf("%s = %q, want %q", k, got[k], want)
		}
	}
	if v := got["tmplwatch_last_poll_timestamp_seconds"]; v == "0" || v == "" {
		t.Error("last poll not set")
	}
	if inc := m.Incomplete(); len(inc) != 1 || inc[0] != "Caddyfile" {
		t.Errorf("incomplete %v", inc)
	}
}
