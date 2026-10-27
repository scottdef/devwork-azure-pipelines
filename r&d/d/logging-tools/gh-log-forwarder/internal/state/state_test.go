package state

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

var t0 = time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)

func TestFileName(t *testing.T) {
	good := map[string]string{"CoolGitOrg/app-a": "CoolGitOrg__app-a.json", "o/x.y_z": "o__x.y_z.json"}
	for in, want := range good {
		got, err := FileName(in)
		if err != nil || got != want {
			t.Errorf("FileName(%q) = %q, %v", in, got, err)
		}
	}
	for _, bad := range []string{"", "noslash", "a/b/c", "../x", "o/..", "o/a b", "o/x/../../etc"} {
		if _, err := FileName(bad); err == nil {
			t.Errorf("FileName(%q) accepted", bad)
		}
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := New("CoolGitOrg/app-a")
	s.HighWater = t0
	s.AddPending(11, t0)
	s.MarkShipped(22, t0)
	if err := s.Save(dir, t0); err != nil {
		t.Fatal(err)
	}
	got, err := Load(dir, "CoolGitOrg/app-a")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, s) {
		t.Fatalf("got %+v\nwant %+v", got, s)
	}
	ents, _ := os.ReadDir(dir)
	if len(ents) != 1 {
		t.Fatalf("temp files left behind: %v", ents)
	}
	fresh, err := Load(dir, "CoolGitOrg/other")
	if err != nil || !fresh.HighWater.IsZero() || len(fresh.Shipped) != 0 {
		t.Fatalf("missing file should be fresh state: %+v %v", fresh, err)
	}
}

func TestLoadRejectsWrongRepoAndGarbage(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "o__a.json"), []byte(`{"version":1,"repo":"o/b"}`), 0o644)
	if _, err := Load(dir, "o/a"); !errors.Is(err, ErrIO) {
		t.Fatalf("wrong repo: %v", err)
	}
	os.WriteFile(filepath.Join(dir, "o__c.json"), []byte(`{nope`), 0o644)
	if _, err := Load(dir, "o/c"); !errors.Is(err, ErrIO) {
		t.Fatalf("garbage: %v", err)
	}
	os.WriteFile(filepath.Join(dir, "o__d.json"), []byte(`{"version":9,"repo":"o/d"}`), 0o644)
	if _, err := Load(dir, "o/d"); !errors.Is(err, ErrIO) {
		t.Fatalf("version: %v", err)
	}
}

func TestPrune(t *testing.T) {
	s := New("o/a")
	s.MarkShipped(1, t0.Add(-49*time.Hour))
	s.MarkShipped(2, t0.Add(-1*time.Hour))
	s.AddPending(3, t0.Add(-13*time.Hour))
	s.AddPending(4, t0.Add(-1*time.Hour))
	dropped := s.Prune(t0, 48*time.Hour, 12*time.Hour)
	if s.IsShipped(1) || !s.IsShipped(2) || len(dropped) != 1 || dropped[0] != 3 || len(s.Pending) != 1 {
		t.Fatalf("after prune: %+v dropped=%v", s, dropped)
	}
}

func TestMergeLaws(t *testing.T) {
	a := New("o/a")
	a.HighWater = t0
	a.MarkShipped(1, t0)
	a.AddPending(10, t0)
	b := New("O/A") // names are case-insensitive
	b.HighWater = t0.Add(-time.Hour)
	b.MarkShipped(1, t0.Add(-time.Minute))
	b.MarkShipped(2, t0)
	b.AddPending(11, t0)
	c := New("o/a")
	c.MarkShipped(3, t0)

	ab, _ := Merge(a, b)
	ba, _ := Merge(b, a)
	if !ab.HighWater.Equal(t0) || len(ab.Shipped) != 2 || len(ab.Pending) != 2 {
		t.Fatalf("merge: %+v", ab)
	}
	if !ab.Shipped[1].Equal(t0.Add(-time.Minute)) {
		t.Error("merge should keep the earliest shipped time")
	}
	if !reflect.DeepEqual(ab.Shipped, ba.Shipped) || !reflect.DeepEqual(ab.Pending, ba.Pending) || !ab.HighWater.Equal(ba.HighWater) {
		t.Error("merge is not commutative")
	}
	abc1, _ := Merge(ab, c)
	bc, _ := Merge(b, c)
	abc2, _ := Merge(a, bc)
	if !reflect.DeepEqual(abc1.Shipped, abc2.Shipped) || !reflect.DeepEqual(abc1.Pending, abc2.Pending) {
		t.Error("merge is not associative")
	}
	aa, _ := Merge(a, a)
	if !reflect.DeepEqual(aa.Shipped, a.Shipped) || !aa.HighWater.Equal(a.HighWater) {
		t.Error("merge is not idempotent")
	}
	if _, err := Merge(a, New("o/z")); err == nil {
		t.Error("merging different repos must fail")
	}
}
