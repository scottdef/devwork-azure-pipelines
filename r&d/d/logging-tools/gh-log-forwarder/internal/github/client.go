// Package github is a small GitHub REST client: just the Actions
// endpoints ghlog2dt needs, Link-header pagination, and retry rules for
// GitHub's primary and secondary rate limits. Standard library only.
package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DefaultAPIVersion is pinned; GitHub supports a version for at least 24
// months after its successor ships.
const DefaultAPIVersion = "2022-11-28"

var (
	// ErrAuth means the token was rejected (401).
	ErrAuth = errors.New("github: authentication failed")
	// ErrRateLimited means the primary limit resets too far in the future.
	ErrRateLimited = errors.New("github: rate limit exhausted")
)

// StatusError is a non-retryable (or retries exhausted) HTTP failure.
type StatusError struct {
	Code    int
	Method  string
	Path    string
	Message string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("github: %s %s: %d %s", e.Method, e.Path, e.Code, e.Message)
}

// IsNotFound reports whether err is a 404.
func IsNotFound(err error) bool {
	var se *StatusError
	return errors.As(err, &se) && se.Code == http.StatusNotFound
}

// Client talks to api.github.com (or a GHES/GHE.com base URL).
type Client struct {
	BaseURL    string
	Token      string
	APIVersion string
	UserAgent  string
	HTTP       *http.Client
	MaxRetries int
	MaxWait    time.Duration // longest primary-limit reset we will sleep through
	Log        *slog.Logger

	// Sleep is a test hook; nil means a context-aware time.Sleep.
	Sleep func(context.Context, time.Duration) error

	mu        sync.Mutex
	remaining int
	reset     time.Time
	calls     int
}

// New returns a client with sane defaults.
func New(baseURL, token string) *Client {
	return &Client{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		Token:      token,
		APIVersion: DefaultAPIVersion,
		UserAgent:  "ghlog2dt",
		HTTP: &http.Client{
			Timeout:       5 * time.Minute,
			CheckRedirect: CheckRedirect,
		},
		MaxRetries: 5,
		MaxWait:    15 * time.Minute,
		Log:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		remaining:  -1,
	}
}

// CheckRedirect refuses https→http downgrades and drops Authorization
// whenever the redirect leaves the original host:port. Go already does
// this across domains; we also do it across ports and say so explicitly,
// because the job-log 302 points at a signed blob URL that must never
// see the installation token.
func CheckRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errors.New("github: stopped after 10 redirects")
	}
	if via[0].URL.Scheme == "https" && req.URL.Scheme != "https" {
		return errors.New("github: refusing https to http redirect")
	}
	if req.URL.Host != via[0].URL.Host {
		req.Header.Del("Authorization")
	}
	return nil
}

// RateRemaining is the last X-RateLimit-Remaining seen (-1 if none).
func (c *Client) RateRemaining() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.remaining
}

// Calls is the number of HTTP requests made (including retries).
func (c *Client) Calls() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

func (c *Client) observe(h http.Header) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls++
	if v := h.Get("X-RateLimit-Remaining"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			c.remaining = n
		}
	}
	if v := h.Get("X-RateLimit-Reset"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			c.reset = time.Unix(n, 0)
		}
	}
}

// request describes one API call.
type request struct {
	method   string
	path     string // "/repos/..." or an absolute URL from a Link header
	query    url.Values
	body     any
	token    string // overrides Client.Token (app JWT)
	retry404 int    // job logs may 404 briefly after a job completes
}

func (c *Client) url(r request) string {
	u := r.path
	if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
		u = c.BaseURL + r.path
	}
	if len(r.query) > 0 {
		u += "?" + r.query.Encode()
	}
	return u
}

