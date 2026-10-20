// Package dispatch builds repository_dispatch payloads and sends them.
//
// GitHub limits client_payload to 10 top-level keys and the whole body to
// 64 KiB; we keep the user payload to 32 KiB so the envelope always fits.
// Payload values that are themselves JSON travel as a string with its
// sha256 next to it, so the receiver can check nothing was reshaped.
package dispatch

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// MaxKeys is GitHub's limit on top-level client_payload keys.
const MaxKeys = 10

// MaxPayload is our limit on the embedded user payload, in bytes.
const MaxPayload = 32 << 10

// Events this package knows how to build.
const (
	EventPublish = "library-publish"
	EventBorrow  = "library-borrow"
	EventReturn  = "library-return"
)

// Meta is what the workflow knows about the issue, outside its body.
type Meta struct {
	Issue     int    // issue number in the intake repo
	Repo      string // owner/name of the intake repo (where to report back)
	Requester string // issue author login; recorded, never used as a key
}

// Build turns a parsed issue (field id -> value) into a client_payload for
// event. It returns an error rather than silently dropping data.
func Build(event string, req map[string]any, m Meta) (map[string]any, error) {
	if m.Issue <= 0 {
		return nil, fmt.Errorf("dispatch: issue number must be positive")
	}
	if !strings.Contains(m.Repo, "/") {
		return nil, fmt.Errorf("dispatch: repo %q is not owner/name", m.Repo)
	}
	if m.Requester == "" {
		return nil, fmt.Errorf("dispatch: requester is empty")
	}
	p := map[string]any{"issue": m.Issue, "repo": m.Repo, "requester": m.Requester}
	switch event {
	case EventPublish:
		raw, _ := req["payload"].(string)
		var buf bytes.Buffer
		if err := json.Compact(&buf, []byte(raw)); err != nil {
			return nil, fmt.Errorf("dispatch: payload: %w", err)
		}
		if buf.Len() > MaxPayload {
			return nil, fmt.Errorf("dispatch: payload is %d bytes; limit %d", buf.Len(), MaxPayload)
		}
		sum := sha256.Sum256(buf.Bytes())
		p["source_id"] = str(req["source-id"])
		p["template_id"] = str(req["template-id"])
		p["phase"] = str(req["phase"])
		p["payload"] = buf.String()
		p["sha256"] = hex.EncodeToString(sum[:])
		for _, k := range []string{"source_id", "template_id", "phase"} {
			if p[k] == "" {
				return nil, fmt.Errorf("dispatch: %s is empty", k)
			}
		}
	case EventBorrow:
		p["item_id"] = str(req["item-id"])
		p["days"] = str(req["days"])
		p["reason"] = clip(str(req["reason"]), 1024)
		if p["item_id"] == "" || p["days"] == "" {
			return nil, fmt.Errorf("dispatch: item-id and days are required")
		}
	case EventReturn:
		p["item_id"] = str(req["item-id"])
		if p["item_id"] == "" {
			return nil, fmt.Errorf("dispatch: item-id is required")
		}
	default:
		return nil, fmt.Errorf("dispatch: unknown event %q", event)
	}
	if len(p) > MaxKeys {
		return nil, fmt.Errorf("dispatch: %d top-level keys; GitHub allows %d", len(p), MaxKeys)
	}
	return p, nil
}

func str(v any) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	// Do not cut a UTF-8 sequence in half.
	for n > 0 && s[n]&0xC0 == 0x80 {
		n--
	}
	return s[:n]
}

// Client posts dispatch events. The zero value is not usable; use New.
type Client struct {
	HTTP    *http.Client
	BaseURL string        // e.g. https://api.github.com
	Token   string        // installation token
	Gap     time.Duration // minimum spacing between write requests
	Retries int           // extra attempts after the first on 5xx / network errors
	Backoff time.Duration // first retry delay; doubles each time
	Sleep   func(time.Duration)
	Agent   string

	mu   sync.Mutex
	last time.Time
}

// New returns a Client with GitHub's recommended write pacing (1s).
func New(base, token string) *Client {
	return &Client{
		HTTP:    &http.Client{Timeout: 30 * time.Second},
		BaseURL: strings.TrimRight(base, "/"),
		Token:   token,
		Gap:     time.Second,
		Retries: 4,
		Backoff: 2 * time.Second,
		Sleep:   time.Sleep,
		Agent:   "issueops-intake-dispatch",
	}
}

// StatusError is a non-2xx response that was not retried away.
type StatusError struct {
	Code int
	Body string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("dispatch: HTTP %d: %s", e.Code, strings.TrimSpace(e.Body))
}

// pace waits until Gap has passed since the previous write.
func (c *Client) pace() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.last.IsZero() {
		if d := c.Gap - time.Since(c.last); d > 0 {
			c.Sleep(d)
		}
	}
	c.last = time.Now()
}

// Send POSTs {event_type, client_payload} to /repos/{repo}/dispatches.
// It retries 5xx, 429 and network errors with exponential backoff,
// honours Retry-After, and never sends two writes less than Gap apart.
func (c *Client) Send(ctx context.Context, repo, event string, payload map[string]any) error {
	if c.Token == "" {
		return errors.New("dispatch: no token (set GH_TOKEN)")
	}
	body, err := json.Marshal(map[string]any{"event_type": event, "client_payload": payload})
	if err != nil {
		return err
	}
	url := c.BaseURL + "/repos/" + repo + "/dispatches"
	delay := c.Backoff
	var lastErr error
	for attempt := 0; attempt <= c.Retries; attempt++ {
		if attempt > 0 {
			c.Sleep(delay)
			delay *= 2
		}
		c.pace()
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+c.Token)
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", c.Agent)
		resp, err := c.HTTP.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			lastErr = err
			continue
		}
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		switch {
		case resp.StatusCode/100 == 2:
			return nil
		case resp.StatusCode >= 500, resp.StatusCode == http.StatusTooManyRequests,
			resp.StatusCode == http.StatusForbidden && resp.Header.Get("Retry-After") != "":
			if s, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && s > 0 {
				delay = time.Duration(s) * time.Second
			}
			lastErr = &StatusError{resp.StatusCode, string(msg)}
		default:
			return &StatusError{resp.StatusCode, string(msg)}
		}
	}
	return fmt.Errorf("dispatch: gave up after %d attempts: %w", c.Retries+1, lastErr)
}
