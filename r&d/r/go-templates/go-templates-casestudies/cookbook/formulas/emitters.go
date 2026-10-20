package formulas

// node builds one entry of the tree in F20.
func node(name string, kids ...any) M {
	if len(kids) == 0 {
		return M{"Name": name}
	}
	return M{"Name": name, "Kids": L(kids)}
}

// emitters is F14..F31: the formats a template most often has to write
// correctly: YAML, JSON, CSV, HCL, shell, SQL.
func emitters() []Formula {
	return []Formula{
		{
			ID:      "F14",
			Name:    "YAML block with toYAML | nindent",
			Problem: "Splicing a map or list into a YAML file by printing it line by line gets the indentation, the quoting and the empty case wrong.",
			Template: `containers:
  - name: {{ .Name }}
    env:{{ toYAML .Env | nindent 6 }}
    args:{{ toYAML .Args | nindent 6 }}
`,
			Data: M{"Name": "api", "Env": M{"LOG": "debug", "PORT": "8080"}, "Args": []string{"--serve", "--verbose"}},
			Want: `containers:
  - name: api
    env:
      LOG: debug
      PORT: "8080"
    args:
      - --serve
      - --verbose
`,
			Notes: "Let an emitter produce the sub-document and only *place* it: `key:` then `nindent N` puts a newline and indents every line by N, so the block lands under the key. " +
				"`indent` (no newline) is for text that continues the current line. `toYAML` quotes what YAML would misread: `\"8080\"` stays a string, whereas hand-printed `PORT: 8080` becomes an integer and `no`/`on` become booleans. " +
				"It writes no trailing newline, so the template owns line endings. The emitter is the stdlib-only one in `funcs/yaml.go`: sorted keys, no anchors, no flow style.",
		},
		{
			ID:      "F15",
			Name:    "JSON array with correct commas",
			Problem: "A hand-built JSON array leaves a trailing comma, or `%q` produces a string JSON cannot parse.",
			Template: `loop:  [{{ range $i, $t := .Tags }}{{ if $i }},{{ end }}{{ toJSON $t }}{{ end }}]
whole: {{ toJSON .Tags }}
%q:    [{{ range $i, $t := .Tags }}{{ if $i }},{{ end }}{{ printf "%q" $t }}{{ end }}]
`,
			Data: M{"Tags": []string{"a", "b", "esc\x1b"}},
			Want: `loop:  ["a","b","esc\u001b"]
whole: ["a","b","esc\u001b"]
%q:    ["a","b","esc\x1b"]
`,
			Notes: "Line 1 is the comma idiom (`{{ if $i }},{{ end }}` before every item but the first) with each element encoded by `toJSON`. Line 2 is what you should write: encode the whole value. " +
				"Line 3 is the trap: `printf \"%q\"` makes a **Go** literal, and Go escapes ESC as `\\x1b`, which is not valid JSON (`\\u001b` is). It is right for ASCII text, which is why it survives review. " +
				"Never build JSON from fragments if the values are not constants.",
		},
		{
			ID:      "F16",
			Name:    "JSON object from a map (sorted keys)",
			Problem: "Labels have to be emitted as a JSON object, with stable order, correct escaping and commas only between members.",
			Template: `compact: {{ toJSON .Labels }}
pretty:
{{ toPrettyJSON .Labels }}
by hand: {
{{- range $k, $v := .Labels }}{{ if ne $k (index (sortedKeys $.Labels) 0) }},{{ end }} {{ toJSON $k }}: {{ toJSON $v }}{{ end }} }
`,
			Data: M{"Labels": M{"team": "platform", "app": "api", "motd": `say "hi"`}},
			Want: `compact: {"app":"api","motd":"say \"hi\"","team":"platform"}
pretty:
{
  "app": "api",
  "motd": "say \"hi\"",
  "team": "platform"
}
by hand: { "app": "api", "motd": "say \"hi\"", "team": "platform" }
`,
			Notes: "`toJSON` marshals maps with **sorted keys**, so the output is byte-stable across runs, which is what makes generated files diffable. `toPrettyJSON` indents by two spaces. " +
				"The hand-built version shows the problem with maps: `range` over a map gives the key but no index, so the first-member test compares against the first sorted key. " +
				"Prefer the encoders; keep the hand-built form for the rare case where the surrounding text is not JSON.",
		},
		{
			ID:      "F17",
			Name:    "base64 and SHA-256",
			Problem: "A header needs `user:password` in base64 and an ETag needs a digest, and neither should be computed outside the template.",
			Template: `auth:   Basic {{ printf "%s:%s" .User .Pass | b64enc }}
back:   {{ printf "%s:%s" .User .Pass | b64enc | b64dec }}
sha256: {{ sha256sum .Pass }}
etag:   "{{ sha256sum .Body | printf "%.12s" }}"
`,
			Data: M{"User": "rob", "Pass": "hunter2", "Body": "hello, world\n"},
			Want: `auth:   Basic cm9iOmh1bnRlcjI=
back:   rob:hunter2
sha256: f52fbd32b2b3b86ff88ef6c490628285f482af15ddcb29541f94bcf526a3f6c7
etag:   "853ff93762a0"
`,
			Notes: "`b64enc` is standard padded base64; `b64dec` returns an error on bad input rather than garbage. `sha256sum` returns lowercase hex. " +
				"Two warnings: base64 is an encoding, not encryption, and a bare SHA-256 is not a password hash (use bcrypt/scrypt/argon2 in the program that stores passwords). " +
				"And a secret placed into a rendered file is in that file: treat the output as sensitive.",
		},
		{
			ID:      "F18",
			Name:    "env lookup (trusted templates) with a default",
			Problem: "A rendered config should pick up the deployment environment, falling back when a variable is unset.",
			Template: `app:  {{ .App }}
env:  {{ default "dev" (env "APP_ENV") }}
home: {{ env "HOME" }}
log:  {{ default "info" (env "LOG_LEVEL") }}
`,
			Data: M{"App": "cookbook"},
			Want: `app:  cookbook
env:  staging
home: /home/rob
log:  info
`,
			Notes: "`env` returns the empty string for an unset variable, so `default` supplies the fallback; there is no way to tell 'unset' from 'set to empty' with `os.Getenv`. " +
				"**Only trusted templates may call `env`.** Anyone who can write the template can print `AWS_SECRET_ACCESS_KEY`. " +
				"For templates from users, build the FuncMap with `funcs.Options{Env: allow(\"APP_ENV\", \"HOME\")}` or leave `env` out. In these examples the environment is a fixed map, so the output is the same everywhere.",
		},
		{
			ID:      "F19",
			Name:    "Include a file from a rooted filesystem",
			Problem: "Boilerplate headers and message-of-the-day text live in files; a naive include reads whatever path the template names, `../../etc/passwd` included.",
			Template: `# {{ .Title }}
== motd ==
{{ file "motd.txt" | trim }}
== header, indented ==
{{ file "header.txt" | trim | indent 4 }}
`,
			Data:    M{"Title": "Release notes"},
			Run:     glueFile,
			GlueRef: "glueFile",
			Want: `# Release notes
== motd ==
Welcome to the cookbook.
Every formula here is tested.
== header, indented ==
    # Generated file
    # Do not edit
file "../formulas.go": template: probe:1:3: executing "probe" at <file .>: error calling file: file: invalid path "../formulas.go"
file "/etc/passwd": template: probe:1:3: executing "probe" at <file .>: error calling file: file: invalid path "/etc/passwd"
file "missing.txt": template: probe:1:3: executing "probe" at <file .>: error calling file: file: open missing.txt: file does not exist
`,
			Notes: "`file` reads from the `fs.FS` given in `funcs.Options.FS`, and only names that satisfy `fs.ValidPath` are accepted: no leading `/`, no `.` or `..` elements. " +
				"Hand it `os.DirFS(dir)` (or, from Go 1.24, `os.OpenRoot(dir)` and `root.FS()`, which also refuses symlink escapes) and the template can see that tree and nothing else. " +
				"The glue runs the escape attempt and prints the error. With no FS configured the function fails closed.",
		},
		{
			ID:      "F20",
			Name:    "Recursive tree with define and template",
			Problem: "A nested menu or directory tree has unknown depth; `range` inside `range` only reaches a fixed number of levels.",
			Template: `{{ define "node" }}{{ repeat .Depth "  " }}{{ .Node.Name }}{{ if .Node.Kids }}/{{ end }}
{{ range .Node.Kids }}{{ template "node" (dict "Node" . "Depth" (add $.Depth 1)) }}{{ end }}{{ end -}}
{{ template "node" (dict "Node" .Tree "Depth" 0) }}`,
			Data: M{"Tree": node("cookbook",
				node("funcs", node("funcs.go"), node("yaml.go")),
				node("cmd", node("cookbook", node("main.go"))),
				node("go.mod"))},
			Want: `cookbook/
  funcs/
    funcs.go
    yaml.go
  cmd/
    cookbook/
      main.go
  go.mod
`,
			Notes: "A `define`d template can call itself; the recursion ends when `.Node.Kids` is empty, so `range` has nothing to do. " +
				"Two traps. First, `template` passes **one** value, so depth and node travel together in a `dict`. Second, inside the definition `$` is that dict, not the page data, which is why `add $.Depth 1` works. " +
				"A cyclic data structure recurses until the template's execution depth limit stops it with an error; the data must be a tree.",
		},
		{
			ID:      "F21",
			Name:    "Aligned columns with text/tabwriter",
			Problem: "`printf \"%-20s\"` hard-codes a width that is wrong the moment a name is longer, and counts bytes, not runes.",
			Template: "NAME\tSTATUS\tAGE\n" +
				"{{ range .Pods }}{{ .Name }}\t{{ .Status }}\t{{ .Age }}\n{{ end -}}\n",
			Data: M{"Pods": L{
				M{"Name": "api-7d9f", "Status": "Running", "Age": "3d"},
				M{"Name": "db-0", "Status": "CrashLoopBackOff", "Age": "12h"},
				M{"Name": "cache", "Status": "Pending", "Age": "5m"},
			}},
			PostProcess: ppTabwriter,
			GlueRef:     "ppTabwriter",
			Want: `NAME      STATUS            AGE
api-7d9f  Running           3d
db-0      CrashLoopBackOff  12h
cache     Pending           5m
`,
			Notes: "The template emits **tab-separated** cells and knows nothing about widths; a post-process step runs the text through `text/tabwriter`, which measures every column and pads. " +
				"Only cells *terminated* by a tab are aligned, so the last cell of each row (no trailing tab) may be any length. " +
				"This is how `kubectl` and `go` print tables. The same text piped through `column -t` works for ASCII but not for wide runes or embedded spaces in cells.",
		},
		{
			ID:      "F22",
			Name:    "CSV: never join with commas",
			Problem: "A field containing a comma, a quote or a newline breaks a CSV row that was assembled with `{{ .A }},{{ .B }}`.",
			Template: `# naive: template only
{{ range .Rows }}{{ .Name }},{{ .Note }},{{ .Qty }}
{{ end -}}
# safe: encoding/csv
{{ range .Rows }}{{ csvrow .Name .Note .Qty }}{{ end -}}
`,
			Data: M{"Rows": L{
				M{"Name": "widget", "Note": "fancy, shiny", "Qty": 3},
				M{"Name": "gadget", "Note": `say "hi"`, "Qty": 1},
				M{"Name": "gizmo", "Note": "two\nlines", "Qty": 2},
			}},
			Funcs:   map[string]any{"csvrow": csvRow},
			GlueRef: "csvRow",
			Want: `# naive: template only
widget,fancy, shiny,3
gadget,say "hi",1
gizmo,two
lines,2
# safe: encoding/csv
widget,"fancy, shiny",3
gadget,"say ""hi""",1
gizmo,"two
lines",2
`,
			Notes: "RFC 4180 quoting is a state machine: wrap in quotes if the field has a comma, quote or line break, and double any quotes inside. A template can express it, but the version that is correct on the first try is `encoding/csv`. " +
				"The glue adds one function, `csvrow`, that writes a record through `csv.Writer`. The naive block above produces rows with the wrong number of fields, and 'two lines' splits into two records. " +
				"Spreadsheets also execute cells that begin with `=`, `+`, `-` or `@` (CSV injection); prefix such cells with `'` if the file will be opened by people.",
		},
		{
			ID:      "F23",
			Name:    "Markdown table",
			Problem: "A pipe or a newline in a cell silently ends the cell or the row.",
			Template: `| Name | Notes |
|------|-------|
{{ range .Rows }}| {{ .Name }} | {{ .Note | replace "|" "\\|" | replace "\n" "<br>" }} |
{{ end -}}
`,
			Data: M{"Rows": L{
				M{"Name": "pipe", "Note": "a | b"},
				M{"Name": "multi", "Note": "line1\nline2"},
				M{"Name": "plain", "Note": "ok"},
			}},
			Want: `| Name | Notes |
|------|-------|
| pipe | a \| b |
| multi | line1<br>line2 |
| plain | ok |
`,
			Notes: "In a pipe-table cell, `|` must become `\\|` and a newline must become `<br>`. Backslash comes first if you also escape backslashes. " +
				"`\"\\\\|\"` in the template is the two-character string `\\|`: the template lexer processes string escapes like Go. " +
				"This is text/template: Markdown is not HTML, so html/template's escaping would be wrong here (it would turn `&` into `&amp;`).",
		},
		{
			ID:      "F24",
			Name:    "HCL / Terraform emission",
			Problem: "A string containing `${` or `%{` becomes an interpolation in HCL, and a Go-quoted string is not always a valid HCL string.",
			Template: `{{- define "hcl" }}{{ printf "%q" . | replace "${" "$${" | replace "%{" "%%{" }}{{ end -}}
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
`,
			Data: M{
				"Name": "web", "AMI": "ami-0abc",
				"Ports": []int{80, 443},
				"Motd":  "cost is ${total}\n",
				"Tags":  M{"env": "prod", "owner": "platform"},
			},
			Want: `resource "aws_instance" "web" {
  ami   = "ami-0abc"
  ports = [80, 443]
  motd  = "cost is $${total}\n"

  tags = {
    env   = "prod"
    owner = "platform"
  }
}
`,
			Notes: "`printf \"%q\"` gives HCL's escapes for `\\\"`, `\\\\`, `\\n` and `\\t`, but not for every control or non-printable character (HCL knows `\\uNNNN`, Go emits `\\x1b`). Validate or restrict the input if it is not plain text. " +
				"The decisive trap is interpolation: HCL expands `${...}` and `%{...}` inside quoted strings, so user data containing them executes. Escape them as `$${` and `%%{` in one place (the `hcl` partial) and never `print` a value into HCL any other way. " +
				"The `=` signs are aligned the way `terraform fmt` wants them: the longest key is measured first with an accumulator (F50) and `%-*s` pads to it. `terraform` is not installed in the build environment, so this output is checked against that style by eye and by the test, not by `terraform fmt`.",
		},
		{
			ID:      "F25",
			Name:    "GitHub Actions workflow with [[ ]] delimiters",
			Problem: "`${{ secrets.X }}` is a GitHub expression, but the template engine reads `{{ secrets.X }}` as its own action and fails or eats it.",
			Template: `name: [[ .Name ]]
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
`,
			Delims: [2]string{"[[", "]]"},
			Data: M{
				"Name": "ci", "GoVersion": "1.24.x", "Branches": []string{"main"},
				"Targets": L{M{"OS": "linux", "Arch": "amd64"}, M{"OS": "plan9", "Arch": "amd64"}},
			},
			Want: `name: ci
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
`,
			Notes: "Change the *template's* delimiters with `Delims(\"[[\", \"]]\")` and everything else in the file, including `${{ ... }}`, is plain text. That is cleaner than escaping each expression (F54). " +
				"Choose delimiters that do not occur in the target: `[[` collides with YAML flow sequences of sequences (`[[1,2]]`) and with TOML/INI headers, so for those use `<%`/`%>` or `@@`. " +
				"Per the repo conventions, untrusted values (issue titles, inputs) must reach `run:` through `env:` and be read as `\"$VAR\"`, never interpolated into the script.",
		},
		{
			ID:      "F26",
			Name:    "GitHub Issue Form YAML",
			Problem: "An issue-form definition is structured YAML where a colon in a description or a leading `[` in a title changes its meaning.",
			Template: `name: {{ toYAML .Name }}
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
`,
			Data: M{
				"Name": "Bug report", "Desc": "Something is broken: tell us", "Title": "[bug] ",
				"Labels": []string{"bug", "triage"},
				"Fields": L{
					M{"Type": "input", "ID": "version", "Label": "Version", "Required": true},
					M{"Type": "dropdown", "ID": "os", "Label": "Operating system", "Options": []string{"linux", "darwin", "plan9"}, "Required": true},
					M{"Type": "textarea", "ID": "repro", "Label": "Steps to reproduce", "Required": false},
				},
			},
			Want: `name: Bug report
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
`,
			Notes: "Every free-text scalar goes through `toYAML` so it is quoted when it must be: the description contains `: ` and the title starts with `[` and ends with a space; unquoted, both would be parse errors or silent changes. " +
				"Fixed vocabulary (`type`, `id`) is printed bare because the template author controls it. `{{ if .Options }}` makes the key optional, since a missing map key is nil. " +
				"The file belongs in `.github/ISSUE_TEMPLATE/`. Issue forms do not use `{{ }}`, so no delimiter change is needed.",
		},
		{
			ID:      "F27",
			Name:    "Dockerfile with a JSON-array ENTRYPOINT",
			Problem: "Shell-form `ENTRYPOINT server -a :8080` runs under `/bin/sh -c`, swallows signals and breaks on spaces; the exec form must be a JSON array with double quotes.",
			Template: `FROM {{ .Base }}
LABEL org.opencontainers.image.source={{ printf "%q" .Source }}
COPY --chmod=0755 server {{ index .Entrypoint 0 }}
EXPOSE {{ .Port }}
USER nonroot:nonroot
ENTRYPOINT {{ toJSON .Entrypoint }}
`,
			Data: M{
				"Base": "gcr.io/distroless/static:nonroot", "Port": 8080,
				"Source":     "https://github.com/CoolGitOrg/go-template-cookbook",
				"Entrypoint": []string{"/app/server", "-a", ":8080", "-c", "/etc/app.conf"},
			},
			Want: `FROM gcr.io/distroless/static:nonroot
LABEL org.opencontainers.image.source="https://github.com/CoolGitOrg/go-template-cookbook"
COPY --chmod=0755 server /app/server
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/app/server","-a",":8080","-c","/etc/app.conf"]
`,
			Notes: "Dockerfile exec form is parsed as JSON, so `toJSON` is exactly right and handles an argument with a space or a quote for free. `printf \"%q\"` is acceptable for `LABEL` values. " +
				"`toJSON` leaves `<`, `>` and `&` alone (no HTML escaping), which matters: `\\u0026` in an ENTRYPOINT would still parse, but nobody wants to read it. " +
				"With exec form the process is PID 1 and receives SIGTERM directly; with shell form it does not.",
		},
		{
			ID:      "F28",
			Name:    "Makefile with literal tabs",
			Problem: "Recipe lines must start with a TAB; editors, formatters and chat paste turn it into spaces and `make` stops with 'missing separator'.",
			Template: "BIN := {{ .Bin }}\n" +
				".PHONY: {{ join \" \" .Phony }}\n" +
				"\n" +
				"all: build test\n" +
				"{{ range .Targets }}\n" +
				"{{ .Name }}:{{ if .Deps }} {{ .Deps }}{{ end }}\n" +
				"{{- range .Cmds }}\n" +
				"\t{{ . }}\n" +
				"{{- end }}\n" +
				"{{ end -}}\n",
			Data: M{
				"Bin": "cookbook", "Phony": []string{"all", "build", "test", "clean"},
				"Targets": L{
					M{"Name": "build", "Cmds": []string{"go build -o bin/$(BIN) ./cmd/$(BIN)"}},
					M{"Name": "test", "Deps": "build", "Cmds": []string{"go vet ./...", "go test ./..."}},
					M{"Name": "clean", "Cmds": []string{"rm -rf bin"}},
				},
			},
			Want: `BIN := cookbook
.PHONY: all build test clean

all: build test

build:
	go build -o bin/$(BIN) ./cmd/$(BIN)

test: build
	go vet ./...
	go test ./...

clean:
	rm -rf bin
`,
			Notes: "The template file must contain a real tab character before `{{ . }}`; the test also checks that every recipe line in the output starts with one. " +
				"Keep that file out of editors that expand tabs, add a `.editorconfig` section `[Makefile]` / `indent_style = tab`, or write the tab as `{{ \"\\t\" }}` when the source is not under your control. " +
				"`$(BIN)` and `$@` pass through untouched: the template engine only reacts to its own delimiters. A `$` that must reach make as a literal is written `$$` in the *data*.",
		},
		{
			ID:      "F29",
			Name:    "Plan 9 mkfile",
			Problem: "mk is not make: phony targets are marked `:V:`, recipes run under rc, and variables are `$BIN`, not `$(BIN)`.",
			Template: "BIN={{ .Bin }}\n" +
				"\n" +
				"all:V: {{ join \" \" .All }}\n" +
				"{{ range .Targets }}\n" +
				"{{ .Name }}:V:{{ if .Deps }} {{ .Deps }}{{ end }}\n" +
				"{{- range .Cmds }}\n" +
				"\t{{ . }}\n" +
				"{{- end }}\n" +
				"{{ end -}}\n",
			Data: M{
				"Bin": "cookbook", "All": []string{"build", "test"},
				"Targets": L{
					M{"Name": "build", "Cmds": []string{"CGO_ENABLED=0 GOOS=plan9 GOARCH=amd64 go build -o bin/$BIN ./cmd/$BIN"}},
					M{"Name": "test", "Deps": "build", "Cmds": []string{"go test ./..."}},
					M{"Name": "clean", "Cmds": []string{"rm -r bin"}},
				},
			},
			Want: `BIN=cookbook

all:V: build test

build:V:
	CGO_ENABLED=0 GOOS=plan9 GOARCH=amd64 go build -o bin/$BIN ./cmd/$BIN

test:V: build
	go test ./...

clean:V:
	rm -r bin
`,
			Notes: "Same skeleton as F28 with mk's spelling: `target:V: prereqs` (V = virtual, always run), a tab-indented recipe, and `$BIN` for variables. " +
				"The recipe body is handed to `rc`, so `VAR=value cmd` prefix assignments work and `&&` does not (use `;` or `if`). " +
				"The Go toolchain on Plan 9 reads `GOOS`/`GOARCH` from the environment, which rc sets with exactly this prefix form.",
		},
		{
			ID:      "F30",
			Name:    "Shell script with shq quoting",
			Problem: "Interpolating a value into a script is command injection: a name like `x'; rm -rf /` ends the string and runs the rest.",
			Template: `#!/usr/bin/env bash
set -euo pipefail

name={{ shq .Name }}
dest={{ shq .Dest }}

mkdir -p -- "$dest"
printf '%s\n' "$name" > "$dest/name.txt"
{{- range .Files }}
cp -- {{ shq . }} "$dest/"
{{- end }}
`,
			Data: M{
				"Name":  `it's "quoted"; rm -rf /`,
				"Dest":  "/tmp/my dir",
				"Files": []string{"a b.txt", "c'd.txt", "plain.txt"},
			},
			Want: `#!/usr/bin/env bash
set -euo pipefail

name='it'\''s "quoted"; rm -rf /'
dest='/tmp/my dir'

mkdir -p -- "$dest"
printf '%s\n' "$name" > "$dest/name.txt"
cp -- 'a b.txt' "$dest/"
cp -- 'c'\''d.txt' "$dest/"
cp -- plain.txt "$dest/"
`,
			Notes: "`shq` wraps a value in single quotes and writes each embedded `'` as `'\\''`, the only quoting a POSIX shell does not reinterpret. Words made only of safe characters are left bare, which keeps the output readable. " +
				"The script then assigns to variables once and uses `\"$var\"` everywhere, so the template touches quoting in exactly one place. `--` stops option parsing for values that begin with `-`. " +
				"The test runs `bash -n` over this output when bash is installed. Where a script can read its inputs from the environment instead, do that and avoid generating code from data at all.",
		},
		{
			ID:      "F31",
			Name:    "SQL: identifier allowlist and placeholders",
			Problem: "Values and identifiers cannot both be parameterised: placeholders stop at values, so a table or column name taken from input is injection.",
			Template: `SELECT {{ range $i, $c := .Cols }}{{ if $i }}, {{ end }}{{ ident $c }}{{ end }}
FROM {{ ident .Table }}
WHERE {{ ident .Filter }} = {{ bind .Value }};
`,
			Data: M{
				"Table": "users", "Cols": []string{"id", "name"},
				"Filter": "name", "Value": "Robert'); DROP TABLE students;--",
			},
			Run:     glueSQL,
			GlueRef: "allowed,glueSQL",
			Want: `SELECT "id", "name"
FROM "users"
WHERE "name" = $1;
-- args: ["Robert'); DROP TABLE students;--"]
-- rejected: template: sql:2:8: executing "sql" at <ident .Table>: error calling ident: ident: "users; DROP TABLE users" is not in the allowlist
`,
			Notes: "Two rules, enforced by two functions the glue adds. **Values** never appear in the SQL text: `bind` records the value and prints `$1`, `$2`, ... and the driver sends the values separately. " +
				"**Identifiers** cannot be bound, so `ident` accepts only names on a fixed allowlist and quotes them; anything else is a template error, not a query. " +
				"The second run in the glue shows the rejection. Do not 'sanitise' with `replace`: allowlist.",
		},
	}
}
