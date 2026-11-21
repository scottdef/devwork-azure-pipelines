package ops

import (
	"math"
	"math/rand/v2"
	"testing"
)

func rvec(n int, seed uint64) []float32 {
	r := rand.New(rand.NewPCG(seed, 1))
	v := make([]float32, n)
	for i := range v {
		v[i] = float32(r.NormFloat64())
	}
	return v
}

func dot(a, b []float32) (s float64) {
	for i := range a {
		s += float64(a[i]) * float64(b[i])
	}
	return s
}

func TestRMSNormGivesUnitRMS(t *testing.T) {
	x, w, out := rvec(64, 1), make([]float32, 64), make([]float32, 64)
	for i := range w {
		w[i] = 1
	}
	RMSNorm(out, x, w, 1e-6)
	if rms := math.Sqrt(dot(out, out) / 64); math.Abs(rms-1) > 1e-3 {
		t.Fatalf("rms = %g", rms)
	}
}

func TestSoftmax(t *testing.T) {
	x := []float32{1, 2, 3, 1000} // would overflow without the shift
	Softmax(x)
	var sum float64
	for _, v := range x {
		sum += float64(v)
	}
	if math.Abs(sum-1) > 1e-6 || x[3] < 0.999 || x[0] > x[1] {
		t.Fatalf("softmax = %v (sum %g)", x, sum)
	}
}

// The defining property of rotary encoding: the score between a query at
// position m and a key at position n depends only on m - n.
func TestRoPEDependsOnlyOnDistance(t *testing.T) {
	q, k := rvec(64, 2), rvec(64, 3)
	score := func(m, n int) float64 {
		a, b := append([]float32(nil), q...), append([]float32(nil), k...)
		RoPE(a, m, 10000)
		RoPE(b, n, 10000)
		return dot(a, b)
	}
	if a, b := score(5, 3), score(105, 103); math.Abs(a-b) > 1e-3 {
		t.Fatalf("same distance, different score: %g and %g", a, b)
	}
	if a, b := score(5, 3), score(5, 4); math.Abs(a-b) < 1e-6 {
		t.Fatal("different distance, same score")
	}
	x := rvec(64, 4)
	before := dot(x, x)
	RoPE(x, 77, 10000)
	if math.Abs(dot(x, x)-before) > 1e-3 {
		t.Fatal("rotation changed the vector's length")
	}
}

// With one key that matches the query far better than the others, attention
// returns that key's value.
func TestAttendPicksTheMatchingKey(t *testing.T) {
	const d, n = 16, 5
	q := rvec(d, 5)
	keys, values := rvec(n*d, 6), rvec(n*d, 7)
	for i := range q {
		keys[3*d+i] = 20 * q[i]
	}
	out, scores := make([]float32, d), make([]float32, n)
	Attend(out, q, keys, values, scores)
	for i := range out {
		if math.Abs(float64(out[i]-values[3*d+i])) > 1e-3 {
			t.Fatalf("out[%d] = %g, want %g (weights %v)", i, out[i], values[3*d+i], scores)
		}
	}
}

func TestSiLU(t *testing.T) {
	if SiLU(0) != 0 || math.Abs(float64(SiLU(10))-10) > 1e-3 || math.Abs(float64(SiLU(-10))) > 1e-3 {
		t.Fatal("SiLU is wrong at 0 or at the tails")
	}
}
