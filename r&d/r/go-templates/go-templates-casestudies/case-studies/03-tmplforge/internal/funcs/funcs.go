// Package funcs is tmplforge's template function library.
//
// Every function is plain Go with no dependencies beyond the standard
// library, so the whole package can be copied verbatim into an exported
// bundle (see forge.Export) and still compile. Functions take their
// "subject" argument last so they read naturally in pipelines:
//
//	{{ .name | default "world" | upper }}
//
// Two maps are offered. All is for trusted operators (the TUI, the CLI);
// Safe is for untrusted template authors (the web GUI sandbox) and omits
// the functions that reach outside the process: env and file.
package funcs

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"math"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"time"
)

// Limits that keep a hostile template from turning one function call into
// an allocation bomb. The sandbox also caps total output; these stop the
// damage before it reaches the writer.
const (
	MaxRepeatBytes = 1 << 20 // repeat/indent results
	MaxRegexLen    = 1 << 10 // regex source length
	MaxFileBytes   = 1 << 20 // file() read cap
	MaxSeqLen      = 1 << 31 // seq count cap (iteration is lazy; time-out bounds it)
)

// All returns the complete FuncMap, including env and file.
func All() template.FuncMap {
	return template.FuncMap{
		"dict":            dict,
		"list":            list,
		"default":         dflt,
		"coalesce":        coalesce,
		"ternary":         ternary,
		"join":            join,
		"split":           split,
		"upper":           strings.ToUpper,
		"lower":           strings.ToLower,
		"trim":            strings.TrimSpace,
		"replace":         replace,
		"repeat":          repeat,
		"indent":          indent,
		"nindent":         nindent,
		"plural":          plural,
		"add":             add,
		"sub":             sub,
		"mul":             mul,
		"div":             div,
		"mod":             mod,
		"seq":             seq,
		"sortedKeys":      sortedKeys,
		"toJSON":          toJSON,
		"toPrettyJSON":    toPrettyJSON,
		"fromJSON":        fromJSON,
		"toYAML":          toYAML,
		"b64enc":          b64enc,
		"b64dec":          b64dec,
		"sha256sum":       sha256sum,
		"now":             now,
		"dateFmt":         dateFmt,
		"regexMatch":      regexMatch,
		"regexReplaceAll": regexReplaceAll,
		"shq":             shq,
		"env":             os.Getenv,
		"file":            file,
	}
}

// Unsafe lists the functions Safe removes: they read process or host state.
var Unsafe = []string{"env", "file"}

// Safe returns All minus env and file, for untrusted template authors.
func Safe() template.FuncMap {
	m := All()
	for _, k := range Unsafe {
		delete(m, k)
	}
	return m
}

// dict builds a map from alternating string keys and values.
func dict(kv ...any) (map[string]any, error) {
	if len(kv)%2 != 0 {
		return nil, errors.New("dict: odd number of arguments")
	}
	m := make(map[string]any, len(kv)/2)
	for i := 0; i < len(kv); i += 2 {
		k, ok := kv[i].(string)
		if !ok {
			return nil, fmt.Errorf("dict: key %d is %T, want string", i/2, kv[i])
		}
		m[k] = kv[i+1]
	}
	return m, nil
}

func list(v ...any) []any {
	if v == nil {
		return []any{}
	}
	return v
}

// empty reports whether v is the zero value of its type, or an empty
// collection: the definition default and coalesce use.
func empty(v any) bool {
	if v == nil {
		return true
	}
	r := reflect.ValueOf(v)
	switch r.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return r.Len() == 0
	case reflect.Pointer, reflect.Interface:
		return r.IsNil()
	}
	return r.IsZero()
}

func dflt(d, v any) any {
	if empty(v) {
		return d
	}
	return v
}

func coalesce(v ...any) any {
	for _, x := range v {
		if !empty(x) {
			return x
		}
	}
	return nil
}

func ternary(a, b any, cond bool) any {
	if cond {
		return a
	}
	return b
}

// strs flattens a string or list into []string.
func strs(v any) ([]string, error) {
	switch x := v.(type) {
	case nil:
		return nil, nil
	case []string:
		return x, nil
	case string:
		return []string{x}, nil
	}
	r := reflect.ValueOf(v)
	if r.Kind() != reflect.Slice && r.Kind() != reflect.Array {
		return nil, fmt.Errorf("want list, got %T", v)
	}
	out := make([]string, r.Len())
	for i := range out {
		out[i] = fmt.Sprint(r.Index(i).Interface())
	}
	return out, nil
}

