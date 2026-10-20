# Go Templates Cookbook

> **Generated file. Do not edit.** It is produced by `cookbook gen` from the Go values in
> `cookbook/formulas`, and `go test ./...` renders every formula and compares the result with
> the output printed below. If this text and the code ever disagree, the test fails first.

55 formulas. Each one is a problem, a template, the data it runs on, the Go glue it needs
(if any), the exact output, and the trap it avoids. The shared helper functions are in
[FUNCS-SHARED.md](FUNCS-SHARED.md).

## How to run

```sh
cd cookbook
go run ./cmd/cookbook list                    # IDs and names
go run ./cmd/cookbook show F07                # one formula, all sections
go run ./cmd/cookbook run F07                 # render it
echo '{"Env":{"A":"1"}}' | go run ./cmd/cookbook run F07 -d -    # same template, your data
go run ./cmd/cookbook check                   # every output equals its Want
go run ./cmd/cookbook gen                     # rewrite this file
make test                                     # go test ./... && go vet ./...
```

## Conventions

- Unless a formula says otherwise it uses `text/template` and the delimiters `{{` and `}}`.
  Formulas that change either say so under the template.
- Every template is parsed with the shared FuncMap (`funcs.New(formulas.Options())`). Three of
  its functions touch the world, so the cookbook pins it: `now` is `2024-03-05T14:30:00Z`, `env` reads
  only `APP_ENV=staging HOME=/home/rob USER=rob`, and `file` reads from `formulas/testdata/tmpl`. Output is the same on every machine.
- **Template** is the template source. **Data** is the value passed to `Execute`, as JSON. **Glue** is
  Go from `formulas/glue.go`, copied verbatim; inside it `f.Template` and `f.Data` are the Template
  and Data blocks of the same formula. **Output** is exact, apart from one trailing newline that is not shown.
- Where a tab matters (F21, F28, F29, F33, F38, F41) the template and the output contain real tab characters.

## Contents

