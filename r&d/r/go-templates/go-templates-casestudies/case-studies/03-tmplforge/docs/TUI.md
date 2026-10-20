# TUI: `tmplforge sh`

`sh` is a line-command language for building templates, in the style of
ed(1) and sam(1). It reads one command per line and writes plain text. It
needs no terminal capabilities, so it works the same in a Unix terminal,
a Plan 9 rio window, an acme `win`, over ssh, and from a script.

```
tmplforge sh [-c NAME] [-d DIR] [-n] [script.tf]

  -c NAME   load catalog entry NAME first (same as the c command)
  -d DIR    base directory for relative file names (default .)
  -n        never print the prompt
  script.tf read commands from this file instead of stdin
```

The prompt `tf> ` is printed only when standard input is a character
device (a tty, or `/dev/cons` on Plan 9). ANSI colour is used only when
TERM is set and not `dumb`, NO_COLOR is unset, and standard output is a
character device.

## Commands

Arguments are separated by blanks. `NAME`, `FILE` and `DIR` are relative
to `-d`. Lines beginning with `#` and blank lines are ignored.

| Command | Meaning |
|---|---|
| `c NAME` | Load catalog entry NAME: template, format, delimiters, fixture. Each fixture key is declared as a variable with the catalog value as its default. |
| `f [FORMAT]` | Show or set the output format: `html` `md` `json` `txt` `yaml`. `html` renders with html/template, and the others with text/template. `json` output is validated. |
| `v NAME TYPE [DEFAULT]` | Declare (or redeclare) a variable. TYPE is `string` `int` `bool` `list` `map`. DEFAULT is the rest of the line, spacing preserved. A list default is a JSON array or a comma list, and a map default is a JSON object. |
| `V` | List declared variables: `name<TAB>type[<TAB>default]`. |
| `e` | Edit the template. If `$EDITOR` is set, the template goes to a temp file and the editor runs on the terminal, and the byte count is printed afterwards. Otherwise ed-style input mode reads lines until a line holding a single `.`, and they replace the template. |
| `a` | Append lines until `.`. |
| `i [N]` | Insert lines before line N (default 1) until `.`. |
| `d N[,M]` | Delete line N, or lines N through M. M may be `$`. |
| `n` | Print the template with line numbers. |
| `=` | Print the number of template lines. |
| `r FILE` | Append FILE to the template, then print its byte count. |
| `p` | Preview: render against the fixture, with declared defaults filling any keys the fixture lacks. |
| `D FILE` | Load a fixture (a JSON object). Integral numbers become int. |
| `l` | Lint: parse errors with line numbers (`name:line: message`), then warnings for fields referenced but not declared and for variables declared but never referenced. |
| `t FIX1 FIX2 [...]` | Render against each fixture, overlaid on the declared defaults, and print a unified LCS diff between consecutive outputs, or `A == B` when they are identical. |
| `w FILE` | Write the rendered output and print its byte count. |
| `W FILE` | Write the template source, print its byte count, and clear the modified flag. |
| `x DIR` | Export a bundle: template, fixture, output, `run.go`, `funcs.go`, `yaml.go`, `go.mod`, `Makefile`, `SHA256SUMS`. |
| `g issueops NAME [DIR]` | Generate an IssueOps kit from the declared variables: `.github/ISSUE_TEMPLATE/NAME.yml`, `.github/workflows/NAME-parse.yml`, `cmd/NAME-parse/main.go`. |
| `g cicd KIND [DIR]` | Generate `.github/workflows/KIND.yml`. KIND is `build` `dispatch` `release` `repository_dispatch` `tag`. Declared variables become `workflow_dispatch` inputs (int to number, bool to boolean, others to string; at most 25). |
| `s` | Status: name, format, delimiters, line count, variable count, fixture. |
| `!CMD` | Run CMD with `sh -c` (`rc -c` on Plan 9), with the rendered output on its standard input. A line holding `!` follows, as in ed. |
| `h` | Help. |
| `q` | Quit. Interactively, a modified template (not written with `W`) needs a second `q`. |

Errors print `?` for an unknown command, or `? reason` for a command that
failed. An interactive session continues. A script stops reporting
success: `tmplforge sh < x.tf` exits 1 if any command failed, like
`ed -s`.

### Trust model for `!`

