// Package funcs is tmplwatch's template function library: a
// consul-template-compatible subset plus a few sprig-style helpers.
//
// Every function is listed in Docs, from which docs/FUNCS.md is generated
// (go generate ./internal/funcs). A test keeps the two in step.
package funcs

//go:generate go run ./mkdoc -o ../../docs/FUNCS.md

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"
)

// KVPair is one entry returned by ls and tree. Key is relative to the
// prefix that was listed.
type KVPair struct {
	Key   string
	Value string
}

// Store is the key/value view a template renders against. The brain's
// implementation records every call as a dependency.
type Store interface {
	// Get returns the value at key and whether it exists.
	Get(key string) (string, bool)
	// List returns pairs under prefix: immediate children only, or the
	// whole subtree when recursive is set. Keys are relative to prefix.
	List(prefix string, recursive bool) []KVPair
	// Missing records that the template needed key and it was absent,
	// which marks the render incomplete.
	Missing(key string)
}

// Options are the per-template inputs to Map.
type Options struct {
	Store Store            // nil means an empty store
	Dir   fs.FS            // root for the file function; nil disables it
	Now   func() time.Time // clock for timestamp; nil means time.Now
	Env   func(string) string
}

// Map returns the function map for one render.
func Map(o Options) template.FuncMap {
	if o.Store == nil {
		o.Store = emptyStore{}
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	if o.Env == nil {
		o.Env = os.Getenv
	}
	st := o.Store
	return template.FuncMap{
		// key/value
		"key": func(k string) string {
			v, ok := st.Get(k)
			if !ok {
				st.Missing(k)
			}
			return v
		},
		"keyOrDefault": func(k, def string) string {
			if v, ok := st.Get(k); ok {
				return v
			}
			return def
		},
		"keyExists": func(k string) bool {
			_, ok := st.Get(k)
			return ok
		},
		"ls":   func(p string) []KVPair { return st.List(p, false) },
		"tree": func(p string) []KVPair { return st.List(p, true) },

		// environment
		"env":  o.Env,
		"file": fileFunc(o.Dir),
		"timestamp": func(layout ...string) (string, error) {
			now := o.Now().UTC()
			switch {
			case len(layout) == 0:
				return now.Format(time.RFC3339), nil
			case len(layout) > 1:
				return "", errors.New("timestamp: at most one layout")
			case layout[0] == "unix":
				return strconv.FormatInt(now.Unix(), 10), nil
			}
			return now.Format(layout[0]), nil
		},

		// encoding
		"toJSON":       toJSON,
		"toPrettyJSON": toPrettyJSON,
		"parseJSON":    parseJSON,
		"toYAML":       ToYAML,
		"toHCL":        ToHCL,
		"b64enc":       func(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) },
		"b64dec": func(s string) (string, error) {
			b, err := base64.StdEncoding.DecodeString(s)
			return string(b), err
		},
		"sha256sum": func(s string) string {
			h := sha256.Sum256([]byte(s))
			return hex.EncodeToString(h[:])
		},
		"shq": Shq,

		// strings
		"indent":          indent,
		"nindent":         func(n int, s string) string { return "\n" + indent(n, s) },
		"regexMatch":      regexMatch,
		"regexReplaceAll": regexReplaceAll,
		"split":           func(sep, s string) []string { return strings.Split(s, sep) },
		"join":            join,
		"trimSpace":       strings.TrimSpace,
		"toUpper":         strings.ToUpper,
		"toLower":         strings.ToLower,
		"replaceAll":      func(old, new, s string) string { return strings.ReplaceAll(s, old, new) },
		"contains":        contains,
		"plural":          plural,

		// network
		"GetPrivateIP":     GetPrivateIP,
		"GetPublicIP":      GetPublicIP,
		"GetInterfaceIP":   GetInterfaceIP,
		"GetAllInterfaces": GetAllInterfaces,
		"sockaddr":         Sockaddr,

		// math and data
		"add":        arith("add", func(a, b int64) (int64, error) { return a + b, nil }),
		"sub":        arith("sub", func(a, b int64) (int64, error) { return a - b, nil }),
		"mul":        arith("mul", func(a, b int64) (int64, error) { return a * b, nil }),
		"mod":        arith("mod", modInt),
		"default":    dflt,
		"coalesce":   coalesce,
		"dict":       dict,
		"list":       func(v ...any) []any { return v },
		"sortedKeys": sortedKeys,
		"values":     values,
		"fail":       func(msg string) (string, error) { return "", errors.New(msg) },
	}
}

