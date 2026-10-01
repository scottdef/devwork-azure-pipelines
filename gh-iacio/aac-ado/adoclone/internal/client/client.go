// Package client is a small Azure DevOps REST client: Basic auth with a PAT,
// retries with Retry-After and rate-limit headers, continuation-token paging,
// streaming downloads, and a dry-run switch that blocks writes.
package client

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/coolado/adoclone/internal/metrics"
)

// Service hosts.
const (
	Core  = "dev.azure.com"
	VSSPS = "vssps.dev.azure.com"
	VSRM  = "vsrm.dev.azure.com"
	Feeds = "feeds.dev.azure.com"
)

// Client talks to one organization.
type Client struct {
	Org     string
	auth    string
	HTTP    *http.Client
	Log     *slog.Logger
	DryRun  bool
	MaxTry  int
	Metrics *metrics.Registry
	// Sleep waits between retries; tests replace it.
	Sleep func(context.Context, time.Duration) error
	// Rewrite, when set, rewrites every outgoing URL (tests point it at a fake server).
	Rewrite func(*url.URL)
}

// New returns a client for org.
func New(org, pat string, log *slog.Logger, dry bool) *Client {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.ResponseHeaderTimeout = 2 * time.Minute
	tr.MaxIdleConnsPerHost = 8
	return &Client{
		Org:    org,
		auth:   "Basic " + base64.StdEncoding.EncodeToString([]byte(":"+pat)),
		HTTP:   &http.Client{Transport: tr},
		Log:    log,
		DryRun: dry,
		MaxTry: 8,
		Sleep:  sleepCtx,
	}
}

// AuthHeader is the Authorization header value (used for git over HTTPS).
func (c *Client) AuthHeader() string { return c.auth }

// URL builds https://{host}/{org}/{path}. An empty host means dev.azure.com.
func (c *Client) URL(host, path string) string {
	if host == "" {
		host = Core
	}
	return fmt.Sprintf("https://%s/%s/%s", host, c.Org, path)
}

// P escapes a project, team or other name for use as one path segment.
func P(s string) string { return url.PathEscape(s) }

// HTTPError is a non-2xx response.
type HTTPError struct {
	Method, URL string
	Status      int
	Body        string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("%s %s: %d %s", e.Method, e.URL, e.Status, e.Body)
}

// StatusOf returns the HTTP status inside err, or 0.
func StatusOf(err error) int {
	var he *HTTPError
	if errors.As(err, &he) {
		return he.Status
	}
	return 0
}

// IsNotFound reports a 404.
func IsNotFound(err error) bool { return StatusOf(err) == http.StatusNotFound }

// Option changes an outgoing request.
type Option func(*http.Request)

// WithHeader sets a request header.
func WithHeader(k, v string) Option { return func(r *http.Request) { r.Header.Set(k, v) } }

// readOnlyPost lists POST endpoints that only read, so they run in dry-run mode.
func readOnlyPost(u string) bool {
	for _, p := range []string{"/_apis/wit/wiql", "/_apis/wit/workitemsbatch", "/_apis/graph/subjectlookup"} {
		if strings.Contains(u, p) {
			return true
		}
	}
	return false
}

// IsWrite reports whether a request would change data (and is skipped in dry-run).
func IsWrite(method, u string) bool {
	return method != http.MethodGet && method != http.MethodHead && !readOnlyPost(u)
}

// retryable decides whether to resend. Reads are retried on 429 and any 5xx.
// Writes only on 429 and 503, which mean the request wasn't processed; a 502
// or 504 can arrive after the server already created the object.
func retryable(method string, status int) bool {
	if method == http.MethodGet || method == http.MethodHead {
		return status == http.StatusTooManyRequests || status >= 500
	}
	return status == http.StatusTooManyRequests || status == http.StatusServiceUnavailable
}

