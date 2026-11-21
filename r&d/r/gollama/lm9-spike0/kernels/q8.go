package kernels

import (
	"encoding/binary"

	"lm9spike/quant"
)

// DotQ8F32 returns the dot product of Q8_0-quantized weights with float32
// activations without expanding the weights first. blocks is the raw tensor
// data as it sits in a GGUF file; len(x) must be 32 per block.
func DotQ8F32(blocks []byte, x []float32) float32 {
	n := len(blocks) / quant.Q8_0Bytes
	if len(blocks)%quant.Q8_0Bytes != 0 || len(x) != n*quant.Q8_0Weights {
		panic("kernels: Q8_0 block and activation sizes do not match")
	}
	if n == 0 {
		return 0
	}
	return dotQ8(blocks, x)
}

// dotQ8Ref is the portable version and the reference the assembly is tested against.
func dotQ8Ref(blocks []byte, x []float32) float32 {
	var total float32
	for b := 0; b*quant.Q8_0Bytes < len(blocks); b++ {
		blk := blocks[b*quant.Q8_0Bytes : (b+1)*quant.Q8_0Bytes : (b+1)*quant.Q8_0Bytes]
		xs := x[b*32 : b*32+32 : b*32+32]
		var s float32
		for i, q := range blk[2:] {
			s += float32(int8(q)) * xs[i]
		}
		total += quant.F16ToF32(binary.LittleEndian.Uint16(blk)) * s
	}
	return total
}