type emptyStore struct{}

func (emptyStore) Get(string) (string, bool)  { return "", false }
func (emptyStore) List(string, bool) []KVPair { return nil }
func (emptyStore) Missing(string)             {}
func modInt(a, b int64) (int64, error) {
	if b == 0 {
		return 0, errors.New("mod: division by zero")
	}
	return a % b, nil
}

func fileFunc(dir fs.FS) func(string) (string, error) {
	return func(name string) (string, error) {
		if dir == nil {
			return "", errors.New("file: no template directory")
		}
		// fs.ValidPath rejects "..", absolute paths and empty elements, so
		// a template cannot read outside its own directory.
		name = strings.TrimPrefix(name, "./")
		if !fs.ValidPath(name) {
			return "", fmt.Errorf("file %q: path must be relative to the template directory", name)
		}
		b, err := fs.ReadFile(dir, name)
		if err != nil {
			return "", fmt.Errorf("file: %w", err)
		}
		return string(b), nil
	}
}

func marshal(v any, pretty bool) (string, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	// Config files are not HTML; keep <, > and & readable.
	enc.SetEscapeHTML(false)
	if pretty {
		enc.SetIndent("", "  ")
	}
	if err := enc.Encode(v); err != nil {
		return "", err
	}
	return strings.TrimSuffix(buf.String(), "\n"), nil
}

func toJSON(v any) (string, error)       { return marshal(v, false) }
func toPrettyJSON(v any) (string, error) { return marshal(v, true) }

// parseJSON decodes s. Numbers are json.Number so that ports and counts
// print exactly as written (51820, not 51820.0 or 5.182e+04).
func parseJSON(s string) (any, error) {
	dec := json.NewDecoder(strings.NewReader(s))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, fmt.Errorf("parseJSON: %w", err)
	}
	if dec.More() {
		return nil, errors.New("parseJSON: trailing data")
	}
	return v, nil
}

// indent prefixes every non-empty line of s with n spaces. Empty lines are
// left empty so that output carries no trailing whitespace.
func indent(n int, s string) string {
	if n < 0 {
		n = 0
	}
	pad := strings.Repeat(" ", n)
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if l != "" {
			lines[i] = pad + l
		}
	}
	return strings.Join(lines, "\n")
}

var reCache sync.Map // pattern → *regexp.Regexp

func compile(p string) (*regexp.Regexp, error) {
	if re, ok := reCache.Load(p); ok {
		return re.(*regexp.Regexp), nil
	}
	re, err := regexp.Compile(p)
	if err != nil {
		return nil, err
	}
	reCache.Store(p, re)
	return re, nil
}

func regexMatch(pattern, s string) (bool, error) {
	re, err := compile(pattern)
	if err != nil {
		return false, err
	}
	return re.MatchString(s), nil
}

func regexReplaceAll(pattern, repl, s string) (string, error) {
	re, err := compile(pattern)
	if err != nil {
		return "", err
	}
	return re.ReplaceAllString(s, repl), nil
}

func join(sep string, v any) (string, error) {
	ss, err := strs(v)
	if err != nil {
		return "", fmt.Errorf("join: %w", err)
	}
	return strings.Join(ss, sep), nil
}

// strs converts the list shapes templates produce into []string.
func strs(v any) ([]string, error) {
	switch x := v.(type) {
	case []string:
		return x, nil
	case []any:
		out := make([]string, len(x))
		for i, e := range x {
			out[i] = fmt.Sprint(e)
		}
		return out, nil
	case []KVPair:
		out := make([]string, len(x))
		for i, e := range x {
			out[i] = e.Value
		}
		return out, nil
	case string:
		return []string{x}, nil
	case nil:
		return nil, nil
	}
	return nil, fmt.Errorf("cannot use %T as a list", v)
}

