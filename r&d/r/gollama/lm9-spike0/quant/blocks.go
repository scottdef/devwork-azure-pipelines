package quant

import (
	"encoding/binary"
	"errors"
	"math"
)

// Block geometry. A "block" is the unit a kernel processes in one step.
const (
	Q8_0Weights, Q8_0Bytes = 32, 34   // f16 scale, 32 x int8
	Q4_0Weights, Q4_0Bytes = 32, 18   // f16 scale, 16 bytes of 4-bit pairs
	Q4_KWeights, Q4_KBytes = 256, 144 // f16 d, f16 dmin, 12 bytes of 6-bit scales and mins, 128 bytes of 4-bit pairs
)

var errSize = errors.New("quant: source and destination sizes do not match the block layout")

// DequantQ8_0 expands Q8_0 blocks: weight = scale * int8.
func DequantQ8_0(dst []float32, src []byte) error {
	if len(src)%Q8_0Bytes != 0 || len(dst) != len(src)/Q8_0Bytes*Q8_0Weights {
		return errSize
	}
	for b := 0; b*Q8_0Bytes < len(src); b++ {
		blk := src[b*Q8_0Bytes : (b+1)*Q8_0Bytes : (b+1)*Q8_0Bytes]
		d := F16ToF32(binary.LittleEndian.Uint16(blk))
		out := dst[b*Q8_0Weights : (b+1)*Q8_0Weights : (b+1)*Q8_0Weights]
		for i, q := range blk[2:] {
			out[i] = d * float32(int8(q))
		}
	}
	return nil
}

// QuantQ8_0 is the reference quantizer: scale = max|x| / 127, round to nearest.
func QuantQ8_0(dst []byte, src []float32) error {
	if len(src)%Q8_0Weights != 0 || len(dst) != len(src)/Q8_0Weights*Q8_0Bytes {
		return errSize
	}
	for b := 0; b*Q8_0Weights < len(src); b++ {
		in := src[b*Q8_0Weights : (b+1)*Q8_0Weights]
		blk := dst[b*Q8_0Bytes : (b+1)*Q8_0Bytes]
		var amax float32
		for _, v := range in {
			if a := float32(math.Abs(float64(v))); a > amax {
				amax = a
			}
		}
		d := amax / 127
		var id float32
		if d != 0 {
			id = 1 / d
		}
		binary.LittleEndian.PutUint16(blk, F32ToF16(d))
		for i, v := range in {
			blk[2+i] = byte(int8(math.RoundToEven(float64(v * id))))
		}
	}
	return nil
}

// DequantQ4_0 expands Q4_0 blocks: weight = scale * (nibble - 8). The low
// nibbles of the 16 bytes are weights 0..15 and the high nibbles are 16..31.
func DequantQ4_0(dst []float32, src []byte) error {
	if len(src)%Q4_0Bytes != 0 || len(dst) != len(src)/Q4_0Bytes*Q4_0Weights {
		return errSize
	}
	for b := 0; b*Q4_0Bytes < len(src); b++ {
		blk := src[b*Q4_0Bytes : (b+1)*Q4_0Bytes : (b+1)*Q4_0Bytes]
		d := F16ToF32(binary.LittleEndian.Uint16(blk))
		out := dst[b*Q4_0Weights : (b+1)*Q4_0Weights : (b+1)*Q4_0Weights]
		for i, q := range blk[2:] {
			out[i] = d * float32(int(q&0x0f)-8)
			out[i+16] = d * float32(int(q>>4)-8)
		}
	}
	return nil
}

// QuantQ4_0 is the reference quantizer, following ggml: the scale is the
// signed value of largest magnitude divided by -8, so that value maps to 0.
func QuantQ4_0(dst []byte, src []float32) error {
	if len(src)%Q4_0Weights != 0 || len(dst) != len(src)/Q4_0Weights*Q4_0Bytes {
		return errSize
	}
	for b := 0; b*Q4_0Weights < len(src); b++ {
		in := src[b*Q4_0Weights : (b+1)*Q4_0Weights]
		blk := dst[b*Q4_0Bytes : (b+1)*Q4_0Bytes]
		var amax, max float32
		for _, v := range in {
			if a := float32(math.Abs(float64(v))); a > amax {
				amax, max = a, v
			}
		}
		d := max / -8
		var id float32
		if d != 0 {
			id = 1 / d
		}
		binary.LittleEndian.PutUint16(blk, F32ToF16(d))
		nib := func(v float32) byte {
			q := int(v*id + 8.5)
			if q > 15 {
				q = 15
			}
			if q < 0 {
				q = 0
			}
			return byte(q)
		}
		for i := 0; i < 16; i++ {
			blk[2+i] = nib(in[i]) | nib(in[i+16])<<4
		}
	}
	return nil
}

// scaleMinK4 unpacks the 6-bit scale and 6-bit min of sub-block j from the 12
// packed bytes of a Q4_K block. Sub-blocks 0..3 sit in the low six bits of
// bytes 0..7; sub-blocks 4..7 are split between bytes 8..11 and the top two
// bits of bytes 0..7.
func scaleMinK4(j int, q []byte) (scale, min byte) {
	if j < 4 {
		return q[j] & 63, q[j+4] & 63
	}
	return (q[j+4] & 0x0f) | (q[j-4]>>6)<<4, (q[j+4] >> 4) | (q[j]>>6)<<4
}

// DequantQ4_K expands Q4_K super-blocks of 256 weights. Each super-block has
// eight sub-blocks of 32 weights; weight = d*scale*nibble - dmin*min.
//
// Ported from dequantize_row_q4_K in ggml-quants.c (MIT). The test in this
// package checks it against a block built by hand, which proves the code
// matches this description. It has NOT been checked against llama.cpp output;
// that is a phase 2 oracle test.
func DequantQ4_K(dst []float32, src []byte) error {
	if len(src)%Q4_KBytes != 0 || len(dst) != len(src)/Q4_KBytes*Q4_KWeights {
		return errSize
	}
	for b := 0; b*Q4_KBytes < len(src); b++ {
		blk := src[b*Q4_KBytes : (b+1)*Q4_KBytes : (b+1)*Q4_KBytes]
		d := F16ToF32(binary.LittleEndian.Uint16(blk[0:]))
		dmin := F16ToF32(binary.LittleEndian.Uint16(blk[2:]))
		scales, qs := blk[4:16], blk[16:]
		out := dst[b*Q4_KWeights : (b+1)*Q4_KWeights : (b+1)*Q4_KWeights]
		for pair := 0; pair < 4; pair++ { // two sub-blocks share 32 bytes
			sc1, m1 := scaleMinK4(2*pair, scales)
			sc2, m2 := scaleMinK4(2*pair+1, scales)
			d1, min1 := d*float32(sc1), dmin*float32(m1)
			d2, min2 := d*float32(sc2), dmin*float32(m2)
			q := qs[pair*32 : pair*32+32 : pair*32+32]
			lo := out[pair*64 : pair*64+32 : pair*64+32]
			hi := out[pair*64+32 : pair*64+64 : pair*64+64]
			for l, v := range q {
				lo[l] = d1*float32(v&0x0f) - min1
				hi[l] = d2*float32(v>>4) - min2
			}
		}
	}
	return nil
}
