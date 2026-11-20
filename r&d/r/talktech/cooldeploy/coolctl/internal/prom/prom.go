// Package prom runs instant PromQL queries and judges a release against an
// error budget. It is the gate between the stages of a rolling release.
package prom

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Client queries a Prometheus-compatible endpoint, such as the query endpoint
// of an Azure Monitor workspace.
type Client struct {
	Endpoint string       // base URL; /api/v1/query is appended
	Token    string       // bearer token, optional
	HTTP     *http.Client // default: a client with a 30 second timeout
}

// ErrNoData means the query matched no series, or the result was not a number.
var ErrNoData = errors.New("query returned no data")

// Query evaluates an instant query that yields one number.
func (c Client) Query(ctx context.Context, query string) (float64, error) {
	u := strings.TrimSuffix(c.Endpoint, "/") + "/api/v1/query?query=" + url.QueryEscape(query)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return 0, err
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := hc.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return 0, err
	}
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("query: %s: %s", resp.Status, strings.TrimSpace(string(b)))
	}
	var out struct {
		Status string
		Error  string
		Data   struct {
			ResultType string
			Result     json.RawMessage
		}
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return 0, fmt.Errorf("query: %w", err)
	}
	if out.Status != "success" {
		return 0, fmt.Errorf("query: %s", out.Error)
	}
	// A scalar is [ts, "value"]; a vector is [{"value": [ts, "value"]}].
	var sample []any
	switch out.Data.ResultType {
	case "scalar":
		if err := json.Unmarshal(out.Data.Result, &sample); err != nil {
			return 0, err
		}
	case "vector":
		var series []struct{ Value []any }
		if err := json.Unmarshal(out.Data.Result, &series); err != nil {
			return 0, err
		}
		if len(series) == 0 {
			return 0, ErrNoData
		}
		if len(series) > 1 {
			return 0, fmt.Errorf("query returned %d series; aggregate it to one", len(series))
		}
		sample = series[0].Value
	default:
		return 0, fmt.Errorf("query returned a %s; want a scalar or a vector", out.Data.ResultType)
	}
	if len(sample) != 2 {
		return 0, ErrNoData
	}
	s, _ := sample[1].(string)
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, ErrNoData // 0/0 when the release has had no traffic
	}
	return f, nil
}

// ErrorRate returns the PromQL for the share of 5xx responses a workload
// served over the window, from the mesh's request counter.
func ErrorRate(namespace, workload, window string) string {
	sel := fmt.Sprintf(`destination_workload_namespace=%q,destination_workload=%q`, namespace, workload)
	return fmt.Sprintf(`sum(rate(istio_requests_total{%s,response_code=~"5.."}[%s])) / sum(rate(istio_requests_total{%s}[%s]))`,
		sel, window, sel, window)
}

// P95 returns the PromQL for the 95th percentile request duration of a
// workload over the window, in milliseconds.
func P95(namespace, workload, window string) string {
	return fmt.Sprintf(`histogram_quantile(0.95, sum by (le) (rate(istio_request_duration_milliseconds_bucket{destination_workload_namespace=%q,destination_workload=%q}[%s])))`,
		namespace, workload, window)
}

// Gate is the error budget a release must stay inside.
type Gate struct {
	MaxErrorRate float64 // for example 0.01 for one percent
	MaxP95ms     float64 // zero disables the latency check
	AllowNoData  bool    // pass when the release has had no traffic
}

// Verdict is the outcome of a gate check.
type Verdict struct {
	Pass      bool    `json:"pass"`
	ErrorRate float64 `json:"error_rate"`
	P95ms     float64 `json:"p95_ms,omitempty"`
	Reason    string  `json:"reason"`
}

// Check evaluates the gate for a workload. A release with no traffic fails
// unless AllowNoData is set: no evidence is not the same as good evidence.
func (g Gate) Check(ctx context.Context, c Client, namespace, workload, window string) (Verdict, error) {
	rate, err := c.Query(ctx, ErrorRate(namespace, workload, window))
	if errors.Is(err, ErrNoData) {
		if g.AllowNoData {
			return Verdict{Pass: true, Reason: "no traffic in the window; allowed"}, nil
		}
		return Verdict{Reason: "no traffic in the window, so there is nothing to judge"}, nil
	}
	if err != nil {
		return Verdict{}, err
	}
	v := Verdict{ErrorRate: rate}
	if rate > g.MaxErrorRate {
		v.Reason = fmt.Sprintf("error rate %.2f%% is over the budget of %.2f%%", rate*100, g.MaxErrorRate*100)
		return v, nil
	}
	if g.MaxP95ms > 0 {
		p95, err := c.Query(ctx, P95(namespace, workload, window))
		if err != nil && !errors.Is(err, ErrNoData) {
			return Verdict{}, err
		}
		v.P95ms = p95
		if p95 > g.MaxP95ms {
			v.Reason = fmt.Sprintf("p95 latency %.0f ms is over the budget of %.0f ms", p95, g.MaxP95ms)
			return v, nil
		}
	}
	v.Pass = true
	v.Reason = fmt.Sprintf("error rate %.2f%% is inside the budget of %.2f%%", rate*100, g.MaxErrorRate*100)
	return v, nil
}