`!` runs an arbitrary shell command, and `e` runs `$EDITOR`. That is
intended. Whoever types at this prompt is the operator: they started the
process and already have a shell with the same privileges. The TUI also
renders in-process with the full FuncMap, including `env` and `file`. None
of this is reachable from the web GUI, which runs every template in a
sandboxed child with `funcs.Safe()` (see [SECURITY.md](SECURITY.md)). Do
not expose `tmplforge sh` to untrusted input, for example by wiring it to
a network socket. It is a shell.

## Example session

Commands are shown after the `tf> ` prompt. Everything else is
tmplforge's output. This transcript was captured from a real run (the
fixtures are `{"host":"plan9","user":"glenda","updates":3,"notices":["venti scrub tonight"]}`
and the same for rob with `"updates":1,"notices":[]`):

```
tf> c motd.txt
motd.txt: 13 lines, txt, 7 vars
tf> n
1	== {{ upper .host }} ==
2	{{ .greeting }}, {{ default "operator" .user }}.
3	
4	{{ printf "%-12s %s" "Kernel:" .kernel }}
5	{{ printf "%-12s %s" "Last login:" (dateFmt "Mon Jan 2 15:04 MST 2006" .lastLogin) }}
6	{{ printf "%-12s %d %s" "Updates:" .updates (plural "package" "packages" .updates) }}
7	{{- with .notices }}
8	
9	Notices:
10	{{- range . }}
11	{{ printf "* %s" . | indent 2 }}
12	{{- end }}
13	{{- end }}
tf> d 1
tf> i 1
== {{ upper .host }} ({{ .user | default "nobody" }}) ==
.
tf> D glenda.json
glenda.json: 4 keys
tf> p
== PLAN9 (glenda) ==
Welcome, glenda.

Kernel:      9front 10522
Last login:  Sun Oct 18 21:04 UTC 2026
Updates:     3 packages

Notices:
  * venti scrub tonight
tf> l
ok
tf> v extra string
tf> l
motd.txt:1: warning: variable "extra" declared but never referenced
tf> t glenda.json rob.json
--- glenda.json
+++ rob.json
@@ -1,9 +1,6 @@
-== PLAN9 (glenda) ==
-Welcome, glenda.
+== PLAN9 (rob) ==
+Welcome, rob.
 
 Kernel:      9front 10522
 Last login:  Sun Oct 18 21:04 UTC 2026
-Updates:     3 packages
-
-Notices:
-  * venti scrub tonight
+Updates:     1 package
tf> W motd.tmpl
422
tf> x bundle
bundle/Makefile
bundle/fixture.json
bundle/funcs.go
bundle/go.mod
bundle/motd.txt
bundle/motd.txt.tmpl
bundle/run.go
bundle/yaml.go
bundle/SHA256SUMS
tf> !grep -c .
7
!
tf> zz
?
tf> q
```

Notes on the session:

- `c motd.txt` declared all seven fixture keys with the catalog's values
  as defaults. `D glenda.json` overrode four of them, and the rest
  (`kernel`, `lastLogin`, `greeting`) came from the defaults.
- `d 1` then `i 1` replaced the first line, ed style.
- `l` was clean until `v extra string` declared a variable that the
  template never reads.
- `t` rendered both fixtures and diffed the outputs. The `with .notices`
  block vanished for rob because his list is empty.
- `W` printed the byte count, as ed's `w` does.
- `!grep -c .` received the rendered output on stdin and counted its
  non-empty lines.
- `zz` is not a command, so the answer is `?`.

## Scripting

Every interactive command works in a script, which makes `sh` a small
build language for generated files:

```sh
$ cat gen-motd.tf
c motd.txt
D hosts/plan9.json
l
w /etc/motd
$ tmplforge sh < gen-motd.tf && echo ok
```

Use `-c` to preload a catalog entry and `-d` to resolve paths elsewhere:

```sh
printf 'D %s\np\n' prod.json | tmplforge sh -c config.yml -d deploy/
```

## Plan 9

Build with `GOOS=plan9 GOARCH=amd64` (or `mk cross`). In a rio window
`tmplforge sh` prompts, because `/dev/cons` is a character device. In acme,
run it inside `win`. Edit the template with `e` and `EDITOR=E`, or keep the
template in a file and use `r`. `!` runs `rc -c`. No escape sequences are
ever written unless TERM says the terminal understands them.
