// Package funcs is the shared template FuncMap of the cookbook.
//
// The map is a plain map[string]any, so it converts to text/template.FuncMap
// and html/template.FuncMap without an import of either. Every function is
// pure except env, file and now, and those three read their world through
// Options so that tests (and untrusted-template hosts) can pin or remove it.
package funcs

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"
)

// ErrArgs is wrapped by every error that reports a bad argument list, so a
// host can tell "the template was wrong" from "the world was wrong" with
// errors.Is.
var ErrArgs = errors.New("bad arguments")

// maxOut caps what repeat, seq and indent may allocate. A template is a
// program; do not let it eat the machine.
const maxOut = 1 << 20

// Options is the world the impure functions see.
type Options struct {
	// FS is the root for file. It must be rooted: file never escapes it.
	// A nil FS makes file an error.
	FS fs.FS
	// Env backs env. Nil means os.Getenv. Pass an allowlist closure to
	// run semi-trusted templates.
	Env func(string) string
	// Now backs now. Nil means time.Now.
	Now func() time.Time
}

// Map returns the FuncMap with the process environment, the real clock and
// no filesystem.
func Map() map[string]any { return New(Options{}) }

// New returns the FuncMap bound to o.
func New(o Options) map[string]any {
	if o.Env == nil {
		o.Env = os.Getenv
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	return map[string]any{
		"dict":            Dict,
		"list":            List,
		"sortedKeys":      SortedKeys,
		"seq":             Seq,
		"add":             Add,
		"sub":             Sub,
		"mul":             Mul,
		"div":             Div,
		"mod":             Mod,
		"default":         Default,
		"coalesce":        Coalesce,
		"ternary":         Ternary,
		"join":            Join,
		"split":           Split,
		"upper":           strings.ToUpper,
		"lower":           strings.ToLower,
		"trim":            strings.TrimSpace,
		"replace":         Replace,
		"repeat":          Repeat,
		"indent":          Indent,
		"nindent":         Nindent,
		"plural":          Plural,
		"toJSON":          ToJSON,
		"toPrettyJSON":    ToPrettyJSON,
		"fromJSON":        FromJSON,
		"toYAML":          ToYAML,
		"b64enc":          B64enc,
		"b64dec":          B64dec,
		"sha256sum":       Sha256sum,
		"env":             func(name string) string { return o.Env(name) },
		"file":            func(name string) (string, error) { return readFile(o.FS, name) },
		"shq":             Shq,
		"now":             func() time.Time { return o.Now() },
		"dateFmt":         DateFmt,
		"regexMatch":      RegexMatch,
		"regexReplaceAll": RegexReplaceAll,
	}
}

// Names returns the sorted names in the FuncMap.
func Names() []string {
	m := Map()
	names := make([]string, 0, len(m))
	for k := range m {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// Dict builds a map[string]any from alternating keys and values. It is how a
// template passes several values through the single argument of {{template}}.
func Dict(kv ...any) (map[string]any, error) {
	if len(kv)%2 != 0 {
		return nil, fmt.Errorf("dict: %w: odd number of arguments (%d)", ErrArgs, len(kv))
	}
	m := make(map[string]any, len(kv)/2)
	for i := 0; i < len(kv); i += 2 {
		k, ok := kv[i].(string)
		if !ok {
			return nil, fmt.Errorf("dict: %w: key %d is %T, want string", ErrArgs, i/2, kv[i])
		}
		m[k] = kv[i+1]
	}
	return m, nil
}

// List returns its arguments as a []any: the slice literal templates lack.
func List(v ...any) []any {
	if v == nil {
		return []any{}
	}
	return v
}

// SortedKeys returns the keys of a map with string keys, sorted. A nil value
// has no keys.
func SortedKeys(m any) ([]string, error) {
	if m == nil {
		return []string{}, nil
	}
	rv := reflect.ValueOf(m)
	if rv.Kind() != reflect.Map || rv.Type().Key().Kind() != reflect.String {
		return nil, fmt.Errorf("sortedKeys: %w: want map with string keys, got %T", ErrArgs, m)
	}
	keys := make([]string, 0, rv.Len())
	for _, k := range rv.MapKeys() {
		keys = append(keys, k.String())
	}
	sort.Strings(keys)
	return keys, nil
}

// Seq is shell seq for templates: seq(last), seq(first, last) or
// seq(first, step, last). It is inclusive and 1-based by default.
func Seq(args ...any) ([]int, error) {
	n := make([]int, len(args))
	for i, a := range args {
		v, err := toInt(a)
		if err != nil {
			return nil, fmt.Errorf("seq: %w", err)
		}
		n[i] = v
	}
	first, step, last := 1, 1, 0
	switch len(n) {
	case 1:
		last = n[0]
	case 2:
		first, last = n[0], n[1]
	case 3:
		first, step, last = n[0], n[1], n[2]
	default:
		return nil, fmt.Errorf("seq: %w: want 1 to 3 arguments, got %d", ErrArgs, len(n))
	}
	if step == 0 {
		return nil, fmt.Errorf("seq: %w: step is zero", ErrArgs)
	}
	out := []int{}
	for v := first; (step > 0 && v <= last) || (step < 0 && v >= last); v += step {
		if len(out) >= maxOut {
			return nil, fmt.Errorf("seq: %w: more than %d elements", ErrArgs, maxOut)
		}
		out = append(out, v)
	}
	return out, nil
}

// toInt converts the numeric types a template can hold to int. Floats must be
// integral: JSON data arrives as float64 and 3.0 is fine, 3.5 is not.
func toInt(v any) (int, error) {
	if n, ok := v.(json.Number); ok {
		i, err := n.Int64()
		if err != nil {
			return 0, fmt.Errorf("%w: %s is not an integer", ErrArgs, n)
		}
		return int(i), nil
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return int(rv.Int()), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return int(rv.Uint()), nil
	case reflect.Float32, reflect.Float64:
		f := rv.Float()
		if f == math.Trunc(f) && math.Abs(f) < 1<<53 {
			return int(f), nil
		}
	}
	return 0, fmt.Errorf("%w: %v (%T) is not an integer", ErrArgs, v, v)
}

func ints(name string, args ...any) ([]int, error) {
	out := make([]int, len(args))
	for i, a := range args {
		v, err := toInt(a)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		out[i] = v
	}
	return out, nil
}

// Add sums one or more integers.
func Add(a any, rest ...any) (int, error) {
	n, err := ints("add", append([]any{a}, rest...)...)
	if err != nil {
		return 0, err
	}
	s := 0
	for _, v := range n {
		s += v
	}
	return s, nil
}

// Mul multiplies one or more integers.
func Mul(a any, rest ...any) (int, error) {
	n, err := ints("mul", append([]any{a}, rest...)...)
	if err != nil {
		return 0, err
	}
	p := 1
	for _, v := range n {
		p *= v
	}
	return p, nil
}

// Sub returns a-b.
func Sub(a, b any) (int, error) {
	n, err := ints("sub", a, b)
	if err != nil {
		return 0, err
	}
	return n[0] - n[1], nil
}

// Div returns a/b truncated toward zero. Dividing by zero is an error, not a
// panic.
func Div(a, b any) (int, error) {
	n, err := ints("div", a, b)
	if err != nil {
		return 0, err
	}
	if n[1] == 0 {
		return 0, fmt.Errorf("div: %w: division by zero", ErrArgs)
	}
	return n[0] / n[1], nil
}

// Mod returns a%b with the sign of a. Argument order matters:
// "{{ $i | mod 2 }}" is 2%$i, not $i%2. Write mod $i 2.
func Mod(a, b any) (int, error) {
	n, err := ints("mod", a, b)
	if err != nil {
		return 0, err
	}
	if n[1] == 0 {
		return 0, fmt.Errorf("mod: %w: division by zero", ErrArgs)
	}
	return n[0] % n[1], nil
}

// empty reports whether v is the zero value of its kind or has no elements.
// It is the same notion of emptiness as "if" in a template, with one
// addition: a zero struct is empty.
func empty(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Array, reflect.Map, reflect.Slice, reflect.String:
		return rv.Len() == 0
	case reflect.Pointer, reflect.Interface, reflect.Func, reflect.Chan:
		return rv.IsNil()
	}
	return rv.IsZero()
}

// Default returns v unless it is empty, in which case it returns def. Empty
// includes 0 and false: a deliberate zero is indistinguishable from unset.
func Default(def, v any) any {
	if empty(v) {
		return def
	}
	return v
}

// Coalesce returns the first non-empty argument, or nil.
func Coalesce(vs ...any) any {
	for _, v := range vs {
		if !empty(v) {
			return v
		}
	}
	return nil
}

// Ternary returns t if cond is true, else f. Both are evaluated before the
// call; it is a function, not a conditional.
func Ternary(t, f any, cond bool) any {
	if cond {
		return t
	}
	return f
}

// Join joins the elements of a slice or array with sep, formatting each with
// fmt.Sprint.
func Join(sep string, v any) (string, error) {
	switch x := v.(type) {
	case nil:
		return "", nil
	case []string:
		return strings.Join(x, sep), nil
	}
	rv := reflect.ValueOf(v)
	if k := rv.Kind(); k != reflect.Slice && k != reflect.Array {
		return "", fmt.Errorf("join: %w: want slice, got %T", ErrArgs, v)
	}
	parts := make([]string, rv.Len())
	for i := range parts {
		parts[i] = fmt.Sprint(rv.Index(i).Interface())
	}
	return strings.Join(parts, sep), nil
}

// Split splits s around sep. Note the order: separator first, so it pipes.
func Split(sep, s string) []string { return strings.Split(s, sep) }

// Replace replaces every old in s with new. The subject comes last so that
// "{{ .X | replace "a" "b" }}" works.
func Replace(old, new, s string) string { return strings.ReplaceAll(s, old, new) }

// Repeat returns s repeated n times.
func Repeat(n any, s string) (string, error) {
	c, err := toInt(n)
	if err != nil {
		return "", fmt.Errorf("repeat: %w", err)
	}
	if c < 0 || (len(s) > 0 && c > maxOut/len(s)) {
		return "", fmt.Errorf("repeat: %w: count %d out of range", ErrArgs, c)
	}
	return strings.Repeat(s, c), nil
}

// Indent prefixes every non-empty line of s with n spaces. Empty lines stay
// empty so the result carries no trailing whitespace.
func Indent(n any, s string) (string, error) {
	c, err := toInt(n)
	if err != nil {
		return "", fmt.Errorf("indent: %w", err)
	}
	if c < 0 || c > 1024 {
		return "", fmt.Errorf("indent: %w: width %d out of range", ErrArgs, c)
	}
	pad := strings.Repeat(" ", c)
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if l != "" {
			lines[i] = pad + l
		}
	}
	return strings.Join(lines, "\n"), nil
}

// Nindent is Indent with a leading newline: the idiom for splicing a block
// after "key:" in YAML.
func Nindent(n any, s string) (string, error) {
	r, err := Indent(n, s)
	if err != nil {
		return "", fmt.Errorf("nindent: %w", err)
	}
	return "\n" + r, nil
}

// Plural returns one if n is 1, else other. English rules only; real i18n
// needs CLDR plural categories.
func Plural(n any, one, other string) (string, error) {
	c, err := toInt(n)
	if err != nil {
		return "", fmt.Errorf("plural: %w", err)
	}
	if c == 1 {
		return one, nil
	}
	return other, nil
}

func marshal(v any, indent string) (string, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false) // output is data for a file, not a page
	if indent != "" {
		enc.SetIndent("", indent)
	}
	if err := enc.Encode(v); err != nil {
		return "", err
	}
	return strings.TrimSuffix(b.String(), "\n"), nil
}