// contains reports whether needle is in haystack: a substring of a
// string, an element of a list, or a key of a map.
func contains(needle, haystack any) (bool, error) {
	n := fmt.Sprint(needle)
	switch h := haystack.(type) {
	case string:
		return strings.Contains(h, n), nil
	case map[string]any:
		_, ok := h[n]
		return ok, nil
	case map[string]string:
		_, ok := h[n]
		return ok, nil
	case []KVPair:
		for _, p := range h {
			if p.Key == n {
				return true, nil
			}
		}
		return false, nil
	}
	ss, err := strs(haystack)
	if err != nil {
		return false, fmt.Errorf("contains: %w", err)
	}
	for _, s := range ss {
		if s == n {
			return true, nil
		}
	}
	return false, nil
}

func plural(one, many string, n any) (string, error) {
	i, err := toInt(n)
	if err != nil {
		return "", fmt.Errorf("plural: %w", err)
	}
	if i == 1 {
		return one, nil
	}
	return many, nil
}

// Shq quotes s for a POSIX shell: 'it'\”s'.
func Shq(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func toInt(v any) (int64, error) {
	switch x := v.(type) {
	case int:
		return int64(x), nil
	case int64:
		return x, nil
	case int32:
		return int64(x), nil
	case uint:
		return int64(x), nil
	case float64:
		if x != float64(int64(x)) {
			return 0, fmt.Errorf("%v is not an integer", x)
		}
		return int64(x), nil
	case json.Number:
		return x.Int64()
	case string:
		return strconv.ParseInt(strings.TrimSpace(x), 10, 64)
	}
	return 0, fmt.Errorf("cannot use %T as an integer", v)
}

func arith(name string, f func(a, b int64) (int64, error)) func(a, b any) (int64, error) {
	return func(a, b any) (int64, error) {
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
}

// empty mirrors text/template's notion of truth for the types templates
// actually hold.
func empty(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case string:
		return x == ""
	case bool:
		return !x
	case int:
		return x == 0
	case int64:
		return x == 0
	case float64:
		return x == 0
	case json.Number:
		return x == "" || x == "0"
	case []any:
		return len(x) == 0
	case []string:
		return len(x) == 0
	case []KVPair:
		return len(x) == 0
	case map[string]any:
		return len(x) == 0
	case map[string]string:
		return len(x) == 0
	}
	return false
}

func dflt(def, v any) any {
	if empty(v) {
		return def
	}
	return v
}

func coalesce(vs ...any) any {
	for _, v := range vs {
		if !empty(v) {
			return v
		}
	}
	return nil
}

func dict(kv ...any) (map[string]any, error) {
	if len(kv)%2 != 0 {
		return nil, errors.New("dict: odd number of arguments")
	}
	m := make(map[string]any, len(kv)/2)
	for i := 0; i < len(kv); i += 2 {
		k, ok := kv[i].(string)
		if !ok {
			return nil, fmt.Errorf("dict: key %v is %T, not string", kv[i], kv[i])
		}
		m[k] = kv[i+1]
	}
	return m, nil
}

func sortedKeys(v any) ([]string, error) {
	var keys []string
	switch m := v.(type) {
	case map[string]any:
		for k := range m {
			keys = append(keys, k)
		}
	case map[string]string:
		for k := range m {
			keys = append(keys, k)
		}
	case []KVPair:
		for _, p := range m {
			keys = append(keys, p.Key)
		}
	default:
		return nil, fmt.Errorf("sortedKeys: cannot use %T as a map", v)
	}
	sort.Strings(keys)
	return keys, nil
}

// values returns the values of a []KVPair in order, or of a map sorted
// by key, so they can be fed to join, toJSON or toYAML as a plain list.
func values(v any) ([]any, error) {
	var out []any
	switch m := v.(type) {
	case []KVPair:
		for _, p := range m {
			out = append(out, p.Value)
		}
	case map[string]any:
		keys, _ := sortedKeys(m)
		for _, k := range keys {
			out = append(out, m[k])
		}
	case map[string]string:
		keys, _ := sortedKeys(m)
		for _, k := range keys {
			out = append(out, m[k])
		}
	default:
		return nil, fmt.Errorf("values: cannot use %T", v)
	}
	if out == nil {
		out = []any{}
	}
	return out, nil
}
