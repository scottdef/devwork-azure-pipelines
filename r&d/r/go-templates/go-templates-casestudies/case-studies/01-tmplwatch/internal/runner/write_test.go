package runner

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestWriteAtomic(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "sub", "out.conf")
	ch, err := WriteAtomic(p, []byte("one\n"), 0o640, true)
	if err != nil || !ch {
		t.Fatal(err, ch)
	}
	if b, _ := os.ReadFile(p); string(b) != "one\n" {
		t.Fatalf("content %q", b)
	}
	if runtime.GOOS != "windows" {
		if fi, _ := os.Stat(p); fi.Mode().Perm() != 0o640 {
			t.Errorf("mode %v", fi.Mode())
		}
	}
	if _, err := os.Stat(p + ".bak"); !os.IsNotExist(err) {
		t.Error("backup made for a new file")
	}
	// Identical content: no write, inode unchanged.
	before, _ := os.Stat(p)
	ch, err = WriteAtomic(p, []byte("one\n"), 0o640, true)
	if err != nil || ch {
		t.Fatal("identical content rewritten", err)
	}
	after, _ := os.Stat(p)
	if !os.SameFile(before, after) {
		t.Error("identical content replaced the file")
	}
	// Identical content, drifted mode: repaired, not rewritten.
	os.Chmod(p, 0o600)
	if ch, _ := WriteAtomic(p, []byte("one\n"), 0o640, true); ch {
		t.Error("mode repair reported as change")
	}
	if fi, _ := os.Stat(p); fi.Mode().Perm() != 0o640 {
		t.Errorf("mode not repaired: %v", fi.Mode())
	}
	// Change: rename semantics mean a reader holding the old file still
	// sees the old content.
	held, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()
	if ch, err := WriteAtomic(p, []byte("two\n"), 0o640, true); err != nil || !ch {
		t.Fatal(err)
	}
	old := make([]byte, 16)
	n, _ := held.Read(old)
	if string(old[:n]) != "one\n" {
		t.Errorf("open handle saw %q; rename should leave it on the old inode", old[:n])
	}
	if b, _ := os.ReadFile(p); string(b) != "two\n" {
		t.Errorf("new content %q", b)
	}
	if b, _ := os.ReadFile(p + ".bak"); string(b) != "one\n" {
		t.Errorf("backup %q", b)
	}
	assertNoTemps(t, filepath.Dir(p))
}

func TestWriteAtomicCrash(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "out")
	os.WriteFile(p, []byte("good"), 0o644)
	// Fail between write and rename, as a full disk or I/O error would.
	syncFile = func(*os.File) error { return errors.New("injected fsync failure") }
	defer func() { syncFile = (*os.File).Sync }()
	if _, err := WriteAtomic(p, []byte("half"), 0o644, false); err == nil || !strings.Contains(err.Error(), "injected") {
		t.Fatalf("want injected error, got %v", err)
	}
	if b, _ := os.ReadFile(p); string(b) != "good" {
		t.Errorf("destination damaged: %q", b)
	}
	assertNoTemps(t, dir)

	// A process killed after CreateTemp leaves a temp file; the next start
	// removes it and nothing else.
	stale := filepath.Join(dir, ".out"+tempMark+"12345")
	os.WriteFile(stale, []byte("partial"), 0o600)
	os.WriteFile(filepath.Join(dir, ".bak"+tempMark+"x"), nil, 0o600) // not ours
	gone, err := CleanStale(p)
	if err != nil || len(gone) != 1 {
		t.Fatalf("CleanStale: %v %v", gone, err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Error("stale temp file survived")
	}
	if b, _ := os.ReadFile(p); string(b) != "good" {
		t.Errorf("destination damaged by cleanup: %q", b)
	}
	if gone, err := CleanStale(filepath.Join(dir, "nodir", "x")); err != nil || gone != nil {
		t.Errorf("missing dir: %v %v", gone, err)
	}
}

func TestWriteAtomicRelative(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	for _, p := range []string{"plain", "./dotted", "sub//unclean"} {
		if _, err := WriteAtomic(p, []byte(p), 0o644, true); err != nil {
			t.Errorf("%s: %v", p, err)
		}
		if b, _ := os.ReadFile(p); string(b) != p {
			t.Errorf("%s: read %q", p, b)
		}
	}
	assertNoTemps(t, ".")
}

func assertNoTemps(t *testing.T, dir string) {
	t.Helper()
	ents, _ := os.ReadDir(dir)
	for _, e := range ents {
		if strings.Contains(e.Name(), tempMark) {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}
}
