package web

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// Limiter is a per-client token bucket: each client may make Burst
// requests at once and Rate per second sustained. It is a few dozen lines,
// which is why it is here and not an import of golang.org/x/time/rate.
type Limiter struct {
	Rate  float64 // tokens per second
	Burst float64 // bucket capacity
	Now   func() time.Time

	mu      sync.Mutex
	buckets map[string]*bucket
	swept   time.Time
}

type bucket struct {
	tokens float64
	last   time.Time
}

// maxBuckets bounds memory under a spray of source addresses; idle
// buckets (full again) are swept when the map grows or once a minute.
const maxBuckets = 16384

// NewLimiter returns a limiter allowing rate requests per second with a
// burst of twice that (at least 1).
func NewLimiter(rate float64) *Limiter {
	return &Limiter{Rate: rate, Burst: max(2*rate, 1), Now: time.Now, buckets: map[string]*bucket{}}
}

// Allow takes a token from key's bucket. When none is left it reports
// false and how long until one will be.
func (l *Limiter) Allow(key string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.Now()
	if len(l.buckets) >= maxBuckets || now.Sub(l.swept) > time.Minute {
		l.sweep(now)
	}
	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{tokens: l.Burst, last: now}
		l.buckets[key] = b
	}
	b.tokens = min(l.Burst, b.tokens+now.Sub(b.last).Seconds()*l.Rate)
	b.last = now
	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}
	wait := time.Duration((1 - b.tokens) / l.Rate * float64(time.Second))
	return false, wait
}

// sweep drops buckets that have refilled: forgetting them changes nothing.
func (l *Limiter) sweep(now time.Time) {
	for k, b := range l.buckets {
		if b.tokens+now.Sub(b.last).Seconds()*l.Rate >= l.Burst {
			delete(l.buckets, k)
		}
	}
	if len(l.buckets) >= maxBuckets {
		// still full of active clients: start over rather than grow
		l.buckets = map[string]*bucket{}
	}
	l.swept = now
}

// clientIP is the rate-limit key: the peer address. X-Forwarded-For is
// deliberately ignored; behind a proxy, run one limiter at the proxy.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
