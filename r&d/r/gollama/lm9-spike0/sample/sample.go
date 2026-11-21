// Package sample turns a row of logits into one token id.
//
// The order of operations follows the common chain: top-k, temperature,
// softmax, top-p, then one draw. The generator is ChaCha8 from math/rand/v2,
// whose output for a given seed is specified, so a seed reproduces a
// generation on any machine as long as the logits are the same bits.
package sample

import (
	"encoding/binary"
	"math"
	"math/rand/v2"
	"slices"
)

// Params are the sampler settings for one session.
type Params struct {
	Temperature float32 // 0 means greedy
	TopK        int     // 0 means no limit
	TopP        float32 // 0 or 1 means no limit
	Seed        uint64
}

type cand struct {
	id    int32
	logit float32
	p     float64
}

// Sampler holds the generator and scratch space for one session.
type Sampler struct {
	p   Params
	rng *rand.Rand
	buf []cand
}

// New returns a sampler. Two samplers with the same Params produce the same
// sequence of choices for the same sequence of logits.
func New(p Params) *Sampler {
	var seed [32]byte
	binary.LittleEndian.PutUint64(seed[:], p.Seed)
	return &Sampler{p: p, rng: rand.New(rand.NewChaCha8(seed))}
}

// Sample picks a token id. logits is not modified.
func (s *Sampler) Sample(logits []float32) int {
	if len(logits) == 0 {
		panic("sample: no logits")
	}
	if s.p.Temperature <= 0 || s.p.TopK == 1 {
		return argmax(logits)
	}
	s.buf = s.buf[:0]
	for i, l := range logits {
		s.buf = append(s.buf, cand{id: int32(i), logit: l})
	}
	// Highest logit first; ties go to the lower id so the order is total and
	// the same everywhere. A production sampler selects the top k with a
	// bounded heap instead of sorting the whole vocabulary.
	slices.SortFunc(s.buf, func(a, b cand) int {
		switch {
		case a.logit > b.logit:
			return -1
		case a.logit < b.logit:
			return 1
		}
		return int(a.id - b.id)
	})
	c := s.buf
	if s.p.TopK > 0 && s.p.TopK < len(c) {
		c = c[:s.p.TopK]
	}
	// Softmax with temperature, shifted by the maximum for stability.
	// Deterministic mode replaces math.Exp with the engine's own exp.
	inv := 1 / float64(s.p.Temperature)
	max := float64(c[0].logit)
	var sum float64
	for i := range c {
		c[i].p = math.Exp((float64(c[i].logit) - max) * inv)
		sum += c[i].p
	}
	if p := float64(s.p.TopP); p > 0 && p < 1 {
		var cum float64
		for i := range c {
			cum += c[i].p / sum
			if cum >= p {
				c = c[:i+1]
				break
			}
		}
		sum = 0
		for i := range c {
			sum += c[i].p
		}
	}
	u := s.rng.Float64() * sum
	for i := range c {
		if u -= c[i].p; u < 0 {
			return int(c[i].id)
		}
	}
	return int(c[len(c)-1].id) // rounding left u at zero
}

func argmax(logits []float32) int {
	best := 0
	for i, l := range logits {
		if l > logits[best] {
			best = i
		}
	}
	return best
}
