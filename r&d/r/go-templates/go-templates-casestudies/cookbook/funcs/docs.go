package funcs

import (
	"fmt"
	"strings"
	"testing/fstest"
	"text/template"
	"time"
)

// Doc documents one function of the FuncMap. Example is a template run with
// Data; Want is the exact output, which the tests verify and which
// docs/FUNCS-SHARED.md prints.
type Doc struct {
	Name    string
	Sig     string
	Example string
	Data    any
	Want    string
	Notes   string
}

// DocOptions is the fixed world the examples run in.
func DocOptions() Options {
	return Options{
		FS:  fstest.MapFS{"hello.txt": {Data: []byte("hello, world")}},
		Env: func(k string) string { return map[string]string{"APP_ENV": "staging"}[k] },
		Now: func() time.Time { return time.Date(2024, 3, 5, 14, 30, 0, 0, time.UTC) },
	}
}

// RenderDoc runs d.Example against d.Data in the documentation world.
func RenderDoc(d Doc) (string, error) {
	t, err := template.New(d.Name).Funcs(New(DocOptions())).Parse(d.Example)
	if err != nil {
		return "", fmt.Errorf("%s: %w", d.Name, err)
	}
	var b strings.Builder
	if err := t.Execute(&b, d.Data); err != nil {
		return "", fmt.Errorf("%s: %w", d.Name, err)
	}
	return b.String(), nil
}

