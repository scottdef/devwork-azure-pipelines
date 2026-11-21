//go:build amd64 && !noasm

package kernels

import "unsafe"

// Implemented in dot_amd64.s. Assembly functions use the stack-based ABI0 and
// are never inlined, so kernels are called once per row or block, never once
// per element.

//go:noescape
func dotAVX2FMA(a, b *float32, n int) float32

//go:noescape
func dotAVX2Det(a, b *float32, n int) float32

//go:noescape
func dotQ8F32AVX2(blocks *byte, x *float32, nblocks int) float32

//go:noescape
func dotAVX512(a, b *float32, n int) float32

func cpuid(eaxArg, ecxArg uint32) (eax, ebx, ecx, edx uint32)
func xgetbv() (eax, edx uint32)

var (
	tier    = "generic"
	dotFast = dotScalar
	dotDet  = dotDetRef
	dotQ8   = dotQ8Ref
)

func init() {
	if !hasAVX2FMA() {
		return
	}
	tier = "avx2+fma (Go assembly)"
	dotFast = func(a, b []float32) float32 {
		return dotAVX2FMA(unsafe.SliceData(a), unsafe.SliceData(b), len(a))
	}
	dotDet = func(a, b []float32) float32 {
		return dotAVX2Det(unsafe.SliceData(a), unsafe.SliceData(b), len(a))
	}
	if hasAVX512F() {
		tier = "avx-512 (Go assembly)"
		dotFast = func(a, b []float32) float32 {
			return dotAVX512(unsafe.SliceData(a), unsafe.SliceData(b), len(a))
		}
	}
	if hasF16C() {
		dotQ8 = func(blocks []byte, x []float32) float32 {
			return dotQ8F32AVX2(unsafe.SliceData(blocks), unsafe.SliceData(x), len(blocks)/34)
		}
	}
}

// hasF16C reports the half-precision conversion instructions (CPUID.1:ECX bit 29).
func hasF16C() bool {
	_, _, ecx1, _ := cpuid(1, 0)
	return ecx1&(1<<29) != 0
}

// hasAVX2FMA checks the CPU and the operating system. The OS half matters:
// a kernel that does not save YMM state (XCR0 bits 1 and 2) makes AVX unusable
// even on a CPU that has it, so this is what keeps the fast tier off on such
// systems without a per-OS special case.
func hasAVX2FMA() bool {
	maxLeaf, _, _, _ := cpuid(0, 0)
	if maxLeaf < 7 {
		return false
	}
	_, _, ecx1, _ := cpuid(1, 0)
	const osxsave, avx, fma = 1 << 27, 1 << 28, 1 << 12
	if ecx1&osxsave == 0 || ecx1&avx == 0 || ecx1&fma == 0 {
		return false
	}
	if xcr0, _ := xgetbv(); xcr0&6 != 6 {
		return false
	}
	_, ebx7, _, _ := cpuid(7, 0)
	const avx2 = 1 << 5
	return ebx7&avx2 != 0
}

// hasAVX512F needs the CPU feature (CPUID.7:EBX bit 16) and an OS that saves
// the opmask and ZMM state (XCR0 bits 5, 6 and 7) as well as SSE and AVX.
func hasAVX512F() bool {
	_, ebx7, _, _ := cpuid(7, 0)
	xcr0, _ := xgetbv()
	return ebx7&(1<<16) != 0 && xcr0&0xe6 == 0xe6
}
