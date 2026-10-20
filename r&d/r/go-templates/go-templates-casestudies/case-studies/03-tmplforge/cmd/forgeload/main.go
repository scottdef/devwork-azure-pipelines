// Command forgeload is a constant-rate HTTP load generator for tmplforge,
// for machines without vegeta or hey. Like vegeta it is open-loop: a
// request leaves every 1/rate seconds whether or not earlier ones have
// returned, so a slow server shows up as latency, not as a lower rate
// (no coordinated omission).
//
//	forgeload -u http://127.0.0.1:8088/render -r 20 -d 5m -t catalog/motd.txt.tmpl
//
// It prints a report to stdout (text, or JSON with -json) and exits 1 when
// -p99 is set and exceeded.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"text/template"
	"time"
)

// Report is the load test result. The JSON form feeds the loadtest
// workflow's step summary (rendered by tmplforge itself).
type Report struct {
	Target   string         `json:"target"`
	Rate     float64        `json:"rate"`
	Duration string         `json:"duration"`
	Requests int            `json:"requests"`
	Success  float64        `json:"success"`
	Codes    map[string]int `json:"codes"`
	Errors   int            `json:"errors"`
	MinMS    float64        `json:"min_ms"`
	MeanMS   float64        `json:"mean_ms"`
	P50MS    float64        `json:"p50_ms"`
	P90MS    float64        `json:"p90_ms"`
	P95MS    float64        `json:"p95_ms"`
	P99MS    float64        `json:"p99_ms"`
	MaxMS    float64        `json:"max_ms"`
	SLOMS    float64        `json:"slo_p99_ms"`
	SLOMet   bool           `json:"slo_met"`
}

var reportTmpl = template.Must(template.New("r").Parse(`target    {{.Target}}
rate      {{.Rate}}/s for {{.Duration}}
requests  {{.Requests}}  success {{printf "%.2f" .Success}}%  errors {{.Errors}}
codes     {{range $k, $v := .Codes}}{{$k}}:{{$v}} {{end}}
latency   min {{printf "%.1f" .MinMS}}  mean {{printf "%.1f" .MeanMS}}  p50 {{printf "%.1f" .P50MS}}  p90 {{printf "%.1f" .P90MS}}  p95 {{printf "%.1f" .P95MS}}  p99 {{printf "%.1f" .P99MS}}  max {{printf "%.1f" .MaxMS}} ms
{{- if .SLOMS}}
slo       p99 < {{.SLOMS}} ms: {{if .SLOMet}}met{{else}}MISSED{{end}}
{{- end}}
`))

type result struct {
	code int
	dur  time.Duration
	err  bool
}

func main() {
	target := flag.String("u", "http://127.0.0.1:8088/render", "target `url`")
	rate := flag.Float64("r", 20, "requests per second")
	dur := flag.Duration("d", 30*time.Second, "test `duration`")
	tfile := flag.String("t", "", "template `file` to POST (default: a small built-in)")
	ffile := flag.String("f", "", "fixture `file` to POST")
	token := flag.String("token", os.Getenv("TMPLFORGE_TOKEN"), "bearer `token`")
	p99 := flag.Float64("p99", 0, "fail (exit 1) if p99 latency exceeds `ms`")
	asJSON := flag.Bool("json", false, "print the report as JSON")
	timeout := flag.Duration("timeout", 10*time.Second, "per-request timeout")
	flag.Parse()
	if *rate <= 0 || *dur <= 0 {
		fmt.Fprintln(os.Stderr, "forgeload: -r and -d must be positive")
		os.Exit(2)
	}
	body := url.Values{"template": {"hello {{.name | upper}}, {{len .items}} {{len .items | plural \"item\" \"items\"}}\n"}, "fixture": {`{"name":"glenda","items":[1,2,3]}`}, "format": {"txt"}}
	if *tfile != "" {
		b, err := os.ReadFile(*tfile)
		if err != nil {
			fmt.Fprintln(os.Stderr, "forgeload:", err)
			os.Exit(2)
		}
		body.Set("template", string(b))
		body.Set("fixture", "{}")
	}
	if *ffile != "" {
		b, err := os.ReadFile(*ffile)
		if err != nil {
			fmt.Fprintln(os.Stderr, "forgeload:", err)
			os.Exit(2)
		}
		body.Set("fixture", string(b))
	}
	enc := body.Encode()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	client := &http.Client{Timeout: *timeout, Transport: &http.Transport{MaxIdleConnsPerHost: 256}}
	var (
		mu      sync.Mutex
		results []result
		wg      sync.WaitGroup
	)
	interval := time.Duration(float64(time.Second) / *rate)
	tick := time.NewTicker(interval)
	defer tick.Stop()
	end := time.After(*dur)
loop:
	for {
		select {
		case <-ctx.Done():
			break loop
		case <-end:
			break loop
		case <-tick.C:
			wg.Add(1)
			go func() {
				defer wg.Done()
				r := shoot(client, *target, enc, *token)
				mu.Lock()
				results = append(results, r)
				mu.Unlock()
			}()
		}
	}
	wg.Wait()
	rep := summarize(results, *target, *rate, *dur, *p99)
	if *asJSON {
		e := json.NewEncoder(os.Stdout)
		e.SetIndent("", "  ")
		e.Encode(rep)
	} else {
		reportTmpl.Execute(os.Stdout, rep)
	}
	if *p99 > 0 && !rep.SLOMet {
		os.Exit(1)
	}
}

func shoot(c *http.Client, target, body, token string) result {
	req, err := http.NewRequest(http.MethodPost, target, strings.NewReader(body))
	if err != nil {
		return result{err: true}
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	start := time.Now()
	res, err := c.Do(req)
	if err != nil {
		return result{err: true, dur: time.Since(start)}
	}
	io.Copy(io.Discard, res.Body)
	res.Body.Close()
	return result{code: res.StatusCode, dur: time.Since(start)}
}

func summarize(rs []result, target string, rate float64, d time.Duration, slo float64) Report {
	rep := Report{Target: target, Rate: rate, Duration: d.String(), Requests: len(rs), Codes: map[string]int{}, SLOMS: slo}
	var durs []time.Duration
	var sum time.Duration
	ok := 0
	for _, r := range rs {
		if r.err {
			rep.Errors++
			continue
		}
		rep.Codes[fmt.Sprint(r.code)]++
		if r.code >= 200 && r.code < 300 {
			ok++
		}
		durs = append(durs, r.dur)
		sum += r.dur
	}
	if len(rs) > 0 {
		rep.Success = math.Round(10000*float64(ok)/float64(len(rs))) / 100
	}
	if len(durs) > 0 {
		sort.Slice(durs, func(i, j int) bool { return durs[i] < durs[j] })
		// 0.1 ms resolution: enough for an SLO in tens of ms, and short in reports
		ms := func(d time.Duration) float64 { return math.Round(float64(d.Microseconds())/100) / 10 }
		q := func(p float64) float64 {
			i := int(p*float64(len(durs))+0.999999) - 1
			return ms(durs[max(0, min(i, len(durs)-1))])
		}
		rep.MinMS, rep.MaxMS = ms(durs[0]), ms(durs[len(durs)-1])
		rep.MeanMS = ms(sum / time.Duration(len(durs)))
		rep.P50MS, rep.P90MS, rep.P95MS, rep.P99MS = q(.50), q(.90), q(.95), q(.99)
	}
	rep.SLOMet = slo <= 0 || (rep.P99MS < slo && rep.Errors == 0 && ok == len(rs))
	return rep
}
