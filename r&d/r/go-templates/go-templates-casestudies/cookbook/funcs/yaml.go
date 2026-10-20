package funcs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// ToYAML renders v as block-style YAML with no trailing newline.
//
// The emitter is deliberately small. It takes whatever encoding/json can
// marshal (structs, maps, slices, scalars), so field names follow json tags,
// and it writes map keys in sorted order so output is stable. Sequences are
// indented under their key. Strings are plain when that is unambiguous and
// double-quoted otherwise; multi-line strings become literal blocks.
// There are no anchors, tags, comments or flow style: a template that needs
// those should write them as text.
func ToYAML(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("toYAML: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber() // keep 8080 as 8080, not 8080.0 or 8.08e+03
	var n any
	if err := dec.Decode(&n); err != nil {
		return "", fmt.Errorf("toYAML: %w", err)
	}
	if lit, hdr, ok := literal(n, 0); ok {
		return strings.Join(append([]string{hdr}, lit...), "\n"), nil
	}
	if s, ok := inline(n); ok {
		return s, nil
	}
	return strings.Join(emit(n, 0), "\n"), nil
}

func pad(n int) string { return strings.Repeat(" ", n) }

// emit returns the lines of a non-empty map or slice at indent ind.
func emit(v any, ind int) []string {
	var out []string
	switch x := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			head := pad(ind) + quoteIfNeeded(k) + ":"
			out = append(out, entry(head, x[k], ind)...)
		}
	case []any:
		for _, e := range x {
			if lit, hdr, ok := literal(e, ind+2); ok {
				out = append(out, pad(ind)+"- "+hdr)
				out = append(out, lit...)
			} else if s, ok := inline(e); ok {
				out = append(out, pad(ind)+"- "+s)
			} else {
				sub := emit(e, ind+2)
				sub[0] = pad(ind) + "- " + strings.TrimPrefix(sub[0], pad(ind+2))
				out = append(out, sub...)
			}
		}
	}
	return out
}

// entry renders one "key: value" pair whose key part is head.
func entry(head string, v any, ind int) []string {
	if lit, hdr, ok := literal(v, ind+2); ok {
		return append([]string{head + " " + hdr}, lit...)
	}
	if s, ok := inline(v); ok {
		return []string{head + " " + s}
	}
	return append([]string{head}, emit(v, ind+2)...)
}

// inline renders scalars and empty collections on one line.
func inline(v any) (string, bool) {
	switch x := v.(type) {
	case nil:
		return "null", true
	case bool:
		return strconv.FormatBool(x), true
	case json.Number:
		return x.String(), true
	case string:
		return quoteIfNeeded(x), true
	case map[string]any:
		if len(x) == 0 {
			return "{}", true
		}
	case []any:
		if len(x) == 0 {
			return "[]", true
		}
	}
	return "", false
}

// literal renders a multi-line string as a literal block scalar whose content
// is indented to ind. It declines (ok false) when the string cannot be
// written that way faithfully, and the caller falls back to quoting.
func literal(v any, ind int) (lines []string, header string, ok bool) {
	s, isStr := v.(string)
	if !isStr || !strings.Contains(s, "\n") {
		return nil, "", false
	}
	body := strings.TrimRight(s, "\n")
	if body == "" || body[0] == ' ' || body[0] == '\t' {
		return nil, "", false
	}
	for _, r := range s {
		if r != '\n' && r != '\t' && !unicode.IsPrint(r) {
			return nil, "", false
		}
	}
	ls := strings.Split(body, "\n")
	for _, l := range ls {
		if l != "" && strings.TrimSpace(l) == "" {
			return nil, "", false // whitespace-only line: indentation ambiguity
		}
	}
	trailing := len(s) - len(body)
	switch {
	case trailing == 0:
		header = "|-"
	case trailing == 1:
		header = "|"
	default:
		header = "|+"
		for i := 1; i < trailing; i++ {
			ls = append(ls, "")
		}
	}
	for _, l := range ls {
		if l == "" {
			lines = append(lines, "")
		} else {
			lines = append(lines, pad(ind)+l)
		}
	}
	return lines, header, true
}

var (
	reserved = map[string]bool{
		"true": true, "false": true, "null": true, "yes": true, "no": true,
		"on": true, "off": true, "y": true, "n": true, "~": true, "<<": true, "=": true,
	}
	sexagesimal = regexp.MustCompile(`^[-+]?[0-9][0-9_]*(:[0-5]?[0-9])+(\.[0-9_]*)?$`)
	dateLike    = regexp.MustCompile(`^[0-9]{4}-[0-9]{1,2}-[0-9]{1,2}`)
)

// quoteIfNeeded returns s as a plain scalar if a YAML 1.1 or 1.2 parser would
// read it back as the same string, and as a double-quoted scalar otherwise.
// The rules err toward quoting: an unneeded quote costs nothing, a missing
// one turns "no" into false or "0755" into 493.
func quoteIfNeeded(s string) string {
	if !needsQuote(s) {
		return s
	}
	var b bytes.Buffer
	enc := json.NewEncoder(&b) // JSON strings are valid YAML double-quoted scalars
	enc.SetEscapeHTML(false)
	_ = enc.Encode(s)
	return strings.TrimSuffix(b.String(), "\n")
}

func needsQuote(s string) bool {
	if s == "" || s != strings.Trim(s, " \t") {
		return true
	}
	for _, r := range s {
		if !unicode.IsPrint(r) {
			return true
		}
	}
	if reserved[strings.ToLower(s)] {
		return true
	}
	if _, err := strconv.ParseFloat(s, 64); err == nil {
		return true
	}
	if _, err := strconv.ParseInt(s, 0, 64); err == nil {
		return true
	}
	if sexagesimal.MatchString(s) || dateLike.MatchString(s) {
		return true
	}
	switch s[0] {
	case ',', '[', ']', '{', '}', '#', '&', '*', '!', '|', '>', '\'', '"', '%', '@', '`':
		return true
	case '-', '?', ':':
		if len(s) == 1 || s[1] == ' ' || s[1] == '\t' {
			return true
		}
	}
	if strings.HasSuffix(s, ":") || strings.Contains(s, ": ") || strings.Contains(s, ":\t") ||
		strings.Contains(s, " #") || strings.HasPrefix(s, "---") || strings.HasPrefix(s, "...") {
		return true
	}
	return false
}
