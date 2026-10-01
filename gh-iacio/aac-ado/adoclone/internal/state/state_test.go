package state

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestJournalSurvivesCrash(t *testing.T) {
	p := filepath.Join(t.TempDir(), "state.json")
	s, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetProjects("src", "tgt"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		if err := s.Put("workitem", strconv.Itoa(i), strconv.Itoa(100+i)); err != nil {
			t.Fatal(err)
		}
	}
	// No Close: simulate a killed process. The journal has the puts.
	s2, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := s2.Get("workitem", "9"); got != "109" || s2.TargetProjectID != "tgt" {
		t.Fatalf("after reload: %q %q", got, s2.TargetProjectID)
	}
}

func TestCompactionAndTornLine(t *testing.T) {
	p := filepath.Join(t.TempDir(), "state.json")
	s, _ := Load(p)
	for i := 0; i < compactEvery+5; i++ {
		s.Put("k", strconv.Itoa(i), "v")
	}
	if fi, err := os.Stat(p + ".journal"); err != nil || fi.Size() == 0 {
		t.Fatalf("journal should hold the 5 puts after compaction: %v", err)
	}
	f, _ := os.OpenFile(p+".journal", os.O_APPEND|os.O_WRONLY, 0)
	f.WriteString(`{"k":"k","s":"torn`)
	f.Close()
	s2, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if n := len(s2.Map("k")); n != compactEvery+5 {
		t.Fatalf("got %d entries", n)
	}
	if err := s2.Close(); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(p + ".journal"); fi != nil && fi.Size() != 0 {
		t.Fatal("journal not folded on close")
	}
}

func TestTodosDeduplicate(t *testing.T) {
	p := filepath.Join(t.TempDir(), "state.json")
	s, _ := Load(p)
	s.AddTodo("vargroups", "g / v", "set it")
	s.AddTodo("vargroups", "g / v", "set it")
	s.AddTodo("repos", "r", "enable")
	s2, _ := Load(p)
	if n := len(s2.Todos()); n != 2 {
		t.Fatalf("todos = %d", n)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(p), "todo.json")); err != nil {
		t.Fatal(err)
	}
}

func TestReadOnlyUseWritesNothing(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "state.json")
	s, _ := Load(p)
	s.Get("workitem", "1")
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Fatalf("read-only use created %v", entries)
	}
}

func TestRenameClearsMemory(t *testing.T) {
	p := filepath.Join(t.TempDir(), "state.json")
	s, _ := Load(p)
	s.SetProjects("a", "b")
	s.Put("workitem", "1", "2")
	if err := s.Rename(".old"); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatal("Close after Rename wrote the old checkpoint back")
	}
	if len(s.Map("workitem")) != 0 || s.TargetProjectID != "" {
		t.Fatal("memory not cleared")
	}
}
