// Package source fetches key/value snapshots from a backing store.
//
// A key is a slash-separated path such as "caddy/sites/0"; its value is
// the file's contents with at most one trailing newline removed, so that
// files written by editors and values written by tools read the same.
package source

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path"
	"sort"
	"strings"
	"time"
)

// Snapshot is an immutable view of the store at one version.
// Callers must not modify Data.
type Snapshot struct {
	Data    map[string]string
	Version string
}

// Source produces snapshots. Fetch reports changed=false when the store
// is known to be unchanged since the previous call; the returned snapshot
// is then the previous one.
type Source interface {
	Fetch(ctx context.Context) (s Snapshot, changed bool, err error)
	String() string
}

// Poll results, used as metric label values.
const (
	Changed   = "changed"
	Unchanged = "unchanged"
	Error     = "error"
)

// RateLimitError reports that the source asked us to back off.
type RateLimitError struct {
	Until time.Time
}

// Error implements error.
func (e *RateLimitError) Error() string {
	return fmt.Sprintf("rate limited until %s", e.Until.UTC().Format(time.RFC3339))
}

// Watch calls Fetch immediately and then every interval until ctx is done.
// Each changed snapshot is passed to emit; every poll outcome is passed to
// observe (which may be nil). A rate-limit error stretches the next wait.
func Watch(ctx context.Context, src Source, interval time.Duration, emit func(Snapshot), observe func(result string, err error)) {
	if observe == nil {
		observe = func(string, error) {}
	}
	for {
		wait := interval
		s, changed, err := src.Fetch(ctx)
		switch {
		case err != nil:
			if ctx.Err() != nil {
				return
			}
			observe(Error, err)
			var rl *RateLimitError
			if errors.As(err, &rl) {
				if d := time.Until(rl.Until); d > wait {
					wait = d
				}
			}
		case changed:
			observe(Changed, nil)
			emit(s)
		default:
			observe(Unchanged, nil)
		}
		t := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-t.C:
		}
	}
}

// keyFor maps a slash path to a key under prefix. It reports false for
// paths outside the prefix and for hidden files such as .gitkeep.
func keyFor(p, prefix string) (string, bool) {
	if !strings.HasPrefix(p, prefix) {
		return "", false
	}
	k := strings.TrimPrefix(p, prefix)
	if k == "" || strings.HasPrefix(path.Base(k), ".") {
		return "", false
	}
	for _, el := range strings.Split(k, "/") {
		if strings.HasPrefix(el, ".") {
			return "", false
		}
	}
	return k, true
}

// value normalises file contents into a value.
func value(b []byte) string {
	s := string(b)
	s = strings.TrimSuffix(s, "\n")
	return strings.TrimSuffix(s, "\r")
}

// Hash returns a stable content hash of data, used as a version when the
// store has none of its own.
func Hash(data map[string]string) string {
	keys := make([]string, 0, len(data))
	for k := range data {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	h := sha256.New()
	for _, k := range keys {
		fmt.Fprintf(h, "%d:%s%d:%s", len(k), k, len(data[k]), data[k])
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}
