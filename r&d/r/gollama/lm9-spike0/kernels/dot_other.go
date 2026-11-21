//go:build (!amd64 && !(arm64 && lm9_neon)) || noasm

package kernels

// Every other GOARCH gets the portable tier until it has its own kernels.
// arm64 has a NEON kernel behind the lm9_neon build tag (see dot_arm64.s).
var (
	tier    = "generic"
	dotFast = dotScalar
	dotDet  = dotDetRef
	dotQ8   = dotQ8Ref
)
