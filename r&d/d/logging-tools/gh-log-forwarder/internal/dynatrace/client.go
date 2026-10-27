// Package dynatrace ships log records to the Log Monitoring API v2,
// POST /api/v2/logs/ingest, honouring its documented limits and
// response codes. Standard library only.
package dynatrace

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// IngestPath is the Log Monitoring API v2 ingest endpoint.
const IngestPath = "/api/v2/logs/ingest"

var (
	// ErrAuth is a 401/403: wrong token or missing logs.ingest scope.
	ErrAuth = errors.New("dynatrace: authentication or scope failure")
	// ErrBudget is a 402: DPS/licence exhausted. Do not retry.
	ErrBudget = errors.New("dynatrace: 402 payment required (DPS budget)")
	// ErrTooLarge is a 413: the caller should split the batch.
	ErrTooLarge = errors.New("dynatrace: 413 payload too large")
)

// RejectedError is a 400 that is not the all-stale case, or any other
// non-retryable status.
type RejectedError struct {
	Code    int
	Message string
}

func (e *RejectedError) Error() string {
	return fmt.Sprintf("dynatrace: %d %s", e.Code, e.Message)
}

// Outcome classifies a successful (non-error) response.
type Outcome int

const (
	Accepted Outcome = iota // 204
	Partial                 // 200: some records invalid or out of range
	Stale                   // 400 "All logs are out of correct time range."
)

// Response is what Post learned.
type Response struct {
	Outcome Outcome
	Code    int
	Message string
	Retries int
}

// Poster is satisfied by *Client; tests substitute their own.
type Poster interface {
	Post(ctx context.Context, payload []byte, headerAttrs string) (Response, error)
}

// Client posts to one Dynatrace environment (SaaS or Environment
// ActiveGate: .../e/{env-id}).
type Client struct {
	URL        string // https://abc12345.live.dynatrace.com
	Token      string
	Scheme     string // "Api-Token" (classic, scope logs.ingest) or "Bearer" (platform token)
	Gzip       bool   // UNVERIFIED for this endpoint; off by default
	HTTP       *http.Client
	MaxRetries int
	Log        *slog.Logger
	Sleep      func(context.Context, time.Duration) error
}

// NewClient returns a client with defaults.
func NewClient(url, token, scheme string, timeout time.Duration) *Client {
	return &Client{
		URL:        strings.TrimRight(url, "/"),
		Token:      token,
		Scheme:     scheme,
		HTTP:       &http.Client{Timeout: timeout},
		MaxRetries: 5,
		Log:        slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

// Post sends one JSON array. 429/502/503/504 and transport errors are
// retried with exponential backoff and jitter, as Dynatrace documents;
// a Retry-After header is honoured if present (Dynatrace does not
// document one).
func (c *Client) Post(ctx context.Context, payload []byte, headerAttrs string) (Response, error) {
	body := payload
	if c.Gzip {
		var z bytes.Buffer
		zw := gzip.NewWriter(&z)
		if _, err := zw.Write(payload); err != nil {
			return Response{}, err
		}
		if err := zw.Close(); err != nil {
			return Response{}, err
		}
		body = z.Bytes()
	}
	var resp Response
	for attempt := 0; ; attempt++ {
		resp.Retries = attempt
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL+IngestPath, bytes.NewReader(body))
		if err != nil {
			return resp, err
		}
		req.Header.Set("Content-Type", "application/json; charset=utf-8")
		req.Header.Set("Accept", "application/json")
		req.Header.Set("Authorization", c.Scheme+" "+c.Token)
		req.Header.Set("User-Agent", "ghlog2dt")
		if c.Gzip {
			req.Header.Set("Content-Encoding", "gzip")
		}
		if headerAttrs != "" {
			req.Header.Set("X-Dynatrace-Attr", headerAttrs)
		}

		hr, err := c.HTTP.Do(req)
		var wait time.Duration
		if err != nil {
			if ctx.Err() != nil {
				return resp, ctx.Err()
			}
			if attempt >= c.MaxRetries {
				return resp, fmt.Errorf("dynatrace: %w", err)
			}
			wait = backoff(attempt)
			c.Log.Warn("dynatrace transport error, retrying", "attempt", attempt+1, "wait", wait, "err", err)
		} else {
			msg, _ := io.ReadAll(io.LimitReader(hr.Body, 64<<10))
			hr.Body.Close()
			resp.Code, resp.Message = hr.StatusCode, strings.TrimSpace(string(msg))
			switch hr.StatusCode {
			case http.StatusNoContent:
				resp.Outcome = Accepted
				return resp, nil
			case http.StatusOK:
				resp.Outcome = Partial
				return resp, nil
			case http.StatusBadRequest:
				if strings.Contains(resp.Message, "out of correct time range") {
					resp.Outcome = Stale
					return resp, nil
				}
				return resp, &RejectedError{Code: hr.StatusCode, Message: clip(resp.Message)}
			case http.StatusUnauthorized, http.StatusForbidden:
				return resp, fmt.Errorf("%w: %d %s", ErrAuth, hr.StatusCode, clip(resp.Message))
			case http.StatusPaymentRequired:
				return resp, ErrBudget
			case http.StatusRequestEntityTooLarge:
				return resp, ErrTooLarge
			case http.StatusTooManyRequests, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
				if attempt >= c.MaxRetries {
					return resp, fmt.Errorf("dynatrace: %d after %d retries", hr.StatusCode, attempt)
				}
				wait = backoff(attempt)
				if s, perr := strconv.Atoi(hr.Header.Get("Retry-After")); perr == nil && s > 0 && s <= 300 {
					wait = time.Duration(s) * time.Second
				}
				c.Log.Warn("dynatrace throttled, retrying", "status", hr.StatusCode, "attempt", attempt+1, "wait", wait)
			default:
				return resp, &RejectedError{Code: hr.StatusCode, Message: clip(resp.Message)}
			}
		}
		if err := c.sleep(ctx, wait); err != nil {
			return resp, err
		}
	}
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

func backoff(attempt int) time.Duration {
	return time.Duration(1<<min(attempt, 6))*time.Second + time.Duration(rand.Int64N(int64(time.Second)))
}

func clip(s string) string {
	if len(s) > 300 {
		return s[:300] + "…"
	}
	return s
}
