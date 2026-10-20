package funcs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// ToHCL renders v as an HCL2 expression: strings quoted with template
// sequences escaped ("${" → "$${", "%{" → "%%{"), lists as [a, b], maps
// as { k = v } with sorted keys. KV data can therefore never inject
// interpolation into generated Terraform or Nomad files.
func ToHCL(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("toHCL: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var n any
	if err := dec.Decode(&n); err != nil {
		return "", fmt.Errorf("toHCL: %w", err)
	}
	var sb strings.Builder
	hclValue(&sb, n)
	return sb.String(), nil
}

func hclValue(sb *strings.Builder, v any) {
	switch x := v.(type) {
	case nil:
		sb.WriteString("null")
	case bool:
		sb.WriteString(strconv.FormatBool(x))
	case json.Number:
		sb.WriteString(x.String())
	case string:
		sb.WriteString(hclString(x))
	case []any:
		sb.WriteByte('[')
		for i, e := range x {
			if i > 0 {
				sb.WriteString(", ")
			}
			hclValue(sb, e)
		}
		sb.WriteByte(']')
	case map[string]any:
		if len(x) == 0 {
			sb.WriteString("{}")
			return
		}
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		sb.WriteString("{ ")
		for i, k := range keys {
			if i > 0 {
				sb.WriteString(", ")
			}
			if isIdent(k) {
				sb.WriteString(k)
			} else {
				sb.WriteString(hclString(k))
			}
			sb.WriteString(" = ")
			hclValue(sb, x[k])
		}
		sb.WriteString(" }")
	}
}

func hclString(s string) string {
	var sb strings.Builder
	sb.WriteByte('"')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\\':
			sb.WriteString(`\\`)
		case c == '"':
			sb.WriteString(`\"`)
		case c == '\n':
			sb.WriteString(`\n`)
		case c == '\r':
			sb.WriteString(`\r`)
		case c == '\t':
			sb.WriteString(`\t`)
		case (c == '$' || c == '%') && i+1 < len(s) && s[i+1] == '{':
			sb.WriteByte(c)
			sb.WriteByte(c)
		case c < 0x20 || c == 0x7f:
			fmt.Fprintf(&sb, `\u%04x`, c)
		default:
			sb.WriteByte(c)
		}
	}
	sb.WriteByte('"')
	return sb.String()
}

// isIdent reports whether s is a valid HCL identifier.
func isIdent(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		switch {
		case r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z':
		case i > 0 && (r == '-' || r >= '0' && r <= '9'):
		default:
			return false
		}
	}
	return true
}
