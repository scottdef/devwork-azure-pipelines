// Tmplload is a small open-loop HTTP load generator, for when vegeta and
// hey are not installed.
//
// Usage:
//
//	tmplload -url URL [-rate N] [-duration 30s] [-c 8] [-timeout 5s] [-json]
//
// Requests are issued at a fixed rate whatever the response times (open
// loop), so a slow server shows up as latency rather than as a lower
// request rate. Latency is measured from each request's scheduled start,
// which avoids coordinated omission when workers fall behind.
// Exit status is 1 if any request failed.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"sort"
	"sync"
	"time"
)

var version = "dev"

// Report is the result; -json prints it for templates such as
// scripts/loadtest-summary.md.tmpl.
type Report struct {
	URL         string         `json:"url"`
	Rate        int            `json:"rate"`
	Duration    string         `json:"duration"`
	Concurrency int            `json:"concurrency"`
	Requests    int            `json:"requests"`
	Errors      int            `json:"errors"`
	Dropped     int            `json:"dropped"`
	Throughput  float64        `json:"throughput_rps"`
	P50         float64        `json:"p50_ms"`
	P95         float64        `json:"p95_ms"`
	P99         float64        `json:"p99_ms"`
	Max         float64        `json:"max_ms"`
	Mean        float64        `json:"mean_ms"`
	Status      map[string]int `json:"status"`
	FirstError  string         `json:"first_error,omitempty"`
}

type result struct {
	lat    time.Duration
	status int
	err    error
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("tmplload", flag.ContinueOnError)
	fs.SetOutput(stderr)
	url := fs.String("url", "", "target `URL` (required)")
	rate := fs.Int("rate", 50, "requests per second")
	dur := fs.Duration("duration", 30*time.Second, "test `duration`")
	conc := fs.Int("c", 8, "maximum in-flight requests")
	timeout := fs.Duration("timeout", 5*time.Second, "per-request `timeout`")
	asJSON := fs.Bool("json", false, "print the report as JSON")
	showVer := fs.Bool("version", false, "print version and exit")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *showVer {
		fmt.Fprintln(stdout, "tmplload", version)
		return 0
	}
	if *url == "" || *rate <= 0 || *conc <= 0 || *dur <= 0 {
		fmt.Fprintln(stderr, "usage: tmplload -url URL [-rate N] [-duration 30s] [-c 8] [-json]")
		return 2
	}
	rep := attack(context.Background(), *url, *rate, *dur, *conc, *timeout)
	if *asJSON {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		enc.Encode(rep)
	} else {
		fmt.Fprintf(stdout, "url\t%s\nrate\t%d/s for %s, c=%d\nrequests\t%d (%.1f/s)\nerrors\t%d\ndropped\t%d\n"+
			"latency\tp50 %.2fms  p95 %.2fms  p99 %.2fms  max %.2fms  mean %.2fms\n",
			rep.URL, rep.Rate, rep.Duration, rep.Concurrency, rep.Requests, rep.Throughput,
			rep.Errors, rep.Dropped, rep.P50, rep.P95, rep.P99, rep.Max, rep.Mean)
		codes := make([]string, 0, len(rep.Status))
		for k := range rep.Status {
			codes = append(codes, k)
		}
		sort.Strings(codes)
		for _, k := range codes {
			fmt.Fprintf(stdout, "status\t%s\t%d\n", k, rep.Status[k])
		}
		if rep.FirstError != "" {
			fmt.Fprintf(stdout, "first_error\t%s\n", rep.FirstError)
		}
	}
	if rep.Errors > 0 || rep.Dropped > 0 {
		return 1
	}
	return 0
}

func attack(ctx context.Context, url string, rate int, dur time.Duration, conc int, timeout time.Duration) Report {
	tr := &http.Transport{MaxIdleConnsPerHost: conc, MaxConnsPerHost: conc, DisableCompression: true}
	client := &http.Client{Timeout: timeout, Transport: tr}
	defer tr.CloseIdleConnections()

	n := int(math.Round(float64(rate) * dur.Seconds()))
	interval := time.Second / time.Duration(rate)
	// Workers take scheduled start times; a full queue means every worker
	// is busy and the backlog exceeds one second, so the request is
	// dropped and counted rather than silently delayed forever.
	jobs := make(chan time.Time, rate)
	results := make(chan result, n)
	var wg sync.WaitGroup
	for range conc {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for at := range jobs {
				results <- hit(ctx, client, url, at)
			}
		}()
	}
	start := time.Now()
	dropped := 0
	for i := 0; i < n; i++ {
		at := start.Add(time.Duration(i) * interval)
		if d := time.Until(at); d > 0 {
			time.Sleep(d)
		}
		select {
		case jobs <- at:
		default:
			dropped++
		}
	}
	close(jobs)
	wg.Wait()
	elapsed := time.Since(start)
	close(results)

	rep := Report{URL: url, Rate: rate, Duration: dur.String(), Concurrency: conc, Dropped: dropped, Status: map[string]int{}}
	var lats []float64
	var sum float64
	for r := range results {
		rep.Requests++
		ms := float64(r.lat) / float64(time.Millisecond)
		lats = append(lats, ms)
		sum += ms
		switch {
		case r.err != nil:
			rep.Errors++
			rep.Status["error"]++
			if rep.FirstError == "" {
				rep.FirstError = r.err.Error()
			}
		default:
			rep.Status[fmt.Sprint(r.status)]++
			if r.status >= 400 {
				rep.Errors++
				if rep.FirstError == "" {
					rep.FirstError = fmt.Sprintf("HTTP %d", r.status)
				}
			}
		}
	}
	sort.Float64s(lats)
	if len(lats) > 0 {
		rep.P50, rep.P95, rep.P99 = q(lats, .5), q(lats, .95), q(lats, .99)
		rep.Max, rep.Mean = round(lats[len(lats)-1]), round(sum/float64(len(lats)))
	}
	rep.Throughput = round(float64(rep.Requests) / elapsed.Seconds())
	return rep
}

func hit(ctx context.Context, c *http.Client, url string, at time.Time) result {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return result{err: err}
	}
	resp, err := c.Do(req)
	if err != nil {
		return result{lat: time.Since(at), err: err}
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return result{lat: time.Since(at), status: resp.StatusCode}
}

// q is the nearest-rank quantile of sorted s.
func q(s []float64, p float64) float64 {
	i := int(math.Ceil(p*float64(len(s)))) - 1
	return round(s[max(0, min(i, len(s)-1))])
}

func round(v float64) float64 { return math.Round(v*1000) / 1000 }
