// Package state is ghlog2dt's checkpoint: one small JSON file per
// repository, so parallel shards never touch the same file, and a merge
// rule (max / union) so two writers converge without coordination.
package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Version is the file format version.
const Version = 1

// ErrIO wraps every failure to read or write a checkpoint file.
var ErrIO = errors.New("state")

// State is the checkpoint for one repository.
type State struct {
	Version   int                 `json:"version"`
	Repo      string              `json:"repo"`
	HighWater time.Time           `json:"high_water,omitzero"`
	Pending   map[int64]time.Time `json:"pending_runs"` // run ID -> first seen unfinished
	Shipped   map[int64]time.Time `json:"shipped_jobs"` // job ID -> shipped at
	UpdatedAt time.Time           `json:"updated_at,omitzero"`
}

// New returns an empty state for repo.
func New(repo string) *State {
	return &State{Version: Version, Repo: repo, Pending: map[int64]time.Time{}, Shipped: map[int64]time.Time{}}
}

var nameRe = regexp.MustCompile(`^[A-Za-z0-9-]+__[A-Za-z0-9._-]+\.json$`)

// FileName maps owner/name to owner__name.json.
func FileName(repo string) (string, error) {
	owner, name, ok := strings.Cut(repo, "/")
	f := owner + "__" + name + ".json"
	if !ok || strings.Contains(name, "/") || !nameRe.MatchString(f) || strings.Contains(f, "..") {
		return "", fmt.Errorf("state: bad repo name %q", repo)
	}
	return f, nil
}

// Load reads dir/owner__name.json; a missing file yields a fresh state.
func Load(dir, repo string) (*State, error) {
	f, err := FileName(repo)
	if err != nil {
		return nil, err
	}
	s, err := ReadFile(filepath.Join(dir, f))
	if errors.Is(err, os.ErrNotExist) {
		return New(repo), nil
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrIO, err)
	}
	if !strings.EqualFold(s.Repo, repo) {
		return nil, fmt.Errorf("%w: %s holds repo %q, want %q", ErrIO, f, s.Repo, repo)
	}
	return s, nil
}

// ReadFile reads one state file.
func ReadFile(path string) (*State, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var s State
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("state: %s: %w", path, err)
	}
	if s.Version != Version {
		return nil, fmt.Errorf("state: %s: version %d, want %d", path, s.Version, Version)
	}
	if s.Pending == nil {
		s.Pending = map[int64]time.Time{}
	}
	if s.Shipped == nil {
		s.Shipped = map[int64]time.Time{}
	}
	return &s, nil
}

// Save writes atomically: temp file in the same directory, fsync,
// rename. (os.Root would confine this further, but Root.Rename only
// arrives in Go 1.25; FileName's whitelist does the confining here.)
func (s *State) Save(dir string, now time.Time) error {
	if err := s.save(dir, now); err != nil {
		return fmt.Errorf("%w: save %s: %w", ErrIO, s.Repo, err)
	}
	return nil
}

func (s *State) save(dir string, now time.Time) error {
	f, err := FileName(s.Repo)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	s.UpdatedAt = now.UTC()
	b, err := s.Marshal()
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".state-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(dir, f))
}

// Marshal renders the state as indented JSON with a trailing newline,
// so git diffs of the state branch stay readable.
func (s *State) Marshal() ([]byte, error) {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// IsShipped reports whether a job has been shipped.
func (s *State) IsShipped(job int64) bool { _, ok := s.Shipped[job]; return ok }

// MarkShipped records a shipped job.
func (s *State) MarkShipped(job int64, t time.Time) {
	if _, ok := s.Shipped[job]; !ok {
		s.Shipped[job] = t.UTC()
	}
}

// AddPending remembers an unfinished run, keeping the first-seen time.
func (s *State) AddPending(run int64, t time.Time) {
	if _, ok := s.Pending[run]; !ok {
		s.Pending[run] = t.UTC()
	}
}

// RemovePending forgets a run.
func (s *State) RemovePending(run int64) { delete(s.Pending, run) }

// Prune drops shipped IDs older than shippedTTL (they can no longer be
// listed: lookback is bounded by max_age) and pending runs older than
// pendingTTL (their lines would be too old for Dynatrace anyway). It
// returns the pending runs it abandoned.
func (s *State) Prune(now time.Time, shippedTTL, pendingTTL time.Duration) []int64 {
	for id, t := range s.Shipped {
		if now.Sub(t) > shippedTTL {
			delete(s.Shipped, id)
		}
	}
	var dropped []int64
	for id, t := range s.Pending {
		if now.Sub(t) > pendingTTL {
			delete(s.Pending, id)
			dropped = append(dropped, id)
		}
	}
	return dropped
}

// Merge combines two checkpoints of the same repo: latest high-water
// mark, union of pending runs and shipped jobs with the earliest
// timestamps. It is commutative, associative and idempotent, so the
// commit step can merge any number of writers in any order.
func Merge(a, b *State) (*State, error) {
	if !strings.EqualFold(a.Repo, b.Repo) {
		return nil, fmt.Errorf("state: cannot merge %q with %q", a.Repo, b.Repo)
	}
	m := New(a.Repo)
	m.HighWater = later(a.HighWater, b.HighWater)
	m.UpdatedAt = later(a.UpdatedAt, b.UpdatedAt)
	for _, src := range []*State{a, b} {
		for id, t := range src.Pending {
			m.Pending[id] = earlier(m.Pending[id], t)
		}
		for id, t := range src.Shipped {
			m.Shipped[id] = earlier(m.Shipped[id], t)
		}
	}
	return m, nil
}

func later(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

func earlier(a, b time.Time) time.Time {
	if a.IsZero() || (!b.IsZero() && b.Before(a)) {
		return b
	}
	return a
}