// Docs has one entry per function, in presentation order.
var Docs = []Doc{
	{Name: "dict", Sig: "dict(k1, v1, k2, v2, ...) map[string]any, error",
		Example: `{{ dict "a" 1 "b" 2 | toJSON }}`, Want: `{"a":1,"b":2}`,
		Notes: "Odd argument count or a non-string key is an error wrapping ErrArgs."},
	{Name: "list", Sig: "list(v ...any) []any",
		Example: `{{ list 1 "x" true }}`, Want: `[1 x true]`},
	{Name: "sortedKeys", Sig: "sortedKeys(m) []string, error",
		Example: `{{ sortedKeys . }}`, Data: map[string]int{"b": 1, "a": 2}, Want: `[a b]`,
		Notes: "Any map with string keys. range already visits keys in order; use this when you need the slice."},
	{Name: "seq", Sig: "seq(last) / seq(first, last) / seq(first, step, last) []int, error",
		Example: `{{ seq 3 }} {{ seq 2 4 }} {{ seq 10 -5 0 }}`, Want: `[1 2 3] [2 3 4] [10 5 0]`,
		Notes: "Inclusive, like shell seq. Capped at 1M elements."},
	{Name: "add", Sig: "add(a, b ...) int, error",
		Example: `{{ add 1 2 3 }}`, Want: `6`, Notes: "Accepts any integer type and integral floats (JSON numbers)."},
	{Name: "sub", Sig: "sub(a, b) int, error",
		Example: `{{ sub 10 4 }}`, Want: `6`},
	{Name: "mul", Sig: "mul(a, b ...) int, error",
		Example: `{{ mul 6 7 }}`, Want: `42`},
	{Name: "div", Sig: "div(a, b) int, error",
		Example: `{{ div 7 2 }}`, Want: `3`, Notes: "Truncates toward zero. Division by zero is an error."},
	{Name: "mod", Sig: "mod(a, b) int, error",
		Example: `{{ mod 7 3 }}`, Want: `1`,
		Notes: "Dividend first: `{{ $i | mod 2 }}` is 2 mod $i. Write `mod $i 2`."},
	{Name: "default", Sig: "default(def, v) any",
		Example: `{{ default "n/a" "" }}|{{ default 8080 0 }}`, Want: `n/a|8080`,
		Notes: "Empty means zero value, empty collection or nil. 0 and false are empty."},
	{Name: "coalesce", Sig: "coalesce(v ...any) any",
		Example: `{{ coalesce "" 0 "x" "y" }}`, Want: `x`, Notes: "Returns nil (prints `<no value>`) if every argument is empty."},
	{Name: "ternary", Sig: "ternary(ifTrue, ifFalse, cond) any",
		Example: `{{ ternary "yes" "no" true }}`, Want: `yes`, Notes: "Both branches are evaluated first; use `if` when one of them can fail."},
	{Name: "join", Sig: "join(sep, slice) string, error",
		Example: `{{ join ", " (list "a" 2 "c") }}`, Want: `a, 2, c`},
	{Name: "split", Sig: "split(sep, s) []string",
		Example: `{{ split "," "a,b,c" }}`, Want: `[a b c]`},
	{Name: "upper", Sig: "upper(s) string",
		Example: `{{ upper "abc" }}`, Want: `ABC`},
	{Name: "lower", Sig: "lower(s) string",
		Example: `{{ lower "ABC" }}`, Want: `abc`},
	{Name: "trim", Sig: "trim(s) string",
		Example: `[{{ trim "  x  " }}]`, Want: `[x]`, Notes: "Trims white space at both ends."},
	{Name: "replace", Sig: "replace(old, new, s) string",
		Example: `{{ replace "a" "o" "banana" }}`, Want: `bonono`, Notes: "Subject last, so it pipes."},
	{Name: "repeat", Sig: "repeat(n, s) string, error",
		Example: `{{ repeat 3 "ab" }}`, Want: `ababab`, Notes: "Capped at 1 MiB of output."},
	{Name: "indent", Sig: "indent(n, s) string, error",
		Example: `{{ indent 2 "a\nb\n\nc" }}`, Want: "  a\n  b\n\n  c", Notes: "Empty lines are left empty."},
	{Name: "nindent", Sig: "nindent(n, s) string, error",
		Example: `k:{{ nindent 2 "a\nb" }}`, Want: "k:\n  a\n  b", Notes: "A newline, then indent. The YAML splice idiom."},
	{Name: "plural", Sig: "plural(n, one, other) string, error",
		Example: `1 {{ plural 1 "file" "files" }}, 2 {{ plural 2 "file" "files" }}`, Want: `1 file, 2 files`, Notes: "English only."},
	{Name: "toJSON", Sig: "toJSON(v) string, error",
		Example: `{{ toJSON . }}`, Data: map[string]any{"b": []int{1, 2}, "a": "<x>"}, Want: `{"a":"<x>","b":[1,2]}`,
		Notes: "Sorted keys; `<`, `>` and `&` are not escaped (html/template escapes by context)."},
	{Name: "toPrettyJSON", Sig: "toPrettyJSON(v) string, error",
		Example: `{{ toPrettyJSON . }}`, Data: map[string]any{"a": 1, "b": []int{2}}, Want: "{\n  \"a\": 1,\n  \"b\": [\n    2\n  ]\n}"},
	{Name: "fromJSON", Sig: "fromJSON(s) any, error",
		Example: `{{ (fromJSON "{\"a\":[1,2]}").a }}`, Want: `[1 2]`, Notes: "Numbers decode as float64."},
	{Name: "toYAML", Sig: "toYAML(v) string, error",
		Example: `{{ toYAML . }}`, Data: map[string]any{"name": "x", "on": "yes", "ports": []int{80, 443}},
		Want:  "name: x\n\"on\": \"yes\"\nports:\n  - 80\n  - 443",
		Notes: "Stdlib emitter: sorted keys, quotes strings YAML would reinterpret, literal blocks for multi-line strings."},
	{Name: "b64enc", Sig: "b64enc(s) string",
		Example: `{{ b64enc "hello" }}`, Want: `aGVsbG8=`},
	{Name: "b64dec", Sig: "b64dec(s) string, error",
		Example: `{{ b64dec "aGVsbG8=" }}`, Want: `hello`},
	{Name: "sha256sum", Sig: "sha256sum(s) string",
		Example: `{{ sha256sum "abc" }}`, Want: `ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad`},
	{Name: "env", Sig: "env(name) string",
		Example: `{{ env "APP_ENV" }}`, Want: `staging`, Notes: "Trusted templates only: it reads the process environment unless Options.Env says otherwise."},
	{Name: "file", Sig: "file(path) string, error",
		Example: `{{ file "hello.txt" }}`, Want: `hello, world`, Notes: "Reads from Options.FS; paths must satisfy fs.ValidPath, so `../x` and `/x` fail."},
	{Name: "shq", Sig: "shq(s) string",
		Example: `{{ shq "it's" }} {{ shq "ok-1" }}`, Want: `'it'\''s' ok-1`, Notes: "POSIX single-quoting; safe words pass through."},
	{Name: "now", Sig: "now() time.Time",
		Example: `{{ (now).Format "2006-01-02" }}`, Want: `2024-03-05`, Notes: "Pinned here; time.Now in production."},
	{Name: "dateFmt", Sig: "dateFmt(layout, t) string, error",
		Example: `{{ dateFmt "Jan _2" now }}|{{ dateFmt "2006" 86400 }}`, Want: `Mar  5|1970`, Notes: "Takes time.Time, Unix seconds or an RFC 3339 string."},
	{Name: "regexMatch", Sig: "regexMatch(pattern, s) bool, error",
		Example: `{{ regexMatch "^v[0-9]+$" "v12" }}`, Want: `true`, Notes: "RE2: linear time."},
	{Name: "regexReplaceAll", Sig: "regexReplaceAll(pattern, repl, s) string, error",
		Example: `{{ regexReplaceAll "(a+)" "<${1}>" "caaandy" }}`, Want: `c<aaa>ndy`, Notes: "Write `${1}`, not `$1`."},
}
