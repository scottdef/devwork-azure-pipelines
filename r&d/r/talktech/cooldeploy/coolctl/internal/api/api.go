// Package api is the HTTP interface of cool-api: the one writer to
// cool-gitops. Each write endpoint checks the caller's OIDC token, edits one
// values file and commits it. Argo CD does the rest.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/CoolEngOrg/coolctl/internal/argo"
	"github.com/CoolEngOrg/coolctl/internal/gitops"
	"github.com/CoolEngOrg/coolctl/internal/manifest"
	"github.com/CoolEngOrg/coolctl/internal/metrics"
	"github.com/CoolEngOrg/coolctl/internal/oidc"
	"github.com/CoolEngOrg/coolctl/internal/release"
	"github.com/CoolEngOrg/coolctl/internal/tmpl"
)

// Verifier turns a bearer token into verified claims.
type Verifier interface {
	Verify(ctx context.Context, token string) (oidc.Claims, error)
}

// Server holds what the handlers need. Set every exported field.
type Server struct {
	Store    gitops.Store
	Verifier Verifier
	Policy   oidc.Policy
	Argo     argo.Client
	Domain   string // DNS zone of app hosts, such as coolapps.example
	Chart    string // chart tag given to newly registered apps
	Origin   string // portal origin allowed to read across origins; optional
	Log      *slog.Logger
	Metrics  *metrics.Registry
	Now      func() time.Time

	mu sync.Mutex // one read-modify-write cycle at a time in this process
}

// Result is the answer to every write.
type Result struct {
	Team        string            `json:"team"`
	App         string            `json:"app"`
	Environment string            `json:"environment"`
	Preview     int               `json:"preview,omitempty"`
	Release     string            `json:"release,omitempty"`
	Application string            `json:"application"`
	URL         string            `json:"url"`
	Releases    []release.Release `json:"releases"`
	Message     string            `json:"message"`
}

// EnvStatus is an app's state in one environment.
type EnvStatus struct {
	Environment string            `json:"environment"`
	Application string            `json:"application"`
	URL         string            `json:"url"`
	Releases    []release.Release `json:"releases"`
	argo.Status
}

// Preview is one open pull-request preview.
type Preview struct {
	PR      int    `json:"pr"`
	URL     string `json:"url"`
	Release string `json:"release"`
}

// AppStatus is an app's registration with its state everywhere.
type AppStatus struct {
	gitops.App
	Environments []EnvStatus `json:"environments"`
	Previews     []Preview   `json:"previews,omitempty"`
}

type httpError struct {
	code int
	msg  string
}

func (e *httpError) Error() string { return e.msg }

func errf(code int, format string, a ...any) error {
	return &httpError{code, fmt.Sprintf(format, a...)}
}

var (
	reRepo  = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,39}/[A-Za-z0-9_.-]{1,100}$`)
	reIssue = regexp.MustCompile(`^https://github\.com/[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+/issues/[0-9]+$`)
	reGUID  = regexp.MustCompile(`^[0-9a-f]{8}(-[0-9a-f]{4}){3}-[0-9a-f]{12}$`)
)

// Handler returns the routes of cool-api.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok\n")) })
	mux.Handle("GET /metrics", s.Metrics)
	mux.HandleFunc("GET /v1/apps", s.json(s.listApps))
	mux.HandleFunc("POST /v1/apps", s.json(s.register))
	mux.HandleFunc("GET /v1/apps/{team}/{app}", s.json(s.status))
	mux.HandleFunc("GET /v1/apps/{team}/{app}/status", s.statusFragment)
	mux.HandleFunc("DELETE /v1/apps/{team}/{app}", s.json(s.retire))
	mux.HandleFunc("POST /v1/apps/{team}/{app}/deployments", s.json(s.deploy))
	mux.HandleFunc("DELETE /v1/apps/{team}/{app}/previews/{pr}", s.json(s.previewDown))
	mux.HandleFunc("POST /v1/apps/{team}/{app}/promote", s.json(s.promote))
	mux.HandleFunc("POST /v1/apps/{team}/{app}/rollback", s.json(s.rollback))
	mux.HandleFunc("POST /v1/apps/{team}/{app}/weights", s.json(s.weights))
	mux.HandleFunc("POST /v1/apps/{team}/{app}/config", s.json(s.config))
	return s.wrap(mux)
}

