// Package ops holds tier-0 reference implementations of the operations in a
// transformer block. They are written for clarity, in float64 where that
// costs nothing, and exist to be the thing faster kernels are tested against.
package ops

import "math"

// RMSNorm writes x / sqrt(mean(x^2) + eps) * weight into dst.
func RMSNorm(dst, x, weight []float32, eps float32) {
	var ss float64
	for _, v := range x {
		ss += float64(v) * float64(v)
	}
	scale := float32(1 / math.Sqrt(ss/float64(len(x))+float64(eps)))
	for i, v := range x {
		dst[i] = v * scale * weight[i]
	}
}

// Softmax replaces x with exp(x) / sum(exp(x)), shifted by the maximum so
// large logits do not overflow.
func Softmax(x []float32) {
	max := x[0]
	for _, v := range x {
		if v > max {
			max = v
		}
	}
	var sum float64
	for i, v := range x {
		e := math.Exp(float64(v - max))
		x[i] = float32(e)
		sum += e
	}
	inv := float32(1 / sum)
	for i := range x {
		x[i] *= inv
	}
}

// SiLU is x * sigmoid(x), the activation inside SwiGLU feed-forward layers.
func SiLU(x float32) float32 {
	return float32(float64(x) / (1 + math.Exp(-float64(x))))
}

// RoPE applies rotary position encoding in place: each pair (x[2i], x[2i+1])
// is rotated by pos * base^(-2i/d). Applied to queries and keys, it makes
// their dot product depend only on the distance between positions.
func RoPE(x []float32, pos int, base float64) {
	d := len(x)
	for i := 0; i+1 < d; i += 2 {
		sin, cos := math.Sincos(float64(pos) * math.Pow(base, -float64(i)/float64(d)))
		a, b := float64(x[i]), float64(x[i+1])
		x[i] = float32(a*cos - b*sin)
		x[i+1] = float32(a*sin + b*cos)
	}
}

// Attend computes single-head attention for one query over n cached keys and
// values (row-major, d wide): out = sum_j softmax(q.k_j / sqrt(d))_j * v_j.
// scores is scratch of length n. This is the loop the KV cache exists for:
// at generation time only the newest query is computed, against every key
// and value already stored.
func Attend(out, q, keys, values, scores []float32) {
	d, n := len(q), len(scores)
	inv := 1 / math.Sqrt(float64(d))
	for j := 0; j < n; j++ {
		var s float64
		for i, qv := range q {
			s += float64(qv) * float64(keys[j*d+i])
		}
		scores[j] = float32(s * inv)
	}
	Softmax(scores)
	for i := range out {
		out[i] = 0
	}
	for j, p := range scores {
		for i := range out {
			out[i] += p * values[j*d+i]
		}
	}
}
