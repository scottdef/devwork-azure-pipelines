// Package testutil locates repository files from tests.
package testutil

import (
	"os"
	"path/filepath"
	"testing"
)

// Root returns the repository root (the directory containing go.mod).
func Root(t testing.TB) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

// Path joins elements onto the repository root.
func Path(t testing.TB, elem ...string) string {
	return filepath.Join(append([]string{Root(t)}, elem...)...)
}
