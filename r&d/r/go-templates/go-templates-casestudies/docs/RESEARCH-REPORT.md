# Research Report — Go Templates: Design Lineage, Tooling, Hardening, and Platform Facts

**Date.** 2026-10-19 (America/New_York). **Status.** Verified against primary sources where
cited; items marked ⚠ were not re-verified in this pass and are listed in §9.
**Companions.** `CLASSIFICATION-REPORT.md`, `CHEATSHEET-BASIC.md`, `CHEATSHEET-ADVANCED.md`,
`COOKBOOK.md` (generated from the executable cookbook), `CASE-STUDY-DESIGN-SPECS.md`.

---

## 1. Executive summary

1. **Go template is a deliberately weak language for trusted authors.** It has exactly nineteen
   builtins (`and call html index slice js len not or print printf println urlquery eq ge gt le
   lt ne`), no I/O, and no way to reach anything the FuncMap and the data value do not expose.
   Weakness is the design lever: every system in the classification is "FuncMap + source +
   destination," and the FuncMap is both the extension and the security boundary.
2. **Go version.** The project ceiling is `~=1.24`; the local toolchain is go1.24.7 and every
   module here builds with it. Go 1.24 is past upstream support (two newer majors exist;
   go1.24.13 was its last point release, Feb 2026) and seven `html/template` CVEs were fixed
   only in 1.25.x/1.26.x. Each case study's `SECURITY.md` records this and parameterises the
   CI toolchain (`GO_VERSION`) so an operator can lift it without changing language level.
