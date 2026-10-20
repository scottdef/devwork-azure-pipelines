package source

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestKeyFor(t *testing.T) {
	for _, tc := range []struct {
		p, prefix, want string
		ok              bool
	}{
		{"kv/a/b", "kv/", "a/b", true},
		{"kv/a/.gitkeep", "kv/", "", false},
		{"kv/.git/HEAD", "kv/", "", false},
		{"other/a", "kv/", "", false},
		{"kv/", "kv/", "", false},
		{"a", "", "a", true},
	} {
		k, ok := keyFor(tc.p, tc.prefix)
		if k != tc.want || ok != tc.ok {
			t.Errorf("keyFor(%q,%q) = %q,%v", tc.p, tc.prefix, k, ok)
		}
	}
}

func TestDir(t *testing.T) {
	root := t.TempDir()
	write := func(p, s string) {
		p = filepath.Join(root, p)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("kv/a", "1\n")
	write("kv/b/c", "2")
	write("kv/.hidden", "x")
	write("skip", "x")
	d := NewDir(root, "kv")
	ctx := context.Background()
	s, ch, err := d.Fetch(ctx)
	if err != nil || !ch {
		t.Fatal(err, ch)
	}
	if len(s.Data) != 2 || s.Data["a"] != "1" || s.Data["b/c"] != "2" {
		t.Fatalf("data %v", s.Data)
	}
	if _, ch, _ := d.Fetch(ctx); ch {
		t.Error("unchanged tree reported change")
	}
	// Touch without content change: not a change.
	future := time.Now().Add(time.Hour)
	os.Chtimes(filepath.Join(root, "kv/a"), future, future)
	if _, ch, _ := d.Fetch(ctx); ch {
		t.Error("mtime-only change reported as change")
	}
	write("kv/a", "3")
	s, ch, _ = d.Fetch(ctx)
	if !ch || s.Data["a"] != "3" {
		t.Errorf("edit not seen: %v %v", ch, s.Data)
	}
}

// fakeGitHub serves the trees and blobs API with ETag support.
type fakeGitHub struct {
	files     map[string]string // path → content
	treeHits  atomic.Int32
	blobHits  atomic.Int32
	notMod    atomic.Int32
	sawBearer atomic.Bool
}

func (f *fakeGitHub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Authorization") == "Bearer s3cret" {
		f.sawBearer.Store(true)
	}
	switch {
	case r.URL.Path == "/repos/CoolGitOrg/tmplwatch-kv/git/trees/main":
		f.treeHits.Add(1)
		if r.URL.Query().Get("recursive") != "1" {
			http.Error(w, "want recursive", 400)
			return
		}
		etag := fmt.Sprintf(`W/"%d"`, len(f.files))
		if r.Header.Get("If-None-Match") == etag {
			f.notMod.Add(1)
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", etag)
		fmt.Fprint(w, `{"sha":"t1","truncated":false,"tree":[{"path":"kv","type":"tree","sha":"d"}`)
		for p, c := range f.files {
			fmt.Fprintf(w, `,{"path":%q,"type":"blob","sha":"sha-%s","size":%d}`, p, c, len(c))
		}
		fmt.Fprint(w, `]}`)
	case len(r.URL.Path) > len("/repos/CoolGitOrg/tmplwatch-kv/git/blobs/sha-"):
		f.blobHits.Add(1)
		c := r.URL.Path[len("/repos/CoolGitOrg/tmplwatch-kv/git/blobs/sha-"):]
		enc := base64.StdEncoding.EncodeToString([]byte(c + "\n"))
		// GitHub wraps base64 at 60 columns; include a newline.
		fmt.Fprintf(w, `{"content":"%s\n","encoding":"base64"}`, enc)
	default:
		http.NotFound(w, r)
	}
}

func TestGitHubPollsAndETag(t *testing.T) {
	f := &fakeGitHub{files: map[string]string{"kv/motd/text": "hello", "kv/a/b": "x", "README.md": "no"}}
	srv := httptest.NewServer(f)
	defer srv.Close()
	g, err := NewGitHub(srv.URL, "CoolGitOrg/tmplwatch-kv", "main", "kv/", func(context.Context) (string, error) { return "s3cret", nil })
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	var snaps []Snapshot
	ctx, cancel := context.WithCancel(context.Background())
	n := 0
	src := countingSource{g, func() {
		n++
		if n == 3 {
			cancel()
		}
	}}
	Watch(ctx, src, time.Millisecond, func(s Snapshot) { snaps = append(snaps, s) }, func(r string, err error) {
		if err != nil {
			t.Errorf("poll error: %v", err)
		}
		counts[r]++
	})
	if counts[Changed] != 1 || counts[Unchanged] != 2 || counts[Error] != 0 {
		t.Errorf("poll counts %v", counts)
	}
	if f.notMod.Load() < 1 {
		t.Error("server never answered 304")
	}
	if got := f.blobHits.Load(); got != 2 {
		t.Errorf("blob fetches = %d, want 2", got)
	}
	if !f.sawBearer.Load() {
		t.Error("no bearer token sent")
	}
	if len(snaps) != 1 || snaps[0].Data["motd/text"] != "hello" || len(snaps[0].Data) != 2 {
		t.Errorf("snapshots %v", snaps)
	}
	// A new file changes the ETag; only the new blob is fetched.
	f.files["kv/new"] = "n"
	s, ch, err := g.Fetch(context.Background())
	if err != nil || !ch || s.Data["new"] != "n" {
		t.Errorf("after add: %v %v %v", ch, err, s.Data)
	}
	if got := f.blobHits.Load(); got != 3 {
		t.Errorf("blob fetches after add = %d, want 3", got)
	}
}

type countingSource struct {
	Source
	tick func()
}

func (c countingSource) Fetch(ctx context.Context) (Snapshot, bool, error) {
	defer c.tick()
	return c.Source.Fetch(ctx)
}

func TestGitHubRateLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", fmt.Sprint(time.Now().Add(time.Minute).Unix()))
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	g, _ := NewGitHub(srv.URL, "o/r", "main", "", nil)
	_, _, err := g.Fetch(context.Background())
	rl, ok := err.(*RateLimitError)
	if !ok || time.Until(rl.Until) < 30*time.Second {
		t.Fatalf("want RateLimitError, got %v", err)
	}
}

func TestGitHubError(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	g, _ := NewGitHub(srv.URL, "o/r", "main", "", nil)
	if _, _, err := g.Fetch(context.Background()); err == nil {
		t.Fatal("want error on 404")
	}
}
