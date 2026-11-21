// Package quant holds the block formats that GGUF weights are stored in:
// half-precision scales and the simple Q8_0 and Q4_0 layouts are implemented
// and round-trip tested here; Q4_K is ported from ggml's reference code and
// still has to be checked against the llama.cpp oracle.
package quant

import "math"

// F16ToF32 converts an IEEE 754 half-precision value to float32. Every
// quantized block starts with one or two of these, so the engine calls it
// once per block, not once per weight.
func F16ToF32(h uint16) float32 {
	sign := uint32(h>>15) << 31
	exp := uint32(h>>10) & 0x1f
	frac := uint32(h) & 0x3ff
	switch exp {
	case 0:
		if frac == 0 {
			return math.Float32frombits(sign)
		}
		e := uint32(127 - 15 + 1) // subnormal: shift until the hidden bit appears
		for frac&0x400 == 0 {
			frac <<= 1
			e--
		}
		return math.Float32frombits(sign | e<<23 | (frac&0x3ff)<<13)
	case 0x1f:
		return math.Float32frombits(sign | 0xff<<23 | frac<<13)
	}
	return math.Float32frombits(sign | (exp+127-15)<<23 | frac<<13)
}

// F32ToF16 converts float32 to half precision, rounding to nearest even.
// Only quantizers and tests need it; inference never does.
func F32ToF16(f float32) uint16 {
	b := math.Float32bits(f)
	sign := uint16(b>>16) & 0x8000
	exp := int32(b>>23) & 0xff
	frac := b & 0x7fffff
	if exp == 0xff {
		if frac == 0 {
			return sign | 0x7c00 // infinity
		}
		m := uint16(frac >> 13)
		if m == 0 {
			m = 1 // keep it a NaN
		}
		return sign | 0x7c00 | m
	}
	e := exp - 127 + 15
	if e >= 0x1f {
		return sign | 0x7c00 // too large: infinity
	}
	if e <= 0 {
		if e < -10 {
			return sign // too small: zero
		}
		m := frac | 0x800000
		shift := uint32(14 - e)
		half := m >> shift
		rem := m & (1<<shift - 1)
		mid := uint32(1) << (shift - 1)
		if rem > mid || (rem == mid && half&1 == 1) {
			half++
		}
		return sign | uint16(half)
	}
	half := uint32(e)<<10 | frac>>13
	rem := frac & 0x1fff
	if rem > 0x1000 || (rem == 0x1000 && half&1 == 1) {
		half++ // a carry into the exponent is the correct result
	}
	return sign | uint16(half)
}
