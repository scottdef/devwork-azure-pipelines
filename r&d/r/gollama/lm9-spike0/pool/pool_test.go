package pool

import (
	"math"
	"math/rand/v2"
	"testing"

	"lm9spike/kernels"
)

// matVec computes out[r] = W[r] . x for a row-major matrix, one row per
// kernel call, rows split across the pool.
func matVec(p *Pool, out, w, x []float32) {
	cols := len(x)
	p.Rows(len(out), func(lo, hi int) {
		for r := lo; r < hi; r++ {
			out[r] = kernels.Dot(w[r*cols:(r+1)*cols], x)
		}
	})
}

func TestResultDoesNotDependOnThreadCount(t *testing.T) {
	const rows, cols = 257, 193
	r := rand.New(rand.NewPCG(7, 9))
	w, x := make([]float32, rows*cols), make([]float32, cols)
	for i := range w {
		w[i] = float32(r.NormFloat64())
	}
	for i := range x {
		x[i] = float32(r.NormFloat64())
	}
	var want []float32
	for _, n := range []int{1, 2, 3, 4, 7} {
		p := New(n, true)
		out := make([]float32, rows)
		matVec(p, out, w, x)
		p.Close()
		if want == nil {
			want = out
			continue
		}
		for i := range out {
			if math.Float32bits(out[i]) != math.Float32bits(want[i]) {
				t.Fatalf("%d workers: row %d differs from the single-worker result", n, i)
			}
		}
	}
}

func TestMoreWorkersThanRows(t *testing.T) {
	p := New(8, false)
	defer p.Close()
	seen := make([]int, 3)
	p.Rows(3, func(lo, hi int) {
		for r := lo; r < hi; r++ {
			seen[r]++
		}
	})
	for r, n := range seen {
		if n != 1 {
			t.Fatalf("row %d computed %d times", r, n)
		}
	}
}

func TestPinningIsAvailableOrSkipped(t *testing.T) {
	cpus := allowedCPUs()
	t.Logf("CPUs this process may use: %v", cpus)
	for _, c := range cpus {
		if c < 0 {
			t.Fatal("negative CPU index")
		}
	}
}