// wrap adds logging, metrics, panic recovery and read-only CORS.
func (s *Server) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		if s.Origin != "" && r.Header.Get("Origin") == s.Origin {
			w.Header().Set("Access-Control-Allow-Origin", s.Origin)
			w.Header().Set("Vary", "Origin")
			if r.Method == http.MethodOptions {
				w.Header().Set("Access-Control-Allow-Methods", "GET")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization, HX-Request, HX-Current-URL, HX-Target, HX-Trigger")
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		rec := &recorder{ResponseWriter: w, code: http.StatusOK}
		defer func() {
			if p := recover(); p != nil {
				s.Log.Error("panic", "path", r.URL.Path, "panic", fmt.Sprint(p))
				http.Error(rec, `{"error":"internal error"}`, http.StatusInternalServerError)
			}
			route := r.Pattern
			if route == "" {
				route = "unmatched"
			}
			s.Metrics.Inc("cool_api_requests_total", "route", route, "code", strconv.Itoa(rec.code))
			s.Metrics.Observe("cool_api_request_seconds", time.Since(start).Seconds(), "route", route)
			if r.URL.Path != "/healthz" && r.URL.Path != "/metrics" {
				s.Log.Info("request", "method", r.Method, "path", r.URL.Path, "code", rec.code,
					"ms", time.Since(start).Milliseconds())
			}
		}()
		next.ServeHTTP(rec, r)
	})
}

type recorder struct {
	http.ResponseWriter
	code int
}

func (r *recorder) WriteHeader(code int) {
	r.code = code
	r.ResponseWriter.WriteHeader(code)
}

func (s *Server) json(h func(*http.Request) (any, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		out, err := h(r)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	e := json.NewEncoder(w)
	e.SetEscapeHTML(false)
	e.Encode(v)
}

func (s *Server) fail(w http.ResponseWriter, r *http.Request, err error) {
	code := http.StatusInternalServerError
	var he *httpError
	switch {
	case errors.As(err, &he):
		code = he.code
	case errors.Is(err, gitops.ErrNotFound):
		code = http.StatusNotFound
	case errors.Is(err, gitops.ErrConflict):
		code = http.StatusConflict
	}
	msg := err.Error()
	if code == http.StatusInternalServerError {
		s.Log.Error("request failed", "path", r.URL.Path, "err", err)
		msg = "internal error"
	}
	writeJSON(w, code, map[string]string{"error": msg})
}

func decode(r *http.Request, v any) error {
	d := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return errf(http.StatusBadRequest, "request body: %v", err)
	}
	return nil
}

func appOf(r *http.Request) (team, app string, err error) {
	team, app = r.PathValue("team"), r.PathValue("app")
	if !manifest.ValidTeam(team) || !manifest.ValidApp(app) {
		return "", "", errf(http.StatusBadRequest, "%q/%q is not a team/app name", team, app)
	}
	return team, app, nil
}

func (s *Server) loadApp(ctx context.Context, team, app string) (gitops.App, error) {
	var a gitops.App
	_, err := gitops.ReadJSON(ctx, s.Store, gitops.AppPath(team, app), &a)
	if errors.Is(err, gitops.ErrNotFound) {
		return a, errf(http.StatusNotFound, "app %s/%s is not registered; open a cool-app-create request", team, app)
	}
	return a, err
}

// auth verifies the bearer token and asks the policy whether its job may act.
func (s *Server) auth(r *http.Request, repo, env string, act oidc.Action) (oidc.Claims, error) {
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || token == "" {
		return oidc.Claims{}, errf(http.StatusUnauthorized, "send a GitHub Actions OIDC token as a bearer token")
	}
	c, err := s.Verifier.Verify(r.Context(), token)
	if err != nil {
		return c, errf(http.StatusUnauthorized, "%v", err)
	}
	if err := s.Policy.Authorize(c, repo, env, act); err != nil {
		s.Log.Warn("denied", "action", act, "repo", repo, "env", env, "job", c.JobWorkflowRef, "by", c.Repository)
		return c, errf(http.StatusForbidden, "%v", err)
	}
	return c, nil
}

