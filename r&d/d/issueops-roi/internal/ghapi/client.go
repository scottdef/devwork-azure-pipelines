// Package ghapi is a small GitHub REST client built on net/http. It covers
// only the endpoints the IssueOps platform needs, handles pagination via the
// Link header, retries transient failures and rate limits, and never logs
// tokens.
package ghapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	// VersionDefault is the long-lived REST API version.
	VersionDefault = "2022-11-28"
	// VersionLatest is used for endpoints documented against the 2026 API
	// version (Copilot usage-metrics reports, billing budgets and usage).
	VersionLatest = "2026-03-10"
)

// Client talks to the GitHub REST API.
type Client struct {
	BaseURL    string
	Token      string
	HTTP       *http.Client
	UserAgent  string
	MaxRetries int
	// Sleep is replaceable in tests.
	Sleep func(time.Duration)
}

// New returns a client for GITHUB_API_URL (default https://api.github.com).
func New(token string) *Client {
	base := os.Getenv("GITHUB_API_URL")
	if base == "" {
		base = "https://api.github.com"
	}
	return &Client{
		BaseURL:    strings.TrimRight(base, "/"),
		Token:      token,
		HTTP:       &http.Client{Timeout: 60 * time.Second},
		UserAgent:  "CoolEngOrg-issueops/1.0",
		MaxRetries: 4,
		Sleep:      time.Sleep,
	}
}

// NewFromEnv reads the token from GH_TOKEN (then GITHUB_TOKEN).
func NewFromEnv() (*Client, error) {
	tok := os.Getenv("GH_TOKEN")
	if tok == "" {
		tok = os.Getenv("GITHUB_TOKEN")
	}
	if tok == "" {
		return nil, errors.New("ghapi: GH_TOKEN is not set")
	}
	return New(tok), nil
}

// APIError is a non-2xx response.
type APIError struct {
	Status  int
	Method  string
	Path    string
	Message string
	DocURL  string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("github %s %s: %d %s", e.Method, e.Path, e.Status, e.Message)
}

// IsStatus reports whether err is an APIError with the given status.
func IsStatus(err error, status int) bool {
	var ae *APIError
	return errors.As(err, &ae) && ae.Status == status
}

// Request describes one API call.
type Request struct {
	Method  string
	Path    string // path relative to BaseURL, or an absolute URL for pagination
	Query   url.Values
	Body    any
	Version string
	Accept  string
	// OK lists additional non-error statuses (e.g. 202, 204, 404 for lookups).
	OK []int
}

// Response is a decoded API response.
type Response struct {
	Status int
	Header http.Header
	Body   []byte
}

// Do executes a request with retries for 5xx, 429 and secondary rate limits.
func (c *Client) Do(ctx context.Context, r Request) (*Response, error) {
	if r.Method == "" {
		r.Method = http.MethodGet
	}
	u := r.Path
	if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
		u = c.BaseURL + "/" + strings.TrimLeft(r.Path, "/")
	}
	if len(r.Query) > 0 {
		sep := "?"
		if strings.Contains(u, "?") {
			sep = "&"
		}
		u += sep + r.Query.Encode()
	}
	var payload []byte
	if r.Body != nil {
		var err error
		if payload, err = json.Marshal(r.Body); err != nil {
			return nil, fmt.Errorf("ghapi: encode body: %w", err)
		}
	}
	version := r.Version
	if version == "" {
		version = VersionDefault
	}
	accept := r.Accept
	if accept == "" {
		accept = "application/vnd.github+json"
	}
	var lastErr error
	for attempt := 0; attempt <= c.MaxRetries; attempt++ {
		req, err := http.NewRequestWithContext(ctx, r.Method, u, bytes.NewReader(payload))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", accept)
		req.Header.Set("X-GitHub-Api-Version", version)
		req.Header.Set("User-Agent", c.UserAgent)
		if c.Token != "" {
			req.Header.Set("Authorization", "Bearer "+c.Token)
		}
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := c.HTTP.Do(req)
		if err != nil {
			lastErr = err
			c.backoff(attempt, 0)
			continue
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
		resp.Body.Close()
		if err != nil {
			lastErr = err
			c.backoff(attempt, 0)
			continue
		}
		out := &Response{Status: resp.StatusCode, Header: resp.Header, Body: body}
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return out, nil
		}
		for _, s := range r.OK {
			if s == resp.StatusCode {
				return out, nil
			}
		}
		apiErr := &APIError{Status: resp.StatusCode, Method: r.Method, Path: redactPath(r.Path)}
		var m struct {
			Message string `json:"message"`
			DocURL  string `json:"documentation_url"`
		}
		if json.Unmarshal(body, &m) == nil {
			apiErr.Message, apiErr.DocURL = m.Message, m.DocURL
		}
		if retryable(resp) && attempt < c.MaxRetries {
			lastErr = apiErr
			c.backoff(attempt, retryAfter(resp))
			continue
		}
		return out, apiErr
	}
	return nil, fmt.Errorf("ghapi: %s %s: giving up after retries: %w", r.Method, redactPath(r.Path), lastErr)
}