// do performs a request with retries. On success the caller owns the body.
func (c *Client) do(ctx context.Context, r request) (*http.Response, error) {
	u := c.url(r)
	pathOnly := r.path
	if i := strings.IndexByte(pathOnly, '?'); i >= 0 {
		pathOnly = pathOnly[:i]
	}
	var payload []byte
	if r.body != nil {
		var err error
		if payload, err = json.Marshal(r.body); err != nil {
			return nil, err
		}
	}
	token := c.Token
	if r.token != "" {
		token = r.token
	}
	for attempt := 0; ; attempt++ {
		var body io.Reader
		if payload != nil {
			body = bytes.NewReader(payload)
		}
		req, err := http.NewRequestWithContext(ctx, r.method, u, body)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", c.APIVersion)
		req.Header.Set("User-Agent", c.UserAgent)
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}

		resp, err := c.HTTP.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if attempt >= c.MaxRetries {
				return nil, fmt.Errorf("github: %s %s: %w", r.method, pathOnly, err)
			}
			wait := expBackoff(attempt)
			c.Log.Warn("github transport error, retrying", "path", pathOnly, "attempt", attempt+1, "wait", wait, "err", err)
			if err := c.sleep(ctx, wait); err != nil {
				return nil, err
			}
			continue
		}
		c.observe(resp.Header)
		if resp.StatusCode < 300 {
			return resp, nil
		}
		msg := readMessage(resp)
		wait, retry, fatal := c.classify(resp, msg, attempt, r.retry404)
		if fatal != nil {
			return nil, fatal
		}
		if !retry || attempt >= c.MaxRetries {
			return nil, &StatusError{Code: resp.StatusCode, Method: r.method, Path: pathOnly, Message: msg}
		}
		c.Log.Warn("github retry", "path", pathOnly, "status", resp.StatusCode, "attempt", attempt+1, "wait", wait)
		if err := c.sleep(ctx, wait); err != nil {
			return nil, err
		}
	}
}

// classify decides between retry (and how long to wait), give up, or a
// typed fatal error.
func (c *Client) classify(resp *http.Response, msg string, attempt, retry404 int) (time.Duration, bool, error) {
	h := resp.Header
	switch code := resp.StatusCode; {
	case code == http.StatusUnauthorized:
		return 0, false, ErrAuth
	case code == http.StatusForbidden || code == http.StatusTooManyRequests:
		if ra := h.Get("Retry-After"); ra != "" { // secondary limit
			if s, err := strconv.Atoi(ra); err == nil {
				return time.Duration(s)*time.Second + jitter(), true, nil
			}
		}
		if h.Get("X-RateLimit-Remaining") == "0" { // primary limit
			reset, err := strconv.ParseInt(h.Get("X-RateLimit-Reset"), 10, 64)
			if err != nil {
				return 0, false, ErrRateLimited
			}
			d := time.Until(time.Unix(reset, 0)) + jitter()
			if d > c.MaxWait {
				return 0, false, fmt.Errorf("%w: resets at %s", ErrRateLimited, time.Unix(reset, 0).UTC().Format(time.RFC3339))
			}
			if d < time.Second {
				d = time.Second
			}
			return d, true, nil
		}
		if strings.Contains(strings.ToLower(msg), "secondary rate limit") {
			// GitHub: wait at least a minute, then back off exponentially.
			return time.Minute*time.Duration(1<<min(attempt, 3)) + jitter(), true, nil
		}
		if code == http.StatusTooManyRequests {
			return expBackoff(attempt), true, nil
		}
		return 0, false, nil // permission problem: retrying will not help
	case code == http.StatusNotFound:
		return expBackoff(attempt), attempt < retry404, nil
	case code >= 500:
		return expBackoff(attempt), true, nil
	}
	return 0, false, nil
}

func (c *Client) sleep(ctx context.Context, d time.Duration) error {
	if c.Sleep != nil {
		return c.Sleep(ctx, d)
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func expBackoff(attempt int) time.Duration {
	return time.Duration(1<<min(attempt, 6))*time.Second + jitter()
}

func jitter() time.Duration { return time.Duration(rand.Int64N(int64(time.Second))) }

// readMessage drains and closes the body, returning GitHub's "message".
func readMessage(resp *http.Response) string {
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	var m struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(b, &m) == nil && m.Message != "" {
		return m.Message
	}
	s := strings.TrimSpace(string(b))
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}

var nextRe = regexp.MustCompile(`<([^>]+)>;\s*rel="next"`)

// NextLink extracts rel="next" from a Link header.
func NextLink(h http.Header) string {
	for _, v := range h.Values("Link") {
		if m := nextRe.FindStringSubmatch(v); m != nil {
			return m[1]
		}
	}
	return ""
}

// getJSON GETs and decodes one page, returning the next-page URL.
func (c *Client) getJSON(ctx context.Context, r request, v any) (string, error) {
	r.method = http.MethodGet
	resp, err := c.do(ctx, r)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		return "", fmt.Errorf("github: decode %s: %w", r.path, err)
	}
	return NextLink(resp.Header), nil
}
