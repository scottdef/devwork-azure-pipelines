// Package yamlite parses the small subset of YAML that GitHub issue forms
// use: block mappings, block sequences, plain and quoted scalars, flow
// sequences of scalars ([a, b]) and literal/folded block scalars (| and >).
//
// It is a line scanner, not a YAML implementation. Anchors, aliases, tags,
// flow mappings, multi-document streams and multi-line plain scalars are
// not supported; they do not appear in issue forms and refusing them keeps
// the code small enough to read in one sitting.
//
// Values come back as map[string]any, []any, string, bool or nil.
// Numbers stay strings: issue forms treat every option as text.
package yamlite

import (
	"fmt"
	"strconv"
	"strings"
)

type line struct {
	n      int    // 1-based line number
	indent int    // leading spaces
	text   string // line without leading spaces
	raw    string // original line without trailing \r
	skip   bool   // blank or comment-only
}

type parser struct {
	lines []line
	i     int
}

// Parse parses src and returns the root value.
func Parse(src []byte) (any, error) {
	p := &parser{}
	for n, raw := range strings.Split(string(src), "\n") {
		raw = strings.TrimSuffix(raw, "\r")
		if n == 0 {
			raw = strings.TrimPrefix(raw, "\ufeff")
		}
		text := strings.TrimLeft(raw, " ")
		if strings.HasPrefix(text, "\t") {
			return nil, fmt.Errorf("line %d: tab indentation is not allowed in YAML", n+1)
		}
		ind := len(raw) - len(text)
		t := strings.TrimSpace(text)
		p.lines = append(p.lines, line{
			n: n + 1, indent: ind, text: strings.TrimRight(text, " \t"), raw: raw,
			skip: t == "" || strings.HasPrefix(t, "#") || t == "---",
		})
	}
	j := p.peek()
	if j < 0 {
		return nil, nil
	}
	v, err := p.node(p.lines[j].indent)
	if err != nil {
		return nil, err
	}
	if k := p.peek(); k >= 0 {
		return nil, fmt.Errorf("line %d: unexpected content %q", p.lines[k].n, p.lines[k].text)
	}
	return v, nil
}

// peek returns the index of the next significant line, or -1.
func (p *parser) peek() int {
	for j := p.i; j < len(p.lines); j++ {
		if !p.lines[j].skip {
			return j
		}
	}
	return -1
}

func isSeq(text string) bool { return text == "-" || strings.HasPrefix(text, "- ") }

func (p *parser) node(indent int) (any, error) {
	j := p.peek()
	if j < 0 || p.lines[j].indent < indent {
		return nil, nil
	}
	if isSeq(p.lines[j].text) {
		return p.seq(p.lines[j].indent)
	}
	return p.mapping(p.lines[j].indent)
}

