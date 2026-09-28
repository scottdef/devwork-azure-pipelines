package main

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
	"sync"
	"time"
)

// ---- credentials ------------------------------------------------------------

// credential returns request headers for the data plane.
type credential interface {
	Header(ctx context.Context) (string, string, error)
}

// workloadIdentity exchanges the projected Kubernetes service-account token
// (injected by the Azure Workload Identity webhook) for a Microsoft Entra
// access token with the client-credentials flow and a federated assertion.
//
// Environment injected by the webhook: AZURE_CLIENT_ID, AZURE_TENANT_ID,
// AZURE_FEDERATED_TOKEN_FILE, AZURE_AUTHORITY_HOST.
type workloadIdentity struct {
	clientID, tenantID, tokenFile, authority string
	http                                     *http.Client
	mu                                       sync.Mutex
	token                                    string
	expires                                  time.Time
}

const cognitiveScope = "https://cognitiveservices.azure.com/.default"

func newCredential(log *logger) (credential, error) {
	if f := os.Getenv("AZURE_FEDERATED_TOKEN_FILE"); f != "" {
		wi := &workloadIdentity{clientID: os.Getenv("AZURE_CLIENT_ID"), tenantID: os.Getenv("AZURE_TENANT_ID"), tokenFile: f,
			authority: envOr("AZURE_AUTHORITY_HOST", "https://login.microsoftonline.com/"), http: &http.Client{Timeout: 30 * time.Second}}
		if wi.clientID == "" || wi.tenantID == "" {
			return nil, errors.New("workload identity: AZURE_CLIENT_ID and AZURE_TENANT_ID must be set (is the pod labeled azure.workload.identity/use=true and the service account annotated?)")
		}
		log.info("using Azure Workload Identity", map[string]any{"client_id": wi.clientID})
		return wi, nil
	}
	if k := os.Getenv("AZURE_OPENAI_API_KEY"); k != "" {
		// Local development only; production pods never receive keys.
		log.info("using API key authentication (local development only)", nil)
		return apiKey(k), nil
	}
	return nil, errors.New("no credentials: expected Azure Workload Identity (AZURE_FEDERATED_TOKEN_FILE) or AZURE_OPENAI_API_KEY for local runs")
}

type apiKey string

func (k apiKey) Header(context.Context) (string, string, error) { return "api-key", string(k), nil }

