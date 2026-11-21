package pretok

// MergeBPE applies byte-pair-encoding merges to one pre-token. rank gives the
// priority of each mergeable pair; the pair with the lowest rank present is
// merged everywhere it occurs, and the process repeats until none is left.
//
// This is the textbook loop, quadratic in the length of the piece. Pieces are
// words, so that is fine for a reference; the engine caches results per piece
// and uses a priority queue for long ones.
func MergeBPE(piece string, rank map[[2]string]int) []string {
	var syms []string
	for _, r := range piece {
		syms = append(syms, string(r))
	}
	for len(syms) > 1 {
		best, at := int(^uint(0)>>1), -1
		for i := 0; i+1 < len(syms); i++ {
			if r, ok := rank[[2]string{syms[i], syms[i+1]}]; ok && r < best {
				best, at = r, i
			}
		}
		if at < 0 {
			break
		}
		a, b := syms[at], syms[at+1]
		out := syms[:0] // safe in place: writes never overtake reads
		for i := 0; i < len(syms); i++ {
			if i+1 < len(syms) && syms[i] == a && syms[i+1] == b {
				out = append(out, a+b)
				i++
				continue
			}
			out = append(out, syms[i])
		}
		syms = out
	}
	return syms
}
