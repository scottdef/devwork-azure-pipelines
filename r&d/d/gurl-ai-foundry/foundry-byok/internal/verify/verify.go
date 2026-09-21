// Package verify checks a resolved deployment against the live Azure resource.
//
// It answers the two questions that cause almost every gh-aw "model not found"
// failure:
//
//  1. Is engine.model one of the IDs returned by GET {endpoint}/openai/v1/models?
//  2. Does the deployment name resolve on the chosen wire API?
//
// The second check sends a real (tiny) inference request, so it is opt-in.
package verify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/CoolGitOrg/foundry-byok/internal/foundry"
)

// Credentials authenticate to the Azure resource. BearerToken wins when both
// are set. Neither value is ever logged or included in an error.
type Credentials struct {
	APIKey      string
	BearerToken string
}

// Empty reports whether no credential is available.
func (c Credentials) Empty() bool { return c.APIKey == "" && c.BearerToken == "" }

// Options tunes Check.
type Options struct {
	// Client defaults to a client that never follows redirects, so a
	// credential header can never be replayed to another host.
	Client *http.Client
	// Probe sends one minimal inference request to the deployment.
	Probe bool
	// MaxRetries bounds retries on HTTP 429/5xx (default 2).
	MaxRetries int
	// MaxRetryWait caps a single Retry-After sleep (default 10s).
	MaxRetryWait time.Duration
}

// Status is the outcome of one check.
type Status string

const (
	StatusOK      Status = "ok"
	StatusFail    Status = "fail"
	StatusSkipped Status = "skipped"
)

// Finding is one check result.
type Finding struct {
	Check  string `json:"check"`
	Status Status `json:"status"`
	Detail string `json:"detail"`
	// Hint is an actionable next step for failures.
	Hint string `json:"hint,omitempty"`
}

// Result is everything Check learned about one deployment.
type Result struct {
	Environment string    `json:"environment,omitempty"`
	Resource    string    `json:"resource,omitempty"`
	Deployment  string    `json:"deployment"`
	ModelID     string    `json:"model_id"`
	Findings    []Finding `json:"findings"`
}

// OK reports whether no check failed.
func (r Result) OK() bool {
	for _, f := range r.Findings {
		if f.Status == StatusFail {
			return false
		}
	}
	return true
}

