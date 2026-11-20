// Cool-api is the service that turns verified requests into commits.
//
// Configuration comes from the environment:
//
//	COOL_ADDR             listen address, default :8080
//	COOL_STORE            dir:/path/to/cool-gitops, or github:CoolEngOrg/cool-gitops@main
//	COOL_GH_CLIENT_ID     client ID of the CoolDeploy GitHub App       (github store)
//	COOL_GH_INSTALLATION  installation ID of that App                  (github store)
//	COOL_GH_KEY_FILE      path of the App's private key, PEM           (github store)
//	COOL_GH_API           GitHub API base, default https://api.github.com
//	COOL_OIDC_ISSUER      default https://token.actions.githubusercontent.com
//	COOL_OIDC_AUDIENCE    default cool-api
//	COOL_ACTIONS_REPO     default CoolEngOrg/cool-actions
//	COOL_ACTIONS_REF      default refs/tags/v1
//	COOL_ISSUEOPS_REPO    default CoolEngOrg/issueops
//	COOL_ISSUEOPS_REF     default refs/heads/main
//	COOL_ARGO             incluster or none, default none
//	COOL_DOMAIN           DNS zone of app hosts, default coolapps.example
//	COOL_CHART            chart tag for new apps, default cool-app-1.0.0
//	COOL_PORTAL_ORIGIN    origin of the portal, allowed to read across origins
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/CoolEngOrg/coolctl/internal/api"
	"github.com/CoolEngOrg/coolctl/internal/argo"
	"github.com/CoolEngOrg/coolctl/internal/ghapp"
	"github.com/CoolEngOrg/coolctl/internal/gitops"
	"github.com/CoolEngOrg/coolctl/internal/metrics"
	"github.com/CoolEngOrg/coolctl/internal/oidc"
)

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(log); err != nil {
		log.Error("cool-api", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger) error {
	store, err := openStore(env("COOL_STORE", ""))
	if err != nil {
		return err
	}
	var ac argo.Client = argo.None{}
	if env("COOL_ARGO", "none") == "incluster" {
		if ac, err = argo.NewInCluster("argocd"); err != nil {
			return err
		}
	}
	hc := &http.Client{Timeout: 15 * time.Second}
	s := &api.Server{
		Store:    store,
		Verifier: &oidc.Verifier{Issuer: env("COOL_OIDC_ISSUER", oidc.GitHubIssuer), Audience: env("COOL_OIDC_AUDIENCE", "cool-api"), HTTP: hc},
		Policy: oidc.Policy{
			ActionsRepo:  env("COOL_ACTIONS_REPO", oidc.DefaultPolicy.ActionsRepo),
			ActionsRef:   env("COOL_ACTIONS_REF", oidc.DefaultPolicy.ActionsRef),
			IssueOpsRepo: env("COOL_ISSUEOPS_REPO", oidc.DefaultPolicy.IssueOpsRepo),
			IssueOpsRef:  env("COOL_ISSUEOPS_REF", oidc.DefaultPolicy.IssueOpsRef),
		},
		Argo:    ac,
		Domain:  env("COOL_DOMAIN", "coolapps.example"),
		Chart:   env("COOL_CHART", "cool-app-1.0.0"),
		Origin:  env("COOL_PORTAL_ORIGIN", ""),
		Log:     log,
		Metrics: &metrics.Registry{},
		Now:     time.Now,
	}
	srv := &http.Server{
		Addr:              env("COOL_ADDR", ":8080"),
		Handler:           s.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
	}
	failed := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			failed <- err
		}
	}()
	log.Info("listening", "addr", srv.Addr, "store", env("COOL_STORE", ""), "argo", env("COOL_ARGO", "none"))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()
	select {
	case err := <-failed:
		return err
	case <-ctx.Done():
	}
	drain, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	return srv.Shutdown(drain)
}

// openStore parses COOL_STORE.
func openStore(spec string) (gitops.Store, error) {
	kind, rest, _ := strings.Cut(spec, ":")
	switch kind {
	case "dir":
		if _, err := os.Stat(rest); err != nil {
			return nil, err
		}
		return gitops.NewDirStore(rest), nil
	case "github":
		repo, branch, ok := strings.Cut(rest, "@")
		owner, name, ok2 := strings.Cut(repo, "/")
		if !ok || !ok2 {
			return nil, fmt.Errorf("COOL_STORE %q: want github:owner/repo@branch", spec)
		}
		pem, err := os.ReadFile(env("COOL_GH_KEY_FILE", "/mnt/secrets/cooldeploy-app-key"))
		if err != nil {
			return nil, err
		}
		key, err := ghapp.ParseKey(pem)
		if err != nil {
			return nil, err
		}
		inst, err := strconv.ParseInt(os.Getenv("COOL_GH_INSTALLATION"), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("COOL_GH_INSTALLATION: %w", err)
		}
		app := &ghapp.App{
			ClientID: os.Getenv("COOL_GH_CLIENT_ID"), Installation: inst, Key: key,
			API: env("COOL_GH_API", ""), HTTP: &http.Client{Timeout: 30 * time.Second},
		}
		return &ghapp.Store{App: app, Owner: owner, Repo: name, Branch: branch}, nil
	}
	return nil, fmt.Errorf("COOL_STORE %q: want dir:<path> or github:owner/repo@branch", spec)
}