3. **Newest template features usable at `go 1.24`:** `{{range 5}}` (range over int) and
   `{{range .Seq}}` over `iter.Seq`/`iter.Seq2` (both landed in `text/template` in Go 1.24,
   golang/go#66107); `{{break}}`/`{{continue}}` (1.18); `$x = v` reassignment (1.11).
4. **GitHub platform (Oct 2026):** `checkout@v6`, `setup-go@v6`, `configure-pages@v5`,
   `upload-pages-artifact@v5`, `deploy-pages@v5`, `create-github-app-token@v3`
   (`client-id` input). `workflow_dispatch` ≤25 inputs; `repository_dispatch` `client_payload`
   ≤10 top-level keys; issue-form fields prefill from URL query parameters keyed by field `id`;
   Pages: 1 GB site, 100 GB/month soft bandwidth, 10-minute deploy timeout, builds/hour limit
   waived for custom Actions workflows.
5. **HashiCorp:** consul-template remains MPL-2.0 (0.43.x); Consul/Nomad/Vault/Terraform/
   Packer are BUSL-1.1 (each release reverts to MPL after four years). A clean-room stdlib
   reimplementation of the consul-template pattern (CS1) has no licence exposure.
6. **Vendored third-party:** go-echarts v2.7.3 (Apache-2.0, commit a154e55, 2026-09-13) and
   htmx v2.0.11 (0BSD, commit fa978b2, 2026-09-22) are vendored with checksums and VENDOR.md
   provenance; no module-proxy access is needed to build anything in this collection.

---

## 2. Rob Pike and the design lineage

### 2.1 Sources

| Source | Where |
|---|---|
| "Lexical Scanning in Go", GTUG Sydney, 30 Aug 2011 (slides) | https://go.dev/talks/2011/lex.slide |
| Same talk, video | https://www.youtube.com/watch?v=HxaD_trXwRE |
| `text/template` package documentation (the language spec, written by Pike) | https://pkg.go.dev/text/template |
| `text/template/parse` (lexer `lex.go`, parser `parse.go`, nodes `node.go`) | https://pkg.go.dev/text/template/parse |
| "Go Concurrency Patterns", Google I/O 2012 | https://go.dev/talks/2012/concurrency.slide |
| "Simplicity is Complicated", dotGo 2015 | https://go.dev/talks/2015/simplicity-is-complicated.slide |
| "Notes on Programming in C" (1989) | https://doc.cat-v.org/bell_labs/pikestyle |
| Eli Bendersky, "Performance of coroutine-style lexers in Go" (2022) | https://eli.thegreenplace.net/2022/performance-of-coroutine-style-lexers-in-go/ |
| golang/go#53261 — lexer goroutine discussion | https://github.com/golang/go/issues/53261 |
| Mike Samuel, html/template contextual autoescaping (package doc + design) | https://pkg.go.dev/html/template |

### 2.2 Lesson 1 — state functions, not state tables

The template lexer is the canonical example of the "state function" pattern: each state is a
function that does some work and returns the next state.

```go
// text/template/parse/lex.go (shape)
type stateFn func(*lexer) stateFn

func (l *lexer) run() {
	for state := lexText; state != nil; {
		state = state(l)
	}
}

func lexText(l *lexer) stateFn {
	if x := strings.Index(l.input[l.pos:], l.leftDelim); x >= 0 {
		l.pos += Pos(x)
		l.emit(itemText)
		return lexLeftDelim
	}
	l.pos = Pos(len(l.input))
	l.emit(itemText)
	l.emit(itemEOF)
	return nil
}
```

Templates alternate between two macro-states — *text* and *action* — and the lexer is where
whitespace trimming happens: `{{- ` must have the space after the dash so the lexer can
distinguish a trim marker from a negative number.

```gotemplate
{{- range .Items }}{{ .Name }}{{- end }}
```

Design consequence for us: CS3's `lint` reports `template: name:LINE: message` because the
lexer records line numbers in every item; a template linter needs no second parser.

### 2.3 Lesson 2 — concurrency for clarity, then remove it when it costs

The 2011 talk ran the lexer as a goroutine emitting items on a channel: "Goroutines allow
lexer and caller (parser) each to run at its own rate, as clean sequential code." Pike later
said the aim "was to demonstrate how concurrency can make programs nice even without obvious
parallelism in the problem… the code is pretty but not necessarily fast." Two facts followed:

- Before Go 1, goroutines could not run during package `init`, so
  `var t = template.Must(template.New("x").Parse(src))` at package level was impossible with
  the channel lexer. The slide deck notes the restriction was lifted in Go 1.
- The goroutine was eventually replaced by a synchronous `nextItem` loop (the channel had
  measurable cost and complicated error recovery). ⚠ The exact commit was not pinned;
  `git log -- src/text/template/parse/lex.go` in the Go repo shows it.

Lesson for systems: CS1 uses a goroutine per *watcher*, not per *render*; rendering is
synchronous because `Execute` is already safe to call concurrently after `Parse`.

### 2.4 Lesson 3 — pipelines are Unix pipes for values

`{{ .Title | lower | printf "slug-%s" }}` — the previous result becomes the **last** argument.
This is the rc/sh pipe idiom applied to values, and it is why FuncMap functions in every
mature library (Sprig, Hugo, gomplate) put the "data" argument last: `indent 2 $s`,
`default "x" .Y`, `join "," .List`.

### 2.5 Lesson 4 — data-driven, logic-light, author-trusted

The package doc says templates are "data-driven" and that "the input text for a template is
UTF-8-encoded text in any format." There is no arithmetic, no assignment to data, and no I/O
without a FuncMap. Logic belongs in Go. The security model and the extension model are the
same thing: a function is either in the FuncMap or it does not exist.

```go
funcs := template.FuncMap{"shq": shellQuote} // the only way a template can quote for sh
```

### 2.6 Lesson 5 — simplicity is complicated

The action set is tiny (`if with range define template block break continue else end`
plus comments and variables) and the builtin set is nineteen functions. Every other
capability is opt-in. Compare Jinja2 (filters, tests, extensions, sandbox mode) — Go chose
to have no sandbox because there is nothing to sandbox *unless you add it*.

### 2.7 html/template — contextual autoescaping

Mike Samuel designed `html/template` as a drop-in with the same API that tracks the HTML
parsing context (HTML text, attribute name/value, URL, CSS, JS, JS string, JS template
literal, `<script type>` handling) and escapes each interpolation for its context; untrusted
values that cannot be safely emitted become `ZgotmplZ`. The 2026 CVE stream (§6) shows how
hard context tracking remains — all fixes were in context-detection edge cases
(meta-refresh URLs, template literals across branches, regexp context, empty script type).

### 2.8 Plan 9 heritage (analysis, not sourced)

The pipeline syntax, the "text as universal interface" stance, and the state-function lexer
all echo Plan 9's rc and `mk`: minimal syntax, composition over features, structural regular
expressions rather than ad-hoc parsers. No source ties `text/template` to Sawzall; treat any
such claim as unverified. Ken Thompson's influence is indirect: regex-free lexing by hand
(the lexer never uses `regexp`) and the pipe.

---

## 3. CLI tools that operate through templates

| Tool | Install | One-liner | Uses text/template how | Formats | Emits templates for other tools? |
|---|---|---|---|---|---|
| gomplate | `go install github.com/hairyhenderson/gomplate/v4/cmd/gomplate@latest` | `echo '{{ .Env.USER }}' \| gomplate` | datasources + 100+ funcs, alt delims `--left-delim` | any text | yes |
| kubectl | — | `kubectl get po -o go-template='{{range .items}}{{.metadata.name}}{{"\n"}}{{end}}'` | object JSON as data | text | — |
| docker | — | `docker ps --format '{{json .}}'` / `--format 'table {{.Names}}\t{{.Status}}'` | struct data; `json`, `table` | text/JSON | — |
| gh | — | `gh api repos/CoolGitOrg/x --template '{{.name}} {{.stargazers_count}}'` | API JSON; `tablerow`, `timeago`, `color`, `hyperlink` | text | yes (`{{"{{"}}` escaping, cookbook F54) |
| helm | — | `helm template ./chart` | Sprig + `include`/`tpl` | YAML | yes |
| goreleaser | — | `name_template: '{{ .ProjectName }}_{{ .Os }}_{{ .Arch }}'` | template fields in YAML | — | — |
| go-task | — | `cmds: ['echo {{.VAR}}']` | templates inside shell lines | shell | yes (shell) |
| hugo | — | `hugo new posts/x.md` | archetypes | MD/HTML | yes |
| cobra-cli | `go install github.com/spf13/cobra-cli@latest` | `cobra-cli add serve` | scaffold | Go | — |
| **tpl** (this collection) | `go build ./cmd/tpl` in CS1/CS2 | `tpl -d data.json -t f.tmpl -delims '[[,]]' -missingkey error -o out` | stdlib renderer with the shared FuncMap | any | yes |
| **tmplforge** (CS3) | `go build ./cmd/tmplforge` | `tmplforge render -t x.tmpl -d fix.json -f yaml` | sandboxed render, lint, vars, test, export, new issueops/cicd | HTML/MD/JSON/TXT/YAML | yes |
| CUE, ytt (Starlark), Jsonnet, envsubst, pongo2/jet/plush/mustache | — | — | **contrast**: different languages/engines; excluded by the dependency policy | — | — |

### 3.1 Template → shell → execute, safely

1. Prefer `exec.Command(name, args...)` with one argument per rendered line. Avoid `sh -c`
   unless every interpolation is quoted.
2. Quote with a `shq` function (POSIX single quotes, `'` → `'\''`). `printf "%q"` is **Go**
   quoting and is wrong for sh (`\x00`, `\u` escapes).
3. Emitted scripts start with `set -euo pipefail`; gate with `bash -n` (and shellcheck when
   available).
4. `Option("missingkey=error")` so `<no value>` never reaches a command line.

```go
func shellQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }
```
```gotemplate
#!/usr/bin/env bash
set -euo pipefail
{{- range .Repos }}
gh repo clone {{ shq (printf "%s/%s" $.Org .) }} -- --depth=1
{{- end }}
```

---

## 4. Cheat sheets

The full sheets are separate files (`CHEATSHEET-BASIC.md`, `CHEATSHEET-ADVANCED.md`); the
executable cookbook (`cookbook/`) proves every row. Highlights:

- **Truthiness:** `false`, `0`, `nil`, empty string/slice/map/array are false in `if`/`with`.
- **`eq` is variadic:** `{{if eq .Env "dev" "stg"}}` is true if equal to *any* argument.
- **Comparison** only on basic types; `lt 1 1.5` errors (mixed int/float not allowed
  except via `printf`-style conversion in a func).
- **Map range is sorted** by key when keys are basic ordered types.
- **`range $i, $v`** on a map yields key, value; on a slice index, value; on an int (1.24)
  yields 0..n-1; on `iter.Seq2` yields pairs.
- **`missingkey`** applies to maps only; unknown struct fields always error.
- **Funcs before Parse**; `Delims` before `Parse`; `Clone` before page-specific `define`s.
- **Concurrency:** parse once, `Execute` concurrently; a `*Template` is immutable after
  parse unless you call `Parse`/`AddParseTree` again.
- **No context:** `Execute` cannot be cancelled. Wrap in a goroutine or subprocess with a
  deadline and a bounded writer (CS3 `internal/forge/sandbox.go`).
- **Depth limits:** `maxExecDepth` = 100 000 (1 000 on wasm); parenthesis/pipeline nesting
  `maxStackDepth` = 10 000; `if`/`range` nesting at parse time is unbounded (golang/go#81546).

---

## 5. Cookbook

Generated, with proofs, at `docs/COOKBOOK.md` by `cookbook gen` from
`cookbook/formulas/formulas.go`. Every formula has a `Want` string asserted by
`go test ./...`. The shared FuncMap is documented in `docs/FUNCS-SHARED.md`.

---

## 6. Security and production hardening

### 6.1 Engine selection

| Output | Engine | Why |
|---|---|---|
| HTML for browsers | `html/template` | contextual escaping |
| Config, code, YAML, shell | `text/template` + typed quoting funcs (`shq`, `printf "%q"`, `toJSON`) | no HTML escaping wanted |
| User-authored templates | `text/template` **in a subprocess** with a minimal FuncMap | author is untrusted |

### 6.2 What a template can reach

Exported fields and methods of the data value, FuncMap functions, `call` on func-typed
fields, and the 19 builtins. Nothing else. Therefore for untrusted authors: data is
`map[string]any` (no methods), FuncMap omits `env`, `file`, `shq`-like escapers that could
be abused for injection into *your* commands, and the render runs out-of-process.

### 6.3 Limits built in vs added

| Limit | Built in | Added in CS3 |
|---|---|---|
| Execution recursion | 100 000 | n/a |
| Expression nesting | 10 000 | n/a |
| Control-structure nesting (parse) | **none** — stack overflow is fatal | subprocess so the server survives |
| Template size | none | 64 KiB |
| Output size | none | 1 MiB `limitWriter` |
| Time | none | 2 s `exec.CommandContext` |

### 6.4 2026 Go template advisories (html/template)

| CVE | Issue | Fixed in |
|---|---|---|
| CVE-2026-27142 | meta-refresh URL escaping (`GODEBUG=htmlmetacontenturlescape=0` reverts — don't) | 1.25.8 / 1.26.1 |
| CVE-2026-39823 | bypass of the above | 1.25.x / 1.26.x |
| CVE-2026-32289 | JS template literals across branches | 1.25.9 / 1.26.2 |
| CVE-2026-39826 | empty `<script type>` | 1.25.x / 1.26.x |
| CVE-2026-56858 | JS regexp context tracking | 1.25.13 / 1.26.6 |
| CVE-2026-94448, CVE-2026-97030 | `yield`/JS escaping (published 2026-10-08) | 1.26.x |

Source: https://pkg.go.dev/vuln/list. Run `govulncheck ./...` in CI (it needs network; the
workflows call it, the offline preflight skips it with a message). `text/template`-only
services (CS1) are not affected by these HTML-context bugs.

### 6.5 Operational patterns

- **Hot reload without fsnotify:** poll mtimes, re-parse into a new `*Template`, swap with
  `atomic.Pointer[template.Template]`; keep the old one on parse failure and count
  `template_parse_errors_total`.
- **Observability:** `expvar`, `net/http/pprof` on loopback only, Prometheus text exposition
  rendered by a template (CS1/CS3 `metrics.tmpl`).
- **Load tools:** vegeta (Go, constant-rate; recommended), hey (Go), bombardier (Go),
  k6 (Go engine, JS scripts), wrk (C), oha (Rust). Each repo ships a stdlib fallback loader
  so CI needs no download.
- **Beta gating:** GitHub Pages cannot canary. Use a `flags.json` on Pages read by the form
  JS, or link the beta form only from a beta README; for Go servers use a token allowlist and
  a hand-written token bucket (`x/time/rate` is not stdlib).

---

## 7. GitHub platform facts (October 2026)

### 7.1 Pages
- Static only. Site ≤ 1 GB; 100 GB/month soft bandwidth; deploy timeout 10 min; the
  10-builds/hour soft limit does not apply to custom Actions workflows.
- Actions deploy: `permissions: pages: write, id-token: write`, environment `github-pages`,
  `actions/configure-pages@v5` → `actions/upload-pages-artifact@v5` → `actions/deploy-pages@v5`.
  `upload-pages-artifact@v4+` excludes dotfiles; `.nojekyll` is unnecessary with Actions deploys.
- Project-site URL for an org: `https://coolgitorg.github.io/<repo>/`. Large downloads belong
  in Releases (git file limit 100 MB).

### 7.2 Actions
- Majors used: `checkout@v6`, `setup-go@v6` (caches on `go.mod` by default), the Pages trio
  above, `create-github-app-token@v3` (inputs `client-id`, `private-key`, `repositories`).
  ⚠ `github-script`/`upload-artifact` current majors were not re-verified; the case studies
  avoid both by using `gh` and their own binaries.
- `workflow_dispatch`: ≤25 inputs (changelog 2025-12-04), ≤65 535 characters total.
- `repository_dispatch`: `client_payload` ≤10 top-level properties; byte limit undocumented
  (third-party actions report "client_payload is too large" around tens of KiB) — design for
  ≤32 KiB and pass large data by reference (issue number + sha).
- Pushes made with `GITHUB_TOKEN` do **not** trigger workflows; pushes with an App
  installation token do (used in CS2 for the cross-repo chain).
- Bot identities: `github-actions[bot]` / `41898282+github-actions[bot]@users.noreply.github.com`;
  an App's bot is `<app-slug>[bot]` with id from `gh api /users/<app-slug>%5Bbot%5D --jq .id`.

### 7.3 Issue forms and IssueOps
- Schema: `.github/ISSUE_TEMPLATE/*.yml` with `body:` elements `markdown`, `input`,
  `textarea` (`render:` for code fences), `dropdown`, `checkboxes`; `validations.required`.
- **URL prefill:** `…/issues/new?template=request.yml&title=…&<field-id>=<value>` fills
  `input`/`textarea` fields by `id`; dropdown/checkbox prefill is limited; malformed template
  names 404, oversized URLs 414; the GitHub mobile app may drop prefilled values.
- Toolkit (github.com/issue-ops): `parser` (v5; keys are field ids), `validator` (v3),
  `labeler`, `semver`, `releaser`, and the **`self-service`** template repo ("deploy a
  self-service IssueOps portal with GitHub Pages") — the closest prior art to CS2. Canonical
  guide: GitHub blog, "IssueOps: Automate CI/CD (and more!) with GitHub Issues and Actions."
  CS2's `issueparse` emits JSON compatible with `issue-ops/parser` keying.

### 7.4 REST endpoints used
`POST/GET/PATCH /repos/{o}/{r}/issues[/{n}]`, `POST …/issues/{n}/comments`,
`POST /repos/{o}/{r}/dispatches`, `PUT /repos/{o}/{r}/contents/{path}`,
`GET /repos/{o}/{r}/git/trees/{ref}?recursive=1`, `GET /repos/{o}/{r}/contents/{path}`
(ETag/`If-None-Match`; a 304 does not count against the primary limit when authenticated),
`GET /repos/{o}/{r}/installation`, `POST /app/installations/{id}/access_tokens`,
`POST /repos/{o}/{r}/releases`. Rate limit: installation tokens ≥5 000 req/h (15 000 on
GHEC orgs); write requests serially with ≥1 s spacing.

### 7.5 GitHub App JWT without a JWT library (CS1 `internal/ghapp`)
```go
func appJWT(clientID string, key *rsa.PrivateKey, now time.Time) (string, error) {
	enc := base64.RawURLEncoding
	h := enc.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	c, _ := json.Marshal(map[string]any{"iat": now.Add(-60*time.Second).Unix(), "exp": now.Add(9*time.Minute).Unix(), "iss": clientID})
	si := h + "." + enc.EncodeToString(c)
	sum := sha256.Sum256([]byte(si))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
	if err != nil { return "", err }
	return si + "." + enc.EncodeToString(sig), nil
}
```

### 7.6 Terraform
`integrations/github` v6.13.0 (2026-07-08) is current; pin `~> 6.13`. `github_repository`
has a `pages { build_type = "workflow" }` block ⚠ (schema recalled, not re-verified —
`terraform` is not installed in this environment; `terraform validate` runs in CI).

---

## 8. HashiCorp's use of Go templates (foundation for CS1)

### 8.1 consul-template
"Populates configuration files from Consul, Vault, or Nomad clusters using customizable
templates and optional command execution." MPL-2.0.

- **FuncMap** (⚠ from project docs, not re-fetched): `key`, `keyOrDefault`, `keyExists`,
  `ls`, `tree`, `service`, `services`, `nodes`, `secret`, `secrets`, `file`, `env`, `plugin`,
  `timestamp`, `loop`, `toJSON`, `toJSONPretty`, `toYAML`, `toTOML`, `parseJSON`, `parseYAML`,
  `regexMatch`, `regexReplaceAll`, `byTag`, `byKey`, `byMeta`, `sockaddr`, `indent`, `join`,
  `split`, `trimSpace`, `toUpper`, `toLower`, `replaceAll`, `contains`, `sha256Hex`, `base64Encode`.
- **Lifecycle.** Each dependency function registers a dependency → the Watcher fetches and
  fills the Brain → the Runner re-renders affected templates, writes atomically with
  `perms`/`backup`, and runs `command` after `wait` + `splay`; `-exec` supervises a child and
  forwards `change_signal`. Template spec string: `in.ctmpl:out.file:/bin/sh -c 'reload'`.
- **Nomad `template`:** `data|source`, `destination`, `change_mode` (noop|restart|signal|script),
  `change_signal`, `splay`, `left_delimiter`/`right_delimiter`, `perms`, `error_on_missing_key`,
  `wait { min max }`.
- **Vault Agent `template`:** `source|contents`, `destination`, `error_on_missing_key`,
  `command`, `exec { command timeout }`.

### 8.2 Terraform and Packer
Terraform uses HCL string templates (`${}`, `%{ for }`), not Go templates; `template_file` is
deprecated in favour of `templatefile()`. Packer's legacy JSON used Go-template-style
`{{user "x"}}`; HCL2 replaced it with typed variables.

### 8.3 go-sockaddr
`github.com/hashicorp/go-sockaddr/template` evaluates `{{ GetPrivateIP }}`-style templates as a
network-query DSL for `bind_addr`. CS1 reimplements the common subset on `net.Interfaces`.

### 8.4 Licensing (Oct 2026)
Consul, Nomad, Vault, Terraform, Packer, Boundary, Waypoint: BUSL-1.1 since Aug 2023 (each
release reverts to MPL after four years). APIs, SDKs, providers and consul-template: MPL-2.0.
OpenTofu is the MPL fork of Terraform. No source indicates IBM ownership changed these terms.

### 8.5 CS1 as a HashiCorp-pattern reimplementation
See `CASE-STUDY-DESIGN-SPECS.md` §1 and `case-studies/01-tmplwatch/`. Differences from
consul-template are tabulated in its README (JSON config instead of HCL; GitHub repo or local
dir as the KV source instead of Consul; no Vault/Nomad; same FuncMap vocabulary where it
matters: `key keyOrDefault keyExists ls tree env file toJSON toYAML parseJSON indent timestamp
regexMatch regexReplaceAll sockaddr`).

---

## 9. Items not re-verified in this pass (⚠)

Star counts; `github-script`/`upload-artifact` current majors; whether `checkout@v7` is GA (go-echarts upstream CI already uses `actions/checkout@v7` and `actions/configure-pages@v6` as of Sep 2026, so both exist; this collection pins v6/v5 as verified releases — lift when convenient);
the exact `github_repository.pages` schema in provider 6.13; the go-echarts `AssetsHost`
option name (verified in vendored source: `opts.Initialization.AssetsHost`, default
`https://go-echarts.github.io/go-echarts-assets/assets/`); load-tool maintenance; the
lexer-goroutine removal commit; the first Go release with `maxStackDepth`; consul-template's
full FuncMap; app-level template CVEs in Gitea/Grafana/Hugo/Harbor.

---

## 10. Bibliography

- Go release policy and history: https://go.dev/doc/devel/release — https://endoflife.date/go
- Go 1.24 release notes: https://go.dev/doc/go1.24 — range-over-func in templates: https://github.com/golang/go/issues/66107
- `text/template`: https://pkg.go.dev/text/template — `html/template`: https://pkg.go.dev/html/template — `text/template/parse`: https://pkg.go.dev/text/template/parse
- Go vulnerability database: https://pkg.go.dev/vuln/list — CVE-2026-27142: https://www.sentinelone.com/vulnerability-database/cve-2026-27142/ — CVE-2026-56858: https://hol.org/guard/security/cves/CVE-2026-56858-fix-javascript-regexp-context-tracking-in
- Parse-depth issue: https://github.com/golang/go/issues/81546 — maxStackDepth commit: https://github.com/golang/go/commit/42f9ee904caf6681ee32e7b048f15ab7cddf3eb3
- Rob Pike, Lexical Scanning in Go: https://go.dev/talks/2011/lex.slide — video https://www.youtube.com/watch?v=HxaD_trXwRE — golang-nuts thread: https://groups.google.com/g/golang-nuts/c/DcONCTJUDhY — Bendersky 2022: https://eli.thegreenplace.net/2022/performance-of-coroutine-style-lexers-in-go/
- GitHub Actions: checkout https://github.com/actions/checkout/releases — setup-go https://github.com/actions/setup-go/releases — upload-pages-artifact https://github.com/actions/upload-pages-artifact/releases — deploy-pages https://github.com/actions/deploy-pages/releases — create-github-app-token https://github.com/actions/create-github-app-token
- workflow_dispatch 25 inputs: https://github.blog/changelog/2025-12-04-actions-workflow-dispatch-workflows-now-support-25-inputs/ — events: https://docs.github.com/en/actions/reference/workflows-and-actions/events-that-trigger-workflows — repository_dispatch payload: https://github.com/octokit/plugin-rest-endpoint-methods.js/blob/main/docs/repos/createDispatchEvent.md
- Issue forms and URL prefill: https://docs.github.com/en/issues/tracking-your-work-with-issues/using-issues/creating-an-issue — https://github.com/orgs/community/discussions/113726
- IssueOps: https://github.blog/engineering/issueops-automate-ci-cd-and-more-with-github-issues-and-actions/ — https://github.com/orgs/issue-ops/repositories
- Terraform GitHub provider: https://github.com/integrations/terraform-provider-github — v6.13.0: https://newreleases.io/project/github/integrations/terraform-provider-github/release/v6.13.0
- htmx: https://github.com/bigskysoftware/htmx (v2.0.11) — https://www.npmjs.com/package/htmx.org
- go-echarts: https://github.com/go-echarts/go-echarts (v2.7.3) — https://pkg.go.dev/github.com/go-echarts/go-echarts/v2
- Caddy templates: https://caddyserver.com/docs/caddyfile/directives/templates — browse template: https://caddyserver.com/docs/caddyfile/directives/file_server
- CoreDNS template plugin: https://coredns.io/plugins/template/ — https://github.com/coredns/coredns/tree/master/plugin/template
- Upspin: https://github.com/upspin — https://upspin.io
- consul-template: https://github.com/hashicorp/consul-template — Nomad template stanza: https://developer.hashicorp.com/nomad/docs/job-specification/template — Vault Agent templates: https://developer.hashicorp.com/vault/docs/agent-and-proxy/agent/template — Terraform templatefile: https://developer.hashicorp.com/terraform/language/functions/templatefile
- HashiCorp licensing: https://www.hashicorp.com/license-faq — https://flowfactor.be/blogs/hashicorp-licensing-changes-much-ado-about-nothing
- Sprig: https://github.com/Masterminds/sprig — gomplate: https://github.com/hairyhenderson/gomplate
- vegeta: https://github.com/tsenart/vegeta — hey: https://github.com/rakyll/hey — oha: https://github.com/hatoo/oha
