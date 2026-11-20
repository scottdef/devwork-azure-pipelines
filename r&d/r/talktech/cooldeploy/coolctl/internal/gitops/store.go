// Package gitops reads and writes the desired state in cool-gitops.
//
// A Store is the small file interface the rest of the code needs. DirStore
// backs it with a directory, for tests and the local demo. The ghapp package
// backs it with the GitHub contents API, for production.
package gitops

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

var (
	// ErrNotFound means the path does not exist.
	ErrNotFound = errors.New("not found")
	// ErrConflict means the file changed after the caller read it.
	ErrConflict = errors.New("conflict: the file changed since it was read")
)

// Store is a versioned file store. Rev is an opaque version of one file: pass
// the rev from Read back to Write or Delete, or "" to create a new file.
type Store interface {
	Read(ctx context.Context, path string) (data []byte, rev string, err error)
	Write(ctx context.Context, path string, data []byte, rev, message string) error
	Delete(ctx context.Context, path, rev, message string) error
	// List returns the names in dir, sorted. Directory names end in "/".
	List(ctx context.Context, dir string) ([]string, error)
}

// App is apps/<team>/<app>/app.json: the registration of an app.
type App struct {
	Team    string `json:"team"`
	App     string `json:"app"`
	Repo    string `json:"repo"`
	Chart   string `json:"chart"`
	Group   string `json:"group,omitempty"`
	Created string `json:"created,omitempty"`
	// Identities maps an environment to the client ID of the app's managed
	// identity there. A new values file starts with its environment's entry.
	Identities map[string]string `json:"identities,omitempty"`
}

// Team is teams/<team>/team.json.
type Team struct {
	Team  string `json:"team"`
	Group string `json:"group"`
	Quota string `json:"quota"`
}

// AppPath returns the path of an app's registration.
func AppPath(team, app string) string { return "apps/" + team + "/" + app + "/app.json" }

// ValuesPath returns the path of an app's values in an environment.
func ValuesPath(team, app, env string) string {
	return "apps/" + team + "/" + app + "/" + env + "/values.json"
}

// PreviewPath returns the path of the values of a pull-request preview.
func PreviewPath(team, app string, pr int) string {
	return fmt.Sprintf("apps/%s/%s/previews/pr-%d/values.json", team, app, pr)
}

// TeamPath returns the path of a team's record.
func TeamPath(team string) string { return "teams/" + team + "/team.json" }

// ReadJSON decodes the file at path into v and returns its rev.
func ReadJSON(ctx context.Context, s Store, path string, v any) (string, error) {
	b, rev, err := s.Read(ctx, path)
	if err != nil {
		return "", err
	}
	if err := json.Unmarshal(b, v); err != nil {
		return "", fmt.Errorf("%s: %w", path, err)
	}
	return rev, nil
}

// WriteJSON encodes v with two-space indentation and writes it to path.
// HTML escaping is off so that URLs and digests stay readable in diffs.
func WriteJSON(ctx context.Context, s Store, path string, v any, rev, message string) error {
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	e.SetIndent("", "  ")
	if err := e.Encode(v); err != nil {
		return err
	}
	return s.Write(ctx, path, b.Bytes(), rev, message)
}

// Apps returns every registered app, sorted by team and name.
func Apps(ctx context.Context, s Store) ([]App, error) {
	teams, err := s.List(ctx, "apps")
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var out []App
	for _, t := range teams {
		if !strings.HasSuffix(t, "/") {
			continue
		}
		names, err := s.List(ctx, "apps/"+strings.TrimSuffix(t, "/"))
		if err != nil {
			return nil, err
		}
		for _, n := range names {
			if !strings.HasSuffix(n, "/") {
				continue
			}
			var a App
			_, err := ReadJSON(ctx, s, AppPath(strings.TrimSuffix(t, "/"), strings.TrimSuffix(n, "/")), &a)
			if errors.Is(err, ErrNotFound) {
				continue
			}
			if err != nil {
				return nil, err
			}
			out = append(out, a)
		}
	}
	return out, nil
}

// DirStore is a Store over a directory. When the directory is a Git work
// tree, every change is committed, so the demo shows the same history that
// production would.
type DirStore struct {
	Root string
	Git  bool
	mu   sync.Mutex
}

// NewDirStore returns a store rooted at root.
func NewDirStore(root string) *DirStore {
	_, err := os.Stat(filepath.Join(root, ".git"))
	return &DirStore{Root: root, Git: err == nil}
}

func (d *DirStore) file(path string) (string, error) {
	if !fs.ValidPath(path) {
		return "", fmt.Errorf("invalid path %q", path)
	}
	return filepath.Join(d.Root, filepath.FromSlash(path)), nil
}

func revOf(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// Read implements Store.
func (d *DirStore) Read(_ context.Context, path string) ([]byte, string, error) {
	f, err := d.file(path)
	if err != nil {
		return nil, "", err
	}
	b, err := os.ReadFile(f)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, "", fmt.Errorf("%s: %w", path, ErrNotFound)
	}
	if err != nil {
		return nil, "", err
	}
	return b, revOf(b), nil
}

// check compares rev with the file on disk. The caller holds d.mu.
func (d *DirStore) check(f, path, rev string) error {
	cur, err := os.ReadFile(f)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		if rev != "" {
			return fmt.Errorf("%s: %w", path, ErrConflict)
		}
	case err != nil:
		return err
	case rev != revOf(cur):
		return fmt.Errorf("%s: %w", path, ErrConflict)
	}
	return nil
}

// Write implements Store.
func (d *DirStore) Write(ctx context.Context, path string, data []byte, rev, message string) error {
	f, err := d.file(path)
	if err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.check(f, path, rev); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(f), 0o755); err != nil {
		return err
	}
	tmp := f + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, f); err != nil {
		return err
	}
	return d.commit(ctx, path, message)
}

// Delete implements Store.
func (d *DirStore) Delete(ctx context.Context, path, rev, message string) error {
	f, err := d.file(path)
	if err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if _, err := os.Stat(f); errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("%s: %w", path, ErrNotFound)
	}
	if err := d.check(f, path, rev); err != nil {
		return err
	}
	if err := os.Remove(f); err != nil {
		return err
	}
	// Remove directories the delete left empty, as Git would.
	for dir := filepath.Dir(f); dir != d.Root; dir = filepath.Dir(dir) {
		if os.Remove(dir) != nil {
			break
		}
	}
	return d.commit(ctx, path, message)
}

// List implements Store.
func (d *DirStore) List(_ context.Context, dir string) ([]string, error) {
	f, err := d.file(dir)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(f)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("%s: %w", dir, ErrNotFound)
	}
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		if e.IsDir() {
			names = append(names, e.Name()+"/")
		} else {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

func (d *DirStore) commit(ctx context.Context, path, message string) error {
	if !d.Git {
		return nil
	}
	git := func(args ...string) error {
		cmd := exec.CommandContext(ctx, "git", append([]string{"-C", d.Root}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("git %s: %v: %s", args[0], err, bytes.TrimSpace(out))
		}
		return nil
	}
	if err := git("add", "-A", "--", path); err != nil {
		return err
	}
	return git("-c", "user.name=cool-api", "-c", "user.email=cool-api@coolengorg.invalid",
		"commit", "-q", "-m", message, "--", path)
}