func (w *workloadIdentity) Header(ctx context.Context) (string, string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.token != "" && time.Until(w.expires) > 2*time.Minute {
		return "Authorization", "Bearer " + w.token, nil
	}
	assertion, err := os.ReadFile(w.tokenFile) // re-read: the kubelet rotates it
	if err != nil {
		return "", "", fmt.Errorf("workload identity: read federated token: %w", err)
	}
	form := url.Values{
		"client_id":             {w.clientID},
		"scope":                 {cognitiveScope},
		"grant_type":            {"client_credentials"},
		"client_assertion_type": {"urn:ietf:params:oauth:client-assertion-type:jwt-bearer"},
		"client_assertion":      {strings.TrimSpace(string(assertion))},
	}
	u := strings.TrimRight(w.authority, "/") + "/" + url.PathEscape(w.tenantID) + "/oauth2/v2.0/token"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, strings.NewReader(form.Encode()))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := w.http.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("workload identity: token request: %w", err)
	}
	defer resp.Body.Close()
	var tr struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		Error       string `json:"error"`
		Description string `json:"error_description"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&tr); err != nil {
		return "", "", fmt.Errorf("workload identity: decode token response (HTTP %d): %w", resp.StatusCode, err)
	}
	if resp.StatusCode != http.StatusOK || tr.AccessToken == "" {
		return "", "", fmt.Errorf("workload identity: HTTP %d %s: %s", resp.StatusCode, tr.Error, firstLine(tr.Description))
	}
	w.token, w.expires = tr.AccessToken, time.Now().Add(time.Duration(tr.ExpiresIn)*time.Second)
	return "Authorization", "Bearer " + w.token, nil
}

func firstLine(s string) string {
	l, _, _ := strings.Cut(s, "\n")
	return l
}

// ---- chat completions -----------------------------------------------------------

type foundryClient struct {
	endpoint, deployment, apiVersion string
	maxTokens                        int
	cred                             credential
	log                              *logger
	http                             *http.Client
}

// Chat calls the Azure OpenAI chat-completions data plane of a Foundry
// resource: POST {endpoint}/openai/deployments/{deployment}/chat/completions.
// JSON mode keeps replies machine-readable; temperature is omitted so the same
// code works with reasoning models (o-series) that reject it.
func (f *foundryClient) Chat(ctx context.Context, msgs []message) (string, usage, error) {
	if f.http == nil {
		f.http = &http.Client{Timeout: 5 * time.Minute}
	}
	body, _ := json.Marshal(map[string]any{
		"messages":              msgs,
		"max_completion_tokens": f.maxTokens,
		"response_format":       map[string]string{"type": "json_object"},
	})
	u := fmt.Sprintf("%s/openai/deployments/%s/chat/completions?api-version=%s", f.endpoint, url.PathEscape(f.deployment), url.QueryEscape(f.apiVersion))
	var lastErr error
	for attempt := 0; attempt < 4; attempt++ {
		if attempt > 0 {
			wait := time.Duration(1<<attempt) * time.Second
			select {
			case <-ctx.Done():
				return "", usage{}, ctx.Err()
			case <-time.After(wait):
			}
		}
		k, v, err := f.cred.Header(ctx)
		if err != nil {
			return "", usage{}, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
		if err != nil {
			return "", usage{}, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(k, v)
		resp, err := f.http.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)
			f.log.info("retrying model call", map[string]any{"status": resp.StatusCode, "attempt": attempt + 1})
			continue
		}
		if resp.StatusCode != http.StatusOK {
			return "", usage{}, fmt.Errorf("chat completions: HTTP %d: %s", resp.StatusCode, firstLine(string(raw)))
		}
		var cr struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
			Usage usage `json:"usage"`
		}
		if err := json.Unmarshal(raw, &cr); err != nil {
			return "", usage{}, fmt.Errorf("chat completions: decode: %w", err)
		}
		if len(cr.Choices) == 0 {
			return "", cr.Usage, errors.New("chat completions: no choices (content filtered?)")
		}
		if cr.Choices[0].FinishReason == "content_filter" {
			return "", cr.Usage, errors.New("chat completions: response blocked by the content filter")
		}
		return cr.Choices[0].Message.Content, cr.Usage, nil
	}
	return "", usage{}, fmt.Errorf("chat completions: giving up after retries: %v", lastErr)
}

// ---- replay (tests and local dry runs) -----------------------------------------

type replayChatter struct {
	replies []string
	i       int
}

func newReplay(path string) (*replayChatter, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var raw []json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("replay file: %w", err)
	}
	r := &replayChatter{}
	for _, m := range raw {
		// Each entry is either a JSON string (the reply text) or an object
		// (serialized as the reply).
		var s string
		if json.Unmarshal(m, &s) == nil {
			r.replies = append(r.replies, s)
		} else {
			r.replies = append(r.replies, string(m))
		}
	}
	return r, nil
}

func (r *replayChatter) Chat(context.Context, []message) (string, usage, error) {
	if r.i >= len(r.replies) {
		return "", usage{}, errors.New("replay: no more responses")
	}
	s := r.replies[r.i]
	r.i++
	return s, usage{PromptTokens: 1000, CompletionTokens: len(s) / 4}, nil
}

// ---- logging ------------------------------------------------------------------------

type logger struct {
	w  io.Writer
	mu sync.Mutex
}

func newLogger(w io.Writer) *logger { return &logger{w: w} }

func (l *logger) info(msg string, fields map[string]any) {
	rec := map[string]any{"ts": time.Now().UTC().Format(time.RFC3339), "level": "info", "msg": msg}
	for k, v := range fields {
		rec[k] = v
	}
	b, _ := json.Marshal(rec)
	l.mu.Lock()
	defer l.mu.Unlock()
	fmt.Fprintln(l.w, string(b))
}
