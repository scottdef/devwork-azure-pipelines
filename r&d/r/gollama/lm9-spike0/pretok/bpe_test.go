package pretok

import (
	"slices"
	"testing"
)

func TestMergeBPE(t *testing.T) {
	rank := map[[2]string]int{{"l", "o"}: 0, {"lo", "w"}: 1, {"e", "r"}: 2, {"n", "e"}: 3}
	for _, c := range []struct {
		in   string
		want []string
	}{
		{"lower", []string{"low", "er"}},
		{"lowlow", []string{"low", "low"}},
		{"newer", []string{"ne", "w", "er"}},
		{"xyz", []string{"x", "y", "z"}},
		{"o", []string{"o"}},
		{"", nil},
	} {
		if got := MergeBPE(c.in, rank); !slices.Equal(got, c.want) {
			t.Errorf("MergeBPE(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// Pre-tokenize, then merge each piece: the two halves of a BPE tokenizer.
func TestPipeline(t *testing.T) {
	rank := map[[2]string]int{{"l", "o"}: 0, {"lo", "w"}: 1, {" ", "l"}: 2}
	var got []string
	for _, piece := range SplitGPT2("low low") {
		got = append(got, MergeBPE(piece, rank)...)
	}
	// " low" merges l+o, then lo+w; " l" can no longer form because l is gone.
	if want := []string{"low", " ", "low"}; !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}