// Do sends a request. body may be nil, []byte, an io.Reader or any JSON value;
// out may be nil, *[]byte or a pointer to decode JSON into.
func (c *Client) Do(ctx context.Context, method, u, ctype string, body any, out any, opts ...Option) (http.Header, error) {
	if IsWrite(method, u) && c.DryRun {
		c.Log.Info("dry-run", "method", method, "url", u)
		return nil, nil
	}
	var raw []byte
	switch b := body.(type) {
	case nil:
	case []byte:
		raw = b
	case io.Reader:
		var err error
		if raw, err = io.ReadAll(b); err != nil {
			return nil, err
		}
	default:
		var err error
		if raw, err = json.Marshal(b); err != nil {
			return nil, err
		}
	}
	if ctype == "" {
		ctype = "application/json"
	}
	var lastErr error
	for attempt := 0; attempt < c.MaxTry; attempt++ {
		req, err := c.newRequest(ctx, method, u, raw, ctype, opts)
		if err != nil {
			return nil, err
		}
		resp, err := c.HTTP.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			lastErr = err
			c.Log.Warn("retry", "method", method, "url", u, "err", err, "attempt", attempt)
			if err := c.wait(ctx, backoff(attempt)); err != nil {
				return nil, err
			}
			continue
		}
		data, rerr := io.ReadAll(resp.Body)
		resp.Body.Close()
		c.Metrics.Inc("adoclone_requests_total", "method", method, "code", strconv.Itoa(resp.StatusCode))
		if rerr != nil {
			lastErr = rerr
			if err := c.wait(ctx, backoff(attempt)); err != nil {
				return nil, err
			}
			continue
		}
		if err := c.throttle(ctx, resp.Header); err != nil {
			return nil, err
		}
		ra := retryAfter(resp.Header)
		if retryable(method, resp.StatusCode) {
			lastErr = &HTTPError{method, u, resp.StatusCode, trunc(data)}
			c.Log.Warn("retry", "method", method, "url", u, "status", resp.StatusCode, "retryAfter", ra, "attempt", attempt)
			d := ra
			if d == 0 {
				d = backoff(attempt)
			}
			if err := c.wait(ctx, d); err != nil {
				return nil, err
			}
			continue
		}
		if ra > 0 { // soft throttle on a successful response
			if err := c.wait(ctx, ra); err != nil {
				return nil, err
			}
		}
		if resp.StatusCode >= 300 {
			c.Metrics.Inc("adoclone_errors_total")
			return resp.Header, &HTTPError{method, u, resp.StatusCode, trunc(data)}
		}
		if out != nil && len(data) > 0 {
			if b, ok := out.(*[]byte); ok {
				*b = data
			} else if err := json.Unmarshal(data, out); err != nil {
				return resp.Header, fmt.Errorf("%s %s: decode: %w", method, u, err)
			}
		}
		return resp.Header, nil
	}
	c.Metrics.Inc("adoclone_errors_total")
	return nil, lastErr
}

func (c *Client) newRequest(ctx context.Context, method, u string, raw []byte, ctype string, opts []Option) (*http.Request, error) {
	var rd io.Reader
	if raw != nil {
		rd = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, rd)
	if err != nil {
		return nil, err
	}
	if c.Rewrite != nil {
		c.Rewrite(req.URL)
		req.Host = req.URL.Host
	}
	req.Header.Set("Authorization", c.auth)
	req.Header.Set("Accept", "application/json")
	if raw != nil {
		req.Header.Set("Content-Type", ctype)
	}
	for _, o := range opts {
		o(req)
	}
	return req, nil
}

// Get, Post, Put, Patch and Delete are JSON shorthands for Do.
func (c *Client) Get(ctx context.Context, u string, out any) error {
	_, err := c.Do(ctx, http.MethodGet, u, "", nil, out)
	return err
}

func (c *Client) Post(ctx context.Context, u string, body, out any) error {
	_, err := c.Do(ctx, http.MethodPost, u, "", body, out)
	return err
}

func (c *Client) Put(ctx context.Context, u string, body, out any) error {
	_, err := c.Do(ctx, http.MethodPut, u, "", body, out)
	return err
}

func (c *Client) Patch(ctx context.Context, u string, body, out any) error {
	_, err := c.Do(ctx, http.MethodPatch, u, "", body, out)
	return err
}

func (c *Client) Delete(ctx context.Context, u string, out any) error {
	_, err := c.Do(ctx, http.MethodDelete, u, "", nil, out)
	return err
}

