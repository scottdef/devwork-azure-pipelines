package source

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// TokenFunc returns a bearer token, or "" for anonymous access.
type TokenFunc func(ctx context.Context) (string, error)

// GitHub is a source backed by a path prefix in a GitHub repository.
//
// One poll is one conditional GET of the recursive git tree for Ref. When
// the ref has not moved GitHub answers 304, which costs no rate limit.
// When it has, only blobs whose SHA is new are fetched: blobs are content
// addressed, so the cache never needs invalidating.
type GitHub struct {
	API    string // e.g. https://api.github.com
	Owner  string
	Repo   string
	Ref    string
	Prefix string
	Token  TokenFunc
	Agent  string
	HTTP   *http.Client

	etag  string
	last  Snapshot
	blobs map[string]string // sha → value
}

// NewGitHub returns a GitHub source for "owner/name".
func NewGitHub(api, repo, ref, prefix string, tok TokenFunc) (*GitHub, error) {
	owner, name, ok := strings.Cut(repo, "/")
	if !ok || owner == "" || name == "" {
		return nil, fmt.Errorf("repo %q: want owner/name", repo)
	}
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}
	return &GitHub{
		API: strings.TrimSuffix(api, "/"), Owner: owner, Repo: name, Ref: ref, Prefix: prefix,
		Token: tok, Agent: "tmplwatch", HTTP: &http.Client{Timeout: 30 * time.Second},
		blobs: map[string]string{},
	}, nil
}

// String names the source in logs.
func (g *GitHub) String() string {
	return fmt.Sprintf("github:%s/%s@%s/%s", g.Owner, g.Repo, g.Ref, g.Prefix)
}

type treeResp struct {
	SHA       string `json:"sha"`
	Truncated bool   `json:"truncated"`
	Tree      []struct {
		Path string `json:"path"`
		Type string `json:"type"`
		SHA  string `json:"sha"`
		Size int64  `json:"size"`
	} `json:"tree"`
}

type blobResp struct {
	Content  string `json:"content"`
	Encoding string `json:"encoding"`
}

// maxValue bounds a single value; config files are not media.
const maxValue = 1 << 20

// Fetch performs one poll.
func (g *GitHub) Fetch(ctx context.Context) (Snapshot, bool, error) {
	u := fmt.Sprintf("%s/repos/%s/%s/git/trees/%s?recursive=1", g.API,
		url.PathEscape(g.Owner), url.PathEscape(g.Repo), url.PathEscape(g.Ref))
	resp, err := g.get(ctx, u, g.etag)
	if err != nil {
		return Snapshot{}, false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotModified {
		return g.last, false, nil
	}
	var tr treeResp
	if err := decode(resp, &tr, 64<<20); err != nil {
		return Snapshot{}, false, fmt.Errorf("tree: %w", err)
	}
	if tr.Truncated {
		return Snapshot{}, false, fmt.Errorf("tree %s truncated by GitHub: repository too large for one request", tr.SHA)
	}
	data := map[string]string{}
	seen := map[string]bool{}
	for _, e := range tr.Tree {
		if e.Type != "blob" {
			continue
		}
		k, ok := keyFor(e.Path, g.Prefix)
		if !ok {
			continue
		}
		if e.Size > maxValue {
			return Snapshot{}, false, fmt.Errorf("%s: %d bytes exceeds %d", e.Path, e.Size, maxValue)
		}
		v, ok := g.blobs[e.SHA]
		if !ok {
			if v, err = g.blob(ctx, e.SHA); err != nil {
				return Snapshot{}, false, fmt.Errorf("%s: %w", e.Path, err)
			}
		}
		data[k] = v
		seen[e.SHA] = true
		g.blobs[e.SHA] = v
	}
	for sha := range g.blobs {
		if !seen[sha] {
			delete(g.blobs, sha)
		}
	}
	// Commit the ETag only after every blob arrived, so a failed poll is
	// retried in full rather than masked by a 304.
	g.etag = resp.Header.Get("ETag")
	s := Snapshot{Data: data, Version: tr.SHA}
	// The tree moves on any commit; the prefix may not have.
	changed := Hash(data) != Hash(g.last.Data) || g.last.Data == nil
	g.last = s
	return s, changed, nil
}

func (g *GitHub) blob(ctx context.Context, sha string) (string, error) {
	u := fmt.Sprintf("%s/repos/%s/%s/git/blobs/%s", g.API, url.PathEscape(g.Owner), url.PathEscape(g.Repo), sha)
	resp, err := g.get(ctx, u, "")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var b blobResp
	if err := decode(resp, &b, 2*maxValue); err != nil {
		return "", fmt.Errorf("blob %s: %w", sha, err)
	}
	if b.Encoding != "base64" {
		return "", fmt.Errorf("blob %s: encoding %q", sha, b.Encoding)
	}
	raw, err := base64.StdEncoding.DecodeString(strings.ReplaceAll(b.Content, "\n", ""))
	if err != nil {
		return "", fmt.Errorf("blob %s: %w", sha, err)
	}
	return value(raw), nil
}

func (g *GitHub) get(ctx context.Context, u, etag string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("User-Agent", g.Agent)
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	if g.Token != nil {
		tok, err := g.Token(ctx)
		if err != nil {
			return nil, fmt.Errorf("token: %w", err)
		}
		if tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}
	}
	resp, err := g.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	if rl := rateLimited(resp); rl != nil {
		resp.Body.Close()
		return nil, rl
	}
	return resp, nil
}

// rateLimited recognises GitHub's primary and secondary rate limits.
func rateLimited(resp *http.Response) error {
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusTooManyRequests {
		return nil
	}
	if s := resp.Header.Get("Retry-After"); s != "" {
		if n, err := strconv.Atoi(s); err == nil {
			return &RateLimitError{Until: time.Now().Add(time.Duration(n) * time.Second)}
		}
	}
	if resp.Header.Get("X-RateLimit-Remaining") == "0" {
		if n, err := strconv.ParseInt(resp.Header.Get("X-RateLimit-Reset"), 10, 64); err == nil {
			return &RateLimitError{Until: time.Unix(n, 0)}
		}
	}
	return nil
}

func decode(resp *http.Response, v any, limit int64) error {
	body := io.LimitReader(resp.Body, limit)
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(body, 512))
		return fmt.Errorf("%s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}
	return json.NewDecoder(body).Decode(v)
}