// values loads a values file, or a fresh one with rev "" if none exists.
func (s *Server) values(ctx context.Context, a gitops.App, env string, pr int) (release.Values, string, string, error) {
	path := gitops.ValuesPath(a.Team, a.App, env)
	if pr > 0 {
		path = gitops.PreviewPath(a.Team, a.App, pr)
	}
	var v release.Values
	rev, err := gitops.ReadJSON(ctx, s.Store, path, &v)
	if errors.Is(err, gitops.ErrNotFound) {
		v, rev, err = release.New(a.Team, a.App, env, a.Chart), "", nil
	}
	v.Chart, v.Group, v.Preview = a.Chart, a.Group, pr
	if v.Identity == "" {
		v.Identity = a.Identities[env]
	}
	return v, rev, path, err
}

func (s *Server) save(ctx context.Context, path string, v release.Values, rev, msg string) error {
	if len(v.Releases) > 0 {
		if err := release.Validate(v.Releases); err != nil {
			return errf(http.StatusConflict, "%v", err)
		}
	}
	return gitops.WriteJSON(ctx, s.Store, path, v, rev, msg)
}

func (s *Server) url(app, env string, pr int) string {
	if pr > 0 {
		return fmt.Sprintf("https://%s-pr-%d.preview.%s.%s", app, pr, env, s.Domain)
	}
	return fmt.Sprintf("https://%s.%s.%s", app, env, s.Domain)
}

func (s *Server) result(v release.Values, id, msg string) Result {
	name := argo.Name(v.Team, v.App, v.Environment, v.Preview)
	// Ask Argo CD to look now. A failure only delays the sync to its next poll.
	if err := s.Argo.Refresh(context.Background(), name); err != nil {
		s.Log.Warn("argo refresh", "application", name, "err", err)
	}
	return Result{
		Team: v.Team, App: v.App, Environment: v.Environment, Preview: v.Preview, Release: id,
		Application: name, URL: s.url(v.App, v.Environment, v.Preview), Releases: v.Releases, Message: msg,
	}
}

// message builds a commit message whose trailers say who asked and why.
func message(subject string, c oidc.Claims, issue string) string {
	var b strings.Builder
	b.WriteString(subject + "\n\n")
	fmt.Fprintf(&b, "Actor: %s\nWorkflow: %s\n", c.Actor, c.JobWorkflowRef)
	if c.RunID != "" {
		fmt.Fprintf(&b, "Run: https://github.com/%s/actions/runs/%s\n", c.Repository, c.RunID)
	}
	if issue != "" {
		fmt.Fprintf(&b, "Issue: %s\n", issue)
	}
	return b.String()
}

func checkIssue(issue string) error {
	if issue != "" && !reIssue.MatchString(issue) {
		return errf(http.StatusBadRequest, "issue %q: want an issue URL", issue)
	}
	return nil
}

func (s *Server) stamp() string { return s.Now().UTC().Format(time.RFC3339) }

