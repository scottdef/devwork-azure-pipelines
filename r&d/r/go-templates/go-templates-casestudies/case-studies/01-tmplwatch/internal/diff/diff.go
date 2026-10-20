// Package diff writes unified diffs of small text files, enough for
// tmplwatch -dry to show what a render would change.
package diff

import (
	"fmt"
	"strings"
)

// maxCells bounds the LCS table; beyond it the diff degrades to
// "replace everything", which is still correct, just not minimal.
const maxCells = 4 << 20

type op struct {
	kind byte // ' ', '-', '+'
	line string
}

// Unified returns a unified diff from a to b with three lines of
// context, or "" if they are equal.
func Unified(aName, bName, a, b string) string {
	if a == b {
		return ""
	}
	al, bl := lines(a), lines(b)
	ops := script(al, bl)
	var sb strings.Builder
	fmt.Fprintf(&sb, "--- %s\n+++ %s\n", aName, bName)
	const ctx = 3
	// Walk ops, emitting hunks around changes.
	ai, bi := 0, 0 // line numbers before op i
	for i := 0; i < len(ops); {
		if ops[i].kind == ' ' {
			ai, bi, i = ai+1, bi+1, i+1
			continue
		}
		// Back up for leading context.
		start := i
		for n := 0; n < ctx && start > 0 && ops[start-1].kind == ' '; n++ {
			start--
		}
		as, bs := ai-(i-start), bi-(i-start)
		// Extend until ctx*2 unchanged lines in a row, or the end.
		end, run := i, 0
		for end < len(ops) && run < 2*ctx+1 {
			if ops[end].kind == ' ' {
				run++
			} else {
				run = 0
			}
			end++
		}
		// end now follows `run` unchanged lines; keep ctx of them.
		if run > ctx {
			end -= run - ctx
		}
		var an, bn int
		var body strings.Builder
		for _, o := range ops[start:end] {
			body.WriteByte(o.kind)
			body.WriteString(o.line)
			body.WriteByte('\n')
			if o.kind != '+' {
				an++
			}
			if o.kind != '-' {
				bn++
			}
		}
		fmt.Fprintf(&sb, "@@ -%s +%s @@\n", rng(as, an), rng(bs, bn))
		sb.WriteString(body.String())
		for _, o := range ops[i:end] {
			if o.kind != '+' {
				ai++
			}
			if o.kind != '-' {
				bi++
			}
		}
		i = end
	}
	if a != "" && !strings.HasSuffix(a, "\n") || b != "" && !strings.HasSuffix(b, "\n") {
		sb.WriteString("\\ No newline at end of file (one or both sides)\n")
	}
	return sb.String()
}

func rng(start, n int) string {
	if n == 0 {
		return fmt.Sprintf("%d,0", start)
	}
	if n == 1 {
		return fmt.Sprint(start + 1)
	}
	return fmt.Sprintf("%d,%d", start+1, n)
}

func lines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}

// script returns an edit script from a to b via longest common
// subsequence after trimming the common prefix and suffix.
func script(a, b []string) []op {
	var pre, suf []op
	for len(a) > 0 && len(b) > 0 && a[0] == b[0] {
		pre = append(pre, op{' ', a[0]})
		a, b = a[1:], b[1:]
	}
	for len(a) > 0 && len(b) > 0 && a[len(a)-1] == b[len(b)-1] {
		suf = append([]op{{' ', a[len(a)-1]}}, suf...)
		a, b = a[:len(a)-1], b[:len(b)-1]
	}
	var mid []op
	if (len(a)+1)*(len(b)+1) > maxCells {
		for _, l := range a {
			mid = append(mid, op{'-', l})
		}
		for _, l := range b {
			mid = append(mid, op{'+', l})
		}
	} else {
		mid = lcs(a, b)
	}
	return append(append(pre, mid...), suf...)
}

func lcs(a, b []string) []op {
	n, m := len(a), len(b)
	t := make([][]int32, n+1)
	for i := range t {
		t[i] = make([]int32, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				t[i][j] = t[i+1][j+1] + 1
			} else {
				t[i][j] = max(t[i+1][j], t[i][j+1])
			}
		}
	}
	var out []op
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			out = append(out, op{' ', a[i]})
			i, j = i+1, j+1
		case t[i+1][j] >= t[i][j+1]:
			out = append(out, op{'-', a[i]})
			i++
		default:
			out = append(out, op{'+', b[j]})
			j++
		}
	}
	for ; i < n; i++ {
		out = append(out, op{'-', a[i]})
	}
	for ; j < m; j++ {
		out = append(out, op{'+', b[j]})
	}
	return out
}
