package prom

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// server answers error-rate queries with rate and latency queries with p95.
// An empty value answers with an empty vector.
func server(t *testing.T, rate, p95 string) Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		v := rate
		if strings.Contains(r.URL.Query().Get("query"), "histogram_quantile") {
			v = p95
		}
		if v == "" {
			fmt.Fprint(w, `{"status":"success","data":{"resultType":"vector","result":[]}}`)
			return
		}
		fmt.Fprintf(w, `{"status":"success","data":{"resultType":"vector","result":[{"metric":{},"value":[1731400000,%q]}]}}`, v)
	}))
	t.Cleanup(srv.Close)
	return Client{Endpoint: srv.URL, Token: "tok"}
}

func TestGate(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name      string
		rate, p95 string
		gate      Gate
		pass      bool
	}{
		{"inside the budget", "0.002", "120", Gate{MaxErrorRate: 0.01, MaxP95ms: 500}, true},
		{"error rate over", "0.08", "120", Gate{MaxErrorRate: 0.01}, false},
		{"latency over", "0", "900", Gate{MaxErrorRate: 0.01, MaxP95ms: 500}, false},
		{"no traffic fails closed", "", "", Gate{MaxErrorRate: 0.01}, false},
		{"no traffic allowed", "", "", Gate{MaxErrorRate: 0.01, AllowNoData: true}, true},
		{"NaN is no data", "NaN", "", Gate{MaxErrorRate: 0.01}, false},
	}
	for _, tt := range tests {
		v, err := tt.gate.Check(ctx, server(t, tt.rate, tt.p95), "payments-checkout", "checkout-r3f9c2ab", "5m")
		if err != nil {
			t.Errorf("%s: %v", tt.name, err)
			continue
		}
		if v.Pass != tt.pass {
			t.Errorf("%s: pass = %v (%s), want %v", tt.name, v.Pass, v.Reason, tt.pass)
		}
	}
}

func TestQueryErrors(t *testing.T) {
	c := server(t, "0.1", "")
	c.Token = "wrong"
	if _, err := c.Query(context.Background(), "up"); err == nil {
		t.Error("a 401 was not reported")
	}
}

func TestErrorRateQuery(t *testing.T) {
	q := ErrorRate("payments-checkout", "checkout-r3f9c2ab", "5m")
	for _, want := range []string{`destination_workload="checkout-r3f9c2ab"`, `response_code=~"5.."`, `[5m]`} {
		if !strings.Contains(q, want) {
			t.Errorf("query %s lacks %s", q, want)
		}
	}
}
