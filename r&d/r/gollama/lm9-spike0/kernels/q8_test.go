package kernels

import (
	"math"
	"testing"

	"lm9spike/quant"
)

func q8Fixture(nblocks int, seed byte) (blocks []byte, x []float32) {
	w := vec(nblocks*32, seed)
	blocks = make([]byte, nblocks*quant.Q8_0Bytes)
	if err := quant.QuantQ8_0(blocks, w); err != nil {
		panic(err)
	}
	return blocks, vec(nblocks*32, seed+1)
}

// The kernel reads the file layout directly, so it is checked two ways:
// against the portable reference, and against "expand, then float dot".
func TestDotQ8MatchesReferenceAndExpanded(t *testing.T) {
	for _, n := range []int{0, 1, 2, 3, 7, 64, 129} {
		blocks, x := q8Fixture(n, 20)
		got := float64(DotQ8F32(blocks, x))
		ref := float64(dotQ8Ref(blocks, x))
		w := make([]float32, n*32)
		if err := quant.DequantQ8_0(w, blocks); err != nil {
			t.Fatal(err)
		}
		var exact, mag float64
		for i := range w {
			p := float64(w[i]) * float64(x[i])
			exact += p
			mag += math.Abs(p)
		}
		tol := 1e-5*mag + 1e-6
		if math.Abs(got-ref) > tol || math.Abs(got-exact) > tol {
			t.Fatalf("blocks=%d: kernel %g, reference %g, expanded %g", n, got, ref, exact)
		}
	}
}

func BenchmarkDotQ8Ref(b *testing.B) {
	blocks, x := q8Fixture(128, 30) // 4,096 weights
	b.SetBytes(int64(len(blocks) + 4*len(x)))
	for i := 0; i < b.N; i++ {
		sink = dotQ8Ref(blocks, x)
	}
}

func BenchmarkDotQ8Kernel(b *testing.B) {
	blocks, x := q8Fixture(128, 30)
	b.SetBytes(int64(len(blocks) + 4*len(x)))
	for i := 0; i < b.N; i++ {
		sink = DotQ8F32(blocks, x)
	}
}
