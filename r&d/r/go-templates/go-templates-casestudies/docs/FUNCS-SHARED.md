# Shared FuncMap

> **Generated file. Do not edit.** Produced by `cookbook gen -funcs` from `funcs.Docs`; every example
> is executed by `go test ./funcs` and must print exactly the output shown.

36 functions, available to every formula in [COOKBOOK.md](COOKBOOK.md) and to any program that
calls `funcs.New(funcs.Options{...})` or `funcs.Map()`. The map is a plain `map[string]any`, so it
converts to both `text/template.FuncMap` and `html/template.FuncMap`.

`env`, `file` and `now` are the only functions that touch the outside world; `funcs.Options` replaces
all three. Argument order puts the value last wherever a pipeline would supply it
(`{{ .X | default "a" }}`). Every argument error wraps `funcs.ErrArgs`.

| Function | Signature | Example | Output | Notes |
|----------|-----------|---------|--------|-------|
| `dict` | `dict(k1, v1, k2, v2, ...) map[string]any, error` | `{{ dict "a" 1 "b" 2 \| toJSON }}` | `{"a":1,"b":2}` | Odd argument count or a non-string key is an error wrapping ErrArgs. |
| `list` | `list(v ...any) []any` | `{{ list 1 "x" true }}` | `[1 x true]` |  |
| `sortedKeys` | `sortedKeys(m) []string, error` | `{{ sortedKeys . }}` | `[a b]` | Any map with string keys. range already visits keys in order; use this when you need the slice. |
| `seq` | `seq(last) / seq(first, last) / seq(first, step, last) []int, error` | `{{ seq 3 }} {{ seq 2 4 }} {{ seq 10 -5 0 }}` | `[1 2 3] [2 3 4] [10 5 0]` | Inclusive, like shell seq. Capped at 1M elements. |
| `add` | `add(a, b ...) int, error` | `{{ add 1 2 3 }}` | `6` | Accepts any integer type and integral floats (JSON numbers). |
| `sub` | `sub(a, b) int, error` | `{{ sub 10 4 }}` | `6` |  |
| `mul` | `mul(a, b ...) int, error` | `{{ mul 6 7 }}` | `42` |  |
| `div` | `div(a, b) int, error` | `{{ div 7 2 }}` | `3` | Truncates toward zero. Division by zero is an error. |
| `mod` | `mod(a, b) int, error` | `{{ mod 7 3 }}` | `1` | Dividend first: `{{ $i \| mod 2 }}` is 2 mod $i. Write `mod $i 2`. |
| `default` | `default(def, v) any` | `{{ default "n/a" "" }}\|{{ default 8080 0 }}` | `n/a\|8080` | Empty means zero value, empty collection or nil. 0 and false are empty. |
| `coalesce` | `coalesce(v ...any) any` | `{{ coalesce "" 0 "x" "y" }}` | `x` | Returns nil (prints `<no value>`) if every argument is empty. |
| `ternary` | `ternary(ifTrue, ifFalse, cond) any` | `{{ ternary "yes" "no" true }}` | `yes` | Both branches are evaluated first; use `if` when one of them can fail. |
| `join` | `join(sep, slice) string, error` | `{{ join ", " (list "a" 2 "c") }}` | `a, 2, c` |  |
| `split` | `split(sep, s) []string` | `{{ split "," "a,b,c" }}` | `[a b c]` |  |
| `upper` | `upper(s) string` | `{{ upper "abc" }}` | `ABC` |  |
| `lower` | `lower(s) string` | `{{ lower "ABC" }}` | `abc` |  |
| `trim` | `trim(s) string` | `[{{ trim "  x  " }}]` | `[x]` | Trims white space at both ends. |
| `replace` | `replace(old, new, s) string` | `{{ replace "a" "o" "banana" }}` | `bonono` | Subject last, so it pipes. |
| `repeat` | `repeat(n, s) string, error` | `{{ repeat 3 "ab" }}` | `ababab` | Capped at 1 MiB of output. |
| `indent` | `indent(n, s) string, error` | `{{ indent 2 "a\nb\n\nc" }}` | `  a\n  b\n\n  c` | Empty lines are left empty. |
| `nindent` | `nindent(n, s) string, error` | `k:{{ nindent 2 "a\nb" }}` | `k:\n  a\n  b` | A newline, then indent. The YAML splice idiom. |
| `plural` | `plural(n, one, other) string, error` | `1 {{ plural 1 "file" "files" }}, 2 {{ plural 2 "file" "files" }}` | `1 file, 2 files` | English only. |
| `toJSON` | `toJSON(v) string, error` | `{{ toJSON . }}` | `{"a":"<x>","b":[1,2]}` | Sorted keys; `<`, `>` and `&` are not escaped (html/template escapes by context). |
| `toPrettyJSON` | `toPrettyJSON(v) string, error` | `{{ toPrettyJSON . }}` | `{\n  "a": 1,\n  "b": [\n    2\n  ]\n}` |  |
| `fromJSON` | `fromJSON(s) any, error` | `{{ (fromJSON "{\"a\":[1,2]}").a }}` | `[1 2]` | Numbers decode as float64. |
| `toYAML` | `toYAML(v) string, error` | `{{ toYAML . }}` | `name: x\n"on": "yes"\nports:\n  - 80\n  - 443` | Stdlib emitter: sorted keys, quotes strings YAML would reinterpret, literal blocks for multi-line strings. |
| `b64enc` | `b64enc(s) string` | `{{ b64enc "hello" }}` | `aGVsbG8=` |  |
| `b64dec` | `b64dec(s) string, error` | `{{ b64dec "aGVsbG8=" }}` | `hello` |  |
| `sha256sum` | `sha256sum(s) string` | `{{ sha256sum "abc" }}` | `ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad` |  |
| `env` | `env(name) string` | `{{ env "APP_ENV" }}` | `staging` | Trusted templates only: it reads the process environment unless Options.Env says otherwise. |
| `file` | `file(path) string, error` | `{{ file "hello.txt" }}` | `hello, world` | Reads from Options.FS; paths must satisfy fs.ValidPath, so `../x` and `/x` fail. |
| `shq` | `shq(s) string` | `{{ shq "it's" }} {{ shq "ok-1" }}` | `'it'\''s' ok-1` | POSIX single-quoting; safe words pass through. |
| `now` | `now() time.Time` | `{{ (now).Format "2006-01-02" }}` | `2024-03-05` | Pinned here; time.Now in production. |
| `dateFmt` | `dateFmt(layout, t) string, error` | `{{ dateFmt "Jan _2" now }}\|{{ dateFmt "2006" 86400 }}` | `Mar  5\|1970` | Takes time.Time, Unix seconds or an RFC 3339 string. |
| `regexMatch` | `regexMatch(pattern, s) bool, error` | `{{ regexMatch "^v[0-9]+$" "v12" }}` | `true` | RE2: linear time. |
| `regexReplaceAll` | `regexReplaceAll(pattern, repl, s) string, error` | `{{ regexReplaceAll "(a+)" "<${1}>" "caaandy" }}` | `c<aaa>ndy` | Write `${1}`, not `$1`. |
