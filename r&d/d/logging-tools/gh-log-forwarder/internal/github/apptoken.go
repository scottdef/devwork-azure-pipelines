package github

import (
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
	"net/http"
	"strconv"
	"time"
)

// ParsePrivateKey reads a GitHub App private key (PKCS#1 or PKCS#8 PEM).
func ParsePrivateKey(b []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(b)
	if block == nil {
		return nil, errors.New("github app key: no PEM block found")
	}
	switch block.Type {
	case "RSA PRIVATE KEY":
		return x509.ParsePKCS1PrivateKey(block.Bytes)
	case "PRIVATE KEY":
		k, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return nil, err
		}
		rk, ok := k.(*rsa.PrivateKey)
		if !ok {
			return nil, errors.New("github app key: PKCS#8 key is not RSA")
		}
		return rk, nil
	}
	return nil, fmt.Errorf("github app key: unexpected PEM type %q", block.Type)
}

var b64 = base64.RawURLEncoding

// AppJWT mints the short-lived RS256 JWT a GitHub App uses to call /app
// endpoints. iss is the client ID (recommended) or the numeric app ID.
// iat is backdated 60s for clock skew; exp is 9 minutes out (max 10).
func AppJWT(iss string, key *rsa.PrivateKey, now time.Time) (string, error) {
	var issuer any = iss
	if n, err := strconv.ParseInt(iss, 10, 64); err == nil {
		issuer = n
	}
	claims, err := json.Marshal(map[string]any{
		"iat": now.Add(-60 * time.Second).Unix(),
		"exp": now.Add(9 * time.Minute).Unix(),
		"iss": issuer,
	})
	if err != nil {
		return "", err
	}
	signing := b64.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`)) + "." + b64.EncodeToString(claims)
	sum := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil {
		return "", err
	}
	return signing + "." + b64.EncodeToString(sig), nil
}

// OrgInstallationID looks up the app's installation on an organization.
func (c *Client) OrgInstallationID(ctx context.Context, jwt, org string) (int64, error) {
	var v struct {
		ID int64 `json:"id"`
	}
	_, err := c.getJSON(ctx, request{path: "/orgs/" + org + "/installation", token: jwt}, &v)
	return v.ID, err
}

// InstallationToken is a minted installation access token.
type InstallationToken struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// NewInstallationToken mints an installation token, optionally narrowed
// to some repositories (names without owner) and permissions.
func (c *Client) NewInstallationToken(ctx context.Context, jwt string, installationID int64, repos []string, perms map[string]string) (InstallationToken, error) {
	body := map[string]any{}
	if len(repos) > 0 {
		body["repositories"] = repos
	}
	if len(perms) > 0 {
		body["permissions"] = perms
	}
	resp, err := c.do(ctx, request{
		method: http.MethodPost,
		path:   "/app/installations/" + strconv.FormatInt(installationID, 10) + "/access_tokens",
		body:   body,
		token:  jwt,
	})
	if err != nil {
		return InstallationToken{}, err
	}
	defer resp.Body.Close()
	var t InstallationToken
	if err := json.NewDecoder(resp.Body).Decode(&t); err != nil {
		return InstallationToken{}, err
	}
	if t.Token == "" {
		return InstallationToken{}, errors.New("github: empty installation token")
	}
	return t, nil
}
