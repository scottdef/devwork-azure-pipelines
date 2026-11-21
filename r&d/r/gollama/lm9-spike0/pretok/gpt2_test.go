package pretok

import (
	"slices"
	"strings"
	"testing"
)

// Expected splits were worked out by hand from the regular expression in the
// package comment. They have not been compared with a reference tokenizer in
// this spike; in the engine this table is generated from the oracle.
func TestSplitGPT2(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"Hello world", []string{"Hello", " world"}},
		{"Hello  world", []string{"Hello", " ", " world"}},
		{"I'm 2x faster!", []string{"I", "'m", " 2", "x", " faster", "!"}},
		{"don't", []string{"don", "'t"}},
		{"a\n\nb", []string{"a", "\n", "\n", "b"}},
		{"a\n\n", []string{"a", "\n\n"}},
		{"  x", []string{" ", " x"}},
		{"x  ", []string{"x", "  "}},
		{"x ", []string{"x", " "}},
		{"a\tb", []string{"a", "\t", "b"}},
		{"price: $5.00", []string{"price", ":", " $", "5", ".", "00"}},
		{"日本語 text", []string{"日本語", " text"}},
		{"they'll", []string{"they", "'ll"}},
		{" 's", []string{" '", "s"}},
		{"...  ", []string{"...", "  "}},
	}
	for _, c := range cases {
		got := SplitGPT2(c.in)
		if !slices.Equal(got, c.want) {
			t.Errorf("SplitGPT2(%q)\n got  %q\n want %q", c.in, got, c.want)
		}
	}
}

func FuzzSplitIsLossless(f *testing.F) {
	for _, s := range []string{"Hello  world", "a\n\nb", "x  y", "\xff\xfe", "'ll 'd"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		parts := SplitGPT2(s)
		if strings.Join(parts, "") != s {
			t.Fatalf("pieces of %q do not join back to it: %q", s, parts)
		}
		for _, p := range parts {
			if p == "" {
				t.Fatalf("empty piece in %q", parts)
			}
		}
	})
}
