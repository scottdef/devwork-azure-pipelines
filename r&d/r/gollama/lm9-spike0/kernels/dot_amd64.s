//go:build amd64 && !noasm

#include "textflag.h"

// func dotAVX2FMA(a, b *float32, n int) float32
// Fast tier: four accumulators, fused multiply-add, 32 floats per iteration.
TEXT ·dotAVX2FMA(SB), NOSPLIT, $0-28
	MOVQ a+0(FP), SI
	MOVQ b+8(FP), DI
	MOVQ n+16(FP), CX
	VXORPS Y0, Y0, Y0
	VXORPS Y1, Y1, Y1
	VXORPS Y2, Y2, Y2
	VXORPS Y3, Y3, Y3

fma32:
	CMPQ CX, $32
	JLT  fma8
	VMOVUPS (SI), Y4
	VMOVUPS 32(SI), Y5
	VMOVUPS 64(SI), Y6
	VMOVUPS 96(SI), Y7
	VFMADD231PS (DI), Y4, Y0
	VFMADD231PS 32(DI), Y5, Y1
	VFMADD231PS 64(DI), Y6, Y2
	VFMADD231PS 96(DI), Y7, Y3
	ADDQ $128, SI
	ADDQ $128, DI
	SUBQ $32, CX
	JMP  fma32

fma8:
	CMPQ CX, $8
	JLT  fmareduce
	VMOVUPS (SI), Y4
	VFMADD231PS (DI), Y4, Y0
	ADDQ $32, SI
	ADDQ $32, DI
	SUBQ $8, CX
	JMP  fma8

fmareduce:
	VADDPS Y1, Y0, Y0
	VADDPS Y3, Y2, Y2
	VADDPS Y2, Y0, Y0
	VEXTRACTF128 $1, Y0, X1
	VADDPS X1, X0, X0
	VMOVHLPS X0, X0, X1
	VADDPS X1, X0, X0
	VMOVSHDUP X0, X1
	VADDSS X1, X0, X0

fmatail:
	TESTQ CX, CX
	JEQ  fmadone
	VMOVSS (SI), X1
	VFMADD231SS (DI), X1, X0
	ADDQ $4, SI
	ADDQ $4, DI
	DECQ CX
	JMP  fmatail

fmadone:
	VZEROUPPER
	MOVSS X0, ret+24(FP)
	RET

// func dotAVX2Det(a, b *float32, n int) float32
// Deterministic tier: one accumulator of eight lanes, separate multiply and
// add (no fusion), and the same reduction tree as dotDetRef in dot.go:
// (l0+l4, l1+l5, l2+l6, l3+l7) -> (t0+t2, t1+t3) -> u0+u1, then the tail in order.
TEXT ·dotAVX2Det(SB), NOSPLIT, $0-28
	MOVQ a+0(FP), SI
	MOVQ b+8(FP), DI
	MOVQ n+16(FP), CX
	VXORPS Y0, Y0, Y0

det8:
	CMPQ CX, $8
	JLT  detreduce
	VMOVUPS (SI), Y1
	VMULPS (DI), Y1, Y1
	VADDPS Y1, Y0, Y0
	ADDQ $32, SI
	ADDQ $32, DI
	SUBQ $8, CX
	JMP  det8

detreduce:
	VEXTRACTF128 $1, Y0, X1
	VADDPS X1, X0, X0
	VMOVHLPS X0, X0, X1
	VADDPS X1, X0, X0
	VMOVSHDUP X0, X1
	VADDSS X1, X0, X0

dettail:
	TESTQ CX, CX
	JEQ  detdone
	VMOVSS (SI), X1
	VMULSS (DI), X1, X1
	VADDSS X1, X0, X0
	ADDQ $4, SI
	ADDQ $4, DI
	DECQ CX
	JMP  dettail

detdone:
	VZEROUPPER
	MOVSS X0, ret+24(FP)
	RET

// func cpuid(eaxArg, ecxArg uint32) (eax, ebx, ecx, edx uint32)
TEXT ·cpuid(SB), NOSPLIT, $0-24
	MOVL eaxArg+0(FP), AX
	MOVL ecxArg+4(FP), CX
	CPUID
	MOVL AX, eax+8(FP)
	MOVL BX, ebx+12(FP)
	MOVL CX, ecx+16(FP)
	MOVL DX, edx+20(FP)
	RET

