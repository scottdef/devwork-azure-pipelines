package formulas

import (
	htemplate "html/template"
)

// language is F43..F55: the engine itself: loops, whitespace, Option,
// escaping, variables, composition, errors and the two lookalikes.
func language() []Formula {
	return []Formula{
		{
			ID:      "F43",
			Name:    "range over an integer (Go 1.22+)",
			Problem: "Repeating something N times used to need a helper that builds a slice of N elements.",
			Template: `{{ range 3 }}tick {{ . }}
{{ end -}}
{{ range $i := .N }}[{{ add $i 1 }}]{{ end }}
`,
			Data: M{"N": 4},
			Want: `tick 0
tick 1
tick 2
[1][2][3][4]
`,
			Notes: "`{{ range 3 }}` iterates 0, 1, 2; the count can be a constant or any integer value. `{{ range $i := .N }}` names the index. Only **one** variable is allowed over an integer: `{{ range $i, $x := 3 }}` is an error ('can't use 3 to iterate over more than one variable'). " +
				"`add $i 1` gives the 1-based count. A negative or zero count runs the `else` branch if there is one. For ranges with a start or step, use `seq` (F09).",
		},
		{
			ID:      "F44",
			Name:    "range over iter.Seq and iter.Seq2 (Go 1.23+)",
			Problem: "A lazily produced sequence (a database cursor, a directory walk) had to be collected into a slice before a template could loop over it.",
			Template: `squares: {{ range .Seq }}<{{ . }}>{{ end }}
pairs:   {{ range $k, $v := .Pairs }}{{ $k }}={{ $v }};{{ end }}
none:    {{ range .Empty }}never{{ else }}empty sequence{{ end }}
`,
			Data:    newSeqData(),
			GlueRef: "SeqData,SeqData.MarshalJSON,newSeqData",
			Want: `squares: <1><4><9>
pairs:   a=1;b=2;
none:    empty sequence
`,
			Notes: "A `func(yield func(V) bool)` (an `iter.Seq[V]`) ranges with one variable; an `iter.Seq2[K, V]` with two. The template stops the producer early when it hits `{{ break }}`, because `yield` returns false. " +
				"The data above is a struct of three sequences; since a function cannot be printed as JSON, `MarshalJSON` runs them to show what they yield. " +
				"The sequence is single-shot in spirit: ranging it twice runs the producer twice.",
		},
		{
			ID:      "F45",
			Name:    "break and continue",
			Problem: "Filtering a loop with a surrounding `if` forces the whole body inside the condition and cannot stop early.",
			Template: `{{ range .Nums }}{{ if eq . 3 }}{{ continue }}{{ end }}{{ if gt . 6 }}{{ break }}{{ end }}{{ . }} {{ end }}
`,
			Data: M{"Nums": []int{1, 2, 3, 4, 5, 6, 7, 8, 9}},
			Want: `1 2 4 5 6 
`,
			Notes: "`{{ continue }}` skips to the next element; `{{ break }}` leaves the innermost `range`. Both are only valid inside `range` and are parse errors elsewhere. " +
				"They act on the innermost loop: to leave two, set a flag variable (F50) and test it in the outer loop. Output is not rolled back: text written before `break` stays.",
		},
		{
			ID:       "F46",
			Name:     "Whitespace control with {{- and -}}",
			Problem:  "Every action sits on its own line in the source, and every one of those lines puts a newline and indent into the output.",
			Template: "{{ range .Items }}\n  {{ . }}\n{{ end }}",
			Data:     M{"Items": []string{"a", "b", "c"}},
			Run:      glueWS,
			GlueRef:  "wsVariant,wsVariants,glueWS",
			Want: `== untrimmed
{{ range .Items }}
  {{ . }}
{{ end }}
=> "\n  a\n\n  b\n\n  c\n"
== left-trim
{{- range .Items }}
  {{ . }}
{{- end }}
=> "\n  a\n  b\n  c"
== both-trim
{{- range .Items -}}
  {{ . }}
{{- end -}}
=> "abc"
== idiom
{{ range .Items -}}
  {{ . }}
{{ end -}}
=> "a\nb\nc\n"
`,
			Notes: "`{{-` removes all white space (spaces, tabs, newlines) **before** the action; `-}}` removes it **after**. There must be a space between the dash and the action's content: `{{- 3}}` trims, `{{-3}}` is the number -3. " +
				"The output is printed with `%q` so every byte is visible. The idiom in the last variant, `{{ range ... -}}` plus `{{ end -}}`, keeps one item per line with no leading or trailing blank line, and is the shape to reach for first. " +
				"Trimming is applied to the *template text* at parse time, not to the output, so it never removes white space produced by data.",
		},
		{
			ID:       "F47",
			Name:     "missingkey: default, zero and error",
			Problem:  "A misspelt key renders as `<no value>` in production and nobody notices.",
			Template: `A={{ .A }} B={{ .B }} C={{ default "fallback" .C }}`,
			Data:     M{"A": 1},
			Run:      glueMissing,
			GlueRef:  "glueMissing",
			Want: `missingkey=default     map[string]any    A=1 B=<no value> C=fallback
missingkey=default     map[string]int    A=1 B=<no value> C=fallback
missingkey=zero        map[string]any    A=1 B=<no value> C=fallback
missingkey=zero        map[string]int    A=1 B=0 C=fallback
missingkey=error       map[string]any    error: template: t:1:16: executing "t" at <.B>: map has no entry for key "B"
missingkey=error       map[string]int    error: template: t:1:16: executing "t" at <.B>: map has no entry for key "B"
missingkey=default     struct            error: template: t:1:16: executing "t" at <.B>: can't evaluate field B in type struct { A int }
`,
			Notes: "`Option(\"missingkey=...\")` decides what a map lookup of an absent key does. `default` (and `invalid`) prints `<no value>`; `zero` yields the element type's zero value, which is `0` for `map[string]int` but still `<no value>` for `map[string]any`, whose zero value is nil; `error` stops execution. " +
				"Use `error` in anything that writes files. The option applies to **maps only**: a missing field on a struct is always an error, as the last line shows. " +
				"Note that `missingkey=error` also makes the `default` function useless for absent keys; use `{{ with }}` or `index` there.",
		},
		{
			ID:      "F48",
			Name:    "html/template contextual escaping",
			Problem: "The same string needs different escaping in text, an attribute, a URL, a script and a style, and getting one wrong is XSS.",
			Template: `<p>text: {{ .X }}</p>
<p title="{{ .X }}">quoted attribute</p>
<p title={{ .X }}>unquoted attribute</p>
<a href="/q?s={{ .X }}">URL query</a>
<a href="{{ .Bad }}">URL scheme</a>
<script>var x = {{ .X }};</script>
<script>var s = "{{ .X }}";</script>
<div style="font-family: {{ .Font }}">CSS</div>
<button onclick="go({{ .X }})">JS in attribute</button>
`,
			Engine: "html",
			Data: M{
				"X":    `O'Reilly & <b>"Sons"</b>`,
				"Bad":  "javascript:alert(1)",
				"Font": "x;}body{background:url(//evil)",
			},
			Want: `<p>text: O&#39;Reilly &amp; &lt;b&gt;&#34;Sons&#34;&lt;/b&gt;</p>
<p title="O&#39;Reilly &amp; &lt;b&gt;&#34;Sons&#34;&lt;/b&gt;">quoted attribute</p>
<p title=O&#39;Reilly&#32;&amp;&#32;&lt;b&gt;&#34;Sons&#34;&lt;/b&gt;>unquoted attribute</p>
<a href="/q?s=O%27Reilly%20%26%20%3cb%3e%22Sons%22%3c%2fb%3e">URL query</a>
<a href="#ZgotmplZ">URL scheme</a>
<script>var x = "O'Reilly \u0026 \u003cb\u003e\"Sons\"\u003c/b\u003e";</script>
<script>var s = "O\u0027Reilly \u0026 \u003cb\u003e\u0022Sons\u0022\u003c\/b\u003e";</script>
<div style="font-family: ZgotmplZ">CSS</div>
<button onclick="go(&#34;O&#39;Reilly \u0026 \u003cb\u003e\&#34;Sons\&#34;\u003c/b\u003e&#34;)">JS in attribute</button>
`,
			Notes: "html/template parses the template as HTML and picks an escaper from the **context** of each action: HTML text, quoted or unquoted attribute, URL component, JavaScript value or string, CSS value. The one `{{ .X }}` renders differently in each of the nine contexts above. " +
				"`ZgotmplZ` is the sentinel it writes when a value cannot be made safe in its context: a `javascript:` URL in `href`, or CSS with unbalanced braces. Seeing it means 'you tried to inject something', not 'a bug in the library'. " +
				"The cost: the template must be well-formed HTML, and pre-escaping a string yourself double-escapes it. Do not call `html` or `urlquery` inside an html/template.",
		},
		{
			ID:      "F49",
			Name:    "Typed bypass: template.HTML, URL, JS and HTMLAttr",
			Problem: "Sometimes the data really is trusted markup (rendered Markdown, a vetted link) and escaping it ruins it.",
			Template: `<p>{{ .Plain }}</p>
<p>{{ .Safe }}</p>
<a href="{{ .BadLink }}">string</a>
<a href="{{ .Link }}">template.URL</a>
<a {{ .Attr }}>template.HTMLAttr</a>
<script>{{ .Code }}</script>
`,
			Engine: "html",
			Data: M{
				"Plain":   "<b>plain</b>",
				"Safe":    htemplate.HTML("<b>safe</b>"),
				"BadLink": "javascript:alert(1)",
				"Link":    htemplate.URL("javascript:alert(1)"),
				"Attr":    htemplate.HTMLAttr(`onclick="x()"`),
				"Code":    htemplate.JS("alert(1)"),
			},
			Want: `<p>&lt;b&gt;plain&lt;/b&gt;</p>
<p><b>safe</b></p>
<a href="#ZgotmplZ">string</a>
<a href="javascript:alert%281%29">template.URL</a>
<a onclick="x()">template.HTMLAttr</a>
<script>alert(1)</script>
`,
			Notes: "A value whose Go type is `template.HTML`, `HTMLAttr`, `JS`, `JSStr`, `CSS` or `URL` is a promise: 'I vetted this'. html/template believes it and writes it verbatim. " +
				"**Warning:** the type conversion is the whole security review. `template.HTML(userInput)` is XSS with extra steps; `template.URL(\"javascript:...\")` defeats the scheme filter, as the output shows. " +
				"Convert only constants, or output of a sanitiser you trust (a Markdown renderer with HTML disabled, `bluemonday`-style policy). Grep for these conversions in code review.",
		},
		{
			ID:      "F50",
			Name:    "Variable reassignment: the accumulator",
			Problem: "A running total or maximum seems to need state, and `:=` inside the loop quietly makes a new variable that dies at `end`.",
			Template: `{{ $sum := 0 }}{{ $max := 0 }}{{ range .Nums }}{{ $sum = add $sum . }}{{ if gt . $max }}{{ $max = . }}{{ end }}{{ end -}}
sum={{ $sum }} max={{ $max }}
{{ $bad := 0 }}{{ range .Nums }}{{ $bad := add $bad . }}{{ end -}}
wrong={{ $bad }}
{{ $s := "" }}{{ range $i, $n := .Nums }}{{ $s = printf "%s%s%d" $s (ternary "" "+" (eq $i 0)) $n }}{{ end -}}
expr={{ $s }}
`,
			Data: M{"Nums": []int{4, 8, 15, 16, 23, 42}},
			Want: `sum=108 max=42
wrong=0
expr=4+8+15+16+23+42
`,
			Notes: "Since Go 1.11 `{{ $x = ... }}` **assigns** to an existing variable, even from inside `range` or `if`. `{{ $x := ... }}` declares a new one scoped to the enclosing block, so `wrong` stays 0: the loop computed a sum and threw it away. " +
				"This is the standard way to count, total, find a maximum or build a string, and it is also how F24 measures the widest key. " +
				"A variable cannot be assigned from a *called template* (`define`): variables do not cross that boundary, so return values must be computed before the call.",
		},
		{
			ID:      "F51",
			Name:    "Clone: per-page layouts with block override",
			Problem: "Parsing every page's `define \"title\"` into one template set means the last definition wins for all pages.",
			Template: `<!doctype html>
<title>{{ block "title" . }}Untitled{{ end }}</title>
<main>{{ block "body" . }}(empty){{ end }}</main>
`,
			Engine:  "html",
			Data:    M{"User": "rob"},
			Run:     glueClone,
			GlueRef: "pages,glueClone",
			Want: `## one clone per page
-- home --
<!doctype html>
<title>Home</title>
<main>Hello, rob</main>
-- about --
<!doctype html>
<title>About</title>
<main>(empty)</main>
## one shared set, home parsed first, then about
<!doctype html>
<title>About</title>
<main>Hello, rob</main>
`,
			Notes: "`{{ block \"x\" . }}default{{ end }}` defines `x` and invokes it in one step; a page overrides it by defining `x` again. In a single template set a name has one body, so the second `define` replaces the first. " +
				"The fix is `base.Clone()` for every page: each clone has its own set of definitions, and the base stays pristine. Clone **before** the first Execute; html/template refuses to clone a template that has run. " +
				"The 'shared' output shows the bug: one set can render only one page, and it is a blend: About's title over Home's body.",
		},
		{
			ID:       "F52",
			Name:     "ParseFS from embed and ExecuteTemplate by name",
			Problem:  "Templates in files must ship inside the binary, and a set of files needs to be addressed by name.",
			Template: mustReadTmpl("page.tmpl", "item.tmpl", "footer.tmpl"),
			Data: M{"Customer": "Ada", "Items": L{
				M{"Name": "engine", "Qty": 1}, M{"Name": "cards", "Qty": 3},
			}},
			Run:     glueFS,
			GlueRef: "glueFS",
			Want: `templates: footer footer.tmpl item.tmpl page.tmpl
Order for Ada
* engine x1
* cards x3
-- 2 items --
`,
			Notes: "`//go:embed testdata/tmpl` puts the directory in the binary; `ParseFS(fsys, \"testdata/tmpl/*.tmpl\")` parses each match and names it by its **base name**, so `ExecuteTemplate(w, \"page.tmpl\", data)` selects it. " +
				"A file may also `define` further names (`footer`), which become addressable as well. The pattern is a `path.Match` glob, with no `**`. " +
				"The Template shown is the three embedded files in order. The set of names is sorted before printing because `Templates()` returns them in map order.",
		},
		{
			ID:       "F53",
			Name:     "Error handling: ExecError and errors.As",
			Problem:  "A template failure is a returned error, but a log line saying 'template: t:1:7: executing' hides what failed and where.",
			Template: `{{ .user.name }}`,
			Data:     M{"user": nil},
			Run:      glueErr,
			GlueRef:  "glueErr",
			Want: `parse  parse error: template: t:1: unclosed action
nil    ExecError in "t": template: t:1:8: executing "t" at <.user.name>: nil pointer evaluating interface {}.name
args   ExecError in "t": template: t:1:3: executing "t" at <dict "a">: error calling dict: dict: bad arguments: odd number of arguments (1)
       errors.Is(err, funcs.ErrArgs) = true
write  not an ExecError: disk full
`,
			Notes: "Parse errors come back from `Parse`. Errors raised while running are wrapped in `template.ExecError`, which carries the template `Name` and the underlying `Err`; extract it with `errors.As`. " +
				"A function's own error is wrapped further, so `errors.Is(err, funcs.ErrArgs)` still sees through to the cause. " +
				"A failure to *write* is not an ExecError: it is the writer's own error, returned as is. Execution stops at the first error and the partial output has already been written, so render into a buffer and discard it on failure.",
		},
		{
			ID:      "F54",
			Name:    "Generating a gh api --template string",
			Problem: "`gh api --template` takes a Go template, so the command line you generate contains `{{` that your own template would swallow.",
			Template: `{{- $l := "{{" }}{{ $r := "}}" -}}
gh api repos/{{ .Org }}/{{ .Repo }}/issues --template '{{"{{"}}range .{{"}}"}}#{{"{{"}}.number{{"}}"}} {{"{{"}}.title{{"}}"}}{{"{{"}}"\n"{{"}}"}}{{"{{"}}end{{"}}"}}'
gh api repos/{{ .Org }}/{{ .Repo }}/issues --template '{{ $l }}range .{{ $r }}#{{ $l }}.number{{ $r }} {{ $l }}.title{{ $r }}{{ $l }}"\n"{{ $r }}{{ $l }}end{{ $r }}'
`,
			Data: M{"Org": "CoolGitOrg", "Repo": "go-template-cookbook"},
			Want: `gh api repos/CoolGitOrg/go-template-cookbook/issues --template '{{range .}}#{{.number}} {{.title}}{{"\n"}}{{end}}'
gh api repos/CoolGitOrg/go-template-cookbook/issues --template '{{range .}}#{{.number}} {{.title}}{{"\n"}}{{end}}'
`,
			Notes: "A literal `{{` in output is an action that prints the string `\"{{\"`: write `{{\"{{\"}}`, and `{{\"}}\"}}` for the closer (the closing braces are only special inside an action, but keep them symmetric). " +
				"Line 2 does it inline; line 3 binds the two strings to `$l` and `$r` once, which reads better when there are many. The result is wrapped in single quotes for the shell (use `shq` when the template text is not fixed). " +
				"The alternative is F25: change the generator's delimiters. Often neither is needed: `gh api ... --jq '.[] | \"#\\(.number) \\(.title)\"'` has no braces at all. " +
				"gh evaluates the template itself and adds helpers such as `join`, `pluck`, `tablerow` and `timeago`.",
		},
		{
			ID:      "F55",
			Name:    "Terraform templatefile: a lookalike, not a Go template",
			Problem: "HCL string templates (`${name}`, `%{ for }`) look like Go templates and are not; mixing them up makes a generator emit broken Terraform.",
			Template: `resource "local_file" {{ printf "%q" .Name }} {
  filename = {{ printf "%q" (print .Name ".txt") }}
  content  = templatefile("{{ .Name }}.tpl", {
    name  = {{ printf "%q" .Vars.name }}
    ports = [{{ range $i, $p := .Vars.ports }}{{ if $i }}, {{ end }}{{ $p }}{{ end }}]
  })
}
`,
			Data: M{"Name": "motd", "Vars": M{"name": "web", "ports": []int{80, 443}}},
			Want: `resource "local_file" "motd" {
  filename = "motd.txt"
  content  = templatefile("motd.tpl", {
    name  = "web"
    ports = [80, 443]
  })
}
`,
			Notes: "Terraform's `templatefile(path, vars)` renders **HCL template syntax**, which is not rendered here (Terraform is not installed in this build). The file it would read, `motd.tpl`, looks like this:\n\n" +
				"```\nHello, ${name}!\n%{ for p in ports ~}\nlistening on ${p}\n%{ endfor ~}\n```\n\n" +
				"The differences that matter: interpolation is `${expr}`, directives are `%{ if cond }` / `%{ for x in xs }` / `%{ endfor }` / `%{ endif }`, `~` strips adjacent white space (our `{{-`), expressions are HCL, not Go, and there are no pipelines, no `define`, no `with`. A literal `${` is written `$${`. " +
				"Go cannot render these files, and Terraform cannot run Go templates: the formula above is the Go side, **generating** the `.tf` that calls `templatefile`. If you must emit `${` from a Go template, see F24.",
		},
	}
}

// mustReadTmpl joins embedded template files for display; see F52.
func mustReadTmpl(names ...string) string {
	var out string
	for i, n := range names {
		b, err := tmplFS.ReadFile("testdata/tmpl/" + n)
		if err != nil {
			return "// " + err.Error()
		}
		if i > 0 {
			out += "\n"
		}
		out += "-- " + n + " --\n" + string(b)
	}
	return out
}