func redactPath(p string) string {
	if i := strings.Index(p, "?"); i >= 0 {
		return p[:i]
	}
	return p
}

func retryable(resp *http.Response) bool {
	if resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests {
		return true
	}
	// Secondary rate limits return 403 with Retry-After or remaining=0.
	if resp.StatusCode == http.StatusForbidden {
		if resp.Header.Get("Retry-After") != "" || resp.Header.Get("X-RateLimit-Remaining") == "0" {
			return true
		}
	}
	return false
}

func retryAfter(resp *http.Response) time.Duration {
	if s := resp.Header.Get("Retry-After"); s != "" {
		if n, err := strconv.Atoi(s); err == nil {
			return time.Duration(n) * time.Second
		}
	}
	if resp.Header.Get("X-RateLimit-Remaining") == "0" {
		if reset, err := strconv.ParseInt(resp.Header.Get("X-RateLimit-Reset"), 10, 64); err == nil {
			d := time.Until(time.Unix(reset, 0)) + time.Second
			if d > 0 && d < 15*time.Minute {
				return d
			}
		}
	}
	return 0
}

func (c *Client) backoff(attempt int, hint time.Duration) {
	d := hint
	if d == 0 {
		d = time.Duration(1<<attempt) * time.Second
	}
	if d > 2*time.Minute {
		d = 2 * time.Minute
	}
	if c.Sleep != nil {
		c.Sleep(d)
	}
}

// JSON executes a request and decodes the body into out (if non-nil).
func (c *Client) JSON(ctx context.Context, r Request, out any) (*Response, error) {
	resp, err := c.Do(ctx, r)
	if err != nil {
		return resp, err
	}
	if out != nil && len(resp.Body) > 0 && resp.Status != http.StatusNoContent {
		if err := json.Unmarshal(resp.Body, out); err != nil {
			return resp, fmt.Errorf("ghapi: decode %s: %w", redactPath(r.Path), err)
		}
	}
	return resp, nil
}

var linkNext = regexp.MustCompile(`<([^>]+)>;\s*rel="next"`)
var linkLast = regexp.MustCompile(`<([^>]+)>;\s*rel="last"`)

// NextLink returns the rel="next" URL from a Link header.
func NextLink(h http.Header) string {
	if m := linkNext.FindStringSubmatch(h.Get("Link")); m != nil {
		return m[1]
	}
	return ""
}

// LastPage returns the page number of rel="last" (0 if absent).
func LastPage(h http.Header) int {
	m := linkLast.FindStringSubmatch(h.Get("Link"))
	if m == nil {
		return 0
	}
	u, err := url.Parse(m[1])
	if err != nil {
		return 0
	}
	n, _ := strconv.Atoi(u.Query().Get("page"))
	return n
}

// PaginateArray walks an endpoint returning a JSON array, invoking fn per page.
func (c *Client) PaginateArray(ctx context.Context, r Request, fn func([]json.RawMessage) error) error {
	if r.Query == nil {
		r.Query = url.Values{}
	}
	if r.Query.Get("per_page") == "" {
		r.Query.Set("per_page", "100")
	}
	for page := 0; page < 1000; page++ {
		resp, err := c.Do(ctx, r)
		if err != nil {
			return err
		}
		var items []json.RawMessage
		if err := json.Unmarshal(resp.Body, &items); err != nil {
			return fmt.Errorf("ghapi: decode page of %s: %w", redactPath(r.Path), err)
		}
		if err := fn(items); err != nil {
			return err
		}
		next := NextLink(resp.Header)
		if next == "" {
			return nil
		}
		r.Path, r.Query = next, nil
	}
	return errors.New("ghapi: pagination limit reached")
}