func join(sep string, v any) (string, error) {
	s, err := strs(v)
	if err != nil {
		return "", fmt.Errorf("join: %w", err)
	}
	return strings.Join(s, sep), nil
}

func split(sep, s string) []string { return strings.Split(s, sep) }

func replace(old, new, s string) string { return strings.ReplaceAll(s, old, new) }

func repeat(n any, s string) (string, error) {
	c, err := toInt(n)
	if err != nil {
		return "", fmt.Errorf("repeat: %w", err)
	}
	if c < 0 {
		return "", errors.New("repeat: negative count")
	}
	if c > 0 && len(s) > MaxRepeatBytes/c {
		return "", fmt.Errorf("repeat: result exceeds %d bytes", MaxRepeatBytes)
	}
	return strings.Repeat(s, c), nil
}

func indent(n any, s string) (string, error) {
	c, err := toInt(n)
	if err != nil {
		return "", fmt.Errorf("indent: %w", err)
	}
	if c < 0 || c > 256 {
		return "", errors.New("indent: width out of range 0..256")
	}
	pad := strings.Repeat(" ", c)
	lines := strings.Split(s, "\n")
	if (len(s)+len(pad))*len(lines) > MaxRepeatBytes*4 {
		return "", fmt.Errorf("indent: result too large")
	}
	for i, l := range lines {
		if l != "" {
			lines[i] = pad + l
		}
	}
	return strings.Join(lines, "\n"), nil
}

func nindent(n any, s string) (string, error) {
	r, err := indent(n, s)
	if err != nil {
		return "", err
	}
	return "\n" + r, nil
}

func plural(one, many string, n any) (string, error) {
	c, err := toInt(n)
	if err != nil {
		return "", fmt.Errorf("plural: %w", err)
	}
	if c == 1 {
		return one, nil
	}
	return many, nil
}

// toInt converts the numeric shapes a fixture or literal can take.
func toInt(v any) (int, error) {
	switch x := v.(type) {
	case int:
		return x, nil
	case int64:
		return int(x), nil
	case int32:
		return int(x), nil
	case uint:
		return int(x), nil
	case uint64:
		return int(x), nil
	case float64:
		if x != math.Trunc(x) || math.IsInf(x, 0) || math.IsNaN(x) {
			return 0, fmt.Errorf("%v is not an integer", x)
		}
		return int(x), nil
	case json.Number:
		i, err := x.Int64()
		return int(i), err
	case string:
		return strconv.Atoi(strings.TrimSpace(x))
	}
	return 0, fmt.Errorf("cannot use %T as integer", v)
}

func arith(name string, a, b any, f func(x, y int) (int, error)) (int, error) {
	x, err := toInt(a)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", name, err)
	}
	y, err := toInt(b)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", name, err)
	}
	return f(x, y)
}

func add(a, b any) (int, error) {
	return arith("add", a, b, func(x, y int) (int, error) { return x + y, nil })
}

func sub(a, b any) (int, error) {
	return arith("sub", a, b, func(x, y int) (int, error) { return x - y, nil })
}

func mul(a, b any) (int, error) {
	return arith("mul", a, b, func(x, y int) (int, error) { return x * y, nil })
}

func div(a, b any) (int, error) {
	return arith("div", a, b, func(x, y int) (int, error) {
		if y == 0 {
			return 0, errors.New("div: division by zero")
		}
		return x / y, nil
	})
}

func mod(a, b any) (int, error) {
	return arith("mod", a, b, func(x, y int) (int, error) {
		if y == 0 {
			return 0, errors.New("mod: division by zero")
		}
		return x % y, nil
	})
}

// seq yields 1..n (one argument) or a..b (two). It is lazy so that a
// large range costs time, which the sandbox bounds, rather than memory,
// which it cannot bound as cheaply.
func seq(args ...any) (iter.Seq[int], error) {
	var lo, hi int
	var err error
	switch len(args) {
	case 1:
		lo = 1
		hi, err = toInt(args[0])
	case 2:
		if lo, err = toInt(args[0]); err == nil {
			hi, err = toInt(args[1])
		}
	default:
		return nil, errors.New("seq: want 1 or 2 arguments")
	}
	if err != nil {
		return nil, fmt.Errorf("seq: %w", err)
	}
	if hi-lo > MaxSeqLen {
		return nil, fmt.Errorf("seq: range larger than %d", MaxSeqLen)
	}
	return func(yield func(int) bool) {
		for i := lo; i <= hi; i++ {
			if !yield(i) {
				return
			}
		}
	}, nil
}

