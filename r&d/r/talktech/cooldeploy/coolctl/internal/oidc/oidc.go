// Package oidc verifies GitHub Actions OIDC tokens and decides what the job
// that holds one may do.
//
// The platform keeps no deploy secret in any app repository. A job proves who
// it is with a token that GitHub signs; the claims in that token say which
// repository it runs for, which workflow file defines it and which GitHub
// environment it passed. Authorize turns those claims into a yes or a no.
package oidc

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"
)

// GitHubIssuer is the issuer of GitHub Actions tokens on github.com.
const GitHubIssuer = "https://token.actions.githubusercontent.com"

// Claims are the token claims the platform reads.
type Claims struct {
	Issuer         string   `json:"iss"`
	Subject        string   `json:"sub"`
	Audience       audience `json:"aud"`
	Expiry         int64    `json:"exp"`
	NotBefore      int64    `json:"nbf"`
	Repository     string   `json:"repository"`
	Environment    string   `json:"environment"`
	JobWorkflowRef string   `json:"job_workflow_ref"`
	Ref            string   `json:"ref"`
	Actor          string   `json:"actor"`
	RunID          string   `json:"run_id"`
}

// audience accepts the aud claim as one string or a list of strings.
type audience []string

func (a *audience) UnmarshalJSON(b []byte) error {
	var one string
	if json.Unmarshal(b, &one) == nil {
		*a = audience{one}
		return nil
	}
	var many []string
	if err := json.Unmarshal(b, &many); err != nil {
		return err
	}
	*a = many
	return nil
}

// Verifier checks token signatures against the issuer's published keys.
type Verifier struct {
	Issuer   string
	Audience string
	HTTP     *http.Client     // default http.DefaultClient
	Now      func() time.Time // default time.Now

	mu      sync.Mutex
	keys    map[string]*rsa.PublicKey
	fetched time.Time
}

func (v *Verifier) now() time.Time {
	if v.Now != nil {
		return v.Now()
	}
	return time.Now()
}

// Verify checks the signature, issuer, audience and validity period of token
// and returns its claims.
func (v *Verifier) Verify(ctx context.Context, token string) (Claims, error) {
	var c Claims
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return c, errors.New("token is not a JWT")
	}
	var header struct{ Alg, Kid string }
	if err := decodePart(parts[0], &header); err != nil {
		return c, fmt.Errorf("token header: %w", err)
	}
	if header.Alg != "RS256" {
		return c, fmt.Errorf("token algorithm %q: want RS256", header.Alg)
	}
	key, err := v.key(ctx, header.Kid)
	if err != nil {
		return c, err
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return c, fmt.Errorf("token signature: %w", err)
	}
	sum := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, sum[:], sig); err != nil {
		return c, errors.New("token signature does not verify")
	}
	if err := decodePart(parts[1], &c); err != nil {
		return c, fmt.Errorf("token claims: %w", err)
	}
	now := v.now().Unix()
	const leeway = 30
	switch {
	case c.Issuer != v.Issuer:
		return c, fmt.Errorf("token issuer %q: want %q", c.Issuer, v.Issuer)
	case !slices.Contains(c.Audience, v.Audience):
		return c, fmt.Errorf("token audience %v: want %q", []string(c.Audience), v.Audience)
	case now > c.Expiry+leeway:
		return c, errors.New("token expired")
	case c.NotBefore != 0 && now < c.NotBefore-leeway:
		return c, errors.New("token not yet valid")
	}
	return c, nil
}

