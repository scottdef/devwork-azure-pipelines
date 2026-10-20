package funcs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// yaml11 matches plain scalars that YAML 1.1 readers (go-yaml v2, used
// by Prometheus and many others) would turn into numbers or timestamps:
// sexagesimal 1:30, and dates 2026-01-02.
var yaml11 = regexp.MustCompile(`^[-+]?[0-9][0-9_]*(:[0-5]?[0-9])+(\.[0-9_]*)?$|^[0-9]{4}-[0-9]{1,2}-[0-9]{1,2}`)

// ToYAML renders v as block-style YAML with sorted map keys and
// two-space indentation. Values are first normalised through
// encoding/json, so anything toJSON accepts works here, and the result
// parses back to the same structure. The output has no trailing newline,
// which suits nindent.
func ToYAML(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("toYAML: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var n any
	if err := dec.Decode(&n); err != nil {
		return "", fmt.Errorf("toYAML: %w", err)
	}
	return strings.Join(yamlLines(n), "\n"), nil
}

// isBlock reports whether v needs lines of its own.
func isBlock(v any) bool {
	switch x := v.(type) {
	case map[string]any:
		return len(x) > 0
	case []any:
		return len(x) > 0
	}
	return false
}

func yamlLines(v any) []string {
	var out []string
	switch x := v.(type) {
	case map[string]any:
		if len(x) == 0 {
			return []string{"{}"}
		}
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			kq := yamlString(k)
			if c := x[k]; isBlock(c) {
				out = append(out, kq+":")
				for _, l := range yamlLines(c) {
					out = append(out, "  "+l)
				}
			} else {
				out = append(out, kq+": "+yamlScalar(c))
			}
		}
	case []any:
		if len(x) == 0 {
			return []string{"[]"}
		}
		for _, e := range x {
			if !isBlock(e) {
				out = append(out, "- "+yamlScalar(e))
				continue
			}
			ls := yamlLines(e)
			out = append(out, "- "+ls[0])
			for _, l := range ls[1:] {
				out = append(out, "  "+l)
			}
		}
	default:
		out = append(out, yamlScalar(v))
	}
	return out
}

func yamlScalar(v any) string {
	switch x := v.(type) {
	case nil:
		return "null"
	case bool:
		return strconv.FormatBool(x)
	case json.Number:
		return x.String()
	case string:
		return yamlString(x)
	case map[string]any:
		return "{}"
	case []any:
		return "[]"
	}
	return yamlString(fmt.Sprint(v))
}

// yamlString writes s plain when that is unambiguous and double-quoted
// otherwise. JSON string syntax is a subset of YAML's double-quoted
// style, so encoding/json does the escaping.
func yamlString(s string) string {
	if !needsQuote(s) {
		return s
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.Encode(s)
	return strings.TrimSuffix(buf.String(), "\n")
}

// reserved are plain scalars that YAML 1.1 or 1.2 would read as
// something other than a string.
var reserved = map[string]bool{
	"true": true, "false": true, "yes": true, "no": true, "on": true, "off": true,
	"y": true, "n": true, "null": true, "~": true,
}

func needsQuote(s string) bool {
	if s == "" || s != strings.TrimSpace(s) {
		return true
	}
	if reserved[strings.ToLower(s)] {
		return true
	}
	// Anything number-like: ints, floats, hex, octal, .inf, .nan, 1_000.
	if _, err := strconv.ParseFloat(strings.ReplaceAll(s, "_", ""), 64); err == nil {
		return true
	}
	if _, err := strconv.ParseInt(s, 0, 64); err == nil {
		return true
	}
	if yaml11.MatchString(s) {
		return true
	}
	switch l := strings.ToLower(s); l {
	case ".inf", "-.inf", "+.inf", ".nan":
		return true
	}
	if strings.ContainsAny(s[:1], "-?:,[]{}#&*!|>'\"%@`=") {
		return true
	}
	if strings.Contains(s, ": ") || strings.Contains(s, " #") || strings.HasSuffix(s, ":") {
		return true
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f || r == 0x2028 || r == 0x2029 || r == 0xfeff {
			return true
		}
	}
	return false
}
