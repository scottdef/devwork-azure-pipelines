# Go Templates — Advanced Cheat Sheet

Verified with go1.24.7; see `docs/verify/cheatsheet_verify.go` for the program that produced
every quoted output.

## 1. API ordering rules

| Rule | Why |
|---|---|
| `New(name).Funcs(fm).Delims(l, r).Option(o).Parse(src)` — **in that order** | `Parse` resolves function names and delimiters; later `Funcs` calls only add names for *subsequent* parses |
| `Clone()` **before** page-specific `{{define}}`s | otherwise every page's define overwrites the shared layout. Verified: after `page := base.Clone(); page.Parse(define content)`, `page` renders `[page p1]` and `base` still renders `[base]` |
| `ParseFS(fsys, "tmpl/*.html")` names templates by **base name** | `ExecuteTemplate(w, "layout.html", d)` |
| `Lookup(name)` returns nil when absent; `DefinedTemplates()` lists `"content", "layout"` | use in tests |
| `AddParseTree(name, tree)` | splice programmatic trees; `html/template` uses it internally |
| `Templates()` | all associated templates (unordered) |

## 2. FuncMap contract

| Rule | Detail |
|---|---|
| Signature | any function; return `T` or `(T, error)`; a non-nil error aborts `Execute` with an `ExecError` |
| Argument conversion | ints/floats are converted to the parameter type when representable; strings are not |
| Pipelines | the piped value is appended as the **last** argument — design helper signatures accordingly (`indent n s`, `default d v`) |
| Variadic | `func(args ...any)` works (`dict "k" v "k2" v2`) |
| Overriding builtins | allowed (e.g. a safer `html`), but don't |
| Errors | `errors.As(err, &template.ExecError{})` — field `Name`; `ExecError` wraps the cause. Verified message: `template: mk:1:3: executing "mk" at <.nope>: map has no entry for key "nope"` |

## 3. Options and delimiters (verified)

| Call | Effect |
|---|---|
| `Option("missingkey=default")` / `"invalid"` | missing map key prints `<no value>` |
| `Option("missingkey=zero")` | prints the zero value: `[0]` for `map[string]int` |
| `Option("missingkey=error")` | `map has no entry for key "nope"` — recommended for config/shell output |
| `Delims("[[", "]]")` | `{{ .Keep }} [[ .X ]]` renders `{{ .Keep }} x` — use for emitting Helm/Actions files and for meta-templates |

## 4. html/template contexts (verified)

Input value: `<b>"x" javascript:alert(1)</b>`

| Context | Template | Rendered |
|---|---|---|
| HTML text | `<p>{{.}}</p>` | `<p>&lt;b&gt;&#34;x&#34; javascript:alert(1)&lt;/b&gt;</p>` |
| URL attribute | `<a href="{{.}}">` | `<a href="#ZgotmplZ">` — value rejected; also for a bare `javascript:alert(1)` |
| JS | `<script>var v = {{.}};</script>` | `var v = "<b>\"x\" javascript:alert(1)</b>";` (JSON-encoded, safe) |
| Attribute | `<div title="{{.}}">` | `title="&lt;b&gt;&#34;x&#34; javascript:alert(1)&lt;/b&gt;"` |
| Trusted | `{{.}}` with `template.HTML("<b>trusted</b>")` | `<b>trusted</b>` — **bypass**; audit every `template.HTML/JS/JSStr/CSS/URL/Srcset/HTMLAttr` |

Rules: escaping is decided at *parse* time per branch, so `{{if}}` branches that end in
different contexts are errors; typed strings skip escaping; `ZgotmplZ` means "I refused."

## 5. Parse errors and linting (verified)

| Source | Error |
|---|---|
| `ok\n{{ if .X }}\nunclosed` | `template: lint:3: unexpected EOF` — line numbers are 1-based |
| `{{ nosuchfunc 1 }}` | `template: lint:1: function "nosuchfunc" not defined` |

Walk `t.Tree.Root` (`*parse.ListNode`) to lint or to discover referenced fields:

