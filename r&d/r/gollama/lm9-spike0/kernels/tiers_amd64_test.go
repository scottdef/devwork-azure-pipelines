//go:build amd64 && !noasm

package kernels

import (
	"math"
	"testing"
	"unsafe"
)

// Each assembly tier called directly, so they can be compared on one machine
// regardless of which one Dot dispatches to.
func avx2(a, b []float32) float32 {
	return dotAVX2FMA(unsafe.SliceData(a), unsafe.SliceData(b), len(a))
}

func avx512(a, b []float32) float32 {
	return dotAVX512(unsafe.SliceData(a), unsafe.SliceData(b), len(a))
}

func TestEveryTierAgreesWithinRounding(t *testing.T) {
	tiers := map[string]func(a, b []float32) float32{}
	if hasAVX2FMA() {
		tiers["avx2"] = avx2
	}
	if hasAVX2FMA() && hasAVX512F() {
		tiers["avx512"] = avx512
	}
	for name, f := range tiers {
		for n := 1; n <= 200; n++ {
			a, b := vec(n, 50), vec(n, 51)
			var exact, mag float64
			for i := range a {
				p := float64(a[i]) * float64(b[i])
				exact += p
				mag += math.Abs(p)
			}
			if d := math.Abs(float64(f(a, b)) - exact); d > 1e-5*mag+1e-6 {
				t.Fatalf("%s, n=%d: off by %g", name, n, d)
			}
		}
	}
	t.Logf("tiers checked on this CPU: %d", len(tiers))
}

func BenchmarkDot4kAVX2(b *testing.B) {
	if !hasAVX2FMA() {
		b.Skip("no AVX2+FMA")
	}
	benchDot(b, 4096, avx2)
}

func BenchmarkDot4kAVX512(b *testing.B) {
	if !hasAVX512F() {
		b.Skip("no AVX-512")
	}
	benchDot(b, 4096, avx512)
}

func BenchmarkDot16MAVX2(b *testing.B) {
	if !hasAVX2FMA() {
		b.Skip("no AVX2+FMA")
	}
	benchDot(b, 1<<24, avx2)
}

func BenchmarkDot16MAVX512(b *testing.B) {
	if !hasAVX512F() {
		b.Skip("no AVX-512")
	}
	benchDot(b, 1<<24, avx512)
}
