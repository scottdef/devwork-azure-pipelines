// Package kernels is spike 0 for the lm9 plan: one float32 dot product written
// three ways, so the plan's claims about kernel tiers, dispatch, determinism
// and memory safety can be checked on a real machine.
package kernels

// Dot returns the dot product of a and b using the fastest kernel the CPU and
// OS support. Results can differ in the last bits between machines, because
// the fast kernels use fused multiply-add and wider accumulators.
func Dot(a, b []float32) float32 {
	if len(a) != len(b) {
		panic("kernels: length mismatch")
	}
	if len(a) == 0 {
		return 0
	}
	return dotFast(a, b)
}

// DotDet returns the dot product under the deterministic numeric contract:
// no fused multiply-add, eight lanes, a fixed reduction tree, then a
// sequential tail. Every kernel that implements this contract returns the
// same bits on every machine, which is what --deterministic is built on.
func DotDet(a, b []float32) float32 {
	if len(a) != len(b) {
		panic("kernels: length mismatch")
	}
	if len(a) == 0 {
		return 0
	}
	return dotDet(a, b)
}

// Tier reports which kernel tier Dot dispatches to on this machine.
func Tier() string { return tier }

// dotScalar is the obvious loop. It is the baseline for benchmarks only.
func dotScalar(a, b []float32) float32 {
	var s float32
	for i := range a {
		s += a[i] * b[i]
	}
	return s
}

// dotDetRef is the portable reference for the deterministic contract. The
// explicit float32 conversions matter: the Go spec lets a compiler fuse
// x*y+z into one operation, and says an explicit conversion prevents it.
func dotDetRef(a, b []float32) float32 {
	var acc [8]float32
	i := 0
	for ; i+8 <= len(a); i += 8 {
		x, y := a[i:i+8:i+8], b[i:i+8:i+8]
		for j := 0; j < 8; j++ {
			acc[j] = acc[j] + float32(x[j]*y[j])
		}
	}
	t0 := acc[0] + acc[4]
	t1 := acc[1] + acc[5]
	t2 := acc[2] + acc[6]
	t3 := acc[3] + acc[7]
	u0 := float32(t0 + t2)
	u1 := float32(t1 + t3)
	s := float32(u0 + u1)
	for ; i < len(a); i++ {
		s = s + float32(a[i]*b[i])
	}
	return s
}
