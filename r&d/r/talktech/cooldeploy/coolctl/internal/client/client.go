// Package client calls cool-api. It is what coolctl uses in a workflow job.
package client

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
	"strings"
	"time"
)

// Client calls one cool-api.
type Client struct {
	BaseURL  string
	Audience string       // default cool-api
	HTTP     *http.Client // default: a client with a 60 second timeout
}

// Error is a non-2xx answer from cool-api.
type Error struct {
	Code    int
	Message string
}

func (e *Error) Error() string { return fmt.Sprintf("cool-api: %d: %s", e.Code, e.Message) }

// Token returns the bearer token for cool-api.
//
// COOL_TOKEN wins if set. Otherwise the token comes from GitHub: a job with
// "id-token: write" has ACTIONS_ID_TOKEN_REQUEST_URL and
// ACTIONS_ID_TOKEN_REQUEST_TOKEN in its environment, and trading them for an
// ID token is one GET.
func (c *Client) Token(ctx context.Context) (string, error) {
	if t := os.Getenv("COOL_TOKEN"); t != "" {
		return t, nil
	}
	reqURL, reqToken := os.Getenv("ACTIONS_ID_TOKEN_REQUEST_URL"), os.Getenv("ACTIONS_ID_TOKEN_REQUEST_TOKEN")
	if reqURL == "" {
		return "", errors.New("no token: set COOL_TOKEN, or run in a GitHub Actions job with id-token: write")
	}
	aud := c.Audience
	if aud == "" {
		aud = "cool-api"
	}
	sep := "?"
	if strings.Contains(reqURL, "?") {
		sep = "&"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL+sep+"audience="+url.QueryEscape(aud), nil)
	if err != nil {
		return "", err
	}
	if reqToken != "" {
		req.Header.Set("Authorization", "Bearer "+reqToken)
	}
	resp, err := c.http().Do(req)
	if err != nil {
		return "", fmt.Errorf("ID token request: %w", err)
	}
	defer resp.Body.Close()
	var out struct{ Value string }
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ID token request: %s", resp.Status)
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil || out.Value == "" {
		return "", fmt.Errorf("ID token request: no token in the reply: %v", err)
	}
	return out.Value, nil
}

func (c *Client) http() *http.Client {
	if c.HTTP != nil {
		return c.HTTP
	}
	return &http.Client{Timeout: 60 * time.Second}
}

// Do sends one request. Writes carry a token; reads do not need one.
// A conflict or a server error is retried three times, because cool-api may
// be mid-commit for another request on the same file.
func (c *Client) Do(ctx context.Context, method, path string, in, out any) error {
	var body []byte
	if in != nil {
		var err error
		if body, err = json.Marshal(in); err != nil {
			return err
		}
	}
	token := ""
	if method != http.MethodGet {
		var err error
		if token, err = c.Token(ctx); err != nil {
			return err
		}
	}
	var last error
	for attempt := 0; attempt < 4; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Duration(attempt) * 2 * time.Second):
			}
		}
		retry, err := c.once(ctx, method, path, token, body, out)
		if err == nil {
			return nil
		}
		if last = err; !retry {
			return err
		}
	}
	return last
}

func (c *Client) once(ctx context.Context, method, path, token string, body []byte, out any) (retry bool, err error) {
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimSuffix(c.BaseURL, "/")+path, bytes.NewReader(body))
	if err != nil {
		return false, err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http().Do(req)
	if err != nil {
		return true, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return true, err
	}
	if resp.StatusCode >= 300 {
		var e struct{ Error string }
		if json.Unmarshal(b, &e) != nil || e.Error == "" {
			e.Error = strings.TrimSpace(string(b))
		}
		// Retry a 409 only when another write got in first. A 409 about release
		// state, such as a rollback with no standby, will not change on a retry.
		stale := resp.StatusCode == http.StatusConflict && strings.Contains(e.Error, "changed since it was read")
		return stale || resp.StatusCode >= 500, &Error{resp.StatusCode, e.Error}
	}
	if out != nil {
		return false, json.Unmarshal(b, out)
	}
	return false, nil
}
