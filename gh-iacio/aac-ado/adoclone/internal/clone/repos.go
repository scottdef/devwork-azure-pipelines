package clone

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type gitRepo struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	DefaultBranch string `json:"defaultBranch"`
	RemoteURL     string `json:"remoteUrl"`
	IsDisabled    bool   `json:"isDisabled"`
}

// CloneRepos creates each repository and copies every branch and tag.
// -repo-mode=mirror uses git (clone --mirror / push --mirror, plus LFS when
// git-lfs is installed); -repo-mode=import uses server-side import requests
// (no git needed, but no LFS objects). The source project's default repo
// (named after the project) maps to the target's default repo.
func CloneRepos(ctx context.Context, x *Ctx) error {
	src, err := x.SrcProject(ctx)
	if err != nil {
		return err
	}
	tgt, err := x.TgtProject(ctx)
	if err != nil {
		return err
	}
	if strings.EqualFold(src.Capabilities.Versioncontrol.SourceControlType, "Tfvc") {
		x.todo("TFVC", "the source uses TFVC; import its tip into a Git repo (importRequests tfvcSource) or keep the source as the archive")
	}
	repos, err := valueOf[gitRepo](ctx, x, x.src("_apis/git/repositories?api-version=7.1"))
	if err != nil {
		return err
	}
	x.read(len(repos))
	byName := map[string]gitRepo{}
	if tgt.ID != "" {
		have, err := valueOf[gitRepo](ctx, x, x.tgt("_apis/git/repositories?api-version=7.1"))
		if err != nil {
			return err
		}
		for _, r := range have {
			byName[fold(r.Name)] = r
		}
	}
	if x.Opt.RepoMode == "mirror" && !x.Dry() {
		if _, err := exec.LookPath(x.Opt.Git); err != nil {
			return fmt.Errorf("-repo-mode=mirror needs git (%q not found); install it or use -repo-mode=import", x.Opt.Git)
		}
	}
	for _, r := range repos {
		if r.IsDisabled {
			x.todo("repo "+r.Name, "disabled in the source; enable it and rerun repos to copy it")
			continue
		}
		t, ok := byName[fold(r.Name)]
		if !ok && strings.EqualFold(r.Name, x.Src) {
			t, ok = byName[fold(x.Tgt)] // default repo
		}
		if !ok {
			body := map[string]any{"name": r.Name, "project": map[string]string{"id": tgt.ID}}
			if err := x.C.Post(ctx, x.tgt("_apis/git/repositories?api-version=7.1"), body, &t); err != nil {
				x.fail("repo "+r.Name, err)
				continue
			}
			x.wrote()
		}
		if x.Dry() {
			continue
		}
		x.record("repo", strings.ToLower(r.ID), t.ID)
		x.record("repoName", r.Name, t.Name)
		if done, _ := x.mapped("repoContent", r.ID); done == "1" {
			continue
		}
		if r.DefaultBranch == "" { // empty repo
			x.record("repoContent", r.ID, "1")
			continue
		}
		var cerr error
		if x.Opt.RepoMode == "import" {
			cerr = x.importRepo(ctx, r, t, tgt)
		} else {
			cerr = x.mirrorRepo(ctx, r.RemoteURL, t.RemoteURL)
		}
		if cerr != nil {
			x.fail("repo content "+r.Name, cerr)
			continue
		}
		if err := x.C.Patch(ctx, x.tgt("_apis/git/repositories/"+t.ID+"?api-version=7.1"), map[string]string{"defaultBranch": r.DefaultBranch}, nil); err != nil {
			x.fail("default branch "+r.Name, err)
			continue
		}
		x.record("repoContent", r.ID, "1")
	}
	return nil
}

// gitEnv passes the PAT through GIT_CONFIG_* (git 2.31+) so it never shows up
// in the process list or in .git/config.
func (x *Ctx) gitEnv() []string {
	return append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"GIT_CONFIG_COUNT=1",
		"GIT_CONFIG_KEY_0=http.extraHeader",
		"GIT_CONFIG_VALUE_0=Authorization: "+x.C.AuthHeader(),
	)
}

func (x *Ctx) runGit(ctx context.Context, dir, stdin string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, x.Opt.Git, args...)
	cmd.Dir = dir
	cmd.Env = x.gitEnv()
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		return out.String(), fmt.Errorf("git %s: %w: %s", args[0], err, tail(out.String(), 6))
	}
	return out.String(), nil
}

func tail(s string, n int) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, " | ")
}

// cleanURL drops the "org@" user info Azure DevOps puts in remote URLs.
func cleanURL(u string) string {
	p, err := url.Parse(u)
	if err != nil {
		return u
	}
	p.User = nil
	return p.String()
}