func (p *parser) seq(indent int) ([]any, error) {
	out := []any{}
	for {
		j := p.peek()
		if j < 0 || p.lines[j].indent != indent || !isSeq(p.lines[j].text) {
			return out, nil
		}
		l := p.lines[j]
		rest := strings.TrimPrefix(l.text, "-")
		content := strings.TrimLeft(rest, " ")
		col := indent + 1 + (len(rest) - len(content))
		p.i = j + 1
		switch {
		case content == "":
			k := p.peek()
			if k >= 0 && p.lines[k].indent > indent {
				v, err := p.node(p.lines[k].indent)
				if err != nil {
					return nil, err
				}
				out = append(out, v)
			} else {
				out = append(out, nil)
			}
		case isSeq(content):
			// "- - x": rewrite the line as a nested sequence item.
			p.lines[j] = line{n: l.n, indent: col, text: content, raw: l.raw}
			p.i = j
			v, err := p.seq(col)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		default:
			if _, _, ok := splitKey(content); ok {
				// "- key: value": the mapping starts on this line at col.
				p.lines[j] = line{n: l.n, indent: col, text: content, raw: l.raw}
				p.i = j
				v, err := p.mapping(col)
				if err != nil {
					return nil, err
				}
				out = append(out, v)
				continue
			}
			v, err := scalar(content, l.n)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
	}
}

func (p *parser) mapping(indent int) (map[string]any, error) {
	out := map[string]any{}
	for {
		j := p.peek()
		if j < 0 || p.lines[j].indent < indent {
			return out, nil
		}
		l := p.lines[j]
		if l.indent > indent {
			return nil, fmt.Errorf("line %d: unexpected indentation", l.n)
		}
		if isSeq(l.text) {
			return out, nil
		}
		key, rest, ok := splitKey(l.text)
		if !ok {
			return nil, fmt.Errorf("line %d: expected \"key: value\", got %q", l.n, l.text)
		}
		if _, dup := out[key]; dup {
			return nil, fmt.Errorf("line %d: duplicate key %q", l.n, key)
		}
		p.i = j + 1
		rest = stripComment(strings.TrimSpace(rest))
		var v any
		var err error
		switch {
		case rest == "":
			k := p.peek()
			switch {
			case k >= 0 && p.lines[k].indent > indent:
				v, err = p.node(p.lines[k].indent)
			case k >= 0 && p.lines[k].indent == indent && isSeq(p.lines[k].text):
				v, err = p.seq(indent) // sequence at the key's own indent
			}
		case rest[0] == '|' || rest[0] == '>':
			v = p.block(indent, rest)
		default:
			v, err = scalar(rest, l.n)
		}
		if err != nil {
			return nil, err
		}
		out[key] = v
	}
}

// block reads a literal (|) or folded (>) block scalar whose header is hdr
// and whose parent key sits at indent.
func (p *parser) block(indent int, hdr string) string {
	fold := hdr[0] == '>'
	chomp := byte(0)
	if strings.Contains(hdr, "-") {
		chomp = '-'
	} else if strings.Contains(hdr, "+") {
		chomp = '+'
	}
	var body []string
	ci := -1
	end := p.i
	for k := p.i; k < len(p.lines); k++ {
		l := p.lines[k]
		if strings.TrimSpace(l.raw) == "" {
			body = append(body, "")
			continue
		}
		if l.indent <= indent {
			break
		}
		if ci < 0 {
			ci = l.indent
		}
		if l.indent < ci {
			break
		}
		body = append(body, l.raw[ci:])
		end = k + 1
	}
	// Trailing blank lines past end belong to whatever follows, except
	// for keep-chomping which wants them.
	content := body
	trail := 0
	for len(content) > 0 && content[len(content)-1] == "" {
		content = content[:len(content)-1]
		trail++
	}
	p.i = end
	var s string
	if fold {
		var b strings.Builder
		prevText := false
		for _, t := range content {
			switch {
			case t == "":
				b.WriteByte('\n')
				prevText = false
			case strings.HasPrefix(t, " "):
				if prevText {
					b.WriteByte('\n')
				}
				b.WriteString(t)
				b.WriteByte('\n')
				prevText = false
			default:
				if prevText {
					b.WriteByte(' ')
				}
				b.WriteString(t)
				prevText = true
			}
		}
		s = strings.TrimRight(b.String(), "\n")
	} else {
		s = strings.Join(content, "\n")
	}
	switch chomp {
	case '-':
		return s
	case '+':
		return s + "\n" + strings.Repeat("\n", trail)
	}
	if s == "" {
		return ""
	}
	return s + "\n"
}

// splitKey splits "key: rest" (or "key:") and reports whether text is a
// mapping entry at all.
func splitKey(text string) (key, rest string, ok bool) {
	if text == "" {
		return "", "", false
	}
	if text[0] == '"' || text[0] == '\'' {
		end := closingQuote(text)
		if end < 0 || end+1 >= len(text) || text[end+1] != ':' {
			return "", "", false
		}
		k, err := scalar(text[:end+1], 0)
		if err != nil {
			return "", "", false
		}
		ks, _ := k.(string)
		after := text[end+2:]
		if after != "" && after[0] != ' ' {
			return "", "", false
		}
		return ks, after, true
	}
	if text[0] == '[' || text[0] == '{' {
		return "", "", false
	}
	for i := 0; i < len(text); i++ {
		if text[i] == ':' && (i+1 == len(text) || text[i+1] == ' ') {
			return strings.TrimSpace(text[:i]), text[i+1:], true
		}
		if text[i] == ' ' && i+1 < len(text) && text[i+1] == '#' {
			return "", "", false
		}
	}
	return "", "", false
}

func closingQuote(s string) int {
	q := s[0]
	for i := 1; i < len(s); i++ {
		switch {
		case q == '"' && s[i] == '\\':
			i++
		case q == '\'' && s[i] == '\'' && i+1 < len(s) && s[i+1] == '\'':
			i++
		case s[i] == q:
			return i
		}
	}
	return -1
}

func stripComment(s string) string {
	if s == "" || s[0] == '"' || s[0] == '\'' || s[0] == '[' {
		return s
	}
	if i := strings.Index(s, " #"); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return s
}

func scalar(s string, n int) (any, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", nil
	}
	switch s[0] {
	case '"':
		end := closingQuote(s)
		if end < 0 {
			return nil, fmt.Errorf("line %d: unterminated string", n)
		}
		u, err := strconv.Unquote(s[:end+1])
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", n, err)
		}
		return u, nil
	case '\'':
		end := closingQuote(s)
		if end < 0 {
			return nil, fmt.Errorf("line %d: unterminated string", n)
		}
		return strings.ReplaceAll(s[1:end], "''", "'"), nil
	case '[':
		return flowSeq(s, n)
	case '{':
		return nil, fmt.Errorf("line %d: flow mappings are not supported", n)
	case '&', '*', '!':
		return nil, fmt.Errorf("line %d: anchors, aliases and tags are not supported", n)
	}
	switch s {
	case "true", "True", "TRUE":
		return true, nil
	case "false", "False", "FALSE":
		return false, nil
	case "~", "null", "Null", "NULL":
		return nil, nil
	}
	return s, nil
}

func flowSeq(s string, n int) ([]any, error) {
	s = stripComment(s)
	if !strings.HasSuffix(s, "]") {
		if i := strings.LastIndex(s, "]"); i > 0 {
			s = s[:i+1]
		} else {
			return nil, fmt.Errorf("line %d: unterminated flow sequence", n)
		}
	}
	inner := strings.TrimSpace(s[1 : len(s)-1])
	out := []any{}
	if inner == "" {
		return out, nil
	}
	var parts []string
	start, q := 0, byte(0)
	for i := 0; i < len(inner); i++ {
		c := inner[i]
		switch {
		case q != 0:
			if c == '\\' && q == '"' {
				i++
			} else if c == q {
				q = 0
			}
		case c == '"' || c == '\'':
			q = c
		case c == '[' || c == '{':
			return nil, fmt.Errorf("line %d: nested flow collections are not supported", n)
		case c == ',':
			parts = append(parts, inner[start:i])
			start = i + 1
		}
	}
	parts = append(parts, inner[start:])
	for _, p := range parts {
		v, err := scalar(p, n)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}