func noRedirectClient() *http.Client {
	return &http.Client{
		Timeout: 90 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

// Check runs the live checks for one deployment.
func Check(ctx context.Context, r foundry.Resolved, cred Credentials, opt Options) Result {
	if opt.Client == nil {
		opt.Client = noRedirectClient()
	}
	if opt.MaxRetries == 0 {
		opt.MaxRetries = 2
	}
	if opt.MaxRetryWait == 0 {
		opt.MaxRetryWait = 10 * time.Second
	}
	res := Result{
		Environment: r.Spec.Environment, Resource: r.Spec.Resource,
		Deployment: r.Spec.Deployment, ModelID: r.ModelID,
	}
	if cred.Empty() {
		res.Findings = append(res.Findings, Finding{
			Check: "credentials", Status: StatusFail,
			Detail: "no credential available",
			Hint:   "export AZURE_FOUNDRY_BEARER_TOKEN, or the variable named by api_key_secret",
		})
		return res
	}

	if r.Surface == foundry.SurfaceAnthropic {
		res.Findings = append(res.Findings, Finding{
			Check: "model-id-listed", Status: StatusSkipped,
			Detail: "the Anthropic surface has no models listing",
		})
	} else {
		res.Findings = append(res.Findings, checkModels(ctx, r, cred, opt))
	}

	if opt.Probe {
		res.Findings = append(res.Findings, probe(ctx, r, cred, opt))
	} else {
		res.Findings = append(res.Findings, Finding{
			Check: "deployment-probe", Status: StatusSkipped,
			Detail: "not requested (use --probe; sends one small billable request)",
		})
	}
	return res
}

type modelList struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

func checkModels(ctx context.Context, r foundry.Resolved, cred Credentials, opt Options) Finding {
	f := Finding{Check: "model-id-listed"}
	status, body, err := do(ctx, opt, cred, r.Surface, http.MethodGet, r.URLs.V1Models, nil)
	if err != nil {
		f.Status, f.Detail = StatusFail, err.Error()
		f.Hint = "check DNS, private endpoint routing and firewall rules for " + r.Host
		return f
	}
	if status != http.StatusOK {
		f.Status = StatusFail
		f.Detail = fmt.Sprintf("GET %s -> HTTP %d: %s", r.URLs.V1Models, status, snippet(body))
		f.Hint = hintForStatus(status)
		return f
	}
	var list modelList
	if err := json.Unmarshal(body, &list); err != nil {
		f.Status, f.Detail = StatusFail, "models response is not the expected JSON: "+err.Error()
		return f
	}

	var ids, near []string
	prefix := strings.ToLower(r.Spec.Model)
	for _, m := range list.Data {
		ids = append(ids, m.ID)
		if strings.HasPrefix(strings.ToLower(m.ID), prefix) {
			near = append(near, m.ID)
		}
	}
	sort.Strings(near)
	for _, id := range ids {
		if id == r.ModelID {
			f.Status = StatusOK
			f.Detail = fmt.Sprintf("%q is listed (%d models on the resource)", r.ModelID, len(ids))
			return f
		}
	}
	f.Status = StatusFail
	f.Detail = fmt.Sprintf("%q is not among the %d model IDs the resource lists", r.ModelID, len(ids))
	if len(near) > 0 {
		f.Hint = "closest IDs: " + strings.Join(near, ", ") + ". Set model_version (or model_id) so engine.model matches one of them"
	} else {
		f.Hint = "no listed ID starts with " + strconv.Quote(r.Spec.Model) + "; confirm the model name and that this is the right resource"
	}
	return f
}

func probe(ctx context.Context, r foundry.Resolved, cred Credentials, opt Options) Finding {
	f := Finding{Check: "deployment-probe"}
	var (
		target string
		body   any
	)
	user := []map[string]string{{"role": "user", "content": "ping"}}
	switch {
	case r.Surface == foundry.SurfaceAnthropic:
		target = r.URLs.AnthropicMessages
		body = map[string]any{"model": r.Spec.Deployment, "max_tokens": 16, "messages": user}
	case r.WireAPI == foundry.WireResponses:
		target = r.URLs.V1Responses
		// 16 is the API minimum; it keeps the probe cheap.
		body = map[string]any{"model": r.Spec.Deployment, "input": "ping", "max_output_tokens": 16}
	default:
		target = r.URLs.V1ChatCompletions
		// No token limit: the parameter name differs between model families
		// (max_tokens vs max_completion_tokens) and "ping" is answered briefly.
		body = map[string]any{"model": r.Spec.Deployment, "messages": user}
	}
	payload, err := json.Marshal(body)
	if err != nil {
		f.Status, f.Detail = StatusFail, err.Error()
		return f
	}

	status, resp, err := do(ctx, opt, cred, r.Surface, http.MethodPost, target, payload)
	if err != nil {
		f.Status, f.Detail = StatusFail, err.Error()
		return f
	}
	if status == http.StatusOK {
		f.Status = StatusOK
		f.Detail = fmt.Sprintf("POST %s with model=%q -> HTTP 200", target, r.Spec.Deployment)
		return f
	}
	f.Status = StatusFail
	f.Detail = fmt.Sprintf("POST %s with model=%q -> HTTP %d: %s", target, r.Spec.Deployment, status, snippet(resp))
	switch {
	case status == http.StatusNotFound:
		f.Hint = "deployment name not found on this resource; check spelling and that the deployment finished provisioning"
	case status == http.StatusBadRequest && r.Surface == foundry.SurfaceOpenAIV1:
		other := foundry.WireCompletions
		if r.WireAPI == foundry.WireCompletions {
			other = foundry.WireResponses
		}
		f.Hint = fmt.Sprintf("the deployment may not support the %s wire API; try wire_api=%s", r.WireAPI, other)
	default:
		f.Hint = hintForStatus(status)
	}
	return f
}

func hintForStatus(status int) string {
	switch status {
	case http.StatusUnauthorized:
		return "credential rejected: wrong key/token for this resource, or key auth is disabled (disableLocalAuth=true)"
	case http.StatusForbidden:
		return "authenticated but not allowed: the identity needs the Cognitive Services OpenAI User role, or the network ACL blocks this client"
	case http.StatusNotFound:
		return "endpoint path not found: confirm the resource supports the /openai/v1 API and the host kind is right"
	case http.StatusTooManyRequests:
		return "rate limited after retries; try again later or raise the deployment's capacity"
	}
	if status >= 300 && status < 400 {
		return "unexpected redirect; redirects are never followed so credentials cannot leak to another host"
	}
	return ""
}

// do performs one authenticated request with bounded retries on 429 and 5xx.
func do(ctx context.Context, opt Options, cred Credentials, surface foundry.APISurface, method, target string, payload []byte) (int, []byte, error) {
	var lastErr error
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(payload))
		if err != nil {
			return 0, nil, err
		}
		if payload != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		req.Header.Set("User-Agent", "foundry-byok-verify")
		switch {
		case cred.BearerToken != "":
			req.Header.Set("Authorization", "Bearer "+cred.BearerToken)
		case surface == foundry.SurfaceAnthropic:
			req.Header.Set("x-api-key", cred.APIKey)
		default:
			req.Header.Set("api-key", cred.APIKey)
		}
		if surface == foundry.SurfaceAnthropic {
			req.Header.Set("anthropic-version", "2023-06-01")
		}

		resp, err := opt.Client.Do(req)
		if err != nil {
			// url.Error includes the URL but never headers, so this is safe.
			lastErr = err
			if ctx.Err() != nil || attempt >= opt.MaxRetries {
				return 0, nil, lastErr
			}
			if !sleep(ctx, backoff(attempt, "", opt.MaxRetryWait)) {
				return 0, nil, ctx.Err()
			}
			continue
		}
		body, rerr := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		resp.Body.Close()
		if rerr != nil {
			return resp.StatusCode, nil, rerr
		}
		retryable := resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500
		if !retryable || attempt >= opt.MaxRetries {
			return resp.StatusCode, body, nil
		}
		if !sleep(ctx, backoff(attempt, resp.Header.Get("Retry-After"), opt.MaxRetryWait)) {
			return 0, nil, ctx.Err()
		}
	}
}

func backoff(attempt int, retryAfter string, max time.Duration) time.Duration {
	d := time.Duration(1<<uint(attempt)) * time.Second
	if secs, err := strconv.Atoi(strings.TrimSpace(retryAfter)); err == nil && secs >= 0 {
		d = time.Duration(secs) * time.Second
	}
	if d > max {
		d = max
	}
	return d
}

func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

// snippet returns a short, single-line excerpt of a response body.
func snippet(b []byte) string {
	s := strings.Join(strings.Fields(string(b)), " ")
	if len(s) > 300 {
		s = s[:300] + "…"
	}
	if s == "" {
		return "(empty body)"
	}
	return s
}

// ErrFailed is returned by callers that aggregate results.
var ErrFailed = errors.New("one or more live checks failed")
