# Go Templates — Basic Cheat Sheet

Every row below was executed with go1.24.7 (`text/template` unless stated); outputs are
exact. Source of the verification program: `docs/verify/cheatsheet_verify.go`.

## 1. Anatomy

| Thing | Syntax | Notes |
|---|---|---|
| Action | `{{ pipeline }}` | everything outside actions is copied verbatim |
| Comment | `{{/* note */}}` | must be the only thing in the action; may trim: `{{- /* c */ -}}` |
| Trim whitespace | `{{- ` and ` -}}` | the **space** after `{{-` / before `-}}` is required |
| Dot | `.` | the current data value |
| Root | `$` | the value passed to `Execute`, even inside `range`/`with` |
| Field | `.Name`, `.A.B` | exported struct fields, map keys, or **methods** |
| Map key with odd chars | `{{index . "k-1"}}` | `.k-1` is not a legal field name |
| Variable | `{{$x := .Name}}` | scope: to the enclosing `end` |
| Reassign (Go 1.11+) | `{{$x = 2}}` | lets you build accumulators |
| Pipeline | `{{.Name \| printf "%q"}}` | previous result becomes the **last** argument |
| Parentheses | `{{printf "%s" (index . 0)}}` | group sub-commands |

## 2. Control flow (verified)

| Template | Data | Output |
|---|---|---|
| `{{if .A}}a{{else if .B}}b{{else}}c{{end}}` | — | falsy: `false`, `0`, `nil`, empty string/slice/map |
| `{{with .}}{{.}}{{else}}empty{{end}}` | `""` | `empty` |
| `{{range .}}x{{else}}none{{end}}` | `[]int{}` | `none` |
| `{{range $i, $v := .}}{{$i}}={{$v}} {{end}}` | `[]string{"x","y"}` | `0=x 1=y ` |
| `{{range $k, $v := .}}{{$k}}={{$v}} {{end}}` | `map[string]int{"b":2,"a":1}` | `a=1 b=2 ` (map keys sorted) |
| `{{range 3}}{{.}}{{end}}` (Go 1.24) | — | `012` — **one** variable only: `{{range $i, $v := 3}}` errors |
| `{{range .}}{{.}} {{end}}` (Go 1.24) | `iter.Seq[int]` yielding 10,11,12 | `10 11 12 ` |
| `{{range $k, $v := .}}{{$k}}={{$v}} {{end}}` (Go 1.24) | `iter.Seq2[string,int]` | `a=1 b=2 ` |
| `{{range .}}{{if eq . 2}}{{break}}{{end}}{{.}}{{end}}` | `[]int{0,1,2,3}` | `01` |
| `{{range .}}{{if eq . 1}}{{continue}}{{end}}{{.}}{{end}}` | `[]int{0,1,2}` | `02` |
| `{{range .L}}{{$.N}}/{{.}} {{end}}` | `{N:"org",L:[a b]}` | `org/a org/b ` |
| `{{$x := 0}}{{range .}}{{$x = add $x .}}{{end}}{{$x}}` | `[]int{1,2,3}` + `add` func | `6` |
| `a {{- 1 -}} b` | — | `a1b` |

## 3. Templates calling templates

| Template | Output |
|---|---|
| `{{define "r"}}<{{.}}>{{end}}{{template "r" "x"}}` | `<x>` |
| `{{block "c" .}}default {{.}}{{end}}` with data `d` | `default d` — `block` = `define` + `template` in one; a later `define "c"` overrides it |
| `{{template "name"}}` | executes with **nil** data; pass `.` explicitly |

## 4. The nineteen builtins (verified)

| Builtin | Example | Output |
|---|---|---|
| `and` | `{{and false (index . 5)}}` on `[]int{}` | `false` — short-circuits (Go 1.18+), so the out-of-range index is never evaluated |
| `or` | `{{or "" "fallback"}}` | `fallback` |
| `not` | `{{not true}}` | `false` |
| `len` | `{{len .}}` on `"héllo"` | `6` (bytes, not runes) |
| `index` | `{{index . "k-1"}}` | map/slice/array lookup; multiple indices for nesting |
| `slice` | `{{slice . 1 3}}` on `[0 1 2 3]` | `[1 2]` |
| `print` / `println` | `{{print 1 "a" 2}}\|{{println "x"}}` | `1a2\|x⏎` (`print` adds spaces only between non-strings) |
| `printf` | `{{printf "%05d\|%.2f\|%q" 42 3.14159 "s"}}` | `00042\|3.14\|"s"` |
| `call` | `{{call . 2 3}}` on `func(a,b int) int` | `6` |
| `html` | `{{"<a&b>" \| html}}` | `&lt;a&amp;b&gt;` |
| `js` | `{{"it's" \| js}}` | `it\'s` |
| `urlquery` | `{{"a b&c" \| urlquery}}` | `a+b%26c` |
| `eq` | `{{if eq . "dev" "stg"}}yes{{end}}` on `"stg"` | `yes` — **variadic**: true if equal to *any* argument |
| `ne` `lt` `le` `gt` `ge` | `{{ge 2 2}}` | `true` — basic types only; `{{lt 1 1.5}}` → `incompatible types for comparison` |

## 5. Data access rules (verified)

| Case | Result |
|---|---|
| Method on value receiver, `{{.Hello "bob"}}` | `hi bob from ann` — works on `U` and `*U` |
| Method with pointer receiver on a non-addressable value | error — pass a pointer |
| Missing **map** key `{{.nope}}` | `<no value>` (default), `0`/zero with `missingkey=zero`, error with `missingkey=error` |
| Missing **struct** field `{{.Nope}}` | always an error: `can't evaluate field Nope in type main.U` |
| Unexported field | error — templates see only exported identifiers |
| `.A.B` through `map[string]any` | works: `7` for `{"A":{"B":7}}` |
| Nil pointer field access | error unless guarded with `{{with .P}}` |

## 6. Go glue (minimum)

```go
t := template.Must(template.New("x").Funcs(fm).Parse(src)) // Funcs BEFORE Parse
if err := t.Execute(w, data); err != nil { /* partial output may already be written */ }
```
Render into a `bytes.Buffer` first when the writer is an HTTP response, so an error cannot
leave a half-written 200.
