// Package ghapp authenticates as a GitHub App and stores files through the
// contents API. It is how cool-api commits to cool-gitops in production.
package ghapp

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/CoolEngOrg/coolctl/internal/gitops"
)

// App is a GitHub App installation that can mint installation tokens.
type App struct {
	ClientID     string
	Installation int64
	Key          *rsa.PrivateKey
	API          string       // default https://api.github.com
	HTTP         *http.Client // default http.DefaultClient

	mu      sync.Mutex
	token   string
	expires time.Time
}

// ParseKey reads an RSA private key in PKCS #1 or PKCS #8 PEM form.
func ParseKey(pemBytes []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.New("no PEM block in the private key")
	}
	if k, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return k, nil
	}
	k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("private key: %w", err)
	}
	rk, ok := k.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("private key is not RSA")
	}
	return rk, nil
}

func (a *App) api() string {
	if a.API == "" {
		return "https://api.github.com"
	}
	return strings.TrimSuffix(a.API, "/")
}

func (a *App) client() *http.Client {
	if a.HTTP == nil {
		return http.DefaultClient
	}
	return a.HTTP
}

// jwt signs the short-lived token that identifies the App itself.
func (a *App) jwt(now time.Time) (string, error) {
	enc := base64.RawURLEncoding.EncodeToString
	claims, _ := json.Marshal(map[string]any{
		"iat": now.Add(-time.Minute).Unix(), // allow for clock drift
		"exp": now.Add(9 * time.Minute).Unix(),
		"iss": a.ClientID,
	})
	signing := enc([]byte(`{"alg":"RS256","typ":"JWT"}`)) + "." + enc(claims)
	sum := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, a.Key, crypto.SHA256, sum[:])
	if err != nil {
		return "", err
	}
	return signing + "." + enc(sig), nil
}

// Token returns an installation token, minting a new one when the cached
// token has less than five minutes left.
func (a *App) Token(ctx context.Context) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.token != "" && time.Until(a.expires) > 5*time.Minute {
		return a.token, nil
	}
	jwt, err := a.jwt(time.Now())
	if err != nil {
		return "", err
	}
	u := a.api() + "/app/installations/" + strconv.FormatInt(a.Installation, 10) + "/access_tokens"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, nil)
	if err != nil {
		return "", err
	}
	var out struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	if _, err := a.do(req, jwt, &out); err != nil {
		return "", fmt.Errorf("installation token: %w", err)
	}
	a.token, a.expires = out.Token, out.ExpiresAt
	return a.token, nil
}

// do sends req with the given bearer token and decodes a JSON reply into out.
func (a *App) do(req *http.Request, bearer string, out any) (int, error) {
	req.Header.Set("Authorization", "Bearer "+bearer)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := a.client().Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return resp.StatusCode, err
	}
	if resp.StatusCode >= 300 {
		return resp.StatusCode, fmt.Errorf("%s %s: %s: %s", req.Method, req.URL.Path, resp.Status, bytes.TrimSpace(b))
	}
	if out != nil && len(b) > 0 {
		return resp.StatusCode, json.Unmarshal(b, out)
	}
	return resp.StatusCode, nil
}

// Store is a gitops.Store over one branch of one repository.
type Store struct {
	App    *App
	Owner  string
	Repo   string
	Branch string
}

func (s *Store) url(path string) string {
	parts := strings.Split(path, "/")
	for i, p := range parts {
		parts[i] = url.PathEscape(p)
	}
	return s.App.api() + "/repos/" + s.Owner + "/" + s.Repo + "/contents/" + strings.Join(parts, "/")
}

func (s *Store) call(ctx context.Context, method, path string, body, out any) (int, error) {
	token, err := s.App.Token(ctx)
	if err != nil {
		return 0, err
	}
	u := s.url(path)
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, err
		}
		r = bytes.NewReader(b)
	} else {
		u += "?ref=" + url.QueryEscape(s.Branch)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, r)
	if err != nil {
		return 0, err
	}
	return s.App.do(req, token, out)
}

// classify maps GitHub status codes onto the Store errors.
func classify(path string, code int, err error) error {
	switch code {
	case http.StatusNotFound:
		return fmt.Errorf("%s: %w", path, gitops.ErrNotFound)
	case http.StatusConflict, http.StatusUnprocessableEntity:
		return fmt.Errorf("%s: %w", path, gitops.ErrConflict)
	}
	return err
}

// Read implements gitops.Store. The rev is the blob SHA.
func (s *Store) Read(ctx context.Context, path string) ([]byte, string, error) {
	var f struct {
		Type, Encoding, Content, SHA string
	}
	if code, err := s.call(ctx, http.MethodGet, path, nil, &f); err != nil {
		return nil, "", classify(path, code, err)
	}
	if f.Type != "file" || f.Encoding != "base64" {
		return nil, "", fmt.Errorf("%s: not a file", path)
	}
	b, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(f.Content, "\n", ""))
	if err != nil {
		return nil, "", fmt.Errorf("%s: %w", path, err)
	}
	return b, f.SHA, nil
}

// Write implements gitops.Store. Each write is one commit.
func (s *Store) Write(ctx context.Context, path string, data []byte, rev, message string) error {
	body := map[string]string{
		"message": message,
		"content": base64.StdEncoding.EncodeToString(data),
		"branch":  s.Branch,
	}
	if rev != "" {
		body["sha"] = rev
	}
	code, err := s.call(ctx, http.MethodPut, path, body, nil)
	if err != nil {
		return classify(path, code, err)
	}
	return nil
}

// Delete implements gitops.Store.
func (s *Store) Delete(ctx context.Context, path, rev, message string) error {
	body := map[string]string{"message": message, "sha": rev, "branch": s.Branch}
	code, err := s.call(ctx, http.MethodDelete, path, body, nil)
	if err != nil {
		return classify(path, code, err)
	}
	return nil
}

// List implements gitops.Store.
func (s *Store) List(ctx context.Context, dir string) ([]string, error) {
	var entries []struct{ Name, Type string }
	if code, err := s.call(ctx, http.MethodGet, dir, nil, &entries); err != nil {
		return nil, classify(dir, code, err)
	}
	var names []string
	for _, e := range entries {
		if e.Type == "dir" {
			names = append(names, e.Name+"/")
		} else {
			names = append(names, e.Name)
		}
	}
	sort.Strings(names)
	return names, nil
}