func sortedKeys(m any) ([]string, error) {
	r := reflect.ValueOf(m)
	if r.Kind() != reflect.Map {
		return nil, fmt.Errorf("sortedKeys: want map, got %T", m)
	}
	keys := make([]string, 0, r.Len())
	for _, k := range r.MapKeys() {
		keys = append(keys, fmt.Sprint(k.Interface()))
	}
	sort.Strings(keys)
	return keys, nil
}

func toJSON(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("toJSON: %w", err)
	}
	return string(b), nil
}

func toPrettyJSON(v any) (string, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", fmt.Errorf("toPrettyJSON: %w", err)
	}
	return string(b), nil
}

func fromJSON(s string) (any, error) {
	var v any
	d := json.NewDecoder(strings.NewReader(s))
	d.UseNumber()
	if err := d.Decode(&v); err != nil {
		return nil, fmt.Errorf("fromJSON: %w", err)
	}
	return Normalize(v), nil
}

// Normalize rewrites json.Number values into int (when integral and in
// range) or float64, recursively. Templates then compare and print numbers
// the way authors expect: {{if gt .port 1024}} works and 8080 prints as
// 8080, not 8.08e+03.
func Normalize(v any) any {
	switch x := v.(type) {
	case json.Number:
		if i, err := x.Int64(); err == nil && i >= math.MinInt && i <= math.MaxInt {
			return int(i)
		}
		f, _ := x.Float64()
		return f
	case map[string]any:
		for k, e := range x {
			x[k] = Normalize(e)
		}
	case []any:
		for i, e := range x {
			x[i] = Normalize(e)
		}
	}
	return v
}

func b64enc(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

func b64dec(s string) (string, error) {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return "", fmt.Errorf("b64dec: %w", err)
	}
	return string(b), nil
}

func sha256sum(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// now honours SOURCE_DATE_EPOCH (reproducible-builds.org) so generated
// artifacts can be byte-for-byte reproducible.
func now() time.Time {
	if s := os.Getenv("SOURCE_DATE_EPOCH"); s != "" {
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			return time.Unix(n, 0).UTC()
		}
	}
	return time.Now().UTC()
}

// dateFmt formats t with a Go layout. t may be a time.Time, an RFC 3339
// string, a YYYY-MM-DD string, or Unix seconds.
func dateFmt(layout string, t any) (string, error) {
	switch x := t.(type) {
	case time.Time:
		return x.Format(layout), nil
	case string:
		for _, l := range []string{time.RFC3339, "2006-01-02"} {
			if p, err := time.Parse(l, x); err == nil {
				return p.Format(layout), nil
			}
		}
		return "", fmt.Errorf("dateFmt: cannot parse %q", x)
	}
	n, err := toInt(t)
	if err != nil {
		return "", fmt.Errorf("dateFmt: %w", err)
	}
	return time.Unix(int64(n), 0).UTC().Format(layout), nil
}

func compile(re string) (*regexp.Regexp, error) {
	if len(re) > MaxRegexLen {
		return nil, fmt.Errorf("regex longer than %d bytes", MaxRegexLen)
	}
	return regexp.Compile(re)
}

func regexMatch(re, s string) (bool, error) {
	r, err := compile(re)
	if err != nil {
		return false, fmt.Errorf("regexMatch: %w", err)
	}
	return r.MatchString(s), nil
}

func regexReplaceAll(re, repl, s string) (string, error) {
	r, err := compile(re)
	if err != nil {
		return "", fmt.Errorf("regexReplaceAll: %w", err)
	}
	return r.ReplaceAllString(s, repl), nil
}

// shq quotes s for a POSIX shell: wrap it in single quotes, and spell
// each embedded single quote as close-quote, backslash-quote, open-quote:
//
//	it's  ->  'it'\''s'
func shq(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func file(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("file: %w", err)
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, MaxFileBytes+1))
	if err != nil {
		return "", fmt.Errorf("file: %w", err)
	}
	if len(b) > MaxFileBytes {
		return "", fmt.Errorf("file: %s exceeds %d bytes", path, MaxFileBytes)
	}
	return string(b), nil
}
