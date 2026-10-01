// Package state keeps the resumable checkpoint: source->target ID maps, phase
// status, and the list of manual follow-ups (todo.json).
//
// Every Put is appended to a journal (state.json.journal) so a 10,000-item run
// doesn't rewrite the whole checkpoint per item; the journal is folded into
// state.json every few hundred writes and on Save. Load replays the journal, so
// a killed Pod loses nothing it had already recorded.
package state

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

const compactEvery = 500

// Todo is a manual follow-up the tool can't do itself (secrets, agents, PRs...).
type Todo struct {
	Component string `json:"component"`
	Item      string `json:"item"`
	Action    string `json:"action"`
}

// State is the checkpoint. Methods are safe for concurrent use.
type State struct {
	mu      sync.Mutex
	path    string
	journal *os.File
	pending int
	dirty   bool
	todos   []Todo
	todoFn  string

	SourceProjectID string                       `json:"sourceProjectId"`
	TargetProjectID string                       `json:"targetProjectId"`
	Maps            map[string]map[string]string `json:"maps"`
	Phases          map[string]map[string]string `json:"phases"`
}

type entry struct {
	K string `json:"k"`
	S string `json:"s"`
	T string `json:"t"`
}

// Load reads path (if it exists), replays its journal and reads todo.json.
func Load(path string) (*State, error) {
	s := &State{path: path, Maps: map[string]map[string]string{}, Phases: map[string]map[string]string{}}
	b, err := os.ReadFile(path)
	switch {
	case err == nil:
		if err := json.Unmarshal(b, s); err != nil {
			return nil, err
		}
		if s.Maps == nil {
			s.Maps = map[string]map[string]string{}
		}
		if s.Phases == nil {
			s.Phases = map[string]map[string]string{}
		}
	case !errors.Is(err, os.ErrNotExist):
		return nil, err
	}
	s.path = path
	if f, err := os.Open(path + ".journal"); err == nil {
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)
		for sc.Scan() {
			var e entry
			if json.Unmarshal(sc.Bytes(), &e) != nil {
				continue // torn last line
			}
			s.set(e.K, e.S, e.T)
		}
		f.Close()
	}
	if b, err := os.ReadFile(s.TodoPath()); err == nil {
		_ = json.Unmarshal(b, &s.todos)
	}
	return s, nil
}

func (s *State) set(kind, src, tgt string) {
	s.dirty = true
	if s.Maps[kind] == nil {
		s.Maps[kind] = map[string]string{}
	}
	s.Maps[kind][src] = tgt
}

// Get returns the target ID recorded for a source ID.
func (s *State) Get(kind, src string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.Maps[kind][src]
	return v, ok
}

// Map returns a copy of one map.
func (s *State) Map(kind string) map[string]string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]string, len(s.Maps[kind]))
	for k, v := range s.Maps[kind] {
		out[k] = v
	}
	return out
}

// Kinds lists the map names in use.
func (s *State) Kinds() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.Maps))
	for k := range s.Maps {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Put records a mapping and appends it to the journal.
func (s *State) Put(kind, src, tgt string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.set(kind, src, tgt)
	if s.journal == nil {
		f, err := os.OpenFile(s.path+".journal", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			return err
		}
		s.journal = f
	}
	line, _ := json.Marshal(entry{K: kind, S: src, T: tgt})
	if _, err := s.journal.Write(append(line, '\n')); err != nil {
		return err
	}
	s.pending++
	if s.pending >= compactEvery {
		return s.compact()
	}
	return nil
}

// SetPhase records phase metadata and saves.
func (s *State) SetPhase(p, k, v string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Phases[p] == nil {
		s.Phases[p] = map[string]string{}
	}
	s.Phases[p][k] = v
	s.dirty = true
	return s.compact()
}

// SetProjects records both project IDs and saves.
func (s *State) SetProjects(src, tgt string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if src != "" {
		s.SourceProjectID = src
	}
	if tgt != "" {
		s.TargetProjectID = tgt
	}
	s.dirty = true
	return s.compact()
}

// Save folds the journal into state.json.
func (s *State) Save() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.compact()
}

// Close saves and releases the journal.
func (s *State) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	err := s.compact()
	if s.journal != nil {
		s.journal.Close()
		s.journal = nil
	}
	return err
}

// compact writes state.json and empties the journal. It does nothing when
// nothing changed, so read-only commands never create a checkpoint.
func (s *State) compact() error {
	if !s.dirty {
		return nil
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	if err := writeAtomic(s.path, b); err != nil {
		return err
	}
	if s.journal != nil {
		if err := s.journal.Truncate(0); err != nil {
			return err
		}
	} else if err := os.Remove(s.path + ".journal"); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	s.pending = 0
	s.dirty = false
	return nil
}

// TodoPath is todo.json (or the file set by SetTodoFile) next to the state file.
func (s *State) TodoPath() string {
	name := s.todoFn
	if name == "" {
		name = "todo.json"
	}
	return filepath.Join(filepath.Dir(s.path), name)
}

// SetTodoFile switches the follow-up list to another file next to the state
// file (dry runs use todo-dryrun.json so they don't leave entries for the real run).
func (s *State) SetTodoFile(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.todoFn = name
	s.todos = nil
	if b, err := os.ReadFile(s.TodoPath()); err == nil {
		_ = json.Unmarshal(b, &s.todos)
	}
}

// AddTodo records a manual follow-up once and rewrites todo.json.
func (s *State) AddTodo(component, item, action string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := Todo{component, item, action}
	for _, x := range s.todos {
		if x == t {
			return nil
		}
	}
	s.todos = append(s.todos, t)
	b, _ := json.MarshalIndent(s.todos, "", "  ")
	return writeAtomic(s.TodoPath(), b)
}

// Todos returns the recorded follow-ups.
func (s *State) Todos() []Todo {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Todo(nil), s.todos...)
}

// Rename moves the checkpoint aside (used after cleanup deletes the target)
// and clears it in memory, so nothing writes the old mappings back.
func (s *State) Rename(suffix string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.journal != nil {
		s.journal.Close()
		s.journal = nil
	}
	for _, p := range []string{s.path, s.path + ".journal", s.TodoPath()} {
		if err := os.Rename(p, p+suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	s.Maps = map[string]map[string]string{}
	s.Phases = map[string]map[string]string{}
	s.SourceProjectID, s.TargetProjectID = "", ""
	s.todos = nil
	s.pending = 0
	s.dirty = false
	return nil
}

func writeAtomic(path string, b []byte) error {
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(b); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
