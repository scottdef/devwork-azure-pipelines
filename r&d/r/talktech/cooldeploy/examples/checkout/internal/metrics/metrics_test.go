package metrics

import (
	"strings"
	"testing"
)

func TestText(t *testing.T) {
	var r Registry
	r.Buckets("req_seconds", 0.1, 1)
	r.Inc("req_total", "code", "200", "app", "checkout")
	r.Inc("req_total", "app", "checkout", "code", "200")
	r.Inc("req_total", "code", "500", "app", `we"ird`)
	r.Observe("req_seconds", 0.05)
	r.Observe("req_seconds", 0.5)
	r.Observe("req_seconds", 5)

	var b strings.Builder
	if _, err := r.WriteTo(&b); err != nil {
		t.Fatal(err)
	}
	want := `# TYPE req_total counter
req_total{app="checkout",code="200"} 2
req_total{app="we\"ird",code="500"} 1
# TYPE req_seconds histogram
req_seconds_bucket{le="0.1"} 1
req_seconds_bucket{le="1"} 2
req_seconds_bucket{le="+Inf"} 3
req_seconds_sum 5.55
req_seconds_count 3
`
	if got := b.String(); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}
