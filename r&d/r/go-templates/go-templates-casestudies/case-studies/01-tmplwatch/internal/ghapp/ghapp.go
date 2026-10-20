// Package ghapp mints GitHub App installation tokens with nothing but the
// standard library: an RS256 JWT is a base64url header and payload signed
// with RSASSA-PKCS1-v1_5 over SHA-256.
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
	"strings"
	"sync"
	"time"
)

// ParseKey decodes a PEM RSA private key in PKCS#1 ("RSA PRIVATE KEY", what
// GitHub hands out) or PKCS#8 ("PRIVATE KEY") form.
func ParseKey(pemBytes []byte) (*rsa.PrivateKey, error) {
	blk, _ := pem.Decode(pemBytes)
	if blk == nil {
		return nil, errors.New("no PEM block in private key")
	}
	if k, err := x509.ParsePKCS1PrivateKey(blk.Bytes); err == nil {
		return k, nil
	}
	k, err := x509.ParsePKCS8PrivateKey(blk.Bytes)
	if err != nil {
		return nil, fmt.Errorf("private key: neither PKCS#1 nor PKCS#8: %w", err)
	}
	rk, ok := k.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("private key: %T is not RSA", k)
	}
	return rk, nil
}

// Claims are the JWT claims GitHub requires.
type Claims struct {
	IssuedAt  int64  `json:"iat"`
	ExpiresAt int64  `json:"exp"`
	Issuer    string `json:"iss"`
}

var b64 = base64.RawURLEncoding

// JWT returns an RS256 token for the app identified by clientID. iat is
// backdated 60s to absorb clock skew; exp is 9 minutes out, under GitHub's
// 10-minute ceiling.
func JWT(clientID string, key *rsa.PrivateKey, now time.Time) (string, error) {
	hdr, _ := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT"})
	pl, err := json.Marshal(Claims{
		IssuedAt:  now.Add(-60 * time.Second).Unix(),
		ExpiresAt: now.Add(9 * time.Minute).Unix(),
		Issuer:    clientID,
	})
	if err != nil {
		return "", err
	}
	signing := b64.EncodeToString(hdr) + "." + b64.EncodeToString(pl)
	sum := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		return "", err
	}
	return signing + "." + b64.EncodeToString(sig), nil
}

// Verify checks an RS256 token against pub and returns its claims. It
// exists for tests and for debugging a key pair; GitHub does the real check.
func Verify(tok string, pub *rsa.PublicKey) (Claims, error) {
	var c Claims
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		return c, errors.New("jwt: want 3 parts")
	}
	hdr, err := b64.DecodeString(parts[0])
	if err != nil {
		return c, fmt.Errorf("jwt header: %w", err)
	}
	var h struct{ Alg string }
	if err := json.Unmarshal(hdr, &h); err != nil || h.Alg != "RS256" {
		return c, fmt.Errorf("jwt: alg %q", h.Alg)
	}
	sig, err := b64.DecodeString(parts[2])
	if err != nil {
		return c, fmt.Errorf("jwt sig: %w", err)
	}
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, sum[:], sig); err != nil {
		return c, fmt.Errorf("jwt: %w", err)
	}
	pl, err := b64.DecodeString(parts[1])
	if err != nil {
		return c, fmt.Errorf("jwt payload: %w", err)
	}
	return c, json.Unmarshal(pl, &c)
}

// Minter caches an installation token for one repository and refreshes it
// five minutes before expiry. It is safe for concurrent use.
type Minter struct {
	API      string
	ClientID string
	Key      *rsa.PrivateKey
	Owner    string
	Repo     string
	HTTP     *http.Client
	Now      func() time.Time

	mu      sync.Mutex
	instID  int64
	token   string
	expires time.Time
}

// Token returns a valid installation token, minting one if needed. Its
// signature matches source.TokenFunc.
func (m *Minter) Token(ctx context.Context) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now()
	if m.token != "" && now.Before(m.expires.Add(-5*time.Minute)) {
		return m.token, nil
	}
	jwt, err := JWT(m.ClientID, m.Key, now)
	if err != nil {
		return "", err
	}
	if m.instID == 0 {
		var inst struct {
			ID int64 `json:"id"`
		}
		u := fmt.Sprintf("%s/repos/%s/%s/installation", m.api(), url.PathEscape(m.Owner), url.PathEscape(m.Repo))
		if err := m.do(ctx, http.MethodGet, u, jwt, nil, &inst); err != nil {
			return "", fmt.Errorf("find installation: %w", err)
		}
		m.instID = inst.ID
	}
	var at struct {
		Token     string    `json:"token"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	// Scope the token to the one repository we read, read-only.
	body, _ := json.Marshal(map[string]any{
		"repositories": []string{m.Repo},
		"permissions":  map[string]string{"contents": "read"},
	})
	u := fmt.Sprintf("%s/app/installations/%d/access_tokens", m.api(), m.instID)
	if err := m.do(ctx, http.MethodPost, u, jwt, body, &at); err != nil {
		return "", fmt.Errorf("installation token: %w", err)
	}
	if at.Token == "" {
		return "", errors.New("installation token: empty")
	}
	m.token, m.expires = at.Token, at.ExpiresAt
	return m.token, nil
}

func (m *Minter) now() time.Time {
	if m.Now != nil {
		return m.Now()
	}
	return time.Now()
}

func (m *Minter) api() string {
	if m.API == "" {
		return "https://api.github.com"
	}
	return strings.TrimSuffix(m.API, "/")
}

func (m *Minter) do(ctx context.Context, method, u, jwt string, body []byte, out any) error {
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, u, r)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+jwt)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", "tmplwatch")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	c := m.HTTP
	if c == nil {
		c = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("%s %s: %s: %s", method, u, resp.Status, strings.TrimSpace(string(msg)))
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out)
}
