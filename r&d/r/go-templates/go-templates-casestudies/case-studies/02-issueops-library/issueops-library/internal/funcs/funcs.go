// Package funcs is the FuncMap shared by every template in the library:
// item artifacts (text/template), pages (html/template), the Atom feed and
// the one-shot tpl command. One map, documented in docs/FUNCS.md, so a
// template that works in tpl works in pagegen.
package funcs

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Map returns a fresh FuncMap. The result is a plain map[string]any so it
// converts to both text/template.FuncMap and html/template.FuncMap.
func Map() map[string]any {
	return map[string]any{
		"dict":      Dict,
		"list":      List,
		"default":   Default,
		"toJSON":    ToJSON,
		"indent":    Indent,
		"nindent":   NIndent,
		"sha256sum": SHA256Sum,
		"humanSize": HumanSize,
		"dateFmt":   DateFmt,
		"plural":    Plural,
		"slugify":   Slugify,
		"add":       Add,
		"lower":     strings.ToLower,
		"upper":     strings.ToUpper,
		"join":      Join,
		"quote":     strconv.Quote,
		"trim":      strings.TrimSpace,
		"hasKey":    HasKey,
	}
}

// Dict builds a map from alternating key, value arguments:
// {{template "row" dict "item" . "odd" true}}.
func Dict(kv ...any) (map[string]any, error) {
	if len(kv)%2 != 0 {
		return nil, errors.New("dict: odd number of arguments")
	}
	m := make(map[string]any, len(kv)/2)
	for i := 0; i < len(kv); i += 2 {
		k, ok := kv[i].(string)
		if !ok {
			return nil, fmt.Errorf("dict: key %d is %T, not string", i/2, kv[i])
		}
		m[k] = kv[i+1]
	}
	return m, nil
}

// List returns its arguments as a slice.
func List(v ...any) []any { return v }

// Default returns def when v is empty (nil, "", 0, false, empty collection).
// Argument order matches the pipeline form: {{.port | default 8080}}.
func Default(def, v any) any {
	if empty(v) {
		return def
	}
	return v
}

func empty(v any) bool {
	if v == nil {
		return true
	}
	r := reflect.ValueOf(v)
	switch r.Kind() {
	case reflect.String, reflect.Slice, reflect.Map, reflect.Array:
		return r.Len() == 0
	case reflect.Bool:
		return !r.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return r.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return r.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return r.Float() == 0
	case reflect.Pointer, reflect.Interface:
		return r.IsNil()
	}
	return false
}

// ToJSON encodes v compactly without HTML escaping.
func ToJSON(v any) (string, error) {
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return "", err
	}
	return strings.TrimSuffix(b.String(), "\n"), nil
}

// Indent prefixes every non-empty line of s with n spaces.
func Indent(n int, s string) string {
	pad := strings.Repeat(" ", n)
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if l != "" {
			lines[i] = pad + l
		}
	}
	return strings.Join(lines, "\n")
}

// NIndent is Indent preceded by a newline, for use after a YAML key.
func NIndent(n int, s string) string { return "\n" + Indent(n, s) }

// SHA256Sum returns the hex sha256 of s (string or []byte).
func SHA256Sum(v any) string {
	var b []byte
	switch t := v.(type) {
	case string:
		b = []byte(t)
	case []byte:
		b = t
	default:
		b = []byte(fmt.Sprint(v))
	}
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// HumanSize formats a byte count: 512 B, 1.5 KiB, 3.0 MiB.
func HumanSize(v any) string {
	n, ok := toFloat(v)
	if !ok {
		return fmt.Sprint(v)
	}
	if n < 1024 {
		return fmt.Sprintf("%d B", int64(n))
	}
	units := []string{"KiB", "MiB", "GiB", "TiB"}
	i := -1
	for n >= 1024 && i < len(units)-1 {
		n /= 1024
		i++
	}
	return fmt.Sprintf("%.1f %s", n, units[i])
}

// DateFmt formats t (time.Time or RFC 3339 string) with a Go layout.
// Named layouts: "date" (2006-01-02), "rfc3339", "human" (2 Jan 2006 15:04 MST).
func DateFmt(layout string, v any) (string, error) {
	var t time.Time
	switch x := v.(type) {
	case time.Time:
		t = x
	case *time.Time:
		if x == nil {
			return "", nil
		}
		t = *x
	case string:
		if x == "" {
			return "", nil
		}
		p, err := time.Parse(time.RFC3339, x)
		if err != nil {
			return "", fmt.Errorf("dateFmt: %w", err)
		}
		t = p
	default:
		return "", fmt.Errorf("dateFmt: cannot format %T", v)
	}
	if t.IsZero() {
		return "", nil
	}
	switch layout {
	case "date":
		layout = "2006-01-02"
	case "rfc3339":
		layout = time.RFC3339
	case "human":
		layout = "2 Jan 2006 15:04 MST"
	}
	return t.UTC().Format(layout), nil
}

// Plural returns "1 item" / "3 items"; with a third argument, the plural
// form is given explicitly: plural 2 "entry" "entries".
func Plural(n any, word string, many ...string) string {
	f, _ := toFloat(n)
	if f == 1 {
		return "1 " + word
	}
	w := word + "s"
	if len(many) > 0 {
		w = many[0]
	}
	return strconv.FormatFloat(f, 'f', -1, 64) + " " + w
}

// Slugify lowercases s and turns runs of anything outside [a-z0-9] into
// single dashes.
func Slugify(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		if r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)) {
			b.WriteRune(r)
			dash = false
		} else if !dash && b.Len() > 0 {
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.TrimSuffix(b.String(), "-")
}

// Add sums its numeric arguments. Integers stay integers.
func Add(v ...any) (any, error) {
	var sum float64
	integral := true
	for _, x := range v {
		f, ok := toFloat(x)
		if !ok {
			return nil, fmt.Errorf("add: %v (%T) is not a number", x, x)
		}
		if f != math.Trunc(f) {
			integral = false
		}
		sum += f
	}
	if integral {
		return int64(sum), nil
	}
	return sum, nil
}

// Join joins the string forms of a list with sep: {{join ", " .routes}}.
func Join(sep string, v any) string {
	r := reflect.ValueOf(v)
	if r.Kind() != reflect.Slice && r.Kind() != reflect.Array {
		return fmt.Sprint(v)
	}
	s := make([]string, r.Len())
	for i := range s {
		s[i] = fmt.Sprint(r.Index(i).Interface())
	}
	return strings.Join(s, sep)
}

// HasKey reports whether map m has key k.
func HasKey(m map[string]any, k string) bool {
	_, ok := m[k]
	return ok
}

func toFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case json.Number:
		f, err := x.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(x, 64)
		return f, err == nil
	}
	r := reflect.ValueOf(v)
	switch r.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(r.Int()), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return float64(r.Uint()), true
	case reflect.Float32, reflect.Float64:
		return r.Float(), true
	}
	return 0, false
}