func (s *Server) deploy(r *http.Request) (any, error) {
	team, app, err := appOf(r)
	if err != nil {
		return nil, err
	}
	var in struct {
		Environment string             `json:"environment"`
		Digest      string             `json:"digest"`
		SHA         string             `json:"sha"`
		Preview     int                `json:"preview"`
		Manifest    *manifest.Manifest `json:"manifest"`
		Issue       string             `json:"issue"`
	}
	if err := decode(r, &in); err != nil {
		return nil, err
	}
	if in.Environment == "" {
		in.Environment = "dev"
	}
	id, idErr := release.ID(in.SHA)
	switch {
	case !manifest.ValidEnv(in.Environment):
		return nil, errf(http.StatusBadRequest, "environment %q is not one of %v", in.Environment, manifest.Environments)
	case in.Preview < 0 || in.Preview > 0 && in.Environment != "dev":
		return nil, errf(http.StatusBadRequest, "previews run in dev and take a positive pull-request number")
	case idErr != nil:
		return nil, errf(http.StatusBadRequest, "%v", idErr)
	case !release.ValidDigest(in.Digest):
		return nil, errf(http.StatusBadRequest, "digest %q: want sha256:<64 hex digits>", in.Digest)
	}
	if err := checkIssue(in.Issue); err != nil {
		return nil, err
	}
	if m := in.Manifest; m != nil {
		m.Defaults()
		if err := m.Validate(); err != nil {
			return nil, errf(http.StatusBadRequest, "manifest: %v", err)
		}
		if m.Team != team || m.App != app {
			return nil, errf(http.StatusBadRequest, "manifest names %s/%s, not %s/%s", m.Team, m.App, team, app)
		}
	}
	a, err := s.loadApp(r.Context(), team, app)
	if err != nil {
		return nil, err
	}
	act := oidc.Deploy
	if in.Preview > 0 {
		act = oidc.Preview
	}
	c, err := s.auth(r, a.Repo, in.Environment, act)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	v, rev, path, err := s.values(r.Context(), a, in.Environment, in.Preview)
	if err != nil {
		return nil, err
	}
	if in.Manifest != nil {
		v.Apply(*in.Manifest)
	}
	v.Releases = release.Deploy(v.Releases, release.Release{ID: id, Digest: in.Digest, SHA: in.SHA, At: s.stamp()})
	where := in.Environment
	if in.Preview > 0 {
		v.Releases = v.Releases[:1] // a preview keeps no standby
		where = fmt.Sprintf("preview of pull request %d", in.Preview)
	}
	subject := fmt.Sprintf("deploy: %s/%s %s to %s", team, app, id, where)
	if err := s.save(r.Context(), path, v, rev, message(subject, c, in.Issue)); err != nil {
		return nil, err
	}
	return s.result(v, id, "deployed "+id+" to "+where), nil
}

func (s *Server) previewDown(r *http.Request) (any, error) {
	team, app, err := appOf(r)
	if err != nil {
		return nil, err
	}
	pr, err := strconv.Atoi(r.PathValue("pr"))
	if err != nil || pr <= 0 {
		return nil, errf(http.StatusBadRequest, "pull-request number %q", r.PathValue("pr"))
	}
	a, err := s.loadApp(r.Context(), team, app)
	if err != nil {
		return nil, err
	}
	c, err := s.auth(r, a.Repo, "dev", oidc.Preview)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	path := gitops.PreviewPath(team, app, pr)
	_, rev, err := s.Store.Read(r.Context(), path)
	if errors.Is(err, gitops.ErrNotFound) {
		return Result{Team: team, App: app, Environment: "dev", Preview: pr, Message: "no preview to remove"}, nil
	}
	if err != nil {
		return nil, err
	}
	subject := fmt.Sprintf("preview: remove %s/%s pull request %d", team, app, pr)
	if err := s.Store.Delete(r.Context(), path, rev, message(subject, c, "")); err != nil {
		return nil, err
	}
	return Result{Team: team, App: app, Environment: "dev", Preview: pr, Message: "preview removed"}, nil
}

func (s *Server) promote(r *http.Request) (any, error) {
	team, app, err := appOf(r)
	if err != nil {
		return nil, err
	}
	var in struct {
		From    string `json:"from"`
		To      string `json:"to"`
		Release string `json:"release"`
		Issue   string `json:"issue"`
	}
	if err := decode(r, &in); err != nil {
		return nil, err
	}
	if !manifest.ValidEnv(in.From) || !manifest.ValidEnv(in.To) || in.From == in.To {
		return nil, errf(http.StatusBadRequest, "promote from %q to %q: want two different environments", in.From, in.To)
	}
	if err := checkIssue(in.Issue); err != nil {
		return nil, err
	}
	a, err := s.loadApp(r.Context(), team, app)
	if err != nil {
		return nil, err
	}
	c, err := s.auth(r, a.Repo, in.To, oidc.Promote)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	rel, m, err := s.source(r.Context(), a, in.From, in.Release)
	if err != nil {
		return nil, err
	}
	v, rev, path, err := s.values(r.Context(), a, in.To, 0)
	if err != nil {
		return nil, err
	}
	if m != nil {
		v.Apply(*m)
	}
	rel.At = s.stamp()
	v.Releases = release.Deploy(v.Releases, rel)
	subject := fmt.Sprintf("promote: %s/%s %s from %s to %s", team, app, rel.ID, in.From, in.To)
	if err := s.save(r.Context(), path, v, rev, message(subject, c, in.Issue)); err != nil {
		return nil, err
	}
	return s.result(v, rel.ID, fmt.Sprintf("promoted %s from %s to %s", rel.ID, in.From, in.To)), nil
}