// ToJSON marshals v compactly, with sorted map keys and without HTML
// escaping. Inside html/template the result is escaped for its context anyway.
func ToJSON(v any) (string, error) { return marshal(v, "") }

// ToPrettyJSON marshals v with two-space indentation.
func ToPrettyJSON(v any) (string, error) { return marshal(v, "  ") }

// FromJSON decodes a JSON document. Numbers become float64.
func FromJSON(s string) (any, error) {
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return nil, fmt.Errorf("fromJSON: %w", err)
	}
	return v, nil
}

// B64enc is standard base64 with padding.
func B64enc(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

// B64dec decodes standard base64 with padding.
func B64dec(s string) (string, error) {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return "", fmt.Errorf("b64dec: %w", err)
	}
	return string(b), nil
}

// Sha256sum returns the lowercase hex SHA-256 of s.
func Sha256sum(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// readFile reads name from fsys. Names must satisfy fs.ValidPath: unrooted,
// slash-separated, no "." or ".." elements. That is the whole sandbox.
func readFile(fsys fs.FS, name string) (string, error) {
	if fsys == nil {
		return "", errors.New("file: no filesystem configured")
	}
	if !fs.ValidPath(name) {
		return "", fmt.Errorf("file: invalid path %q", name)
	}
	b, err := fs.ReadFile(fsys, name)
	if err != nil {
		return "", fmt.Errorf("file: %w", err)
	}
	return string(b), nil
}

// Shq quotes s for a POSIX shell. Strings made only of safe characters are
// returned as is; everything else goes in single quotes, with embedded single
// quotes written as '\” .
func Shq(s string) string {
	if s != "" && strings.Trim(s, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_@%+=:,./-") == "" {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// DateFmt formats a time.Time, a *time.Time, Unix seconds or an RFC 3339
// string with a Go reference layout. The layout comes first so it pipes:
// "{{ .When | dateFmt "2006-01-02" }}".
func DateFmt(layout string, v any) (string, error) {
	switch x := v.(type) {
	case time.Time:
		return x.Format(layout), nil
	case *time.Time:
		if x == nil {
			return "", fmt.Errorf("dateFmt: %w: nil time", ErrArgs)
		}
		return x.Format(layout), nil
	case string:
		t, err := time.Parse(time.RFC3339, x)
		if err != nil {
			return "", fmt.Errorf("dateFmt: %w", err)
		}
		return t.Format(layout), nil
	}
	sec, err := toInt(v)
	if err != nil {
		return "", fmt.Errorf("dateFmt: %w", err)
	}
	return time.Unix(int64(sec), 0).UTC().Format(layout), nil
}

// RegexMatch reports whether s contains a match of the RE2 pattern. RE2 runs
// in linear time, so a hostile pattern cannot hang the renderer.
func RegexMatch(pattern, s string) (bool, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return false, fmt.Errorf("regexMatch: %w", err)
	}
	return re.MatchString(s), nil
}

// RegexReplaceAll replaces every match of pattern in s with repl, in which
// ${1} expands to the first group. Prefer ${1} to $1: "$1x" names group "1x".
func RegexReplaceAll(pattern, repl, s string) (string, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return "", fmt.Errorf("regexReplaceAll: %w", err)
	}
	return re.ReplaceAllString(s, repl), nil
}
