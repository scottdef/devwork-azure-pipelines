package kernels

import (
	"math"
	"math/rand/v2"
	"testing"
)

// vec fills a slice with reproducible values. ChaCha8 with a fixed seed is the
// same generator the plan uses for seeded sampling.
func vec(n int, seed byte) []float32 {
	var s [32]byte
	s[0] = seed
	r := rand.New(rand.NewChaCha8(s))
	v := make([]float32, n)
	for i := range v {
		v[i] = float32(r.NormFloat64())
	}
	return v
}

// The deterministic kernel must match the portable reference bit for bit, at
// every length, including the ones that exercise the tail loop.
func TestDeterministicKernelIsBitIdentical(t *testing.T) {
	for n := 0; n <= 259; n++ {
		a, b := vec(n, 1), vec(n, 2)
		got, want := DotDet(a, b), dotDetRef(a, b)
		if math.Float32bits(got) != math.Float32bits(want) {
			t.Fatalf("n=%d: kernel %x, reference %x", n, math.Float32bits(got), math.Float32bits(want))
		}
	}
}

// The fast kernel may differ from the scalar loop in the last bits, but only
// by rounding: it has to stay inside a relative tolerance.
func TestFastKernelIsClose(t *testing.T) {
	for _, n := range []int{1, 7, 8, 9, 31, 32, 33, 100, 1000, 4096} {
		a, b := vec(n, 3), vec(n, 4)
		var exact, mag float64
		for i := range a {
			p := float64(a[i]) * float64(b[i])
			exact += p
			mag += math.Abs(p)
		}
		got := float64(Dot(a, b))
		if d := math.Abs(got - exact); d > 1e-5*mag+1e-6 {
			t.Fatalf("n=%d: got %g, want %g (diff %g)", n, got, exact, d)
		}
	}
}

// Shows that the fast and deterministic contracts really are different
// numbers, which is why the contract has to be chosen, not assumed.
func TestFastAndDeterministicCanDiffer(t *testing.T) {
	differ := 0
	for seed := byte(10); seed < 60; seed++ {
		a, b := vec(4096, seed), vec(4096, seed+100)
		if math.Float32bits(Dot(a, b)) != math.Float32bits(DotDet(a, b)) {
			differ++
		}
	}
	t.Logf("tier=%q: fast and deterministic kernels gave different bits in %d of 50 trials", Tier(), differ)
}

func TestArenaHoldsTensorsOffHeap(t *testing.T) {
	const n = 1 << 16
	ar, err := NewArena(2 * 4 * n)
	if err != nil {
		t.Fatal(err)
	}
	defer ar.Close()
	a, b := ar.Float32s(0, n), ar.Float32s(4*n, n)
	copy(a, vec(n, 5))
	copy(b, vec(n, 6))
	if got, want := DotDet(a, b), dotDetRef(a, b); math.Float32bits(got) != math.Float32bits(want) {
		t.Fatalf("arena-backed result %v differs from reference %v", got, want)
	}
}

var sink float32

func benchDot(b *testing.B, n int, f func(x, y []float32) float32) {
	x, y := vec(n, 7), vec(n, 8)
	b.SetBytes(int64(8 * n))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sink = f(x, y)
	}
}

// 4096 floats per vector stays in the L1/L2 cache: this measures arithmetic.
func BenchmarkDot4kScalar(b *testing.B) { benchDot(b, 4096, dotScalar) }
func BenchmarkDot4kDetRef(b *testing.B) { benchDot(b, 4096, dotDetRef) }
func BenchmarkDot4kDet(b *testing.B)    { benchDot(b, 4096, DotDet) }
func BenchmarkDot4kFast(b *testing.B)   { benchDot(b, 4096, Dot) }

// 16M floats per vector (64 MiB each) does not fit in any cache: this
// measures memory bandwidth, which is what limits token generation.
func BenchmarkDot16MScalar(b *testing.B) { benchDot(b, 1<<24, dotScalar) }
func BenchmarkDot16MFast(b *testing.B)   { benchDot(b, 1<<24, Dot) }
