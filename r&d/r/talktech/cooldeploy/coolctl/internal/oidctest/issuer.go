// Package oidctest is a stand-in for GitHub's OIDC issuer. Tests and the
// local demo use it to mint the tokens a real workflow job would hold.
// It signs anything it is asked to sign, so it must never face a network.
package oidctest

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"time"
)

// Issuer signs tokens and serves the discovery documents a verifier reads.
type Issuer struct {
	URL string // set by the caller once the listen address is known
	key *rsa.PrivateKey
}

const kid = "oidctest-1"

// New returns an issuer with a fresh key.
func New() (*Issuer, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	return &Issuer{key: key}, nil
}

// Token signs claims, adding iss, iat, nbf and exp unless they are set.
func (i *Issuer) Token(claims map[string]any, ttl time.Duration) (string, error) {
	now := time.Now()
	c := map[string]any{"iss": i.URL, "iat": now.Unix(), "nbf": now.Unix(), "exp": now.Add(ttl).Unix()}
	for k, v := range claims {
		c[k] = v
	}
	enc := base64.RawURLEncoding.EncodeToString
	header, _ := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT", "kid": kid})
	body, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	signing := enc(header) + "." + enc(body)
	sum := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, i.key, crypto.SHA256, sum[:])
	if err != nil {
		return "", err
	}
	return signing + "." + enc(sig), nil
}

// Handler serves discovery, the key set and a token endpoint.
//
// GET /token?audience=cool-api&repository=...&job_workflow_ref=... answers
// {"value": "<jwt>"}, the shape of GitHub's own ID-token endpoint, so the
// same client code works against both.
func (i *Issuer) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"issuer": i.URL, "jwks_uri": i.URL + "/jwks"})
	})
	mux.HandleFunc("GET /jwks", func(w http.ResponseWriter, r *http.Request) {
		enc := base64.RawURLEncoding.EncodeToString
		pub := i.key.PublicKey
		json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
			"kty": "RSA", "alg": "RS256", "use": "sig", "kid": kid,
			"n": enc(pub.N.Bytes()), "e": enc(big.NewInt(int64(pub.E)).Bytes()),
		}}})
	})
	mux.HandleFunc("GET /token", func(w http.ResponseWriter, r *http.Request) {
		claims := map[string]any{}
		for k, v := range r.URL.Query() {
			claims[k] = v[0]
		}
		if aud, ok := claims["audience"]; ok {
			claims["aud"] = aud
			delete(claims, "audience")
		}
		tok, err := i.Token(claims, 10*time.Minute)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"value": tok})
	})
	return mux
}