// source finds the release to copy out of an environment: the named one, or
// the active one. It returns the manifest that release was built with.
func (s *Server) source(ctx context.Context, a gitops.App, env, id string) (release.Release, *manifest.Manifest, error) {
	var src release.Values
	if _, err := gitops.ReadJSON(ctx, s.Store, gitops.ValuesPath(a.Team, a.App, env), &src); err != nil {
		if errors.Is(err, gitops.ErrNotFound) {
			err = errf(http.StatusConflict, "%s/%s has no release in %s", a.Team, a.App, env)
		}
		return release.Release{}, nil, err
	}
	i := release.Active(src.Releases)
	if i < 0 {
		return release.Release{}, nil, errf(http.StatusConflict, "%s/%s has no release in %s", a.Team, a.App, env)
	}
	rel := src.Releases[i]
	if id != "" {
		var ok bool
		if rel, ok = release.Find(src.Releases, id); !ok {
			return rel, nil, errf(http.StatusConflict, "release %s is not in %s", id, env)
		}
	}
	return release.Release{ID: rel.ID, Digest: rel.Digest, SHA: rel.SHA}, src.Manifest, nil
}

func (s *Server) rollback(r *http.Request) (any, error) {
	team, app, err := appOf(r)
	if err != nil {
		return nil, err
	}
	var in struct {
		Environment string `json:"environment"`
		Reason      string `json:"reason"`
		Issue       string `json:"issue"`
	}
	if err := decode(r, &in); err != nil {
		return nil, err
	}
	if !manifest.ValidEnv(in.Environment) {
		return nil, errf(http.StatusBadRequest, "environment %q", in.Environment)
	}
	if err := checkIssue(in.Issue); err != nil {
		return nil, err
	}
	a, err := s.loadApp(r.Context(), team, app)
	if err != nil {
		return nil, err
	}
	c, err := s.auth(r, a.Repo, in.Environment, oidc.Rollback)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	v, rev, path, err := s.values(r.Context(), a, in.Environment, 0)
	if err != nil {
		return nil, err
	}
	if v.Releases, err = release.Rollback(v.Releases); err != nil {
		return nil, errf(http.StatusConflict, "%s/%s in %s: %v", team, app, in.Environment, err)
	}
	id := v.Releases[release.Active(v.Releases)].ID
	subject := fmt.Sprintf("rollback: %s/%s to %s in %s", team, app, id, in.Environment)
	if reason := strings.Join(strings.Fields(in.Reason), " "); reason != "" {
		subject += "\n\nReason: " + reason
	}
	if err := s.save(r.Context(), path, v, rev, message(subject, c, in.Issue)); err != nil {
		return nil, err
	}
	return s.result(v, id, "rolled back to "+id+" in "+in.Environment), nil
}