func decodePart(s string, v any) error {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

// key returns the issuer's key with the given ID. An unknown ID triggers a
// refetch, at most once a minute, so key rotation needs no restart.
func (v *Verifier) key(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if k, ok := v.keys[kid]; ok {
		return k, nil
	}
	if !v.fetched.IsZero() && v.now().Sub(v.fetched) < time.Minute {
		return nil, fmt.Errorf("token key %q is not one of the issuer's keys", kid)
	}
	keys, err := v.fetch(ctx)
	if err != nil {
		return nil, fmt.Errorf("issuer keys: %w", err)
	}
	v.keys, v.fetched = keys, v.now()
	if k, ok := v.keys[kid]; ok {
		return k, nil
	}
	return nil, fmt.Errorf("token key %q is not one of the issuer's keys", kid)
}

func (v *Verifier) fetch(ctx context.Context) (map[string]*rsa.PublicKey, error) {
	var conf struct {
		JWKSURI string `json:"jwks_uri"`
	}
	if err := v.get(ctx, strings.TrimSuffix(v.Issuer, "/")+"/.well-known/openid-configuration", &conf); err != nil {
		return nil, err
	}
	var set struct {
		Keys []struct{ Kty, Kid, N, E string }
	}
	if err := v.get(ctx, conf.JWKSURI, &set); err != nil {
		return nil, err
	}
	keys := map[string]*rsa.PublicKey{}
	for _, k := range set.Keys {
		if k.Kty != "RSA" {
			continue
		}
		n, err1 := base64.RawURLEncoding.DecodeString(k.N)
		e, err2 := base64.RawURLEncoding.DecodeString(k.E)
		if err1 != nil || err2 != nil || len(e) == 0 || len(e) > 4 {
			continue
		}
		exp := 0
		for _, b := range e {
			exp = exp<<8 | int(b)
		}
		keys[k.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(n), E: exp}
	}
	if len(keys) == 0 {
		return nil, errors.New("the issuer published no RSA keys")
	}
	return keys, nil
}

func (v *Verifier) get(ctx context.Context, url string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	hc := v.HTTP
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// Action is something a token holder asks cool-api to do.
type Action string

// The actions cool-api authorizes.
const (
	Deploy   Action = "deploy"   // new release from a build
	Preview  Action = "preview"  // create or remove a pull-request preview
	Promote  Action = "promote"  // copy a release to another environment
	Rollback Action = "rollback" // swap active and standby
	Weights  Action = "weights"  // shift traffic in a rolling release
	Config   Action = "config"   // change overrides, domains or identity
	Register Action = "register" // add an app
	Retire   Action = "retire"   // remove an app
)

// Policy names the workflow files whose jobs the platform trusts.
type Policy struct {
	ActionsRepo  string // default CoolEngOrg/cool-actions
	ActionsRef   string // default refs/tags/v1
	IssueOpsRepo string // default CoolEngOrg/issueops
	IssueOpsRef  string // default refs/heads/main
}

// DefaultPolicy is the policy of the CoolEngOrg organization.
var DefaultPolicy = Policy{
	ActionsRepo:  "CoolEngOrg/cool-actions",
	ActionsRef:   "refs/tags/v1",
	IssueOpsRepo: "CoolEngOrg/issueops",
	IssueOpsRef:  "refs/heads/main",
}

// FromCI reports whether a pipeline file in cool-actions defines the job.
func (p Policy) FromCI(ref string) bool {
	return strings.HasPrefix(ref, p.ActionsRepo+"/.github/workflows/") &&
		strings.HasSuffix(ref, "@"+p.ActionsRef)
}

// FromIssueOps reports whether an IssueOps execute workflow defines the job.
func (p Policy) FromIssueOps(ref string) bool {
	return strings.HasPrefix(ref, p.IssueOpsRepo+"/.github/workflows/execute-cool-") &&
		strings.HasSuffix(ref, "@"+p.IssueOpsRef)
}

// Authorize reports whether the job behind c may perform act on the app whose
// source lives in appRepo, in environment env.
//
// A pipeline job may deploy its own repository's app to dev and manage its
// previews. Everything else needs an IssueOps execute job that ran in the
// GitHub environment cool-<env>, which GitHub grants only after that
// environment's required reviewers approve.
func (p Policy) Authorize(c Claims, appRepo, env string, act Action) error {
	switch {
	case p.FromCI(c.JobWorkflowRef):
		if c.Repository == appRepo && env == "dev" && (act == Deploy || act == Preview) {
			return nil
		}
	case p.FromIssueOps(c.JobWorkflowRef):
		if c.Repository == p.IssueOpsRepo && c.Environment == "cool-"+env {
			return nil
		}
	default:
		return fmt.Errorf("job %q is not defined by a platform workflow", c.JobWorkflowRef)
	}
	return fmt.Errorf("%s (environment %q) may not %s %s in %s", c.Repository, c.Environment, act, appRepo, env)
}