// Download streams a GET response body into w (used for attachments). It
// retries only before any bytes are written.
func (c *Client) Download(ctx context.Context, u string, w io.Writer) (int64, error) {
	var lastErr error
	for attempt := 0; attempt < c.MaxTry; attempt++ {
		req, err := c.newRequest(ctx, http.MethodGet, u, nil, "", []Option{WithHeader("Accept", "application/octet-stream")})
		if err != nil {
			return 0, err
		}
		resp, err := c.HTTP.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return 0, ctx.Err()
			}
			lastErr = err
			if err := c.wait(ctx, backoff(attempt)); err != nil {
				return 0, err
			}
			continue
		}
		c.Metrics.Inc("adoclone_requests_total", "method", "GET", "code", strconv.Itoa(resp.StatusCode))
		if retryable(http.MethodGet, resp.StatusCode) {
			data, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			lastErr = &HTTPError{"GET", u, resp.StatusCode, trunc(data)}
			d := retryAfter(resp.Header)
			if d == 0 {
				d = backoff(attempt)
			}
			if err := c.wait(ctx, d); err != nil {
				return 0, err
			}
			continue
		}
		if resp.StatusCode >= 300 {
			data, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			c.Metrics.Inc("adoclone_errors_total")
			return 0, &HTTPError{"GET", u, resp.StatusCode, trunc(data)}
		}
		n, err := io.Copy(w, resp.Body)
		resp.Body.Close()
		return n, err
	}
	c.Metrics.Inc("adoclone_errors_total")
	return 0, lastErr
}

// List returns every item of a {count, value} list API, following continuation
// tokens from the x-ms-continuationtoken header or a continuationToken field.
func (c *Client) List(ctx context.Context, u string) ([]json.RawMessage, error) {
	var all []json.RawMessage
	token, seen := "", map[string]bool{}
	for {
		pu := u
		if token != "" {
			sep := "?"
			if strings.Contains(pu, "?") {
				sep = "&"
			}
			pu += sep + "continuationToken=" + url.QueryEscape(token)
		}
		var page struct {
			Value             []json.RawMessage `json:"value"`
			ContinuationToken string            `json:"continuationToken"`
		}
		h, err := c.Do(ctx, http.MethodGet, pu, "", nil, &page)
		if err != nil {
			return nil, err
		}
		all = append(all, page.Value...)
		token = h.Get("X-Ms-Continuationtoken")
		if token == "" {
			token = page.ContinuationToken
		}
		if token == "" || seen[token] || len(page.Value) == 0 {
			return all, nil
		}
		seen[token] = true
	}
}

// EscapeWIQL escapes a string literal for WIQL.
func EscapeWIQL(s string) string { return strings.ReplaceAll(s, "'", "''") }

// WorkItemIDs returns every work item ID in project, paging by ID so no single
// WIQL query goes past the 20,000-result cap. extra is an optional "AND ..." clause.
func (c *Client) WorkItemIDs(ctx context.Context, project, extra string) ([]int, error) {
	var ids []int
	last := 0
	for {
		q := fmt.Sprintf("SELECT [System.Id] FROM WorkItems WHERE [System.TeamProject] = '%s' AND [System.Id] > %d %s ORDER BY [System.Id]",
			EscapeWIQL(project), last, extra)
		var r struct {
			WorkItems []struct {
				ID int `json:"id"`
			} `json:"workItems"`
		}
		u := c.URL("", P(project)+"/_apis/wit/wiql?$top=19000&api-version=7.1")
		if err := c.Post(ctx, u, map[string]string{"query": q}, &r); err != nil {
			return nil, err
		}
		if len(r.WorkItems) == 0 {
			return ids, nil
		}
		for _, w := range r.WorkItems {
			ids = append(ids, w.ID)
		}
		last = r.WorkItems[len(r.WorkItems)-1].ID
	}
}

// throttle slows down when the rate-limit headers say the budget is nearly spent.
func (c *Client) throttle(ctx context.Context, h http.Header) error {
	rem, err1 := strconv.ParseFloat(h.Get("X-RateLimit-Remaining"), 64)
	lim, err2 := strconv.ParseFloat(h.Get("X-RateLimit-Limit"), 64)
	if err1 != nil {
		return nil
	}
	if (err2 == nil && lim > 0 && rem/lim < 0.1) || rem < 10 {
		return c.wait(ctx, 2*time.Second)
	}
	return nil
}

func (c *Client) wait(ctx context.Context, d time.Duration) error {
	c.Metrics.Add("adoclone_throttle_seconds_total", d.Seconds())
	return c.Sleep(ctx, d)
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

func retryAfter(h http.Header) time.Duration {
	if v := h.Get("Retry-After"); v != "" {
		if s, err := strconv.Atoi(v); err == nil {
			return time.Duration(s) * time.Second
		}
	}
	return 0
}

func backoff(attempt int) time.Duration {
	d := time.Duration(1<<attempt) * time.Second
	if d > 60*time.Second {
		d = 60 * time.Second
	}
	return d + time.Duration(rand.Intn(1000))*time.Millisecond
}

func trunc(b []byte) string {
	if len(b) > 400 {
		return string(b[:400])
	}
	return string(b)
}