func (s *Server) weights(r *http.Request) (any, error) {
	team, app, err := appOf(r)
	if err != nil {
		return nil, err
	}
	var in struct {
		Environment string `json:"environment"`
		Weight      int    `json:"weight"`
		Release     string `json:"release"` // a release already in Environment, or in From
		From        string `json:"from"`    // bring the release over from this environment
		Issue       string `json:"issue"`
	}
	if err := decode(r, &in); err != nil {
		return nil, err
	}
	if !manifest.ValidEnv(in.Environment) || in.From != "" && (!manifest.ValidEnv(in.From) || in.From == in.Environment) {
		return nil, errf(http.StatusBadRequest, "environment %q, from %q", in.Environment, in.From)
	}
	if err := checkIssue(in.Issue); err != nil {
		return nil, err
	}
	a, err := s.loadApp(r.Context(), team, app)
	if err != nil {
		return nil, err
	}
	c, err := s.auth(r, a.Repo, in.Environment, oidc.Weights)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	v, rev, path, err := s.values(r.Context(), a, in.Environment, 0)
	if err != nil {
		return nil, err
	}
	var n release.Release
	switch {
	case in.From != "":
		var m *manifest.Manifest
		if n, m, err = s.source(r.Context(), a, in.From, in.Release); err != nil {
			return nil, err
		}
		if _, listed := release.Find(v.Releases, n.ID); !listed {
			n.At = s.stamp()
			if m != nil {
				v.Apply(*m)
			}
		}
	case in.Release != "":
		var ok bool
		if n, ok = release.Find(v.Releases, in.Release); !ok {
			return nil, errf(http.StatusConflict, "release %s is not in %s; name the environment to bring it from", in.Release, in.Environment)
		}
	case len(v.Releases) >= 2:
		n = v.Releases[0] // the newest: the one being rolled out
	default:
		return nil, errf(http.StatusConflict, "no rolling release is in progress in %s", in.Environment)
	}
	if v.Releases, err = release.Stage(v.Releases, n, in.Weight); err != nil {
		return nil, errf(http.StatusConflict, "%v", err)
	}
	subject := fmt.Sprintf("weights: %s/%s %s at %d%% in %s", team, app, n.ID, in.Weight, in.Environment)
	if err := s.save(r.Context(), path, v, rev, message(subject, c, in.Issue)); err != nil {
		return nil, err
	}
	return s.result(v, n.ID, fmt.Sprintf("%s takes %d%% of traffic in %s", n.ID, in.Weight, in.Environment)), nil
}

func (s *Server) config(r *http.Request) (any, error) {
	team, app, err := appOf(r)
	if err != nil {
		return nil, err
	}
	var in struct {
		Environment   string            `json:"environment"`
		Set           map[string]string `json:"set"`
		Unset         []string          `json:"unset"`
		AddDomains    []string          `json:"add_domains"`
		RemoveDomains []string          `json:"remove_domains"`
		Identity      *string           `json:"identity"`
		Issue         string            `json:"issue"`
	}
	if err := decode(r, &in); err != nil {
		return nil, err
	}
	if !manifest.ValidEnv(in.Environment) {
		return nil, errf(http.StatusBadRequest, "environment %q", in.Environment)
	}
	for k := range in.Set {
		if !manifest.ValidKey(k) {
			return nil, errf(http.StatusBadRequest, "variable %q: want A-Z, 0-9 and _; PORT and COOL_* are reserved", k)
		}
	}
	for _, h := range slices.Concat(in.AddDomains, in.RemoveDomains) {
		if !manifest.ValidHost(h) {
			return nil, errf(http.StatusBadRequest, "domain %q is not a DNS name", h)
		}
	}
	if in.Identity != nil && *in.Identity != "" && !reGUID.MatchString(*in.Identity) {
		return nil, errf(http.StatusBadRequest, "identity %q: want a client ID", *in.Identity)
	}
	if err := checkIssue(in.Issue); err != nil {
		return nil, err
	}
	a, err := s.loadApp(r.Context(), team, app)
	if err != nil {
		return nil, err
	}
	c, err := s.auth(r, a.Repo, in.Environment, oidc.Config)
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	v, rev, path, err := s.values(r.Context(), a, in.Environment, 0)
	if err != nil {
		return nil, err
	}
	var changed []string
	for k, val := range in.Set {
		if v.Overrides == nil {
			v.Overrides = map[string]string{}
		}
		v.Overrides[k] = val
		changed = append(changed, "set "+k)
	}
	for _, k := range in.Unset {
		delete(v.Overrides, k)
		changed = append(changed, "unset "+k)
	}
	for _, h := range in.AddDomains {
		if !slices.Contains(v.Domains, h) {
			v.Domains = append(v.Domains, h)
			changed = append(changed, "add domain "+h)
		}
	}
	for _, h := range in.RemoveDomains {
		v.Domains = slices.DeleteFunc(v.Domains, func(d string) bool { return d == h })
		changed = append(changed, "remove domain "+h)
	}
	if in.Identity != nil {
		v.Identity = *in.Identity
		changed = append(changed, "set identity")
	}
	if len(changed) == 0 {
		return nil, errf(http.StatusBadRequest, "nothing to change")
	}
	slices.Sort(changed)
	// Variable values stay out of the commit subject: they may be long, and
	// the diff shows them anyway.
	subject := fmt.Sprintf("config: %s/%s in %s: %s", team, app, in.Environment, strings.Join(changed, ", "))
	if err := s.save(r.Context(), path, v, rev, message(subject, c, in.Issue)); err != nil {
		return nil, err
	}
	return s.result(v, "", strings.Join(changed, ", ")), nil
}