// func xgetbv() (eax, edx uint32)
TEXT ·xgetbv(SB), NOSPLIT, $0-8
	XORL CX, CX
	XGETBV
	MOVL AX, eax+0(FP)
	MOVL DX, edx+4(FP)
	RET

// func dotQ8F32AVX2(blocks *byte, x *float32, nblocks int) float32
// Dot product of Q8_0 weights with float32 activations, straight from the
// file layout: each 34-byte block is an f16 scale followed by 32 int8.
// Per block: widen int8 to int32, convert to float, multiply by x, sum the
// four vectors, then acc += scale * blocksum. Needs AVX2, FMA and F16C.
// The last load of a block reads bytes 26..33, so it never leaves the block.
TEXT ·dotQ8F32AVX2(SB), NOSPLIT, $0-28
	MOVQ blocks+0(FP), SI
	MOVQ x+8(FP), DI
	MOVQ nblocks+16(FP), CX
	VXORPS Y0, Y0, Y0
	TESTQ CX, CX
	JEQ  q8reduce

q8block:
	MOVWLZX (SI), AX
	VMOVD AX, X5
	VCVTPH2PS X5, X5
	VBROADCASTSS X5, Y5
	VPMOVSXBD 2(SI), Y1
	VPMOVSXBD 10(SI), Y2
	VPMOVSXBD 18(SI), Y3
	VPMOVSXBD 26(SI), Y4
	VCVTDQ2PS Y1, Y1
	VCVTDQ2PS Y2, Y2
	VCVTDQ2PS Y3, Y3
	VCVTDQ2PS Y4, Y4
	VMULPS (DI), Y1, Y1
	VMULPS 32(DI), Y2, Y2
	VMULPS 64(DI), Y3, Y3
	VMULPS 96(DI), Y4, Y4
	VADDPS Y2, Y1, Y1
	VADDPS Y4, Y3, Y3
	VADDPS Y3, Y1, Y1
	VFMADD231PS Y5, Y1, Y0
	ADDQ $34, SI
	ADDQ $128, DI
	DECQ CX
	JNE  q8block

q8reduce:
	VEXTRACTF128 $1, Y0, X1
	VADDPS X1, X0, X0
	VMOVHLPS X0, X0, X1
	VADDPS X1, X0, X0
	VMOVSHDUP X0, X1
	VADDSS X1, X0, X0
	VZEROUPPER
	MOVSS X0, ret+24(FP)
	RET

// func dotAVX512(a, b *float32, n int) float32
// AVX-512: sixteen floats per vector, two accumulators, and a mask register
// for the tail. A masked load does not fault on the elements it skips, so
// there is no scalar loop and no read past the end of the slice.
TEXT ·dotAVX512(SB), NOSPLIT, $0-28
	MOVQ a+0(FP), SI
	MOVQ b+8(FP), DI
	MOVQ n+16(FP), CX
	VXORPS Z0, Z0, Z0
	VXORPS Z3, Z3, Z3

z32:
	CMPQ CX, $32
	JLT  z16
	VMOVUPS (SI), Z1
	VMOVUPS 64(SI), Z2
	VFMADD231PS (DI), Z1, Z0
	VFMADD231PS 64(DI), Z2, Z3
	ADDQ $128, SI
	ADDQ $128, DI
	SUBQ $32, CX
	JMP  z32

z16:
	CMPQ CX, $16
	JLT  ztail
	VMOVUPS (SI), Z1
	VFMADD231PS (DI), Z1, Z0
	ADDQ $64, SI
	ADDQ $64, DI
	SUBQ $16, CX

ztail:
	MOVQ $1, AX
	SHLQ CX, AX
	DECQ AX
	KMOVW AX, K1
	VMOVUPS.Z (SI), K1, Z1
	VMOVUPS.Z (DI), K1, Z2
	VFMADD231PS Z2, Z1, Z3
	VADDPS Z3, Z0, Z0
	VEXTRACTF64X4 $1, Z0, Y1
	VADDPS Y1, Y0, Y0
	VEXTRACTF128 $1, Y0, X1
	VADDPS X1, X0, X0
	VMOVHLPS X0, X0, X1
	VADDPS X1, X0, X0
	VMOVSHDUP X0, X1
	VADDSS X1, X0, X0
	VZEROUPPER
	MOVSS X0, ret+24(FP)
	RET