// mirrorRepo copies every branch and tag, and LFS objects when git-lfs is installed.
func (x *Ctx) mirrorRepo(ctx context.Context, srcURL, tgtURL string) error {
	dir, err := os.MkdirTemp(x.Opt.WorkDir, "adoclone-repo-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	bare := filepath.Join(dir, "r.git")
	if _, err := x.runGit(ctx, dir, "", "clone", "--mirror", "--quiet", cleanURL(srcURL), bare); err != nil {
		return err
	}
	// Branches and tags only: pull request refs are read-only on the server,
	// and unlike push --mirror this never deletes a branch that exists only
	// in the target.
	if _, err := x.runGit(ctx, bare, "", "push", "--force", "--quiet", cleanURL(tgtURL),
		"refs/heads/*:refs/heads/*", "refs/tags/*:refs/tags/*"); err != nil {
		return err
	}
	if _, err := exec.LookPath("git-lfs"); err == nil {
		if _, err := x.runGit(ctx, bare, "", "lfs", "fetch", "--all", "origin"); err != nil {
			return err
		}
		if _, err := x.runGit(ctx, bare, "", "lfs", "push", "--all", cleanURL(tgtURL)); err != nil {
			return err
		}
	}
	return nil
}

// pushBranch force-pushes one ref (used for the project wiki's wikiMaster).
func (x *Ctx) pushBranch(ctx context.Context, srcURL, tgtURL, ref string) error {
	dir, err := os.MkdirTemp(x.Opt.WorkDir, "adoclone-wiki-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	bare := filepath.Join(dir, "w.git")
	if _, err := x.runGit(ctx, dir, "", "clone", "--bare", "--quiet", cleanURL(srcURL), bare); err != nil {
		return err
	}
	_, err = x.runGit(ctx, bare, "", "push", "--force", "--quiet", cleanURL(tgtURL), ref+":"+ref)
	return err
}

// importRepo runs a server-side import request through a temporary
// "Other Git" service connection that the server deletes afterwards.
func (x *Ctx) importRepo(ctx context.Context, r, t gitRepo, tgt *Project) error {
	name := "adoclone-import-" + r.Name
	ep := struct {
		ID string `json:"id"`
	}{}
	body := map[string]any{
		"name": name, "type": "git", "url": cleanURL(r.RemoteURL), "isShared": false,
		"authorization": map[string]any{"scheme": "UsernamePassword", "parameters": map[string]string{"username": "adoclone", "password": x.Opt.PAT}},
		"serviceEndpointProjectReferences": []any{map[string]any{
			"projectReference": map[string]string{"id": tgt.ID, "name": tgt.Name}, "name": name,
		}},
	}
	if err := x.C.Post(ctx, x.org("_apis/serviceendpoint/endpoints?api-version=7.1"), body, &ep); err != nil {
		return fmt.Errorf("import connection: %w", err)
	}
	done := false
	defer func() {
		if done { // the server deletes the connection after a successful import
			return
		}
		// The connection holds the PAT: remove it on every other exit,
		// even if ctx was cancelled.
		cctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
		defer cancel()
		_ = x.C.Delete(cctx, x.org("_apis/serviceendpoint/endpoints/"+ep.ID+"?projectIds="+tgt.ID+"&api-version=7.1"), nil)
	}()
	var ir struct {
		ImportRequestID int `json:"importRequestId"`
	}
	req := map[string]any{"parameters": map[string]any{
		"gitSource":                              map[string]any{"url": cleanURL(r.RemoteURL), "overwrite": false},
		"serviceEndpointId":                      ep.ID,
		"deleteServiceEndpointAfterImportIsDone": true,
	}}
	base := x.tgt("_apis/git/repositories/" + t.ID + "/importRequests")
	if err := x.C.Post(ctx, base+"?api-version=7.1", req, &ir); err != nil {
		return fmt.Errorf("import request: %w", err)
	}
	deadline := time.Now().Add(6 * time.Hour)
	for time.Now().Before(deadline) {
		var st struct {
			Status         string `json:"status"`
			DetailedStatus struct {
				ErrorMessage string `json:"errorMessage"`
			} `json:"detailedStatus"`
		}
		if err := x.C.Get(ctx, fmt.Sprintf("%s/%d?api-version=7.1", base, ir.ImportRequestID), &st); err != nil {
			return err
		}
		switch st.Status {
		case "completed":
			done = true
			return nil
		case "failed", "abandoned":
			return fmt.Errorf("import %s: %s", st.Status, st.DetailedStatus.ErrorMessage)
		}
		if err := x.C.Sleep(ctx, 10*time.Second); err != nil {
			return err
		}
	}
	return fmt.Errorf("import of %s didn't finish in 6 hours", r.Name)
}