func (s *Server) register(r *http.Request) (any, error) {
	var in struct {
		Team  string `json:"team"`
		App   string `json:"app"`
		Repo  string `json:"repo"`
		Group string `json:"group"`
		Issue string `json:"issue"`
		// Environment is the GitHub environment the registering job ran in,
		// without its cool- prefix. Default dev; the sandbox uses sbx.
		Environment string `json:"environment"`
		// Identities maps an environment to a managed identity's client ID.
		Identities map[string]string `json:"identities"`
	}
	if err := decode(r, &in); err != nil {
		return nil, err
	}
	for env, id := range in.Identities {
		if !manifest.ValidEnv(env) || !reGUID.MatchString(id) {
			return nil, errf(http.StatusBadRequest, "identities: %q=%q: want an environment and a client ID", env, id)
		}
	}
	switch {
	case !manifest.ValidTeam(in.Team) || !manifest.ValidApp(in.App):
		return nil, errf(http.StatusBadRequest, "%q/%q is not a team/app name", in.Team, in.App)
	case !reRepo.MatchString(in.Repo):
		return nil, errf(http.StatusBadRequest, "repo %q: want owner/name", in.Repo)
	case in.Group != "" && !reGUID.MatchString(in.Group):
		return nil, errf(http.StatusBadRequest, "group %q: want an Entra ID object ID", in.Group)
	}
	if err := checkIssue(in.Issue); err != nil {
		return nil, err
	}
	if in.Environment == "" {
		in.Environment = "dev"
	}
	if !manifest.ValidEnv(in.Environment) {
		return nil, errf(http.StatusBadRequest, "environment %q is not one of %v", in.Environment, manifest.Environments)
	}
	c, err := s.auth(r, in.Repo, in.Environment, oidc.Register)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if in.Group == "" { // fall back to the team's group
		var t gitops.Team
		if _, err := gitops.ReadJSON(r.Context(), s.Store, gitops.TeamPath(in.Team), &t); err == nil {
			in.Group = t.Group
		}
	}
	a := gitops.App{Team: in.Team, App: in.App, Repo: in.Repo, Chart: s.Chart, Group: in.Group, Created: s.stamp(), Identities: in.Identities}
	subject := fmt.Sprintf("register: %s/%s from %s", a.Team, a.App, a.Repo)
	err = gitops.WriteJSON(r.Context(), s.Store, gitops.AppPath(a.Team, a.App), a, "", message(subject, c, in.Issue))
	if errors.Is(err, gitops.ErrConflict) {
		return nil, errf(http.StatusConflict, "app %s/%s is already registered", a.Team, a.App)
	}
	return a, err
}

func (s *Server) retire(r *http.Request) (any, error) {
	team, app, err := appOf(r)
	if err != nil {
		return nil, err
	}
	var in struct {
		Confirm string `json:"confirm"`
		Issue   string `json:"issue"`
	}
	if err := decode(r, &in); err != nil {
		return nil, err
	}
	if in.Confirm != team+"/"+app {
		return nil, errf(http.StatusBadRequest, "confirm must be %q", team+"/"+app)
	}
	if err := checkIssue(in.Issue); err != nil {
		return nil, err
	}
	a, err := s.loadApp(r.Context(), team, app)
	if err != nil {
		return nil, err
	}
	c, err := s.auth(r, a.Repo, "prd", oidc.Retire)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	msg := message(fmt.Sprintf("retire: %s/%s", team, app), c, in.Issue)
	n, err := s.remove(r.Context(), "apps/"+team+"/"+app, msg)
	if err != nil {
		return nil, err
	}
	return map[string]any{"team": team, "app": app, "removed": n, "message": "retired"}, nil
}

