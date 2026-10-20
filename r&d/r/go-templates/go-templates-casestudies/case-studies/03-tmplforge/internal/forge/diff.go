package forge

import (
	"fmt"
	"strings"
)

// Op is one line of an edit script.
type Op struct {
	Kind byte // ' ' keep, '-' delete (only in a), '+' insert (only in b)
	Text string
}

// MaxDiffCells bounds the LCS table (len(a)*len(b)). Beyond it Diff falls
// back to "replace everything", which is correct if not minimal; the GUI
// must not be made to allocate n*m on attacker-sized inputs.
const MaxDiffCells = 4 << 20

// Diff returns a line edit script turning a into b, computed from the
// longest common subsequence of lines. The standard library has no diff;
// the classic O(nm) dynamic programme (as in diff(1)'s ancestors before
// Hunt-McIlroy) is plenty for template outputs. Common prefix and suffix
// are stripped first, so the usual small edit costs almost nothing.
func Diff(a, b []string) []Op {
	var pre, suf []Op
	for len(a) > 0 && len(b) > 0 && a[0] == b[0] {
		pre = append(pre, Op{' ', a[0]})
		a, b = a[1:], b[1:]
	}
	for len(a) > 0 && len(b) > 0 && a[len(a)-1] == b[len(b)-1] {
		suf = append(suf, Op{' ', a[len(a)-1]})
		a, b = a[:len(a)-1], b[:len(b)-1]
	}
	mid := lcsDiff(a, b)
	out := append(pre, mid...)
	for i := len(suf) - 1; i >= 0; i-- {
		out = append(out, suf[i])
	}
	return out
}

func lcsDiff(a, b []string) []Op {
	n, m := len(a), len(b)
	if n*m > MaxDiffCells {
		var out []Op
		for _, l := range a {
			out = append(out, Op{'-', l})
		}
		for _, l := range b {
			out = append(out, Op{'+', l})
		}
		return out
	}
	// t[i][j] = LCS length of a[i:] and b[j:]
	t := make([][]int32, n+1)
	for i := range t {
		t[i] = make([]int32, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				t[i][j] = t[i+1][j+1] + 1
			} else if t[i+1][j] >= t[i][j+1] {
				t[i][j] = t[i+1][j]
			} else {
				t[i][j] = t[i][j+1]
			}
		}
	}
	var out []Op
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			out = append(out, Op{' ', a[i]})
			i++
			j++
		case t[i+1][j] >= t[i][j+1]:
			out = append(out, Op{'-', a[i]})
			i++
		default:
			out = append(out, Op{'+', b[j]})
			j++
		}
	}
	for ; i < n; i++ {
		out = append(out, Op{'-', a[i]})
	}
	for ; j < m; j++ {
		out = append(out, Op{'+', b[j]})
	}
	return out
}

// Lines splits s into lines without the final empty element a trailing
// newline would produce.
func Lines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}

// Changed reports whether the script contains any insert or delete.
func Changed(ops []Op) bool {
	for _, o := range ops {
		if o.Kind != ' ' {
			return true
		}
	}
	return false
}

// Unified formats ops as a unified diff with n lines of context, with
// "--- a" / "+++ b" headers and @@ hunk markers, like diff -u.
func Unified(aName, bName string, ops []Op, n int) string {
	if !Changed(ops) {
		return ""
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "--- %s\n+++ %s\n", aName, bName)
	// positions: line numbers in a and b before op k
	type pos struct{ a, b int }
	p := make([]pos, len(ops)+1)
	for k, o := range ops {
		p[k+1] = p[k]
		if o.Kind != '+' {
			p[k+1].a++
		}
		if o.Kind != '-' {
			p[k+1].b++
		}
	}
	k := 0
	for k < len(ops) {
		for k < len(ops) && ops[k].Kind == ' ' {
			k++
		}
		if k == len(ops) {
			break
		}
		start := max(k-n, 0)
		end := k
		for end < len(ops) {
			if ops[end].Kind != ' ' {
				end++
				continue
			}
			run := end
			for run < len(ops) && ops[run].Kind == ' ' {
				run++
			}
			if run == len(ops) || run-end > 2*n {
				end = min(end+n, len(ops))
				break
			}
			end = run
		}
		na, nb := p[end].a-p[start].a, p[end].b-p[start].b
		fmt.Fprintf(&sb, "@@ -%s +%s @@\n", rng(p[start].a, na), rng(p[start].b, nb))
		for _, o := range ops[start:end] {
			sb.WriteByte(o.Kind)
			sb.WriteString(o.Text)
			sb.WriteByte('\n')
		}
		k = end
	}
	return sb.String()
}

func rng(start, n int) string {
	if n == 0 {
		return fmt.Sprintf("%d,0", start)
	}
	if n == 1 {
		return fmt.Sprintf("%d", start+1)
	}
	return fmt.Sprintf("%d,%d", start+1, n)
}
