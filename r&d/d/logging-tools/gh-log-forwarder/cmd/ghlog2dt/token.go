package main

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/CoolGitOrg/gh-log-forwarder/internal/config"
	"github.com/CoolGitOrg/gh-log-forwarder/internal/github"
)

var outputNameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// cmdToken mints a GitHub App installation token without any Action:
// RS256 JWT -> (installation lookup) -> POST access_tokens.
func cmdToken(ctx context.Context, e *env, args []string) error {
	cfg, fs, lf, err := setup("token", e, args, 0)
	if err != nil {
		return err
	}
	var (
		appID      = e.getenv("GH_APP_ID")
		keyFile    = e.getenv("GH_APP_PRIVATE_KEY_FILE")
		org        = cfg.Owner
		instID     int64
		ghOutput   string
		repos      []string
		perms      = map[string]string{}
		apiBase    = cfg.GitHubAPI
		printExpry bool
	)
	if appID == "" {
		appID = e.getenv("GH_APP_CLIENT_ID")
	}
	fs.StringVar(&appID, "app-id", appID, "GitHub App client ID or numeric app ID (env GH_APP_ID / GH_APP_CLIENT_ID)")
	fs.StringVar(&keyFile, "key-file", keyFile, "PEM private key `file` (env GH_APP_PRIVATE_KEY_FILE; or the key itself in GH_APP_PRIVATE_KEY)")
	fs.StringVar(&org, "org", org, "organization the app is installed on")
	fs.Int64Var(&instID, "installation-id", 0, "installation ID (looked up from --org when 0)")
	fs.StringVar(&apiBase, "gh-api", apiBase, "GitHub REST base `url`")
	fs.StringVar(&ghOutput, "github-output", "", "write the token to $GITHUB_OUTPUT under this `name` and mask it, instead of stdout")
	fs.BoolVar(&printExpry, "print-expiry", false, "print the expiry time to stderr")
	fs.Func("repo", "narrow the token to this repository `name` (no owner); repeatable", func(s string) error {
		repos = append(repos, config.SplitList(s)...)
		return nil
	})
	fs.Func("permission", "narrow the token: `name=level`, e.g. actions=read; repeatable", func(s string) error {
		k, v, ok := strings.Cut(s, "=")
		if !ok || k == "" || (v != "read" && v != "write" && v != "admin") {
			return fmt.Errorf("want name=read|write|admin, got %q", s)
		}
		perms[k] = v
		return nil
	})
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	log, err := lf.logger(e.stderr)
	if err != nil {
		return err
	}
	if appID == "" {
		return usageErr("--app-id (or GH_APP_ID) is required")
	}
	var pemBytes []byte
	switch {
	case keyFile != "":
		if pemBytes, err = os.ReadFile(keyFile); err != nil {
			return usageErr("%v", err)
		}
	case e.getenv("GH_APP_PRIVATE_KEY") != "":
		pemBytes = []byte(e.getenv("GH_APP_PRIVATE_KEY"))
	default:
		return usageErr("--key-file, GH_APP_PRIVATE_KEY_FILE or GH_APP_PRIVATE_KEY is required")
	}
	key, err := github.ParsePrivateKey(pemBytes)
	if err != nil {
		return usageErr("%v", err)
	}
	if ghOutput != "" && !outputNameRe.MatchString(ghOutput) {
		return usageErr("--github-output name %q is not an identifier", ghOutput)
	}
	for _, r := range repos {
		if strings.Contains(r, "/") {
			return usageErr("--repo takes a repository name without the owner: %q", r)
		}
	}

	jwt, err := github.AppJWT(appID, key, time.Now())
	if err != nil {
		return err
	}
	gh := github.New(apiBase, "")
	gh.UserAgent = "ghlog2dt/" + version
	gh.Log = log
	if instID == 0 {
		if instID, err = gh.OrgInstallationID(ctx, jwt, org); err != nil {
			return fmt.Errorf("looking up installation on %s: %w", org, err)
		}
	}
	tok, err := gh.NewInstallationToken(ctx, jwt, instID, repos, perms)
	if err != nil {
		return err
	}
	if printExpry {
		fmt.Fprintf(e.stderr, "expires_at=%s\n", tok.ExpiresAt.UTC().Format(time.RFC3339))
	}
	if ghOutput == "" {
		_, err = fmt.Fprintln(e.stdout, tok.Token)
		return err
	}
	path := e.getenv("GITHUB_OUTPUT")
	if path == "" {
		return usageErr("--github-output needs $GITHUB_OUTPUT (run inside GitHub Actions)")
	}
	// The mask command must reach the runner via stdout before the value
	// is ever printed anywhere else.
	fmt.Fprintf(e.stdout, "::add-mask::%s\n", tok.Token)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "%s=%s\n", ghOutput, tok.Token)
	return err
}
