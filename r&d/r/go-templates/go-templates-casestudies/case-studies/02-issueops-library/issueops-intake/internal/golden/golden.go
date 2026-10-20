// Package golden compares test output with files under testdata/golden.
// Run "go test ./... -update" to rewrite them after an intended change.
// Only tests import it, so the -update flag never reaches a binary.
package golden

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"testing"
)

var update = flag.Bool("update", false, "rewrite golden files")

// Check compares got with the golden file at path (relative to the test's
// package directory), rewriting it when -update is set.
func Check(t testing.TB, path string, got []byte) {
	t.Helper()
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run go test -update to create it)", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("%s differs from output:\n--- want\n%s\n--- got\n%s", path, clip(want), clip(got))
	}
}

func clip(b []byte) []byte {
	if len(b) > 4000 {
		return append(b[:4000:4000], "\n..."...)
	}
	return b
}
