package funcs

import (
	"encoding/json"
	"fmt"
	"strings"
)

// parseYAML reads the block-style subset ToYAML writes: mappings,
// sequences, "- " items whose first line may begin a mapping or nested
// sequence, plain and double-quoted scalars, {} and []. Test-only.
func parseYAML(s string) (any, error) {
	p := &yreader{lines: strings.Split(s, "\n")}
	v, err := p.node(0)
	if err != nil {
		return nil, err
	}
	if p.i != len(p.lines) {
		return nil, fmt.Errorf("line %d: trailing %q", p.i+1, p.lines[p.i])
	}
	return v, nil
}

type yreader struct {
	lines []string
	i     int
}

func indentOf(l string) int { return len(l) - len(strings.TrimLeft(l, " ")) }

func (p *yreader) node(ind int) (any, error) {
	if p.i >= len(p.lines) {
		return nil, fmt.Errorf("unexpected end")
	}
	l := p.lines[p.i][ind:]
	if strings.HasPrefix(l, "- ") || l == "-" {
		return p.seq(ind)
	}
	if _, _, ok := splitKey(l); ok {
		return p.mapping(ind)
	}
	p.i++
	return scalar(l)
}

func (p *yreader) seq(ind int) (any, error) {
	out := []any{}
	for p.i < len(p.lines) && indentOf(p.lines[p.i]) == ind && strings.HasPrefix(p.lines[p.i][ind:], "- ") {
		// Re-read the item body as if "- " were two spaces of indent.
		p.lines[p.i] = p.lines[p.i][:ind] + "  " + p.lines[p.i][ind+2:]
		v, err := p.node(ind + 2)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

func (p *yreader) mapping(ind int) (any, error) {
	out := map[string]any{}
	for p.i < len(p.lines) && indentOf(p.lines[p.i]) == ind {
		k, rest, ok := splitKey(p.lines[p.i][ind:])
		if !ok {
			return nil, fmt.Errorf("line %d: want key", p.i+1)
		}
		p.i++
		if rest == "" {
			if p.i >= len(p.lines) || indentOf(p.lines[p.i]) <= ind {
				return nil, fmt.Errorf("line %d: empty block", p.i)
			}
			v, err := p.node(indentOf(p.lines[p.i]))
			if err != nil {
				return nil, err
			}
			out[k] = v
			continue
		}
		v, err := scalar(rest)
		if err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, nil
}

// splitKey splits `key: rest` or `key:`; keys may be double-quoted.
func splitKey(l string) (key, rest string, ok bool) {
	if strings.HasPrefix(l, `"`) {
		var k string
		dec := json.NewDecoder(strings.NewReader(l))
		if err := dec.Decode(&k); err != nil {
			return "", "", false
		}
		after := l[dec.InputOffset():]
		if after == ":" {
			return k, "", true
		}
		if strings.HasPrefix(after, ": ") {
			return k, after[2:], true
		}
		return "", "", false
	}
	if i := strings.Index(l, ": "); i > 0 {
		return l[:i], l[i+2:], true
	}
	if strings.HasSuffix(l, ":") && !strings.Contains(l, ": ") {
		return l[:len(l)-1], "", true
	}
	return "", "", false
}

func scalar(s string) (any, error) {
	switch s {
	case "null":
		return nil, nil
	case "true":
		return true, nil
	case "false":
		return false, nil
	case "{}":
		return map[string]any{}, nil
	case "[]":
		return []any{}, nil
	}
	if strings.HasPrefix(s, `"`) {
		var v string
		err := json.Unmarshal([]byte(s), &v)
		return v, err
	}
	var n json.Number
	if json.Unmarshal([]byte(s), &n) == nil {
		return n, nil
	}
	return s, nil
}