// remove deletes every file under dir, the registration last, so that a
// failure part-way leaves the app listed and the request retryable.
func (s *Server) remove(ctx context.Context, dir, msg string) (int, error) {
	names, err := s.Store.List(ctx, dir)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, name := range names { // directories first
		if strings.HasSuffix(name, "/") {
			k, err := s.remove(ctx, dir+"/"+strings.TrimSuffix(name, "/"), msg)
			if n += k; err != nil {
				return n, err
			}
		}
	}
	for _, name := range names {
		if strings.HasSuffix(name, "/") {
			continue
		}
		_, rev, err := s.Store.Read(ctx, dir+"/"+name)
		if err != nil {
			return n, err
		}
		if err := s.Store.Delete(ctx, dir+"/"+name, rev, msg); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

func (s *Server) listApps(r *http.Request) (any, error) {
	apps, err := gitops.Apps(r.Context(), s.Store)
	if err != nil {
		return nil, err
	}
	out := make([]AppStatus, 0, len(apps))
	for _, a := range apps {
		st, err := s.appStatus(r.Context(), a, false)
		if err != nil {
			return nil, err
		}
		out = append(out, st)
	}
	return out, nil
}

func (s *Server) status(r *http.Request) (any, error) {
	team, app, err := appOf(r)
	if err != nil {
		return nil, err
	}
	a, err := s.loadApp(r.Context(), team, app)
	if err != nil {
		return nil, err
	}
	return s.appStatus(r.Context(), a, true)
}

// statusFragment serves the same data as HTML, for the portal to swap in.
func (s *Server) statusFragment(w http.ResponseWriter, r *http.Request) {
	out, err := s.status(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.HTML(w, "status.html", out); err != nil {
		s.Log.Error("status fragment", "err", err)
	}
}

// AppStatusOf gathers an app's state from a store. The portal renderer uses
// it too, with no Argo CD.
func AppStatusOf(ctx context.Context, st gitops.Store, ac argo.Client, domain string, a gitops.App, live bool) (AppStatus, error) {
	s := &Server{Store: st, Argo: ac, Domain: domain}
	return s.appStatus(ctx, a, live)
}

func (s *Server) appStatus(ctx context.Context, a gitops.App, live bool) (AppStatus, error) {
	out := AppStatus{App: a, Environments: []EnvStatus{}}
	for _, env := range manifest.Environments {
		var v release.Values
		_, err := gitops.ReadJSON(ctx, s.Store, gitops.ValuesPath(a.Team, a.App, env), &v)
		if errors.Is(err, gitops.ErrNotFound) {
			continue
		}
		if err != nil {
			return out, err
		}
		es := EnvStatus{
			Environment: env, Application: argo.Name(a.Team, a.App, env, 0),
			URL: s.url(a.App, env, 0), Releases: v.Releases,
			Status: argo.Status{Sync: "Unknown", Health: "Unknown"},
		}
		if live {
			if st, err := s.Argo.Status(ctx, es.Application); err == nil {
				es.Status = st
			} else {
				es.Status.Message = err.Error()
			}
		}
		out.Environments = append(out.Environments, es)
	}
	names, err := s.Store.List(ctx, "apps/"+a.Team+"/"+a.App+"/previews")
	if err != nil && !errors.Is(err, gitops.ErrNotFound) {
		return out, err
	}
	for _, name := range names {
		pr, err := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(name, "pr-"), "/"))
		if err != nil {
			continue
		}
		var v release.Values
		if _, err := gitops.ReadJSON(ctx, s.Store, gitops.PreviewPath(a.Team, a.App, pr), &v); err != nil {
			continue
		}
		p := Preview{PR: pr, URL: s.url(a.App, "dev", pr)}
		if i := release.Active(v.Releases); i >= 0 {
			p.Release = v.Releases[i].ID
		}
		out.Previews = append(out.Previews, p)
	}
	slices.SortFunc(out.Previews, func(a, b Preview) int { return a.PR - b.PR })
	return out, nil
}
