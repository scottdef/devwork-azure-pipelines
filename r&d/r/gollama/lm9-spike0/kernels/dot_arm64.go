//go:build arm64 && lm9_neon && !noasm

package kernels

import "unsafe"

//go:noescape
func dotNEON(a, b *float32, n int) float32

// NEON is part of the arm64 baseline, so there is nothing to detect. The
// deterministic and quantized kernels stay on the portable tier here.
var (
	tier    = "neon (Go assembly, opt-in, never executed by its author)"
	dotFast = func(a, b []float32) float32 {
		return dotNEON(unsafe.SliceData(a), unsafe.SliceData(b), len(a))
	}
	dotDet = dotDetRef
	dotQ8  = dotQ8Ref
)
