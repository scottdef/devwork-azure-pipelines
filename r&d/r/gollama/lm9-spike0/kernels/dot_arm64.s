//go:build arm64 && lm9_neon && !noasm

// NEVER EXECUTED. This file assembles and passes vet for arm64, but the
// session that wrote it had no ARM machine or emulator. It is opt-in for
// that reason: build with -tags lm9_neon and run the package tests on real
// hardware before trusting it. The same tests that check the AVX2 kernels
// (reference tolerance, every length, guard pages) apply unchanged.

#include "textflag.h"

// func dotNEON(a, b *float32, n int) float32
// Four floats per vector, fused multiply-add, one accumulator. Go 1.26's
// arm64 assembler has VFMLA but no vector float add, so the four lanes are
// moved to scalar registers and summed there.
TEXT ·dotNEON(SB), NOSPLIT, $0-28
	MOVD a+0(FP), R0
	MOVD b+8(FP), R1
	MOVD n+16(FP), R2
	VEOR V0.B16, V0.B16, V0.B16

neon4:
	CMP  $4, R2
	BLT  neonreduce
	VLD1.P 16(R0), [V1.S4]
	VLD1.P 16(R1), [V2.S4]
	VFMLA V1.S4, V2.S4, V0.S4
	SUB  $4, R2
	B    neon4

neonreduce:
	VMOV V0.S[0], R3
	VMOV V0.S[1], R4
	VMOV V0.S[2], R5
	VMOV V0.S[3], R6
	FMOVS R3, F0
	FMOVS R4, F4
	FMOVS R5, F5
	FMOVS R6, F6
	FADDS F4, F0, F0
	FADDS F6, F5, F5
	FADDS F5, F0, F0

neontail:
	CBZ  R2, neondone
	FMOVS.P 4(R0), F1
	FMOVS.P 4(R1), F2
	FMADDS F1, F0, F2, F0
	SUB  $1, R2
	B    neontail

neondone:
	FMOVS F0, ret+24(FP)
	RET
