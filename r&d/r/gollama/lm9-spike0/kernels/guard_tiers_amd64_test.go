//go:build linux && amd64 && !noasm

package kernels

// guardTiers lists every assembly float kernel this CPU can run, so the
// guard-page test covers the ones Dot does not dispatch to.
func guardTiers() []func(a, b []float32) float32 {
	var t []func(a, b []float32) float32
	if hasAVX2FMA() {
		t = append(t, avx2)
	}
	if hasAVX2FMA() && hasAVX512F() {
		t = append(t, avx512)
	}
	return t
}