```go
var walk func(parse.Node)
walk = func(n parse.Node) {
	switch n := n.(type) {
	case *parse.ListNode:   if n != nil { for _, c := range n.Nodes { walk(c) } }
	case *parse.ActionNode: walk(n.Pipe)
	case *parse.PipeNode:   if n != nil { for _, c := range n.Cmds { walk(c) } }
	case *parse.CommandNode: for _, a := range n.Args { walk(a) }
	case *parse.FieldNode:  seen["."+strings.Join(n.Ident, ".")] = true
	case *parse.IfNode:     walk(n.Pipe); walk(n.List); walk(n.ElseList)
	case *parse.RangeNode:  walk(n.Pipe); walk(n.List); walk(n.ElseList)
	case *parse.WithNode:   walk(n.Pipe); walk(n.List); walk(n.ElseList)
	case *parse.TemplateNode: walk(n.Pipe)
	}
}
```
Fields inside `range`/`with` are relative to the rebound dot; a fixture generator must nest
them under the parent field (CS3 `internal/forge/vars.go`).

## 6. Concurrency, limits, cancellation

| Topic | Fact |
|---|---|
| Thread safety | a parsed `*Template` may be executed concurrently; `Parse`/`AddParseTree` after first use is a data race — clone instead |
| Recursion | `define`+`template` recursion is legal; `maxExecDepth` = 100 000 (1 000 on `js/wasm`) |
| Expression nesting | `maxStackDepth` = 10 000 parentheses/pipelines |
| Control nesting | unbounded at parse time (golang/go#81546): deep `{{if}}` nesting overflows the goroutine stack **fatally** |
| Cancellation | none — `Execute` has no `context`. Pattern: run in a goroutine with a `limitWriter`, or re-exec in a subprocess with `exec.CommandContext` (CS3 sandbox) |
| Output size | unbounded — wrap the writer |
| Hot reload | poll mtimes, parse into a new template, `atomic.Pointer[template.Template]` swap |

## 7. Idioms

| Need | Idiom |
|---|---|
| Multiple args to a sub-template | `{{template "x" (dict "a" .A "b" .B)}}` with a `dict` func |
| Default | `{{.Port \| default 8080}}` |
| Ternary | `{{if .P}}a{{else}}b{{end}}` or `{{ternary "a" "b" .P}}` |
| Join with separators | `{{range $i, $x := .}}{{if $i}}, {{end}}{{$x}}{{end}}` |
| JSON safely | `{{toJSON .}}` (a func over `encoding/json`), never hand-built |
| YAML blocks | `key: {{toYAML .V \| nindent 2}}` |
| Literal `{{` | `{{"{{"}}` or `Delims` |
| Tabs (Makefile) | literal tab in the template or `{{"\t"}}` |
| Line-end trimming in loops | `{{- range .}}` / `{{- end}}` and a trailing `{{"\n"}}` where needed |
| Emit a template | outer `Delims("[[","]]")`, inner text uses `{{ }}` |
| Go source | `format.Source(buf.Bytes())` after `Execute` |

## 8. Testing

| Technique | Notes |
|---|---|
| Golden files | `-update` flag rewrites `testdata/golden/*`; compare bytes |
| `template.Must` at init | fails fast at startup — tests cover it by importing the package |
| Fuzz | `testing.F` over template source (parse never panics) and over data values |
| `go vet` | checks `printf` verbs in template funcs only if they are `fmt`-style; no template vet exists in 1.24 |
| gopls | set `"templateExtensions": ["tmpl","gotmpl"]` to get completion/diagnostics in `.tmpl` files |
| Determinism | map ranges are sorted; pin `now` via a func injected in tests |

## 9. Sprig highlights (not a dependency here; vocabulary only)

`default`, `empty`, `coalesce`, `ternary`, `toJson`/`toYaml`/`fromJson`, `indent`/`nindent`,
`trim*`, `upper`/`lower`/`title`, `replace`, `regexMatch`/`regexReplaceAll`, `list`/`dict`/
`get`/`set`/`hasKey`/`keys`/`pluck`, `b64enc`/`b64dec`, `sha256sum`, `now`/`date`/`dateModify`,
`uuidv4`, `semver`, `required`, `fail`. The shared stdlib FuncMap in this collection
reimplements the subset that the case studies need (`docs/FUNCS-SHARED.md`).
