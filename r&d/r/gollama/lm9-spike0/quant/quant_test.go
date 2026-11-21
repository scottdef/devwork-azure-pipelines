package quant

import (
	"encoding/binary"
	"math"
	"math/rand/v2"
	"testing"
)

func TestF16KnownValues(t *testing.T) {
	for _, c := range []struct {
		h uint16
		f float32
	}{
		{0x0000, 0}, {0x3c00, 1}, {0xc000, -2}, {0x7bff, 65504},
		{0x0001, 5.9604645e-08}, {0x0400, 6.1035156e-05}, {0x3555, 0.33325195},
	} {
		if got := F16ToF32(c.h); got != c.f {
			t.Errorf("F16ToF32(%#04x) = %g, want %g", c.h, got, c.f)
		}
		if got := F32ToF16(c.f); got != c.h {
			t.Errorf("F32ToF16(%g) = %#04x, want %#04x", c.f, got, c.h)
		}
	}
	if !math.IsInf(float64(F16ToF32(0x7c00)), 1) || !math.IsNaN(float64(F16ToF32(0x7e00))) {
		t.Error("infinity or NaN not preserved")
	}
}

// Every one of the 65,536 half values must survive the trip to float32 and back.
func TestF16RoundTripsAllValues(t *testing.T) {
	for h := 0; h < 1<<16; h++ {
		if got := F32ToF16(F16ToF32(uint16(h))); got != uint16(h) {
			t.Fatalf("%#04x came back as %#04x", h, got)
		}
	}
}

func randVec(n int) []float32 {
	r := rand.New(rand.NewPCG(1, 2))
	v := make([]float32, n)
	for i := range v {
		v[i] = float32(r.NormFloat64())
	}
	return v
}

func TestQ8_0RoundTrip(t *testing.T) {
	src := randVec(32 * 9)
	blk := make([]byte, 9*Q8_0Bytes)
	out := make([]float32, len(src))
	if err := QuantQ8_0(blk, src); err != nil {
		t.Fatal(err)
	}
	if err := DequantQ8_0(out, blk); err != nil {
		t.Fatal(err)
	}
	for b := 0; b < 9; b++ {
		d := F16ToF32(binary.LittleEndian.Uint16(blk[b*Q8_0Bytes:]))
		for i := b * 32; i < (b+1)*32; i++ {
			if e := math.Abs(float64(out[i] - src[i])); e > 0.6*float64(d) {
				t.Fatalf("weight %d: error %g exceeds 0.6 of step %g", i, e, d)
			}
		}
	}
}

func TestQ4_0RoundTrip(t *testing.T) {
	src := randVec(32 * 9)
	blk := make([]byte, 9*Q4_0Bytes)
	out := make([]float32, len(src))
	if err := QuantQ4_0(blk, src); err != nil {
		t.Fatal(err)
	}
	if err := DequantQ4_0(out, blk); err != nil {
		t.Fatal(err)
	}
	for b := 0; b < 9; b++ {
		step := math.Abs(float64(F16ToF32(binary.LittleEndian.Uint16(blk[b*Q4_0Bytes:]))))
		for i := b * 32; i < (b+1)*32; i++ {
			if e := math.Abs(float64(out[i] - src[i])); e > 1.01*step {
				t.Fatalf("weight %d: error %g exceeds one step %g", i, e, step)
			}
		}
	}
}

// A Q4_K block built by hand, so the expected output can be read off the
// construction: sub-block j has scale j+1 and min j, d = 0.5, dmin = 0.25,
// and every byte of nibbles is 0x21 (low nibble 1, high nibble 2).
func TestQ4_KHandBuiltBlock(t *testing.T) {
	blk := make([]byte, Q4_KBytes)
	binary.LittleEndian.PutUint16(blk[0:], F32ToF16(0.5))
	binary.LittleEndian.PutUint16(blk[2:], F32ToF16(0.25))
	scales := blk[4:16]
	for j := 0; j < 4; j++ { // sub-blocks 0..3: low six bits
		scales[j] = byte(j + 1)
		scales[j+4] = byte(j)
	}
	for j := 4; j < 8; j++ { // sub-blocks 4..7: low nibbles in bytes 8..11, no high bits needed (< 16)
		scales[j+4] = byte(j+1) | byte(j)<<4
	}
	for i := range blk[16:] {
		blk[16+i] = 0x21
	}
	for j := 0; j < 8; j++ {
		if sc, m := scaleMinK4(j, scales); int(sc) != j+1 || int(m) != j {
			t.Fatalf("sub-block %d unpacked as scale %d min %d", j, sc, m)
		}
	}
	out := make([]float32, Q4_KWeights)
	if err := DequantQ4_K(out, blk); err != nil {
		t.Fatal(err)
	}
	for j := 0; j < 8; j++ {
		nib := float32(1) // even sub-blocks read low nibbles
		if j%2 == 1 {
			nib = 2
		}
		want := 0.5*float32(j+1)*nib - 0.25*float32(j)
		for i := j * 32; i < (j+1)*32; i++ {
			if out[i] != want {
				t.Fatalf("weight %d (sub-block %d) = %g, want %g", i, j, out[i], want)
			}
		}
	}
}

// The top-two-bit path of the scale packing: scales of 16 and above.
func TestQ4_KScalePackingHighBits(t *testing.T) {
	q := make([]byte, 12)
	set := func(j int, sc, m byte) {
		if j < 4 {
			q[j] = q[j]&0xc0 | sc
			q[j+4] = q[j+4]&0xc0 | m
			return
		}
		q[j+4] = sc&0x0f | (m&0x0f)<<4
		q[j-4] = q[j-4]&0x3f | (sc>>4)<<6
		q[j] = q[j]&0x3f | (m>>4)<<6
	}
	want := [8][2]byte{{63, 1}, {2, 62}, {33, 44}, {5, 6}, {63, 63}, {17, 48}, {32, 15}, {1, 31}}
	for j, w := range want {
		set(j, w[0], w[1])
	}
	for j, w := range want {
		if sc, m := scaleMinK4(j, q); sc != w[0] || m != w[1] {
			t.Fatalf("sub-block %d: got (%d,%d), want (%d,%d)", j, sc, m, w[0], w[1])
		}
	}
}

func TestSizeErrors(t *testing.T) {
	if DequantQ8_0(make([]float32, 32), make([]byte, 33)) == nil ||
		DequantQ4_0(make([]float32, 31), make([]byte, 18)) == nil ||
		DequantQ4_K(make([]float32, 256), make([]byte, 143)) == nil {
		t.Fatal("mismatched sizes were accepted")
	}
}
