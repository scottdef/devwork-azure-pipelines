package diff

import (
	"strings"
	"testing"
)

func TestUnified(t *testing.T) {
	for _, tc := range []struct{ a, b, want string }{
		{"x\n", "x\n", ""},
		{"", "a\nb\n", "--- a\n+++ b\n@@ -0,0 +1,2 @@\n+a\n+b\n"},
		{"a\nb\nc\n", "a\nB\nc\n", "--- a\n+++ b\n@@ -1,3 +1,3 @@\n a\n-b\n+B\n c\n"},
		{
			"1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n11\n12\n13\n14\n15\n",
			"1\nTWO\n3\n4\n5\n6\n7\n8\n9\n10\n11\n12\n13\nFOURTEEN\n15\n",
			"--- a\n+++ b\n@@ -1,5 +1,5 @@\n 1\n-2\n+TWO\n 3\n 4\n 5\n@@ -11,5 +11,5 @@\n 11\n 12\n 13\n-14\n+FOURTEEN\n 15\n",
		},
		{"a\n", "a", "--- a\n+++ b\n\\ No newline at end of file (one or both sides)\n"},
	} {
		if got := Unified("a", "b", tc.a, tc.b); got != tc.want {
			t.Errorf("Unified(%q,%q)\n got:\n%s\nwant:\n%s", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestLarge(t *testing.T) {
	a := strings.Repeat("x\n", 5000)
	b := strings.Repeat("y\n", 5000)
	d := Unified("a", "b", a, b)
	if strings.Count(d, "\n-x") != 5000 || strings.Count(d, "\n+y") != 5000 {
		t.Error("large replace not complete")
	}
}
