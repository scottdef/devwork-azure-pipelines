package sample

import (
	"math"
	"testing"
)

func logits(n int) []float32 {
	l := make([]float32, n)
	for i := range l {
		l[i] = float32(math.Sin(float64(i)*0.37) * 3)
	}
	return l
}

func draw(p Params, n int) []int {
	s, l, out := New(p), logits(500), make([]int, n)
	for i := range out {
		out[i] = s.Sample(l)
	}
	return out
}

func TestSameSeedSameChoices(t *testing.T) {
	p := Params{Temperature: 0.8, TopK: 40, TopP: 0.95, Seed: 42}
	a, b := draw(p, 200), draw(p, 200)
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("draw %d differs: %d and %d", i, a[i], b[i])
		}
	}
	p.Seed = 43
	c := draw(p, 200)
	same := 0
	for i := range a {
		if a[i] == c[i] {
			same++
		}
	}
	if same == len(a) {
		t.Fatal("a different seed gave the same 200 choices")
	}
}

func TestGreedyModes(t *testing.T) {
	l := logits(500)
	want := argmax(l)
	for _, p := range []Params{{Temperature: 0}, {Temperature: 1, TopK: 1}} {
		if got := New(p).Sample(l); got != want {
			t.Fatalf("%+v picked %d, want argmax %d", p, got, want)
		}
	}
}

func TestTopKAndTopPRestrictTheChoices(t *testing.T) {
	l := []float32{5, 4, 3, 2, 1, 0}
	s := New(Params{Temperature: 1, TopK: 2, Seed: 1})
	for i := 0; i < 500; i++ {
		if id := s.Sample(l); id > 1 {
			t.Fatalf("top-k 2 picked id %d", id)
		}
	}
	// Probabilities are about 0.63, 0.23, 0.09, ...: top-p 0.8 keeps the first two.
	s = New(Params{Temperature: 1, TopP: 0.8, Seed: 1})
	for i := 0; i < 500; i++ {
		if id := s.Sample(l); id > 1 {
			t.Fatalf("top-p 0.8 picked id %d", id)
		}
	}
}

// Logits of 0 and ln 3 are probabilities 1/4 and 3/4.
func TestDistributionMatchesSoftmax(t *testing.T) {
	l := []float32{0, float32(math.Log(3))}
	s := New(Params{Temperature: 1, Seed: 7})
	const n = 40000
	ones := 0
	for i := 0; i < n; i++ {
		ones += s.Sample(l)
	}
	if f := float64(ones) / n; math.Abs(f-0.75) > 0.01 {
		t.Fatalf("id 1 chosen %.4f of the time, want 0.75", f)
	}
}