| ID | Name | Problem |
|----|------|---------|
| [F01](#f01) | dict: several values through one template call | `{{template "x" a b}}` takes exactly one argument, so a partial that needs a kind, a text and an optional size has nothing to receive them in. |
| [F02](#f02) | list: a slice literal | Templates have no `[...]` syntax, so a fixed set of values has to be declared in the data or hard-coded into repeated text. |
| [F03](#f03) | default: a value when the field is empty | An unset field prints as nothing (or `<no value>`), and the config renders with holes in it. |
| [F04](#f04) | coalesce: first non-empty of several | A display name falls back from nickname to name to email; nested `if/else` chains for that are noisy. |
| [F05](#f05) | ternary: if/else as an expression | A one-word choice in the middle of a line needs a five-token `if/else/end` that breaks up the text. |
| [F06](#f06) | Join struct fields with a comma rule | `join` wants a slice of strings, but the names live in a field of each struct, and a trailing comma after the last one is a bug. |
| [F07](#f07) | Iterate a map in sorted key order | Go randomises map iteration, so you expect generated files to differ on every run and every diff to be noise. |
| [F08](#f08) | Nested range with $ to reach the root | Inside a nested `range` the dot is the inner element, and the organisation name from the top is out of reach. |
| [F09](#f09) | 1-based index arithmetic | `range` indexes from 0 and humans count from 1; numbered lists end up starting at 0. |
| [F10](#f10) | Zebra stripes with mod | Alternating row classes need the parity of the loop index, and there is no `%` in templates. |
| [F11](#f11) | Pluralisation | '1 files' and '0 file' look careless in a report. |
| [F12](#f12) | time.Time: Format and friends | Dates arrive as `time.Time` and need a specific layout; people try `strftime` codes and get nonsense. |
| [F13](#f13) | printf: number formatting | Money, hex, padding and percentages come out as raw `%v` unless you format them. |
| [F14](#f14) | YAML block with toYAML \| nindent | Splicing a map or list into a YAML file by printing it line by line gets the indentation, the quoting and the empty case wrong. |
| [F15](#f15) | JSON array with correct commas | A hand-built JSON array leaves a trailing comma, or `%q` produces a string JSON cannot parse. |
| [F16](#f16) | JSON object from a map (sorted keys) | Labels have to be emitted as a JSON object, with stable order, correct escaping and commas only between members. |
| [F17](#f17) | base64 and SHA-256 | A header needs `user:password` in base64 and an ETag needs a digest, and neither should be computed outside the template. |
| [F18](#f18) | env lookup (trusted templates) with a default | A rendered config should pick up the deployment environment, falling back when a variable is unset. |
| [F19](#f19) | Include a file from a rooted filesystem | Boilerplate headers and message-of-the-day text live in files; a naive include reads whatever path the template names, `../../etc/passwd` included. |
| [F20](#f20) | Recursive tree with define and template | A nested menu or directory tree has unknown depth; `range` inside `range` only reaches a fixed number of levels. |
| [F21](#f21) | Aligned columns with text/tabwriter | `printf "%-20s"` hard-codes a width that is wrong the moment a name is longer, and counts bytes, not runes. |
| [F22](#f22) | CSV: never join with commas | A field containing a comma, a quote or a newline breaks a CSV row that was assembled with `{{ .A }},{{ .B }}`. |
| [F23](#f23) | Markdown table | A pipe or a newline in a cell silently ends the cell or the row. |
| [F24](#f24) | HCL / Terraform emission | A string containing `${` or `%{` becomes an interpolation in HCL, and a Go-quoted string is not always a valid HCL string. |
| [F25](#f25) | GitHub Actions workflow with [[ ]] delimiters | `${{ secrets.X }}` is a GitHub expression, but the template engine reads `{{ secrets.X }}` as its own action and fails or eats it. |
| [F26](#f26) | GitHub Issue Form YAML | An issue-form definition is structured YAML where a colon in a description or a leading `[` in a title changes its meaning. |
| [F27](#f27) | Dockerfile with a JSON-array ENTRYPOINT | Shell-form `ENTRYPOINT server -a :8080` runs under `/bin/sh -c`, swallows signals and breaks on spaces; the exec form must be a JSON array with double quotes. |
| [F28](#f28) | Makefile with literal tabs | Recipe lines must start with a TAB; editors, formatters and chat paste turn it into spaces and `make` stops with 'missing separator'. |
| [F29](#f29) | Plan 9 mkfile | mk is not make: phony targets are marked `:V:`, recipes run under rc, and variables are `$BIN`, not `$(BIN)`. |
| [F30](#f30) | Shell script with shq quoting | Interpolating a value into a script is command injection: a name like `x'; rm -rf /` ends the string and runs the rest. |
| [F31](#f31) | SQL: identifier allowlist and placeholders | Values and identifiers cannot both be parameterised: placeholders stop at values, so a table or column name taken from input is injection. |
| [F32](#f32) | Go source, gofmt'd | Generated Go that is not gofmt-clean shows up as diff noise forever, and a template cannot know the column widths gofmt will choose. |
| [F33](#f33) | C source (Plan 9 style) | C string literals have their own escapes, and the generated file should look like the rest of the tree. |
| [F34](#f34) | SVG sparkline polyline | A tiny trend chart should be one inline element, not an image or a JavaScript library, and the numbers must be scaled into the viewBox. |
| [F35](#f35) | HTML email plus text alternative | A multipart email needs the same facts as HTML (escaped) and as plain text (not escaped), and using one engine for both gets one of them wrong. |
| [F36](#f36) | htmx partial (hx-get, hx-target, hx-swap) | An htmx endpoint returns a *fragment*, not a page, and the 'load more' control has to replace itself with the next rows and a new control. |
| [F37](#f37) | A template that emits a template | Generating a `.tmpl` file means every `{{` you want in the output is an action to the generator. |
| [F38](#f38) | CLI usage text from flag.FlagSet | Hand-written `-h` text drifts away from the flags the program defines. |
| [F39](#f39) | Prometheus text exposition | The exposition format has its own escaping for HELP text and label values, and a wrong backslash makes the scraper drop the whole target. |
| [F40](#f40) | CoreDNS template plugin answer line | The CoreDNS `template` plugin evaluates its own Go template inside the Corefile (`{{ .Name }}`), which collides with the generator's. |
| [F41](#f41) | Caddy browse-style directory listing | A static-file server lists a directory with sizes and dates; the data comes from `ReadDir` and the human-readable size needs a method. |
| [F42](#f42) | Chart option JSON into a script block (html/template) | A charting library wants a JavaScript object literal; string-pasting JSON into a script block is both an escaping bug and an XSS hole (`</script>`). |
| [F43](#f43) | range over an integer (Go 1.22+) | Repeating something N times used to need a helper that builds a slice of N elements. |
| [F44](#f44) | range over iter.Seq and iter.Seq2 (Go 1.23+) | A lazily produced sequence (a database cursor, a directory walk) had to be collected into a slice before a template could loop over it. |
| [F45](#f45) | break and continue | Filtering a loop with a surrounding `if` forces the whole body inside the condition and cannot stop early. |
| [F46](#f46) | Whitespace control with {{- and -}} | Every action sits on its own line in the source, and every one of those lines puts a newline and indent into the output. |
| [F47](#f47) | missingkey: default, zero and error | A misspelt key renders as `<no value>` in production and nobody notices. |
| [F48](#f48) | html/template contextual escaping | The same string needs different escaping in text, an attribute, a URL, a script and a style, and getting one wrong is XSS. |
| [F49](#f49) | Typed bypass: template.HTML, URL, JS and HTMLAttr | Sometimes the data really is trusted markup (rendered Markdown, a vetted link) and escaping it ruins it. |
| [F50](#f50) | Variable reassignment: the accumulator | A running total or maximum seems to need state, and `:=` inside the loop quietly makes a new variable that dies at `end`. |
| [F51](#f51) | Clone: per-page layouts with block override | Parsing every page's `define "title"` into one template set means the last definition wins for all pages. |
| [F52](#f52) | ParseFS from embed and ExecuteTemplate by name | Templates in files must ship inside the binary, and a set of files needs to be addressed by name. |
| [F53](#f53) | Error handling: ExecError and errors.As | A template failure is a returned error, but a log line saying 'template: t:1:7: executing' hides what failed and where. |
| [F54](#f54) | Generating a gh api --template string | `gh api --template` takes a Go template, so the command line you generate contains `{{` that your own template would swallow. |
| [F55](#f55) | Terraform templatefile: a lookalike, not a Go template | HCL string templates (`${name}`, `%{ for }`) look like Go templates and are not; mixing them up makes a generator emit broken Terraform. |

<a id="f01"></a>

## F01. dict: several values through one template call

**Problem.** `{{template "x" a b}}` takes exactly one argument, so a partial that needs a kind, a text and an optional size has nothing to receive them in.

Engine: `text/template`. Delimiters: `{{` `}}`.

### Template

```gotemplate
{{- define "badge" -}}
<{{ .Kind }}:{{ .Text }}{{ if .Size }}:{{ .Size }}{{ end }}>
{{- end -}}
{{ template "badge" (dict "Kind" "info" "Text" .Msg) }}
{{ template "badge" (dict "Kind" "warn" "Text" .Msg "Size" "lg") }}
```

### Data

```json
{
  "Msg": "disk 91% full"
}
```

### Output

```
<info:disk 91% full>
<warn:disk 91% full:lg>
```

### Notes

`dict` packs alternating keys and values into a `map[string]any`, which is the one argument `template` accepts. A key you leave out reads as the zero value, so `{{ if .Size }}` is the optional-argument test. An odd number of arguments or a non-string key is an error at execution time (see F53), not a silent drop. `$` inside the partial is the dict, not the caller's data: pass everything the partial needs.

<a id="f02"></a>

## F02. list: a slice literal

**Problem.** Templates have no `[...]` syntax, so a fixed set of values has to be declared in the data or hard-coded into repeated text.

Engine: `text/template`. Delimiters: `{{` `}}`.

### Template

```gotemplate
{{ $envs := list "dev" "stage" "prod" -}}
count={{ len $envs }} second={{ index $envs 1 }} tail={{ slice $envs 1 }}
mixed={{ list 1 "a" true }}
{{ range $envs }}- {{ . }}
{{ end -}}
```

### Output

```
count=3 second=stage tail=[stage prod]
mixed=[1 a true]
- dev
- stage
- prod
```

### Notes

`list` returns a `[]any`, so every builtin that takes a slice works: `len`, `index`, `slice`, `range`, `join`. Elements are `any`: comparing one with `eq` needs matching types (`eq (index $l 0) "dev"` is fine, `eq (index $l 0) 1` is an error).

<a id="f03"></a>

## F03. default: a value when the field is empty

**Problem.** An unset field prints as nothing (or `<no value>`), and the config renders with holes in it.

Engine: `text/template`. Delimiters: `{{` `}}`.

### Template

```gotemplate
name={{ default "anonymous" .Name }}
port={{ default 8080 .Port }}
host={{ default "localhost" .Host }}
debug={{ default "off" .Debug }}
```

### Data

```json
{
  "Host": "example.org",
  "Name": "",
  "Port": 0
}
```

### Output

```
name=anonymous
port=8080
host=example.org
debug=off
```

### Notes

The value is the **last** argument so it pipes: `{{ .Name | default "anonymous" }}`. *Empty* means the zero value: `""`, `0`, `false`, nil, an empty slice or map. The trap is `Port: 0`: a port the author set deliberately to 0 is replaced by 8080, because `default` cannot tell 'set to zero' from 'unset'. When zero is meaningful, carry a pointer in the data (`*int`): nil is unset, a pointer to 0 is not. A missing map key reaches `default` as nil, which is why `.Debug` works.

<a id="f04"></a>

## F04. coalesce: first non-empty of several

**Problem.** A display name falls back from nickname to name to email; nested `if/else` chains for that are noisy.

Engine: `text/template`. Delimiters: `{{` `}}`.

### Template

```gotemplate
who={{ coalesce .Nick .Name .Email "unknown" }}
retries={{ coalesce .Retries .Fallback }}
```

### Data

```json
{
  "Email": "rob@example.org",
  "Fallback": 3,
  "Name": "",
  "Nick": "",
  "Retries": 0
}
```

### Output

```
who=rob@example.org
retries=3
```

### Notes

`coalesce` is `default` for a chain. It uses the same emptiness, so `Retries: 0` is skipped. If every argument is empty it returns nil, which prints as `<no value>`: end the chain with a literal.

<a id="f05"></a>

## F05. ternary: if/else as an expression

**Problem.** A one-word choice in the middle of a line needs a five-token `if/else/end` that breaks up the text.

Engine: `text/template`. Delimiters: `{{` `}}`.

### Template

```gotemplate
{{ if .Prod }}https{{ else }}http{{ end }}://example.org
{{ ternary "https" "http" .Prod }}://example.org
{{ ternary "https" "http" (eq .Env "prod") }}://example.org
```

### Data

```json
{
  "Env": "stage",
  "Prod": true
}
```

### Output

```
https://example.org
https://example.org
http://example.org
```

### Notes

The condition is the last argument. `ternary` is a **function**: both branches are evaluated before it is called. `{{ ternary .Cfg.Port 80 .HasCfg }}` with a nil `.Cfg` fails with a nil-pointer error even when `.HasCfg` is false, because `.Cfg.Port` was evaluated anyway. Use `if` whenever a branch can fail or has side effects; `and`/`or` short-circuit since Go 1.18.

<a id="f06"></a>

## F06. Join struct fields with a comma rule

**Problem.** `join` wants a slice of strings, but the names live in a field of each struct, and a trailing comma after the last one is a bug.

Engine: `text/template`. Delimiters: `{{` `}}`.

### Template

```gotemplate
{{ range $i, $u := .Users }}{{ if $i }}, {{ end }}{{ $u.Name }}{{ end }}
{{ range $i, $u := .Users }}{{ if $i }}{{ if eq $i (sub (len $.Users) 1) }} and {{ else }}, {{ end }}{{ end }}{{ $u.Name }}{{ end }}
```

### Data

```json
{
  "Users": [
    {
      "Name": "ada"
    },
    {
      "Name": "grace"
    },
    {
      "Name": "ken"
    }
  ]
}
```

### Output

```
ada, grace, ken
ada, grace and ken
```

### Notes

`{{ if $i }}, {{ end }}` writes the separator **before** every item except the first (`$i` is 0, which is false). That is simpler than detecting the last item. The second line shows the 'a, b and c' variant, which does need the length; note `$.Users`, because inside `range` the dot has moved. When the data is already `[]string`, `join` is shorter: `{{ join ", " .Names }}`.

<a id="f07"></a>

## F07. Iterate a map in sorted key order

**Problem.** Go randomises map iteration, so you expect generated files to differ on every run and every diff to be noise.

Engine: `text/template`. Delimiters: `{{` `}}`.

### Template

```gotemplate
{{ range $k, $v := .Env }}{{ $k }}={{ $v }}
{{ end -}}
keys: {{ join "," (sortedKeys .Env) }}
```

### Data

```json
{
  "Env": {
    "APP": "x",
    "HOME": "/h",
    "ZONE": "a"
  }
}
```

### Output

```
APP=x
HOME=/h
ZONE=a
keys: APP,HOME,ZONE
```

### Notes

Good news: `range` over a map in a template **already visits keys in sorted order** (for string, integer and float keys), unlike a Go `for` loop, so the first two lines are deterministic. You need `sortedKeys` only when you want the slice itself: to join it, to take the first key, or to index the map with `index .Env (index $keys 0)`. It never helps with struct fields, which keep declaration order.

<a id="f08"></a>

## F08. Nested range with $ to reach the root

**Problem.** Inside a nested `range` the dot is the inner element, and the organisation name from the top is out of reach.

Engine: `text/template`. Delimiters: `{{` `}}`.

### Template

```gotemplate
{{ range .Teams }}{{ $team := . }}{{ range .Members }}{{ $.Org }}/{{ $team.Name }}: {{ . }}
{{ end }}{{ end -}}
```

### Data

```json
{
  "Org": "CoolGitOrg",
  "Teams": [
    {
      "Members": [
        "ada",
        "ken"
      ],
      "Name": "platform"
    },
    {
      "Members": [
        "grace"
      ],
      "Name": "data"
    }
  ]
}
```

### Output

```
CoolGitOrg/platform: ada
CoolGitOrg/platform: ken
CoolGitOrg/data: grace
```

### Notes

`$` is the data passed to `Execute`: it always reaches the root. To keep an outer *loop* element, bind it with `$team := .` before descending. The trap: inside a `{{ define }}` block `$` is the argument of that `template` call, not the root of the page (see F20).

<a id="f09"></a>

## F09. 1-based index arithmetic

**Problem.** `range` indexes from 0 and humans count from 1; numbered lists end up starting at 0.

Engine: `text/template`. Delimiters: `{{` `}}`.

### Template

```gotemplate
{{ range $i, $x := .Items }}{{ add $i 1 }}/{{ len $.Items }} {{ $x }}
{{ end -}}
```

### Data

```json
{
  "Items": [
    "lint",
    "test",
    "build"
  ]
}
```

### Output

```
1/3 lint
2/3 test
3/3 build
```

### Notes

Templates have no `+` operator; arithmetic is a function call. `add`, `sub`, `mul`, `div` and `mod` come from the shared FuncMap and accept any integer type, plus integral floats because JSON numbers arrive as `float64`. For plain loops `{{ range seq 3 }}` yields 1, 2, 3 directly.

<a id="f10"></a>

## F10. Zebra stripes with mod

**Problem.** Alternating row classes need the parity of the loop index, and there is no `%` in templates.

Engine: `html/template`. Delimiters: `{{` `}}`.

### Template

```gotemplate
<table>
{{- range $i, $r := .Rows }}
  <tr class="{{ if eq (mod $i 2) 0 }}even{{ else }}odd{{ end }}"><td>{{ $r }}</td></tr>
{{- end }}
</table>
```

### Data

```json
{
  "Rows": [
    "ada",
    "grace",
    "ken",
    "dennis"
  ]
}
```

### Output

```
<table>
  <tr class="even"><td>ada</td></tr>
  <tr class="odd"><td>grace</td></tr>
  <tr class="even"><td>ken</td></tr>
  <tr class="odd"><td>dennis</td></tr>
</table>
```

### Notes

`mod` takes the dividend first: `mod $i 2`. The pipe form `{{ $i | mod 2 }}` means `mod 2 $i`, which is 2 mod i, and is wrong in a way that looks plausible for i = 0..2. `ternary "even" "odd" (eq (mod $i 2) 0)` is the one-liner. Better still: do it in CSS with `tr:nth-child(even)` and leave the template out of it.

<a id="f11"></a>

## F11. Pluralisation

**Problem.** '1 files' and '0 file' look careless in a report.

Engine: `text/template`. Delimiters: `{{` `}}`.

### Template

```gotemplate
{{ range .Counts }}{{ . }} {{ plural . "file" "files" }}
{{ end -}}
```

### Data

```json
{
  "Counts": [
    0,
    1,
    2,
    11
  ]
}
```

### Output

```
0 files
1 file
2 files
11 files
```

### Notes

`plural n one other` returns `one` only when `n == 1`; zero is plural in English ("0 files"). This is English-only. Languages like Polish or Arabic have several plural categories chosen by CLDR rules, and no two-way function can express them: use a real message catalogue for user-facing text.

<a id="f12"></a>

## F12. time.Time: Format and friends

**Problem.** Dates arrive as `time.Time` and need a specific layout; people try `strftime` codes and get nonsense.

Engine: `text/template`. Delimiters: `{{` `}}`.

### Template

```gotemplate
iso:     {{ .When.Format "2006-01-02T15:04:05Z07:00" }}
short:   {{ .When.Format "Mon Jan _2 3:04PM" }}
helper:  {{ dateFmt "02 Jan 2006" .When }}
weekday: {{ .When.Weekday }}
next:    {{ (.When.AddDate 0 0 7).Format "2006-01-02" }}
unix:    {{ .When.Unix }}
```

### Data

```json
{
  "When": "2024-03-05T14:30:00Z"
}
```

### Output

```
iso:     2024-03-05T14:30:00Z
short:   Tue Mar  5 2:30PM
helper:  05 Mar 2024
weekday: Tuesday
next:    2024-03-12
unix:    1709649000
```

### Notes

Go layouts are written as the reference time `Mon Jan 2 15:04:05 MST 2006` (1 2 3 4 5 6 7 in order); `%Y-%m-%d` means nothing. Methods are called straight from the template, and arguments follow the method name: `.When.AddDate 0 0 7`. Parenthesise to chain: `(.When.AddDate 0 0 7).Format "..."`. `_2` pads the day with a space. Data decoded from JSON has no `time.Time` (it is a string); `dateFmt` also accepts RFC 3339 strings and Unix seconds for that case.

<a id="f13"></a>

## F13. printf: number formatting

**Problem.** Money, hex, padding and percentages come out as raw `%v` unless you format them.

Engine: `text/template`. Delimiters: `{{` `}}`.

### Template

```gotemplate
pi:      {{ printf "%08.3f" .Pi }}
columns: [{{ printf "%5d|%-5d" 42 42 }}]
bases:   {{ printf "%x %o %b" 255 255 5 }}
quoted:  {{ printf "%q" .Name }}
percent: {{ printf "%.1f%%" .Ratio }}
width:   [{{ printf "%-*s" 8 .Name }}]
```

### Data

```json
{
  "Name": "ken",
  "Pi": 3.14159265,
  "Ratio": 87.46
}
```

### Output

```
pi:      0003.142
columns: [   42|42   ]
bases:   ff 377 101
quoted:  "ken"
percent: 87.5%
width:   [ken     ]
```

### Notes

`printf` is `fmt.Sprintf`, verbs and all, including `*` for a computed width. A literal percent is `%%`. The verb must match the type: `%d` with a `float64` prints `%!d(float64=...)` instead of failing, which is how JSON-decoded numbers bite. Thousands separators are not in `fmt`; format those in Go.

<a id="f14"></a>

## F14. YAML block with toYAML | nindent

**Problem.** Splicing a map or list into a YAML file by printing it line by line gets the indentation, the quoting and the empty case wrong.

Engine: `text/template`. Delimiters: `{{` `}}`.

### Template

```gotemplate
containers:
  - name: {{ .Name }}
    env:{{ toYAML .Env | nindent 6 }}
    args:{{ toYAML .Args | nindent 6 }}
```

### Data

```json
{
  "Args": [
    "--serve",
    "--verbose"
  ],
  "Env": {
    "LOG": "debug",
    "PORT": "8080"
  },
  "Name": "api"
}
```

### Output

```
containers:
  - name: api
    env:
      LOG: debug
      PORT: "8080"
    args:
      - --serve
      - --verbose
```

### Notes

Let an emitter produce the sub-document and only *place* it: `key:` then `nindent N` puts a newline and indents every line by N, so the block lands under the key. `indent` (no newline) is for text that continues the current line. `toYAML` quotes what YAML would misread: `"8080"` stays a string, whereas hand-printed `PORT: 8080` becomes an integer and `no`/`on` become booleans. It writes no trailing newline, so the template owns line endings. The emitter is the stdlib-only one in `funcs/yaml.go`: sorted keys, no anchors, no flow style.

<a id="f15"></a>

## F15. JSON array with correct commas

**Problem.** A hand-built JSON array leaves a trailing comma, or `%q` produces a string JSON cannot parse.

Engine: `text/template`. Delimiters: `{{` `}}`.

### Template

```gotemplate
loop:  [{{ range $i, $t := .Tags }}{{ if $i }},{{ end }}{{ toJSON $t }}{{ end }}]
whole: {{ toJSON .Tags }}
%q:    [{{ range $i, $t := .Tags }}{{ if $i }},{{ end }}{{ printf "%q" $t }}{{ end }}]
```

### Data

```json
{
  "Tags": [
    "a",
    "b",
    "esc\u001b"
  ]
}
```

### Output

```
loop:  ["a","b","esc\u001b"]
whole: ["a","b","esc\u001b"]
%q:    ["a","b","esc\x1b"]
```

### Notes

Line 1 is the comma idiom (`{{ if $i }},{{ end }}` before every item but the first) with each element encoded by `toJSON`. Line 2 is what you should write: encode the whole value. Line 3 is the trap: `printf "%q"` makes a **Go** literal, and Go escapes ESC as `\x1b`, which is not valid JSON (`\u001b` is). It is right for ASCII text, which is why it survives review. Never build JSON from fragments if the values are not constants.

<a id="f16"></a>

## F16. JSON object from a map (sorted keys)

**Problem.** Labels have to be emitted as a JSON object, with stable order, correct escaping and commas only between members.

Engine: `text/template`. Delimiters: `{{` `}}`.

### Template

```gotemplate
compact: {{ toJSON .Labels }}
pretty:
{{ toPrettyJSON .Labels }}
by hand: {
{{- range $k, $v := .Labels }}{{ if ne $k (index (sortedKeys $.Labels) 0) }},{{ end }} {{ toJSON $k }}: {{ toJSON $v }}{{ end }} }
```

### Data

```json
{
  "Labels": {
    "app": "api",
    "motd": "say \"hi\"",
    "team": "platform"
  }
}
```

### Output

```
compact: {"app":"api","motd":"say \"hi\"","team":"platform"}
pretty:
{
  "app": "api",
  "motd": "say \"hi\"",
  "team": "platform"
}
by hand: { "app": "api", "motd": "say \"hi\"", "team": "platform" }
```

### Notes

`toJSON` marshals maps with **sorted keys**, so the output is byte-stable across runs, which is what makes generated files diffable. `toPrettyJSON` indents by two spaces. The hand-built version shows the problem with maps: `range` over a map gives the key but no index, so the first-member test compares against the first sorted key. Prefer the encoders; keep the hand-built form for the rare case where the surrounding text is not JSON.

<a id="f17"></a>

## F17. base64 and SHA-256

**Problem.** A header needs `user:password` in base64 and an ETag needs a digest, and neither should be computed outside the template.

Engine: `text/template`. Delimiters: `{{` `}}`.

### Template

```gotemplate
auth:   Basic {{ printf "%s:%s" .User .Pass | b64enc }}
back:   {{ printf "%s:%s" .User .Pass | b64enc | b64dec }}
sha256: {{ sha256sum .Pass }}
etag:   "{{ sha256sum .Body | printf "%.12s" }}"
```

### Data

```json
{
  "Body": "hello, world\n",
  "Pass": "hunter2",
  "User": "rob"
}
```

### Output

```
auth:   Basic cm9iOmh1bnRlcjI=
back:   rob:hunter2
sha256: f52fbd32b2b3b86ff88ef6c490628285f482af15ddcb29541f94bcf526a3f6c7
etag:   "853ff93762a0"
```

### Notes

`b64enc` is standard padded base64; `b64dec` returns an error on bad input rather than garbage. `sha256sum` returns lowercase hex. Two warnings: base64 is an encoding, not encryption, and a bare SHA-256 is not a password hash (use bcrypt/scrypt/argon2 in the program that stores passwords). And a secret placed into a rendered file is in that file: treat the output as sensitive.

<a id="f18"></a>

## F18. env lookup (trusted templates) with a default

**Problem.** A rendered config should pick up the deployment environment, falling back when a variable is unset.

Engine: `text/template`. Delimiters: `{{` `}}`.

### Template

```gotemplate
app:  {{ .App }}
env:  {{ default "dev" (env "APP_ENV") }}
home: {{ env "HOME" }}
log:  {{ default "info" (env "LOG_LEVEL") }}
```

### Data

```json
{
  "App": "cookbook"
}
```

### Output

```
app:  cookbook
env:  staging
home: /home/rob
log:  info
```

### Notes

`env` returns the empty string for an unset variable, so `default` supplies the fallback; there is no way to tell 'unset' from 'set to empty' with `os.Getenv`. **Only trusted templates may call `env`.** Anyone who can write the template can print `AWS_SECRET_ACCESS_KEY`. For templates from users, build the FuncMap with `funcs.Options{Env: allow("APP_ENV", "HOME")}` or leave `env` out. In these examples the environment is a fixed map, so the output is the same everywhere.

<a id="f19"></a>

## F19. Include a file from a rooted filesystem

**Problem.** Boilerplate headers and message-of-the-day text live in files; a naive include reads whatever path the template names, `../../etc/passwd` included.

Engine: `text/template`. Delimiters: `{{` `}}`.

### Template

```gotemplate
# {{ .Title }}
== motd ==
{{ file "motd.txt" | trim }}
== header, indented ==
{{ file "header.txt" | trim | indent 4 }}
```

### Data

```json
{
  "Title": "Release notes"
}
```

### Glue

Go beyond the FuncMap. This is the code that ran, copied from `formulas/glue.go`.

```go
// glueFile renders the template, then tries three names that must not work.
func glueFile(f Formula) (string, error) {
	fm := funcs.New(Options()) // Options().FS is rooted at testdata/tmpl
	t, err := template.New("page").Funcs(fm).Parse(f.Template)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	if err := t.Execute(&b, f.Data); err != nil {
		return "", err
	}
	probe, err := template.New("probe").Funcs(fm).Parse(`{{ file . }}`)
	if err != nil {
		return "", err
	}
	for _, name := range []string{"../formulas.go", "/etc/passwd", "missing.txt"} {
		err := probe.Execute(io.Discard, name)
		fmt.Fprintf(&b, "file %q: %v\n", name, err)
	}
	return b.String(), nil
}
```

### Output

```
# Release notes
== motd ==
Welcome to the cookbook.
Every formula here is tested.
== header, indented ==
    # Generated file
    # Do not edit
file "../formulas.go": template: probe:1:3: executing "probe" at <file .>: error calling file: file: invalid path "../formulas.go"
file "/etc/passwd": template: probe:1:3: executing "probe" at <file .>: error calling file: file: invalid path "/etc/passwd"
file "missing.txt": template: probe:1:3: executing "probe" at <file .>: error calling file: file: open missing.txt: file does not exist
```

### Notes

`file` reads from the `fs.FS` given in `funcs.Options.FS`, and only names that satisfy `fs.ValidPath` are accepted: no leading `/`, no `.` or `..` elements. Hand it `os.DirFS(dir)` (or, from Go 1.24, `os.OpenRoot(dir)` and `root.FS()`, which also refuses symlink escapes) and the template can see that tree and nothing else. The glue runs the escape attempt and prints the error. With no FS configured the function fails closed.

<a id="f20"></a>

## F20. Recursive tree with define and template

**Problem.** A nested menu or directory tree has unknown depth; `range` inside `range` only reaches a fixed number of levels.

Engine: `text/template`. Delimiters: `{{` `}}`.

### Template

```gotemplate
{{ define "node" }}{{ repeat .Depth "  " }}{{ .Node.Name }}{{ if .Node.Kids }}/{{ end }}
{{ range .Node.Kids }}{{ template "node" (dict "Node" . "Depth" (add $.Depth 1)) }}{{ end }}{{ end -}}
{{ template "node" (dict "Node" .Tree "Depth" 0) }}
```

### Data

```json
{
  "Tree": {
    "Kids": [
      {
        "Kids": [
          {
            "Name": "funcs.go"
          },
          {
            "Name": "yaml.go"
          }
        ],
        "Name": "funcs"
      },
      {
        "Kids": [
          {
            "Kids": [
              {
                "Name": "main.go"
              }
            ],
            "Name": "cookbook"
          }
        ],
        "Name": "cmd"
      },
      {
        "Name": "go.mod"
      }
    ],
    "Name": "cookbook"
  }
}
```

### Output

```
cookbook/
  funcs/
    funcs.go
    yaml.go
  cmd/
    cookbook/
      main.go
  go.mod
```

### Notes

A `define`d template can call itself; the recursion ends when `.Node.Kids` is empty, so `range` has nothing to do. Two traps. First, `template` passes **one** value, so depth and node travel together in a `dict`. Second, inside the definition `$` is that dict, not the page data, which is why `add $.Depth 1` works. A cyclic data structure recurses until the template's execution depth limit stops it with an error; the data must be a tree.

<a id="f21"></a>

## F21. Aligned columns with text/tabwriter

**Problem.** `printf "%-20s"` hard-codes a width that is wrong the moment a name is longer, and counts bytes, not runes.

Engine: `text/template`. Delimiters: `{{` `}}`.

### Template

```gotemplate
NAME	STATUS	AGE
{{ range .Pods }}{{ .Name }}	{{ .Status }}	{{ .Age }}
{{ end -}}
```

### Data

```json
{
  "Pods": [
    {
      "Age": "3d",
      "Name": "api-7d9f",
      "Status": "Running"
    },
    {
      "Age": "12h",
      "Name": "db-0",
      "Status": "CrashLoopBackOff"
    },
    {
      "Age": "5m",
      "Name": "cache",
      "Status": "Pending"
    }
  ]
}
```

### Glue

Go beyond the FuncMap. This is the code that ran, copied from `formulas/glue.go`.

```go
// ppTabwriter aligns tab-separated cells into columns two spaces apart.
// Only cells that end in a tab take part; the last cell of a line is free.
func ppTabwriter(s string) (string, error) {
	var b strings.Builder
	w := tabwriter.NewWriter(&b, 0, 8, 2, ' ', 0)
	if _, err := io.WriteString(w, s); err != nil {
		return "", err
	}
	if err := w.Flush(); err != nil {
		return "", err
	}
	return b.String(), nil
}
```

### Output

```
NAME      STATUS            AGE
api-7d9f  Running           3d
db-0      CrashLoopBackOff  12h
cache     Pending           5m
```

### Notes

The template emits **tab-separated** cells and knows nothing about widths; a post-process step runs the text through `text/tabwriter`, which measures every column and pads. Only cells *terminated* by a tab are aligned, so the last cell of each row (no trailing tab) may be any length. This is how `kubectl` and `go` print tables. The same text piped through `column -t` works for ASCII but not for wide runes or embedded spaces in cells.

<a id="f22"></a>

## F22. CSV: never join with commas

**Problem.** A field containing a comma, a quote or a newline breaks a CSV row that was assembled with `{{ .A }},{{ .B }}`.

Engine: `text/template`. Delimiters: `{{` `}}`.

### Template

```gotemplate
# naive: template only
{{ range .Rows }}{{ .Name }},{{ .Note }},{{ .Qty }}
{{ end -}}
# safe: encoding/csv
{{ range .Rows }}{{ csvrow .Name .Note .Qty }}{{ end -}}
```

### Data

```json
{
  "Rows": [
    {
      "Name": "widget",
      "Note": "fancy, shiny",
      "Qty": 3
    },
    {
      "Name": "gadget",
      "Note": "say \"hi\"",
      "Qty": 1
    },
    {
      "Name": "gizmo",
      "Note": "two\nlines",
      "Qty": 2
    }
  ]
}
```

### Glue

Go beyond the FuncMap. This is the code that ran, copied from `formulas/glue.go`.

```go
// csvRow formats one CSV record, quoting as RFC 4180 requires.
func csvRow(fields ...any) (string, error) {
	rec := make([]string, len(fields))
	for i, f := range fields {
		rec[i] = fmt.Sprint(f)
	}
	var b strings.Builder
	w := csv.NewWriter(&b)
	if err := w.Write(rec); err != nil {
		return "", err
	}
	w.Flush()
	return b.String(), w.Error()
}
```

### Output

```
# naive: template only
widget,fancy, shiny,3
gadget,say "hi",1
gizmo,two
lines,2
# safe: encoding/csv
widget,"fancy, shiny",3
gadget,"say ""hi""",1
gizmo,"two
lines",2
```

### Notes

RFC 4180 quoting is a state machine: wrap in quotes if the field has a comma, quote or line break, and double any quotes inside. A template can express it, but the version that is correct on the first try is `encoding/csv`. The glue adds one function, `csvrow`, that writes a record through `csv.Writer`. The naive block above produces rows with the wrong number of fields, and 'two lines' splits into two records. Spreadsheets also execute cells that begin with `=`, `+`, `-` or `@` (CSV injection); prefix such cells with `'` if the file will be opened by people.

<a id="f23"></a>

## F23. Markdown table

**Problem.** A pipe or a newline in a cell silently ends the cell or the row.

Engine: `text/template`. Delimiters: `{{` `}}`.

### Template

```gotemplate
| Name | Notes |
|------|-------|
{{ range .Rows }}| {{ .Name }} | {{ .Note | replace "|" "\\|" | replace "\n" "<br>" }} |
{{ end -}}
```

### Data

```json
{
  "Rows": [
    {
      "Name": "pipe",
      "Note": "a | b"
    },
    {
      "Name": "multi",
      "Note": "line1\nline2"
    },
    {
      "Name": "plain",
      "Note": "ok"
    }
  ]
}
```

### Output

```
| Name | Notes |
|------|-------|
| pipe | a \| b |
| multi | line1<br>line2 |
| plain | ok |
```

### Notes

In a pipe-table cell, `|` must become `\|` and a newline must become `<br>`. Backslash comes first if you also escape backslashes. `"\\|"` in the template is the two-character string `\|`: the template lexer processes string escapes like Go. This is text/template: Markdown is not HTML, so html/template's escaping would be wrong here (it would turn `&` into `&amp;`).

<a id="f24"></a>

## F24. HCL / Terraform emission

**Problem.** A string containing `${` or `%{` becomes an interpolation in HCL, and a Go-quoted string is not always a valid HCL string.

Engine: `text/template`. Delimiters: `{{` `}}`.

### Template

```gotemplate
{{- define "hcl" }}{{ printf "%q" . | replace "${" "$${" | replace "%{" "%%{" }}{{ end -}}
{{- $w := 0 }}{{ range $k, $_ := .Tags }}{{ if gt (len $k) $w }}{{ $w = len $k }}{{ end }}{{ end -}}
resource "aws_instance" {{ template "hcl" .Name }} {
  ami   = {{ template "hcl" .AMI }}
  ports = [{{ range $i, $p := .Ports }}{{ if $i }}, {{ end }}{{ $p }}{{ end }}]
  motd  = {{ template "hcl" .Motd }}

  tags = {
{{- range $k, $v := .Tags }}
    {{ printf "%-*s" $w $k }} = {{ template "hcl" $v }}
{{- end }}
  }
}
```

### Data

```json
{
  "AMI": "ami-0abc",
  "Motd": "cost is ${total}\n",
  "Name": "web",
  "Ports": [
    80,
    443
  ],
  "Tags": {
    "env": "prod",
    "owner": "platform"
  }
}
```

### Output

```
resource "aws_instance" "web" {
  ami   = "ami-0abc"
  ports = [80, 443]
  motd  = "cost is $${total}\n"

  tags = {
    env   = "prod"
    owner = "platform"
  }
}
```

### Notes

`printf "%q"` gives HCL's escapes for `\"`, `\\`, `\n` and `\t`, but not for every control or non-printable character (HCL knows `\uNNNN`, Go emits `\x1b`). Validate or restrict the input if it is not plain text. The decisive trap is interpolation: HCL expands `${...}` and `%{...}` inside quoted strings, so user data containing them executes. Escape them as `$${` and `%%{` in one place (the `hcl` partial) and never `print` a value into HCL any other way. The `=` signs are aligned the way `terraform fmt` wants them: the longest key is measured first with an accumulator (F50) and `%-*s` pads to it. `terraform` is not installed in the build environment, so this output is checked against that style by eye and by the test, not by `terraform fmt`.

<a id="f25"></a>

## F25. GitHub Actions workflow with [[ ]] delimiters

**Problem.** `${{ secrets.X }}` is a GitHub expression, but the template engine reads `{{ secrets.X }}` as its own action and fails or eats it.

Engine: `text/template`. Delimiters: `[[` `]]`.

### Template

```gotemplate
name: [[ .Name ]]
on:
  push:
    branches: [[ toJSON .Branches ]]
permissions:
  contents: read
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v6
      - uses: actions/setup-go@v6
        with:
          go-version: "[[ .GoVersion ]]"
      - name: Cross-build
        env:
          GH_TOKEN: ${{ github.token }}
          REF: ${{ github.ref_name }}
        run: |
[[- range .Targets ]]
          GOOS=[[ .OS ]] GOARCH=[[ .Arch ]] go build -o /dev/null ./...
[[- end ]]
```

### Data

```json
{
  "Branches": [
    "main"
  ],
  "GoVersion": "1.24.x",
  "Name": "ci",
  "Targets": [
    {
      "Arch": "amd64",
      "OS": "linux"
    },
    {
      "Arch": "amd64",
      "OS": "plan9"
    }
  ]
}
```

### Output

```
name: ci
on:
  push:
    branches: ["main"]
permissions:
  contents: read
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v6
      - uses: actions/setup-go@v6
        with:
          go-version: "1.24.x"
      - name: Cross-build
        env:
          GH_TOKEN: ${{ github.token }}
          REF: ${{ github.ref_name }}
        run: |
          GOOS=linux GOARCH=amd64 go build -o /dev/null ./...
          GOOS=plan9 GOARCH=amd64 go build -o /dev/null ./...
```

### Notes

Change the *template's* delimiters with `Delims("[[", "]]")` and everything else in the file, including `${{ ... }}`, is plain text. That is cleaner than escaping each expression (F54). Choose delimiters that do not occur in the target: `[[` collides with YAML flow sequences of sequences (`[[1,2]]`) and with TOML/INI headers, so for those use `<%`/`%>` or `@@`. Per the repo conventions, untrusted values (issue titles, inputs) must reach `run:` through `env:` and be read as `"$VAR"`, never interpolated into the script.

<a id="f26"></a>

## F26. GitHub Issue Form YAML

**Problem.** An issue-form definition is structured YAML where a colon in a description or a leading `[` in a title changes its meaning.

Engine: `text/template`. Delimiters: `{{` `}}`.

### Template

```gotemplate
name: {{ toYAML .Name }}
description: {{ toYAML .Desc }}
title: {{ toYAML .Title }}
labels: {{ toJSON .Labels }}
body:
{{- range .Fields }}
  - type: {{ .Type }}
    id: {{ .ID }}
    attributes:
      label: {{ toYAML .Label }}
{{- if .Options }}
      options:{{ toYAML .Options | nindent 8 }}
{{- end }}
    validations:
      required: {{ .Required }}
{{- end }}
```

### Data

```json
{
  "Desc": "Something is broken: tell us",
  "Fields": [
    {
      "ID": "version",
      "Label": "Version",
      "Required": true,
      "Type": "input"
    },
    {
      "ID": "os",
      "Label": "Operating system",
      "Options": [
        "linux",
        "darwin",
        "plan9"
      ],
      "Required": true,
      "Type": "dropdown"
    },
    {
      "ID": "repro",
      "Label": "Steps to reproduce",
      "Required": false,
      "Type": "textarea"
    }
  ],
  "Labels": [
    "bug",
    "triage"
  ],
  "Name": "Bug report",
  "Title": "[bug] "
}
```

### Output

```
name: Bug report
description: "Something is broken: tell us"
title: "[bug] "
labels: ["bug","triage"]
body:
  - type: input
    id: version
    attributes:
      label: Version
    validations:
      required: true
  - type: dropdown
    id: os
    attributes:
      label: Operating system
      options:
        - linux
        - darwin
        - plan9
    validations:
      required: true
  - type: textarea
    id: repro
    attributes:
      label: Steps to reproduce
    validations:
      required: false
```

### Notes

Every free-text scalar goes through `toYAML` so it is quoted when it must be: the description contains `: ` and the title starts with `[` and ends with a space; unquoted, both would be parse errors or silent changes. Fixed vocabulary (`type`, `id`) is printed bare because the template author controls it. `{{ if .Options }}` makes the key optional, since a missing map key is nil. The file belongs in `.github/ISSUE_TEMPLATE/`. Issue forms do not use `{{ }}`, so no delimiter change is needed.

<a id="f27"></a>

## F27. Dockerfile with a JSON-array ENTRYPOINT

**Problem.** Shell-form `ENTRYPOINT server -a :8080` runs under `/bin/sh -c`, swallows signals and breaks on spaces; the exec form must be a JSON array with double quotes.

Engine: `text/template`. Delimiters: `{{` `}}`.

### Template

```gotemplate
FROM {{ .Base }}
LABEL org.opencontainers.image.source={{ printf "%q" .Source }}
COPY --chmod=0755 server {{ index .Entrypoint 0 }}
EXPOSE {{ .Port }}
USER nonroot:nonroot
ENTRYPOINT {{ toJSON .Entrypoint }}
```

### Data

```json
{
  "Base": "gcr.io/distroless/static:nonroot",
  "Entrypoint": [
    "/app/server",
    "-a",
    ":8080",
    "-c",
    "/etc/app.conf"
  ],
  "Port": 8080,
  "Source": "https://github.com/CoolGitOrg/go-template-cookbook"
}
```

### Output

```
FROM gcr.io/distroless/static:nonroot
LABEL org.opencontainers.image.source="https://github.com/CoolGitOrg/go-template-cookbook"
COPY --chmod=0755 server /app/server
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/app/server","-a",":8080","-c","/etc/app.conf"]
```

### Notes

Dockerfile exec form is parsed as JSON, so `toJSON` is exactly right and handles an argument with a space or a quote for free. `printf "%q"` is acceptable for `LABEL` values. `toJSON` leaves `<`, `>` and `&` alone (no HTML escaping), which matters: `\u0026` in an ENTRYPOINT would still parse, but nobody wants to read it. With exec form the process is PID 1 and receives SIGTERM directly; with shell form it does not.

<a id="f28"></a>

## F28. Makefile with literal tabs

**Problem.** Recipe lines must start with a TAB; editors, formatters and chat paste turn it into spaces and `make` stops with 'missing separator'.

Engine: `text/template`. Delimiters: `{{` `}}`.

### Template

```gotemplate
BIN := {{ .Bin }}
.PHONY: {{ join " " .Phony }}

all: build test
{{ range .Targets }}
{{ .Name }}:{{ if .Deps }} {{ .Deps }}{{ end }}
{{- range .Cmds }}
	{{ . }}
{{- end }}
{{ end -}}
```

### Data

```json
{
  "Bin": "cookbook",
  "Phony": [
    "all",
    "build",
    "test",
    "clean"
  ],
  "Targets": [
    {
      "Cmds": [
        "go build -o bin/$(BIN) ./cmd/$(BIN)"
      ],
      "Name": "build"
    },
    {
      "Cmds": [
        "go vet ./...",
        "go test ./..."
      ],
      "Deps": "build",
      "Name": "test"
    },
    {
      "Cmds": [
        "rm -rf bin"
      ],
      "Name": "clean"
    }
  ]
}
```

### Output

```
BIN := cookbook
.PHONY: all build test clean

all: build test

build:
	go build -o bin/$(BIN) ./cmd/$(BIN)

test: build
	go vet ./...
	go test ./...

clean:
	rm -rf bin
```

### Notes

The template file must contain a real tab character before `{{ . }}`; the test also checks that every recipe line in the output starts with one. Keep that file out of editors that expand tabs, add a `.editorconfig` section `[Makefile]` / `indent_style = tab`, or write the tab as `{{ "\t" }}` when the source is not under your control. `$(BIN)` and `$@` pass through untouched: the template engine only reacts to its own delimiters. A `$` that must reach make as a literal is written `$$` in the *data*.

<a id="f29"></a>

## F29. Plan 9 mkfile

**Problem.** mk is not make: phony targets are marked `:V:`, recipes run under rc, and variables are `$BIN`, not `$(BIN)`.

Engine: `text/template`. Delimiters: `{{` `}}`.

### Template

```gotemplate
BIN={{ .Bin }}

all:V: {{ join " " .All }}
{{ range .Targets }}
{{ .Name }}:V:{{ if .Deps }} {{ .Deps }}{{ end }}
{{- range .Cmds }}
	{{ . }}
{{- end }}
{{ end -}}
```

### Data

```json
{
  "All": [
    "build",
    "test"
  ],
  "Bin": "cookbook",
  "Targets": [
    {
      "Cmds": [
        "CGO_ENABLED=0 GOOS=plan9 GOARCH=amd64 go build -o bin/$BIN ./cmd/$BIN"
      ],
      "Name": "build"
    },
    {
      "Cmds": [
        "go test ./..."
      ],
      "Deps": "build",
      "Name": "test"
    },
    {
      "Cmds": [
        "rm -r bin"
      ],
      "Name": "clean"
    }
  ]
}
```

### Output

```
BIN=cookbook

all:V: build test

build:V:
	CGO_ENABLED=0 GOOS=plan9 GOARCH=amd64 go build -o bin/$BIN ./cmd/$BIN

test:V: build
	go test ./...

clean:V:
	rm -r bin
```

### Notes

Same skeleton as F28 with mk's spelling: `target:V: prereqs` (V = virtual, always run), a tab-indented recipe, and `$BIN` for variables. The recipe body is handed to `rc`, so `VAR=value cmd` prefix assignments work and `&&` does not (use `;` or `if`). The Go toolchain on Plan 9 reads `GOOS`/`GOARCH` from the environment, which rc sets with exactly this prefix form.

<a id="f30"></a>

## F30. Shell script with shq quoting

**Problem.** Interpolating a value into a script is command injection: a name like `x'; rm -rf /` ends the string and runs the rest.

Engine: `text/template`. Delimiters: `{{` `}}`.

### Template

```gotemplate
#!/usr/bin/env bash
set -euo pipefail

name={{ shq .Name }}
dest={{ shq .Dest }}

mkdir -p -- "$dest"
printf '%s\n' "$name" > "$dest/name.txt"
{{- range .Files }}
cp -- {{ shq . }} "$dest/"
{{- end }}
```

### Data

```json
{
  "Dest": "/tmp/my dir",
  "Files": [
    "a b.txt",
    "c'd.txt",
    "plain.txt"
  ],
  "Name": "it's \"quoted\"; rm -rf /"
}
```

### Output

```
#!/usr/bin/env bash
set -euo pipefail

name='it'\''s "quoted"; rm -rf /'
dest='/tmp/my dir'

mkdir -p -- "$dest"
printf '%s\n' "$name" > "$dest/name.txt"
cp -- 'a b.txt' "$dest/"
cp -- 'c'\''d.txt' "$dest/"
cp -- plain.txt "$dest/"
```

### Notes

`shq` wraps a value in single quotes and writes each embedded `'` as `'\''`, the only quoting a POSIX shell does not reinterpret. Words made only of safe characters are left bare, which keeps the output readable. The script then assigns to variables once and uses `"$var"` everywhere, so the template touches quoting in exactly one place. `--` stops option parsing for values that begin with `-`. The test runs `bash -n` over this output when bash is installed. Where a script can read its inputs from the environment instead, do that and avoid generating code from data at all.

<a id="f31"></a>

## F31. SQL: identifier allowlist and placeholders

**Problem.** Values and identifiers cannot both be parameterised: placeholders stop at values, so a table or column name taken from input is injection.

Engine: `text/template`. Delimiters: `{{` `}}`.

### Template

```gotemplate
SELECT {{ range $i, $c := .Cols }}{{ if $i }}, {{ end }}{{ ident $c }}{{ end }}
FROM {{ ident .Table }}
WHERE {{ ident .Filter }} = {{ bind .Value }};
```

### Data

```json
{
  "Cols": [
    "id",
    "name"
  ],
  "Filter": "name",
  "Table": "users",
  "Value": "Robert'); DROP TABLE students;--"
}
```

### Glue

Go beyond the FuncMap. This is the code that ran, copied from `formulas/glue.go`.

```go
// allowed is the closed set of identifiers a SQL template may name.
var allowed = map[string]bool{"users": true, "id": true, "name": true, "email": true}

// glueSQL adds ident and bind to the FuncMap, renders the template, then
// renders it again with a table name that is not on the allowlist.
func glueSQL(f Formula) (string, error) {
	run := func(data any) (string, error) {
		var args []any
		fm := funcs.New(Options())
		fm["ident"] = func(s string) (string, error) {
			if !allowed[s] {
				return "", fmt.Errorf("ident: %q is not in the allowlist", s)
			}
			return `"` + s + `"`, nil
		}
		fm["bind"] = func(v any) string {
			args = append(args, v)
			return "$" + strconv.Itoa(len(args))
		}
		t, err := template.New("sql").Funcs(fm).Parse(f.Template)
		if err != nil {
			return "", err
		}
		var b strings.Builder
		if err := t.Execute(&b, data); err != nil {
			return "", err
		}
		j, err := json.Marshal(args)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "-- args: %s\n", j)
		return b.String(), nil
	}
	out, err := run(f.Data)
	if err != nil {
		return "", err
	}
	_, err = run(M{"Table": "users; DROP TABLE users", "Cols": L{"id"}, "Filter": "id", "Value": 1})
	if err == nil {
		return "", errors.New("identifier outside the allowlist was accepted")
	}
	return out + "-- rejected: " + err.Error() + "\n", nil
}
```

### Output

```
SELECT "id", "name"
FROM "users"
WHERE "name" = $1;
-- args: ["Robert'); DROP TABLE students;--"]
-- rejected: template: sql:2:8: executing "sql" at <ident .Table>: error calling ident: ident: "users; DROP TABLE users" is not in the allowlist
```

### Notes

Two rules, enforced by two functions the glue adds. **Values** never appear in the SQL text: `bind` records the value and prints `$1`, `$2`, ... and the driver sends the values separately. **Identifiers** cannot be bound, so `ident` accepts only names on a fixed allowlist and quotes them; anything else is a template error, not a query. The second run in the glue shows the rejection. Do not 'sanitise' with `replace`: allowlist.

<a id="f32"></a>

## F32. Go source, gofmt'd

**Problem.** Generated Go that is not gofmt-clean shows up as diff noise forever, and a template cannot know the column widths gofmt will choose.

Engine: `text/template`. Delimiters: `{{` `}}`.

### Template

```gotemplate
// Code generated by cookbook; DO NOT EDIT.

package {{ .Pkg }}

type {{ .Type }} int

const (
{{ range $i, $n := .Names }}{{ $n }} {{ $.Type }} = {{ $i }}
{{ end }})

var names = map[{{ .Type }}]string{
{{ range .Names }}{{ . }}: {{ printf "%q" . }},
{{ end }}}

func (z {{ .Type }}) String() string { return names[z] }
```

### Data

```json
{
  "Names": [
    "Alpha",
    "Beta",
    "Gamma"
  ],
  "Pkg": "zone",
  "Type": "Zone"
}
```

### Glue

Go beyond the FuncMap. This is the code that ran, copied from `formulas/glue.go`.

```go
// ppGofmt formats generated Go source exactly as gofmt would, and fails if
// the source does not parse.
func ppGofmt(s string) (string, error) {
	out, err := format.Source([]byte(s))
	if err != nil {
		return "", err
	}
	return string(out), nil
}
```

### Output

```
// Code generated by cookbook; DO NOT EDIT.

package zone

type Zone int

const (
	Alpha Zone = 0
	Beta  Zone = 1
	Gamma Zone = 2
)

var names = map[Zone]string{
	Alpha: "Alpha",
	Beta:  "Beta",
	Gamma: "Gamma",
}

func (z Zone) String() string { return names[z] }
```

### Notes

Write the template for correctness and let `go/format.Source` do layout: the template above has no indentation at all, and the output is aligned exactly as `gofmt` would leave it. `format.Source` also parses the result, so a template bug that produces invalid Go fails the build of the generator instead of the build of the generated package. `printf "%q"` is a correct Go string literal by definition. Put the `Code generated ... DO NOT EDIT.` line first so linters and `go vet` skip the file.

<a id="f33"></a>

## F33. C source (Plan 9 style)

**Problem.** C string literals have their own escapes, and the generated file should look like the rest of the tree.

Engine: `text/template`. Delimiters: `{{` `}}`.

### Template

```gotemplate
#include <u.h>
#include <libc.h>

static char *names[] = {
{{ range .Names }}	{{ printf "%q" . }},
{{ end }}};

void
main(int, char**)
{
	int i;

	for(i = 0; i < nelem(names); i++)
		print("%s\n", names[i]);
	exits(nil);
}
```

### Data

```json
{
  "Names": [
    "rob",
    "ken",
    "dennis \"dmr\"",
    "plan 9\tfrom bell labs"
  ]
}
```

### Output

```
#include <u.h>
#include <libc.h>

static char *names[] = {
	"rob",
	"ken",
	"dennis \"dmr\"",
	"plan 9\tfrom bell labs",
};

void
main(int, char**)
{
	int i;

	for(i = 0; i < nelem(names); i++)
		print("%s\n", names[i]);
	exits(nil);
}
```

### Notes

Go's `%q` and C agree on `\"`, `\\`, `\n` and `\t`, so for ordinary text `printf "%q"` is a valid C literal. They diverge elsewhere: Go prints non-printable runes as `\u00e9`-style or `\x7f`, and Plan 9 C does not read `\u`. ISO C also treats `??/` as a trigraph. If the data is not plain ASCII, escape in Go with a purpose-built function. Layout follows Plan 9 conventions: return type on its own line, `nelem`, tabs. The `<u.h>`/`<libc.h>` pair is Plan 9's; on Unix use `<stdio.h>`.

<a id="f34"></a>

## F34. SVG sparkline polyline

**Problem.** A tiny trend chart should be one inline element, not an image or a JavaScript library, and the numbers must be scaled into the viewBox.

Engine: `html/template`. Delimiters: `{{` `}}`.

### Template

```gotemplate
<svg xmlns="http://www.w3.org/2000/svg" width="{{ .W }}" height="{{ .H }}" viewBox="0 0 {{ .W }} {{ .H }}" role="img" aria-label="{{ .Label }}">
  <polyline fill="none" stroke="#06c" stroke-width="1.5" points="{{ spark .Vals .W .H }}"/>
</svg>
```

### Data

```json
{
  "H": 30,
  "Label": "requests per minute",
  "Vals": [
    3,
    7,
    4,
    9,
    6,
    12,
    8
  ],
  "W": 120
}
```

### Glue

Go beyond the FuncMap. This is the code that ran, copied from `formulas/glue.go`.

```go
// spark scales vals into a w by h box and returns the points attribute of an
// SVG polyline. SVG's y axis points down.
func spark(vals []int, w, h int) string {
	if len(vals) == 0 {
		return ""
	}
	lo, hi := vals[0], vals[0]
	for _, v := range vals {
		lo, hi = min(lo, v), max(hi, v)
	}
	pts := make([]string, len(vals))
	for i, v := range vals {
		x := 0.0
		if len(vals) > 1 {
			x = float64(i) * float64(w) / float64(len(vals)-1)
		}
		y := float64(h) / 2 // a flat series sits in the middle
		if hi > lo {
			y = float64(h-1) - float64(v-lo)*float64(h-2)/float64(hi-lo)
		}
		pts[i] = fmt.Sprintf("%.1f,%.1f", x, y)
	}
	return strings.Join(pts, " ")
}
```

### Output

```
<svg xmlns="http://www.w3.org/2000/svg" width="120" height="30" viewBox="0 0 120 30" role="img" aria-label="requests per minute">
  <polyline fill="none" stroke="#06c" stroke-width="1.5" points="0.0,29.0 20.0,16.6 40.0,25.9 60.0,10.3 80.0,19.7 100.0,1.0 120.0,13.4"/>
</svg>
```

### Notes

Scaling needs division and min/max, which are easier in Go than in a template, so the glue adds one function, `spark`, that returns the `points` string. The template keeps the markup. SVG's y axis points down, hence `h-1 - ...`. Using html/template means the `aria-label` is escaped properly if it ever contains a quote. A flat series (all values equal) draws a line through the middle instead of dividing by zero.

<a id="f35"></a>

## F35. HTML email plus text alternative

**Problem.** A multipart email needs the same facts as HTML (escaped) and as plain text (not escaped), and using one engine for both gets one of them wrong.

Engine: `html/template`. Delimiters: `{{` `}}`.

### Template

```gotemplate
<p>Hello <b>{{ .Name }}</b>,</p>
<p>Order {{ .Order }}:</p>
<ul>
{{- range .Lines }}
  <li>{{ .Qty }} x {{ .Item }} @ {{ printf "%.2f" .Price }}</li>
{{- end }}
</ul>
<p>Total: {{ printf "%.2f" .Total }}. {{ .Note }}</p>
```

### Data

```json
{
  "Lines": [
    {
      "Item": "Analytical engine",
      "Price": 199.5,
      "Qty": 1
    },
    {
      "Item": "Punch cards <100>",
      "Price": 2.25,
      "Qty": 3
    }
  ],
  "Name": "Ada \"Countess\" <ada@example.org>",
  "Note": "Pay by 5 Apr & enjoy.",
  "Order": "A-1042",
  "Total": 206.25
}
```

### Glue

Go beyond the FuncMap. This is the code that ran, copied from `formulas/glue.go`.

```go
// emailText is the plain-text alternative. It is a text/template: nothing
// in it is escaped.
const emailText = `Hello {{ .Name }},

Order {{ .Order }}:
{{ range .Lines -}}
 * {{ .Qty }} x {{ .Item }} @ {{ printf "%.2f" .Price }}
{{ end }}
Total: {{ printf "%.2f" .Total }}. {{ .Note }}
`

// glueEmail renders the same data with both engines.
func glueEmail(f Formula) (string, error) {
	fm := funcs.New(Options())
	txt, err := template.New("text").Funcs(fm).Parse(emailText)
	if err != nil {
		return "", err
	}
	html, err := htemplate.New("html").Funcs(fm).Parse(f.Template)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("=== text/plain (text/template) ===\n")
	if err := txt.Execute(&b, f.Data); err != nil {
		return "", err
	}
	b.WriteString("\n=== text/html (html/template) ===\n")
	if err := html.Execute(&b, f.Data); err != nil {
		return "", err
	}
	return b.String(), nil
}
```

### Output

```
=== text/plain (text/template) ===
Hello Ada "Countess" <ada@example.org>,

Order A-1042:
* 1 x Analytical engine @ 199.50
* 3 x Punch cards <100> @ 2.25

Total: 206.25. Pay by 5 Apr & enjoy.

=== text/html (html/template) ===
<p>Hello <b>Ada &#34;Countess&#34; &lt;ada@example.org&gt;</b>,</p>
<p>Order A-1042:</p>
<ul>
  <li>1 x Analytical engine @ 199.50</li>
  <li>3 x Punch cards &lt;100&gt; @ 2.25</li>
</ul>
<p>Total: 206.25. Pay by 5 Apr &amp; enjoy.</p>
```

### Notes

Same data, two engines. `html/template` escapes `<`, `>`, `&` and quotes, so `Punch cards <100>` cannot become a tag. `text/template` must **not** escape, or the plain-text part would show `&lt;` to the reader. Using `text/template` for the HTML part is an injection hole; using `html/template` for the text part is a display bug. Build the `multipart/alternative` message with `mime/multipart` and put text first, HTML last (the last part is the preferred one).

<a id="f36"></a>

## F36. htmx partial (hx-get, hx-target, hx-swap)

**Problem.** An htmx endpoint returns a *fragment*, not a page, and the 'load more' control has to replace itself with the next rows and a new control.

Engine: `html/template`. Delimiters: `{{` `}}`.

### Template

```gotemplate
{{ range .Items -}}
<tr id="row-{{ .ID }}"><td>{{ .Name }}</td></tr>
{{ end -}}
{{ if .More -}}
<tr id="more"><td>
  <button hx-get="/items?page={{ .Next }}" hx-target="#more" hx-swap="outerHTML" hx-vals="{{ toJSON .Vals }}">Load more</button>
</td></tr>
{{- end }}
```

### Data

```json
{
  "Items": [
    {
      "ID": 11,
      "Name": "alpha"
    },
    {
      "ID": 12,
      "Name": "beta & co <b>"
    }
  ],
  "More": true,
  "Next": 3,
  "Vals": {
    "q": "go templates",
    "sort": "name"
  }
}
```

### Output

```
<tr id="row-11"><td>alpha</td></tr>
<tr id="row-12"><td>beta &amp; co &lt;b&gt;</td></tr>
<tr id="more"><td>
  <button hx-get="/items?page=3" hx-target="#more" hx-swap="outerHTML" hx-vals="{&#34;q&#34;:&#34;go templates&#34;,&#34;sort&#34;:&#34;name&#34;}">Load more</button>
</td></tr>
```

### Notes

The fragment has no `<html>` wrapper; the same template can render the whole page by being included from a layout (F51). `hx-target="#more"` with `hx-swap="outerHTML"` replaces the button row with whatever comes back, including a fresh button. `hx-vals` takes JSON: `toJSON` produces it and html/template escapes the quotes as `&#34;` for the attribute; the browser decodes them before htmx reads the value. Trap: html/template treats attributes named `on*` as JavaScript, but htmx's `hx-on:click` does not start with `on`, so it is treated as plain text and **not** JS-escaped. Never interpolate data into `hx-on:*`, and keep `hx-vals='js:...'` out of templates.

<a id="f37"></a>

## F37. A template that emits a template

**Problem.** Generating a `.tmpl` file means every `{{` you want in the output is an action to the generator.

Engine: `text/template`. Delimiters: `[[` `]]`.

### Template

```gotemplate
{{/* generated from [[ .Title ]] */ -}}
{{ define "[[ .Title ]]" -}}
[[ range .Fields -]]
[[ . ]]: {{ .[[ . ]] }}
[[ end -]]
{{- end }}
```

### Data

```json
{
  "Fields": [
    "Name",
    "Email"
  ],
  "Title": "Contact"
}
```

### Glue

Go beyond the FuncMap. This is the code that ran, copied from `formulas/glue.go`.

```go
// glueMeta runs the outer template with its own delimiters, then parses and
// executes what it produced, as proof that the output is a working template.
func glueMeta(f Formula) (string, error) {
	fm := funcs.New(Options())
	outer, err := template.New("outer").Delims(f.Delims[0], f.Delims[1]).Funcs(fm).Parse(f.Template)
	if err != nil {
		return "", err
	}
	var gen strings.Builder
	if err := outer.Execute(&gen, f.Data); err != nil {
		return "", err
	}
	inner, err := template.New("inner").Funcs(fm).Parse(gen.String())
	if err != nil {
		return "", fmt.Errorf("generated template does not parse: %w", err)
	}
	var run strings.Builder
	if err := inner.ExecuteTemplate(&run, "Contact", M{"Name": "rob", "Email": "rob@example.org"}); err != nil {
		return "", err
	}
	return gen.String() + "-- executed with Name=rob Email=rob@example.org --\n" + run.String() + "\n", nil
}
```

### Output

```
{{/* generated from Contact */ -}}
{{ define "Contact" -}}
Name: {{ .Name }}
Email: {{ .Email }}
{{- end }}
-- executed with Name=rob Email=rob@example.org --
Name: rob
Email: rob@example.org
```

### Notes

Give the *outer* template different delimiters (`[[ ]]`) and the inner `{{ }}` are plain text, so the output is a template, verbatim. The glue then parses the generated text with default delimiters and executes it, which is the only proof that the generator emits something valid. Trim markers survive: the `-}}` in the output belongs to the inner template. If you cannot change delimiters, write `{{"{{"}}` for each literal brace (F54).

<a id="f38"></a>

## F38. CLI usage text from flag.FlagSet

**Problem.** Hand-written `-h` text drifts away from the flags the program defines.

Engine: `text/template`. Delimiters: `{{` `}}`.

### Template

```gotemplate
usage: {{ .Prog }} [flags]
{{ range .Flags }}  -{{ .Name }}{{ if .Arg }} {{ .Arg }}{{ end }}	{{ .Usage }}{{ if and .Def (ne .Def "false") (ne .Def "0") }} (default {{ .Def }}){{ end }}
{{ end -}}
```

### Data

```json
{
  "Prog": "cookbook",
  "Flags": [
    {
      "Name": "a",
      "Kind": "string",
      "Def": ":8080",
      "Usage": "listen on `addr`"
    },
    {
      "Name": "c",
      "Kind": "string",
      "Def": "",
      "Usage": "read configuration from `file`"
    },
    {
      "Name": "n",
      "Kind": "int",
      "Def": "3",
      "Usage": "retry `count` for failed requests"
    },
    {
      "Name": "v",
      "Kind": "bool",
      "Def": "false",
      "Usage": "print every formula as it runs"
    }
  ]
}
```

### Glue

Go beyond the FuncMap. This is the code that ran, copied from `formulas/glue.go`.

```go
// flagSpec describes one flag; flagSet is a program's worth of them.
type flagSpec struct{ Name, Kind, Def, Usage string }

type flagSet struct {
	Prog  string
	Flags []flagSpec
}

// flagDoc is the flag set whose usage F38 prints.
var flagDoc = flagSet{Prog: "cookbook", Flags: []flagSpec{
	{"a", "string", ":8080", "listen on `addr`"},
	{"c", "string", "", "read configuration from `file`"},
	{"n", "int", "3", "retry `count` for failed requests"},
	{"v", "bool", "false", "print every formula as it runs"},
}}

// glueFlags defines the flags on a real flag.FlagSet and renders the usage
// from what VisitAll reports, not from the spec.
func glueFlags(f Formula) (string, error) {
	doc, ok := f.Data.(flagSet)
	if !ok {
		return "", fmt.Errorf("data is %T, want flagSet", f.Data)
	}
	set := flag.NewFlagSet(doc.Prog, flag.ContinueOnError)
	set.SetOutput(io.Discard)
	for _, s := range doc.Flags {
		switch s.Kind {
		case "string":
			set.String(s.Name, s.Def, s.Usage)
		case "bool":
			set.Bool(s.Name, s.Def == "true", s.Usage)
		case "int":
			n, err := strconv.Atoi(s.Def)
			if err != nil {
				return "", err
			}
			set.Int(s.Name, n, s.Usage)
		default:
			return "", fmt.Errorf("flag %s: unknown kind %q", s.Name, s.Kind)
		}
	}
	var rows []M
	set.VisitAll(func(fl *flag.Flag) {
		arg, usage := flag.UnquoteUsage(fl)
		rows = append(rows, M{"Name": fl.Name, "Arg": arg, "Usage": usage, "Def": fl.DefValue})
	})
	t, err := template.New("usage").Parse(f.Template)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	if err := t.Execute(&b, M{"Prog": doc.Prog, "Flags": rows}); err != nil {
		return "", err
	}
	return b.String(), nil
}
```

### Output

```
usage: cookbook [flags]
  -a addr   listen on addr (default :8080)
  -c file   read configuration from file
  -n count  retry count for failed requests (default 3)
  -v        print every formula as it runs
```

### Notes

`FlagSet.VisitAll` walks the flags in lexical order, and `flag.UnquoteUsage` extracts the placeholder name from a back-quoted word in the usage string (`listen on `addr``) and the type name otherwise. The template is the layout; the FlagSet is the single source of truth. The text goes through tabwriter (F21) for the column. Flags are single-dash, Plan 9 style, as the repo conventions require.

<a id="f39"></a>

## F39. Prometheus text exposition

**Problem.** The exposition format has its own escaping for HELP text and label values, and a wrong backslash makes the scraper drop the whole target.

Engine: `text/template`. Delimiters: `{{` `}}`.

### Template

```gotemplate
{{- define "esc" }}{{ . | replace "\\" "\\\\" | replace "\"" "\\\"" | replace "\n" "\\n" }}{{ end -}}
{{- define "labels" }}{{ if . }}{{ $first := true }}{ {{- range $k, $v := . }}{{ if not $first }},{{ end }}{{ $first = false }}{{ $k }}="{{ template "esc" $v }}"{{ end -}} }{{ end }}{{ end -}}
{{ range .Metrics -}}
# HELP {{ .Name }} {{ .Help | replace "\\" "\\\\" | replace "\n" "\\n" }}
# TYPE {{ .Name }} {{ .Type }}
{{ $name := .Name -}}
{{ range .Samples -}}
{{ $name }}{{ template "labels" .Labels }} {{ .Value }}
{{ end -}}
{{ end -}}
```

### Data

```json
{
  "Metrics": [
    {
      "Help": "Total HTTP requests.",
      "Name": "http_requests_total",
      "Samples": [
        {
          "Labels": {
            "code": "200",
            "method": "get"
          },
          "Value": 1027
        },
        {
          "Labels": {
            "code": "500",
            "method": "post"
          },
          "Value": 3
        }
      ],
      "Type": "counter"
    },
    {
      "Help": "Line one\nLine two with a \\ backslash.",
      "Name": "room_temperature_celsius",
      "Samples": [
        {
          "Labels": {
            "room": "a \"big\" \\ room"
          },
          "Value": 21.5
        },
        {
          "Value": 0.5
        }
      ],
      "Type": "gauge"
    }
  ]
}
```

### Output

```
# HELP http_requests_total Total HTTP requests.
# TYPE http_requests_total counter
http_requests_total{code="200",method="get"} 1027
http_requests_total{code="500",method="post"} 3
# HELP room_temperature_celsius Line one\nLine two with a \\ backslash.
# TYPE room_temperature_celsius gauge
room_temperature_celsius{room="a \"big\" \\ room"} 21.5
room_temperature_celsius 0.5
```

### Notes

Label values escape `\`, `"` and newline (in that order: backslash first, or you double the ones you just added); HELP text escapes only `\` and newline. Label keys come out of `range` already sorted, which makes the exposition stable. A sample without labels prints no braces. Each sample line must end with a newline, including the last. In a real exporter prefer a client library; this is for static files (node-exporter's textfile collector) and tests. Beware `{` directly followed by `{{`: the lexer starts the action at the first pair, so the `labels` partial writes `{ {{-` and lets the trim marker remove the space.

<a id="f40"></a>

## F40. CoreDNS template plugin answer line

**Problem.** The CoreDNS `template` plugin evaluates its own Go template inside the Corefile (`{{ .Name }}`), which collides with the generator's.

Engine: `text/template`. Delimiters: `[[` `]]`.

### Template

```gotemplate
[[ .Zone ]]:53 {
    template IN A {
        match ^(?P<host>[a-z0-9-]+)\.[[ replace "." "\\." .Zone ]]\.$
        answer "{{ .Name }} [[ .TTL ]] IN A [[ .IP ]]"
        fallthrough
    }
}
```

### Data

```json
{
  "IP": "192.0.2.10",
  "TTL": 60,
  "Zone": "example.org"
}
```

### Output

```
example.org:53 {
    template IN A {
        match ^(?P<host>[a-z0-9-]+)\.example\.org\.$
        answer "{{ .Name }} 60 IN A 192.0.2.10"
        fallthrough
    }
}
```

### Notes

The generator uses `[[ ]]`, so `{{ .Name }}` reaches the Corefile untouched, where CoreDNS evaluates it per query (`.Name` is the query name with its trailing dot). The zone is regex-escaped with `replace "." "\\."` before it goes into `match`; an unescaped `.` matches any character. The `answer` line is a complete zone-file RR: name, TTL, class, type, rdata.

<a id="f41"></a>

## F41. Caddy browse-style directory listing

**Problem.** A static-file server lists a directory with sizes and dates; the data comes from `ReadDir` and the human-readable size needs a method.

Engine: `text/template`. Delimiters: `{{` `}}`.

### Template

```gotemplate
Index of {{ .Path }}
../	-	-
{{ range .Items }}{{ .Name }}{{ if .IsDir }}/{{ end }}	{{ if .IsDir }}-{{ else }}{{ .HumanSize }}{{ end }}	{{ .ModTime.Format "2006-01-02 15:04" }}
{{ end -}}
```

### Data

```json
{
  "Items": [
    {
      "Name": "docs",
      "IsDir": true,
      "Size": 0,
      "ModTime": "2024-02-01T09:00:00Z"
    },
    {
      "Name": "README.md",
      "IsDir": false,
      "Size": 1234,
      "ModTime": "2024-03-05T14:30:00Z"
    },
    {
      "Name": "cookbook.tar.gz",
      "IsDir": false,
      "Size": 5300000,
      "ModTime": "2024-03-04T08:15:00Z"
    }
  ],
  "Path": "/pub/"
}
```

### Glue

Go beyond the FuncMap. This is the code that ran, copied from `formulas/glue.go`.

```go
// Item is one directory entry, shaped like the data Caddy's browse feeds its
// template. HumanSize has a value receiver so templates can call it on the
// elements of a []Item.
type Item struct {
	Name    string
	IsDir   bool
	Size    int64
	ModTime time.Time
}

// HumanSize formats Size with binary prefixes: 1234 is "1.2 KiB".
func (i Item) HumanSize() string {
	const unit = 1024
	if i.Size < unit {
		return fmt.Sprintf("%d B", i.Size)
	}
	div, exp := int64(unit), 0
	for n := i.Size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(i.Size)/float64(div), "KMGTPE"[exp])
}

// ppTabwriter aligns tab-separated cells into columns two spaces apart.
// Only cells that end in a tab take part; the last cell of a line is free.
func ppTabwriter(s string) (string, error) {
	var b strings.Builder
	w := tabwriter.NewWriter(&b, 0, 8, 2, ' ', 0)
	if _, err := io.WriteString(w, s); err != nil {
		return "", err
	}
	if err := w.Flush(); err != nil {
		return "", err
	}
	return b.String(), nil
}
```

### Output

```
Index of /pub/
../              -        -
docs/            -        2024-02-01 09:00
README.md        1.2 KiB  2024-03-05 14:30
cookbook.tar.gz  5.1 MiB  2024-03-04 08:15
```

### Notes

Caddy's `file_server browse` feeds an `html/template` the same shape of data (`.Path`, `.Items` with `.Name`, `.IsDir`, `.Size`, `.HumanSize`, `.ModTime`); `file_server browse <template-file>` swaps in your own. Here the type has a `HumanSize` method, so the template calls it like a field. Methods on the *value* work for values in a slice; a method with a pointer receiver is invisible to `range` over `[]Item` because the elements are not addressable. Dates come out in UTC so the listing is reproducible. The Caddyfile that serves a directory is two lines: `root * /srv/pub` and `file_server browse`.

<a id="f42"></a>

## F42. Chart option JSON into a script block (html/template)

**Problem.** A charting library wants a JavaScript object literal; string-pasting JSON into a script block is both an escaping bug and an XSS hole (`</script>`).

Engine: `html/template`. Delimiters: `{{` `}}`.

### Template

```gotemplate
<div id="chart" style="width:600px;height:400px"></div>
<script>
  var viaMap = {{ .Option }};
  var viaJS  = {{ .Raw }};
  var viaStr = {{ .Str }};
  echarts.init(document.getElementById("chart")).setOption(viaMap);
</script>
```

### Data

```json
{
  "Option": {
    "series": [
      {
        "data": [
          5,
          9,
          7
        ],
        "type": "bar"
      }
    ],
    "title": {
      "text": "</script><b>Requests"
    },
    "xAxis": {
      "data": [
        "mon",
        "tue",
        "wed"
      ],
      "type": "category"
    },
    "yAxis": {}
  },
  "Raw": "{\"title\":{\"text\":\"pre-marshalled\"}}",
  "Str": "{\"title\":{\"text\":\"a string\"}}"
}
```

### Output

```
<div id="chart" style="width:600px;height:400px"></div>
<script>
  var viaMap = {"series":[{"data":[5,9,7],"type":"bar"}],"title":{"text":"\u003c/script\u003e\u003cb\u003eRequests"},"xAxis":{"data":["mon","tue","wed"],"type":"category"},"yAxis":{}};
  var viaJS  = {"title":{"text":"pre-marshalled"}};
  var viaStr = "{\"title\":{\"text\":\"a string\"}}";
  echarts.init(document.getElementById("chart")).setOption(viaMap);
</script>
```

### Notes

In a JavaScript context html/template **marshals non-string values to JSON itself**, escaping `<`, `>` and `&` as `\u003c` etc., so `viaMap` is safe even with `</script>` in the title: pass the Go map or struct and do nothing else. `template.JS` is the escape hatch for JSON you produced and trust (go-echarts' `JSONNotEscaped()` output fits): it is emitted verbatim, so it is exactly as safe as its source. A plain `string` is the trap: it becomes a JS *string literal* (`viaStr`), and the chart gets text where it wanted an object. No go-echarts dependency is needed for the pattern.

<a id="f43"></a>

## F43. range over an integer (Go 1.22+)

**Problem.** Repeating something N times used to need a helper that builds a slice of N elements.

Engine: `text/template`. Delimiters: `{{` `}}`.

### Template

```gotemplate
{{ range 3 }}tick {{ . }}
{{ end -}}
{{ range $i := .N }}[{{ add $i 1 }}]{{ end }}
```

### Data

```json
{
  "N": 4
}
```

### Output

```
tick 0
tick 1
tick 2
[1][2][3][4]
```

### Notes

`{{ range 3 }}` iterates 0, 1, 2; the count can be a constant or any integer value. `{{ range $i := .N }}` names the index. Only **one** variable is allowed over an integer: `{{ range $i, $x := 3 }}` is an error ('can't use 3 to iterate over more than one variable'). `add $i 1` gives the 1-based count. A negative or zero count runs the `else` branch if there is one. For ranges with a start or step, use `seq` (F09).

<a id="f44"></a>

## F44. range over iter.Seq and iter.Seq2 (Go 1.23+)

**Problem.** A lazily produced sequence (a database cursor, a directory walk) had to be collected into a slice before a template could loop over it.

Engine: `text/template`. Delimiters: `{{` `}}`.

### Template

```gotemplate
squares: {{ range .Seq }}<{{ . }}>{{ end }}
pairs:   {{ range $k, $v := .Pairs }}{{ $k }}={{ $v }};{{ end }}
none:    {{ range .Empty }}never{{ else }}empty sequence{{ end }}
```

### Data

```json
{
  "Seq": [
    1,
    4,
    9
  ],
  "Pairs": [
    [
      "a",
      1
    ],
    [
      "b",
      2
    ]
  ],
  "Empty": []
}
```

### Glue

Go beyond the FuncMap. This is the code that ran, copied from `formulas/glue.go`.

```go
// SeqData carries three iterators. A template ranges over them directly.
type SeqData struct {
	Seq   iter.Seq[int]
	Pairs iter.Seq2[string, int]
	Empty iter.Seq[int]
}

// MarshalJSON runs the iterators so the document can show what they yield;
// encoding/json cannot encode a func.
func (d SeqData) MarshalJSON() ([]byte, error) {
	out := struct {
		Seq   []int
		Pairs [][2]any
		Empty []int
	}{Seq: []int{}, Pairs: [][2]any{}, Empty: []int{}}
	for v := range d.Seq {
		out.Seq = append(out.Seq, v)
	}
	for k, v := range d.Pairs {
		out.Pairs = append(out.Pairs, [2]any{k, v})
	}
	for v := range d.Empty {
		out.Empty = append(out.Empty, v)
	}
	return json.Marshal(out)
}

// newSeqData returns the squares 1, 4, 9, the pairs a=1 b=2, and nothing.
func newSeqData() SeqData {
	return SeqData{
		Seq: func(yield func(int) bool) {
			for i := 1; i <= 3; i++ {
				if !yield(i * i) {
					return
				}
			}
		},
		Pairs: func(yield func(string, int) bool) {
			for _, p := range []struct {
				k string
				v int
			}{{"a", 1}, {"b", 2}} {
				if !yield(p.k, p.v) {
					return
				}
			}
		},
		Empty: func(yield func(int) bool) {},
	}
}
```

### Output

```
squares: <1><4><9>
pairs:   a=1;b=2;
none:    empty sequence
```

### Notes

A `func(yield func(V) bool)` (an `iter.Seq[V]`) ranges with one variable; an `iter.Seq2[K, V]` with two. The template stops the producer early when it hits `{{ break }}`, because `yield` returns false. The data above is a struct of three sequences; since a function cannot be printed as JSON, `MarshalJSON` runs them to show what they yield. The sequence is single-shot in spirit: ranging it twice runs the producer twice.

<a id="f45"></a>

## F45. break and continue

**Problem.** Filtering a loop with a surrounding `if` forces the whole body inside the condition and cannot stop early.

Engine: `text/template`. Delimiters: `{{` `}}`.

### Template

```gotemplate
{{ range .Nums }}{{ if eq . 3 }}{{ continue }}{{ end }}{{ if gt . 6 }}{{ break }}{{ end }}{{ . }} {{ end }}
```

### Data

```json
{
  "Nums": [
    1,
    2,
    3,
    4,
    5,
    6,
    7,
    8,
    9
  ]
}
```

### Output

```
1 2 4 5 6 
```

### Notes

`{{ continue }}` skips to the next element; `{{ break }}` leaves the innermost `range`. Both are only valid inside `range` and are parse errors elsewhere. They act on the innermost loop: to leave two, set a flag variable (F50) and test it in the outer loop. Output is not rolled back: text written before `break` stays.

<a id="f46"></a>

## F46. Whitespace control with {{- and -}}

**Problem.** Every action sits on its own line in the source, and every one of those lines puts a newline and indent into the output.

Engine: `text/template`. Delimiters: `{{` `}}`.

### Template

```gotemplate
{{ range .Items }}
  {{ . }}
{{ end }}
```

### Data

```json
{
  "Items": [
    "a",
    "b",
    "c"
  ]
}
```

### Glue

Go beyond the FuncMap. This is the code that ran, copied from `formulas/glue.go`.

```go
// wsVariant is one spelling of the same loop.
type wsVariant struct{ label, src string }

// wsVariants are the trimmed spellings; the untrimmed one is f.Template.
var wsVariants = []wsVariant{
	{"left-trim", "{{- range .Items }}\n  {{ . }}\n{{- end }}"},
	{"both-trim", "{{- range .Items -}}\n  {{ . }}\n{{- end -}}"},
	{"idiom", "{{ range .Items -}}\n  {{ . }}\n{{ end -}}"},
}

// glueWS renders every spelling and prints the output with %q.
func glueWS(f Formula) (string, error) {
	var b strings.Builder
	for _, v := range append([]wsVariant{{"untrimmed", f.Template}}, wsVariants...) {
		t, err := template.New(v.label).Parse(v.src)
		if err != nil {
			return "", err
		}
		var out strings.Builder
		if err := t.Execute(&out, f.Data); err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "== %s\n%s\n=> %q\n", v.label, v.src, out.String())
	}
	return b.String(), nil
}
```

### Output

```
== untrimmed
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
```

### Notes

`{{-` removes all white space (spaces, tabs, newlines) **before** the action; `-}}` removes it **after**. There must be a space between the dash and the action's content: `{{- 3}}` trims, `{{-3}}` is the number -3. The output is printed with `%q` so every byte is visible. The idiom in the last variant, `{{ range ... -}}` plus `{{ end -}}`, keeps one item per line with no leading or trailing blank line, and is the shape to reach for first. Trimming is applied to the *template text* at parse time, not to the output, so it never removes white space produced by data.

<a id="f47"></a>

## F47. missingkey: default, zero and error

**Problem.** A misspelt key renders as `<no value>` in production and nobody notices.

Engine: `text/template`. Delimiters: `{{` `}}`.

### Template

```gotemplate
A={{ .A }} B={{ .B }} C={{ default "fallback" .C }}
```

### Data

```json
{
  "A": 1
}
```

### Glue

Go beyond the FuncMap. This is the code that ran, copied from `formulas/glue.go`.

```go
// glueMissing runs one template under every missingkey setting, against a
// map[string]any, a map[string]int and a struct.
func glueMissing(f Formula) (string, error) {
	var b strings.Builder
	try := func(opt string, label string, data any) error {
		t, err := template.New("t").Option(opt).Funcs(funcs.New(Options())).Parse(f.Template)
		if err != nil {
			return err
		}
		var out strings.Builder
		if err := t.Execute(&out, data); err != nil {
			fmt.Fprintf(&b, "%-22s %-17s error: %v\n", opt, label, err)
			return nil
		}
		fmt.Fprintf(&b, "%-22s %-17s %s\n", opt, label, out.String())
		return nil
	}
	for _, opt := range []string{"missingkey=default", "missingkey=zero", "missingkey=error"} {
		for _, d := range []struct {
			label string
			data  any
		}{
			{"map[string]any", f.Data},
			{"map[string]int", map[string]int{"A": 1}},
		} {
			if err := try(opt, d.label, d.data); err != nil {
				return "", err
			}
		}
	}
	if err := try("missingkey=default", "struct", struct{ A int }{1}); err != nil {
		return "", err
	}
	return b.String(), nil
}
```

### Output

```
missingkey=default     map[string]any    A=1 B=<no value> C=fallback
missingkey=default     map[string]int    A=1 B=<no value> C=fallback
missingkey=zero        map[string]any    A=1 B=<no value> C=fallback
missingkey=zero        map[string]int    A=1 B=0 C=fallback
missingkey=error       map[string]any    error: template: t:1:16: executing "t" at <.B>: map has no entry for key "B"
missingkey=error       map[string]int    error: template: t:1:16: executing "t" at <.B>: map has no entry for key "B"
missingkey=default     struct            error: template: t:1:16: executing "t" at <.B>: can't evaluate field B in type struct { A int }
```

### Notes

`Option("missingkey=...")` decides what a map lookup of an absent key does. `default` (and `invalid`) prints `<no value>`; `zero` yields the element type's zero value, which is `0` for `map[string]int` but still `<no value>` for `map[string]any`, whose zero value is nil; `error` stops execution. Use `error` in anything that writes files. The option applies to **maps only**: a missing field on a struct is always an error, as the last line shows. Note that `missingkey=error` also makes the `default` function useless for absent keys; use `{{ with }}` or `index` there.

<a id="f48"></a>

## F48. html/template contextual escaping

**Problem.** The same string needs different escaping in text, an attribute, a URL, a script and a style, and getting one wrong is XSS.

Engine: `html/template`. Delimiters: `{{` `}}`.

### Template

```gotemplate
<p>text: {{ .X }}</p>
<p title="{{ .X }}">quoted attribute</p>
<p title={{ .X }}>unquoted attribute</p>
<a href="/q?s={{ .X }}">URL query</a>
<a href="{{ .Bad }}">URL scheme</a>
<script>var x = {{ .X }};</script>
<script>var s = "{{ .X }}";</script>
<div style="font-family: {{ .Font }}">CSS</div>
<button onclick="go({{ .X }})">JS in attribute</button>
```

### Data

```json
{
  "Bad": "javascript:alert(1)",
  "Font": "x;}body{background:url(//evil)",
  "X": "O'Reilly & <b>\"Sons\"</b>"
}
```

### Output

```
<p>text: O&#39;Reilly &amp; &lt;b&gt;&#34;Sons&#34;&lt;/b&gt;</p>
<p title="O&#39;Reilly &amp; &lt;b&gt;&#34;Sons&#34;&lt;/b&gt;">quoted attribute</p>
<p title=O&#39;Reilly&#32;&amp;&#32;&lt;b&gt;&#34;Sons&#34;&lt;/b&gt;>unquoted attribute</p>
<a href="/q?s=O%27Reilly%20%26%20%3cb%3e%22Sons%22%3c%2fb%3e">URL query</a>
<a href="#ZgotmplZ">URL scheme</a>
<script>var x = "O'Reilly \u0026 \u003cb\u003e\"Sons\"\u003c/b\u003e";</script>
<script>var s = "O\u0027Reilly \u0026 \u003cb\u003e\u0022Sons\u0022\u003c\/b\u003e";</script>
<div style="font-family: ZgotmplZ">CSS</div>
<button onclick="go(&#34;O&#39;Reilly \u0026 \u003cb\u003e\&#34;Sons\&#34;\u003c/b\u003e&#34;)">JS in attribute</button>
```

### Notes

html/template parses the template as HTML and picks an escaper from the **context** of each action: HTML text, quoted or unquoted attribute, URL component, JavaScript value or string, CSS value. The one `{{ .X }}` renders differently in each of the nine contexts above. `ZgotmplZ` is the sentinel it writes when a value cannot be made safe in its context: a `javascript:` URL in `href`, or CSS with unbalanced braces. Seeing it means 'you tried to inject something', not 'a bug in the library'. The cost: the template must be well-formed HTML, and pre-escaping a string yourself double-escapes it. Do not call `html` or `urlquery` inside an html/template.

<a id="f49"></a>

## F49. Typed bypass: template.HTML, URL, JS and HTMLAttr

**Problem.** Sometimes the data really is trusted markup (rendered Markdown, a vetted link) and escaping it ruins it.

Engine: `html/template`. Delimiters: `{{` `}}`.

### Template

```gotemplate
<p>{{ .Plain }}</p>
<p>{{ .Safe }}</p>
<a href="{{ .BadLink }}">string</a>
<a href="{{ .Link }}">template.URL</a>
<a {{ .Attr }}>template.HTMLAttr</a>
<script>{{ .Code }}</script>
```

### Data

```json
{
  "Attr": "onclick=\"x()\"",
  "BadLink": "javascript:alert(1)",
  "Code": "alert(1)",
  "Link": "javascript:alert(1)",
  "Plain": "<b>plain</b>",
  "Safe": "<b>safe</b>"
}
```

### Output

```
<p>&lt;b&gt;plain&lt;/b&gt;</p>
<p><b>safe</b></p>
<a href="#ZgotmplZ">string</a>
<a href="javascript:alert%281%29">template.URL</a>
<a onclick="x()">template.HTMLAttr</a>
<script>alert(1)</script>
```

### Notes

A value whose Go type is `template.HTML`, `HTMLAttr`, `JS`, `JSStr`, `CSS` or `URL` is a promise: 'I vetted this'. html/template believes it and writes it verbatim. **Warning:** the type conversion is the whole security review. `template.HTML(userInput)` is XSS with extra steps; `template.URL("javascript:...")` defeats the scheme filter, as the output shows. Convert only constants, or output of a sanitiser you trust (a Markdown renderer with HTML disabled, `bluemonday`-style policy). Grep for these conversions in code review.

<a id="f50"></a>

## F50. Variable reassignment: the accumulator

**Problem.** A running total or maximum seems to need state, and `:=` inside the loop quietly makes a new variable that dies at `end`.

Engine: `text/template`. Delimiters: `{{` `}}`.

### Template

```gotemplate
{{ $sum := 0 }}{{ $max := 0 }}{{ range .Nums }}{{ $sum = add $sum . }}{{ if gt . $max }}{{ $max = . }}{{ end }}{{ end -}}
sum={{ $sum }} max={{ $max }}
{{ $bad := 0 }}{{ range .Nums }}{{ $bad := add $bad . }}{{ end -}}
wrong={{ $bad }}
{{ $s := "" }}{{ range $i, $n := .Nums }}{{ $s = printf "%s%s%d" $s (ternary "" "+" (eq $i 0)) $n }}{{ end -}}
expr={{ $s }}
```

### Data

```json
{
  "Nums": [
    4,
    8,
    15,
    16,
    23,
    42
  ]
}
```

### Output

```
sum=108 max=42
wrong=0
expr=4+8+15+16+23+42
```

### Notes

Since Go 1.11 `{{ $x = ... }}` **assigns** to an existing variable, even from inside `range` or `if`. `{{ $x := ... }}` declares a new one scoped to the enclosing block, so `wrong` stays 0: the loop computed a sum and threw it away. This is the standard way to count, total, find a maximum or build a string, and it is also how F24 measures the widest key. A variable cannot be assigned from a *called template* (`define`): variables do not cross that boundary, so return values must be computed before the call.

<a id="f51"></a>

## F51. Clone: per-page layouts with block override

**Problem.** Parsing every page's `define "title"` into one template set means the last definition wins for all pages.

Engine: `html/template`. Delimiters: `{{` `}}`.

### Template

```gotemplate
<!doctype html>
<title>{{ block "title" . }}Untitled{{ end }}</title>
<main>{{ block "body" . }}(empty){{ end }}</main>
```

### Data

```json
{
  "User": "rob"
}
```

### Glue

Go beyond the FuncMap. This is the code that ran, copied from `formulas/glue.go`.

```go
// pages are the per-page overrides for F51.
var pages = []struct{ name, src string }{
	{"home", `{{ define "title" }}Home{{ end }}{{ define "body" }}Hello, {{ .User }}{{ end }}`},
	{"about", `{{ define "title" }}About{{ end }}`},
}

// glueClone renders each page from its own Clone of the layout, then shows
// what happens when all pages are parsed into one set.
func glueClone(f Formula) (string, error) {
	base, err := htemplate.New("layout").Parse(f.Template)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("## one clone per page\n")
	for _, p := range pages {
		t, err := base.Clone()
		if err != nil {
			return "", err
		}
		if _, err := t.Parse(p.src); err != nil {
			return "", err
		}
		fmt.Fprintf(&b, "-- %s --\n", p.name)
		if err := t.Execute(&b, f.Data); err != nil {
			return "", err
		}
	}
	shared, err := base.Clone()
	if err != nil {
		return "", err
	}
	b.WriteString("## one shared set, home parsed first, then about\n")
	for _, p := range pages {
		if _, err := shared.Parse(p.src); err != nil {
			return "", err
		}
	}
	if err := shared.Execute(&b, f.Data); err != nil {
		return "", err
	}
	return b.String(), nil
}
```

### Output

```
## one clone per page
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
```

### Notes

`{{ block "x" . }}default{{ end }}` defines `x` and invokes it in one step; a page overrides it by defining `x` again. In a single template set a name has one body, so the second `define` replaces the first. The fix is `base.Clone()` for every page: each clone has its own set of definitions, and the base stays pristine. Clone **before** the first Execute; html/template refuses to clone a template that has run. The 'shared' output shows the bug: one set can render only one page, and it is a blend: About's title over Home's body.

<a id="f52"></a>

## F52. ParseFS from embed and ExecuteTemplate by name

**Problem.** Templates in files must ship inside the binary, and a set of files needs to be addressed by name.

Engine: `text/template`. Delimiters: `{{` `}}`.

### Template

```gotemplate
-- page.tmpl --
Order for {{ .Customer }}
{{ range .Items }}{{ template "item.tmpl" . }}{{ end -}}
{{ template "footer" . }}

-- item.tmpl --
* {{ .Name }} x{{ .Qty }}

-- footer.tmpl --
{{ define "footer" }}-- {{ len .Items }} items --{{ end }}
```

### Data

```json
{
  "Customer": "Ada",
  "Items": [
    {
      "Name": "engine",
      "Qty": 1
    },
    {
      "Name": "cards",
      "Qty": 3
    }
  ]
}
```

### Glue

Go beyond the FuncMap. This is the code that ran, copied from `formulas/glue.go`.

```go
// glueFS parses every *.tmpl file in the embedded directory into one set and
// executes one of them by name.
func glueFS(f Formula) (string, error) {
	t, err := template.New("").ParseFS(tmplFS, "testdata/tmpl/*.tmpl")
	if err != nil {
		return "", err
	}
	var names []string
	for _, x := range t.Templates() {
		if x.Name() != "" {
			names = append(names, x.Name())
		}
	}
	sort.Strings(names) // Templates() is in map order
	var b strings.Builder
	fmt.Fprintf(&b, "templates: %s\n", strings.Join(names, " "))
	if err := t.ExecuteTemplate(&b, "page.tmpl", f.Data); err != nil {
		return "", err
	}
	return b.String(), nil
}
```

### Output

```
templates: footer footer.tmpl item.tmpl page.tmpl
Order for Ada
* engine x1
* cards x3
-- 2 items --
```

### Notes

`//go:embed testdata/tmpl` puts the directory in the binary; `ParseFS(fsys, "testdata/tmpl/*.tmpl")` parses each match and names it by its **base name**, so `ExecuteTemplate(w, "page.tmpl", data)` selects it. A file may also `define` further names (`footer`), which become addressable as well. The pattern is a `path.Match` glob, with no `**`. The Template shown is the three embedded files in order. The set of names is sorted before printing because `Templates()` returns them in map order.

<a id="f53"></a>

## F53. Error handling: ExecError and errors.As

**Problem.** A template failure is a returned error, but a log line saying 'template: t:1:7: executing' hides what failed and where.

Engine: `text/template`. Delimiters: `{{` `}}`.

### Template

```gotemplate
{{ .user.name }}
```

### Data

```json
{
  "user": null
}
```

### Glue

Go beyond the FuncMap. This is the code that ran, copied from `formulas/glue.go`.

```go
// glueErr triggers four kinds of failure and reports what errors.As and
// errors.Is can recover from each.
func glueErr(f Formula) (string, error) {
	fm := funcs.New(Options())
	var b strings.Builder
	try := func(label, src string, data any, w io.Writer) {
		t, err := template.New("t").Funcs(fm).Parse(src)
		if err != nil {
			fmt.Fprintf(&b, "%-6s parse error: %v\n", label, err)
			return
		}
		err = t.Execute(w, data)
		var ee template.ExecError
		switch {
		case errors.As(err, &ee):
			fmt.Fprintf(&b, "%-6s ExecError in %q: %v\n", label, ee.Name, ee.Err)
			if errors.Is(err, funcs.ErrArgs) {
				fmt.Fprintf(&b, "%-6s errors.Is(err, funcs.ErrArgs) = true\n", "")
			}
		case err != nil:
			fmt.Fprintf(&b, "%-6s not an ExecError: %v\n", label, err)
		default:
			fmt.Fprintf(&b, "%-6s ok\n", label)
		}
	}
	try("parse", `{{ .user.name `, nil, io.Discard)
	try("nil", f.Template, f.Data, io.Discard)
	try("args", `{{ dict "a" }}`, nil, io.Discard)
	try("write", `hello`, nil, failWriter{})
	return b.String(), nil
}
```

### Output

```
parse  parse error: template: t:1: unclosed action
nil    ExecError in "t": template: t:1:8: executing "t" at <.user.name>: nil pointer evaluating interface {}.name
args   ExecError in "t": template: t:1:3: executing "t" at <dict "a">: error calling dict: dict: bad arguments: odd number of arguments (1)
       errors.Is(err, funcs.ErrArgs) = true
write  not an ExecError: disk full
```

### Notes

Parse errors come back from `Parse`. Errors raised while running are wrapped in `template.ExecError`, which carries the template `Name` and the underlying `Err`; extract it with `errors.As`. A function's own error is wrapped further, so `errors.Is(err, funcs.ErrArgs)` still sees through to the cause. A failure to *write* is not an ExecError: it is the writer's own error, returned as is. Execution stops at the first error and the partial output has already been written, so render into a buffer and discard it on failure.

<a id="f54"></a>

## F54. Generating a gh api --template string

**Problem.** `gh api --template` takes a Go template, so the command line you generate contains `{{` that your own template would swallow.

Engine: `text/template`. Delimiters: `{{` `}}`.

### Template

```gotemplate
{{- $l := "{{" }}{{ $r := "}}" -}}
gh api repos/{{ .Org }}/{{ .Repo }}/issues --template '{{"{{"}}range .{{"}}"}}#{{"{{"}}.number{{"}}"}} {{"{{"}}.title{{"}}"}}{{"{{"}}"\n"{{"}}"}}{{"{{"}}end{{"}}"}}'
gh api repos/{{ .Org }}/{{ .Repo }}/issues --template '{{ $l }}range .{{ $r }}#{{ $l }}.number{{ $r }} {{ $l }}.title{{ $r }}{{ $l }}"\n"{{ $r }}{{ $l }}end{{ $r }}'
```

### Data

```json
{
  "Org": "CoolGitOrg",
  "Repo": "go-template-cookbook"
}
```

### Output

```
gh api repos/CoolGitOrg/go-template-cookbook/issues --template '{{range .}}#{{.number}} {{.title}}{{"\n"}}{{end}}'
gh api repos/CoolGitOrg/go-template-cookbook/issues --template '{{range .}}#{{.number}} {{.title}}{{"\n"}}{{end}}'
```

### Notes

A literal `{{` in output is an action that prints the string `"{{"`: write `{{"{{"}}`, and `{{"}}"}}` for the closer (the closing braces are only special inside an action, but keep them symmetric). Line 2 does it inline; line 3 binds the two strings to `$l` and `$r` once, which reads better when there are many. The result is wrapped in single quotes for the shell (use `shq` when the template text is not fixed). The alternative is F25: change the generator's delimiters. Often neither is needed: `gh api ... --jq '.[] | "#\(.number) \(.title)"'` has no braces at all. gh evaluates the template itself and adds helpers such as `join`, `pluck`, `tablerow` and `timeago`.

<a id="f55"></a>

## F55. Terraform templatefile: a lookalike, not a Go template

**Problem.** HCL string templates (`${name}`, `%{ for }`) look like Go templates and are not; mixing them up makes a generator emit broken Terraform.

Engine: `text/template`. Delimiters: `{{` `}}`.

### Template

```gotemplate
resource "local_file" {{ printf "%q" .Name }} {
  filename = {{ printf "%q" (print .Name ".txt") }}
  content  = templatefile("{{ .Name }}.tpl", {
    name  = {{ printf "%q" .Vars.name }}
    ports = [{{ range $i, $p := .Vars.ports }}{{ if $i }}, {{ end }}{{ $p }}{{ end }}]
  })
}
```

### Data

```json
{
  "Name": "motd",
  "Vars": {
    "name": "web",
    "ports": [
      80,
      443
    ]
  }
}
```

### Output

```
resource "local_file" "motd" {
  filename = "motd.txt"
  content  = templatefile("motd.tpl", {
    name  = "web"
    ports = [80, 443]
  })
}
```

### Notes

Terraform's `templatefile(path, vars)` renders **HCL template syntax**, which is not rendered here (Terraform is not installed in this build). The file it would read, `motd.tpl`, looks like this:

```
Hello, ${name}!
%{ for p in ports ~}
listening on ${p}
%{ endfor ~}
```

The differences that matter: interpolation is `${expr}`, directives are `%{ if cond }` / `%{ for x in xs }` / `%{ endfor }` / `%{ endif }`, `~` strips adjacent white space (our `{{-`), expressions are HCL, not Go, and there are no pipelines, no `define`, no `with`. A literal `${` is written `$${`. Go cannot render these files, and Terraform cannot run Go templates: the formula above is the Go side, **generating** the `.tf` that calls `templatefile`. If you must emit `${` from a Go template, see F24.

---

*55 formulas. Generated by `cookbook gen`; verify with `cookbook check -docs ../docs`.*
