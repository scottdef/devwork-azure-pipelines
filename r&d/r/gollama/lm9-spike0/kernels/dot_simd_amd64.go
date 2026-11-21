//go:build goexperiment.simd && amd64 && !noasm

// NOT COMPILED in the session that produced this spike: the sandbox had Go
// 1.24.7 only. The calls below were written against the simd/archsimd source
// at tag go1.26.11 (LoadFloat32x8Slice, MulAdd, Add, GetLo, GetHi, AddPairs,
// GetElem, X86.AVX2, X86.FMA). Build with:
//
//	GOEXPERIMENT=simd go test -run Simd -bench Simd ./kernels/
//
// archsimd is experimental and outside the Go 1 compatibility promise, so the
// plan keeps it behind this tag and never makes it the only fast path.
package kernels

import "simd/archsimd"

// SimdAvailable reports whether the intrinsics below can run on this CPU.
func SimdAvailable() bool { return archsimd.X86.AVX2() && archsimd.X86.FMA() }

// DotSimd is the same fast kernel as dotAVX2FMA, written with compiler
// intrinsics. Unlike an assembly function it can be inlined into its caller.
func DotSimd(a, b []float32) float32 {
	var acc0, acc1 archsimd.Float32x8
	i := 0
	for ; i+16 <= len(a); i += 16 {
		acc0 = archsimd.LoadFloat32x8Slice(a[i:]).MulAdd(archsimd.LoadFloat32x8Slice(b[i:]), acc0)
		acc1 = archsimd.LoadFloat32x8Slice(a[i+8:]).MulAdd(archsimd.LoadFloat32x8Slice(b[i+8:]), acc1)
	}
	s := acc0.Add(acc1)
	q := s.GetLo().Add(s.GetHi())
	q = q.AddPairs(q)
	sum := q.GetElem(0) + q.GetElem(1)
	for ; i < len(a); i++ {
		sum += a[i] * b[i]
	}
	return sum
}
