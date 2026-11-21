//go:build linux || darwin || freebsd

package gguf

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenMapsTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tiny.gguf")
	if err := os.WriteFile(path, sample(), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Advise(true); err != nil {
		t.Fatal(err)
	}
	bias := &m.Tensors[1]
	if d := m.Data(m.Bytes, bias); len(d) != 16 || d[0] != 160 {
		t.Fatalf("mapped tensor data is wrong: % x", d)
	}
	if err := m.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(filepath.Join(t.TempDir(), "missing.gguf")); err == nil {
		t.Fatal("opening a missing file succeeded")
	}
}
