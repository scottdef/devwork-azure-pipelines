# Classification Report — Go Templates as a Systems Substrate

**Scope.** `text/template`, `html/template`, and `text/template/parse` as they are used in
the wild, classified by *what the template controls*. Each class gets a definition, the
wiring mechanism, representative projects, and a conventionality rating.
**Date.** 2026-10-19. **Companion.** `RESEARCH-REPORT.md` (design lineage, cheat sheets,
cookbook, security, platform facts), `CASE-STUDY-DESIGN-SPECS.md`, and the three repositories
under `case-studies/`.

Conventionality scale: **C** conventional (the package docs anticipate it), **S**
semi-conventional (common, but the template is doing something the authors did not design
for), **U** unorthodox (the template *is* the protocol, interpreter, or control plane).

---

## 0. The thing being classified

A Go template is a parse tree (`parse.Tree`) executed against a data value. Everything a
template can do is one of:

| Capability | Where it lives | Reachable from template text? |
|---|---|---|
| Walk fields/keys/indices of the data (`.A.B`, `index`) | `text/template/exec.go` | yes |
| Call **exported methods** on the data | `exec.go` evalField → `reflect` | yes |
| Call functions from the **FuncMap** or the 19 builtins | `funcs.go` | yes |
| Control flow: `if/else`, `range`, `with`, `break/continue` (1.18), `define/template/block` | `parse` | yes |
| Variables `$x :=` / `$x =` (1.11) | `exec.go` | yes |
| I/O of any kind | **nowhere** | **no** — only through a FuncMap function |

So every system in this report is "a FuncMap + a data source + a destination." The FuncMap is
the capability boundary, which is why it is also the security boundary (§8).

---

## 1. Taxonomy

| # | Class | Template controls… | Engine | Typical wiring | Rating |
|---|---|---|---|---|---|
| C1 | Generators / scaffolders | Source code, project trees | `text/template` | `go generate`, `embed.FS` + `ParseFS`, gofmt post-pass | C |
| C2 | Config rendering & automation | Config files written to disk in a watch loop | `text/template` + rich FuncMap | long-lived `*Template`, re-`Execute` on change, atomic write, exec hook | C |
| C3 | Output formatters / scrapers | The *shape* of API, CLI, and log output | `text/template` | `--format`, `-o go-template`, `--template` flags | C |
| C4 | Web servers | HTTP response bodies | `html/template` (Caddy: `text/template`) | layouts with `block`, partials, `ExecuteTemplate`, `Clone` | C→S |
| C5 | File-transfer servers | Directory listings, banners, page chrome of FTP/HTTP/SFTP file servers | both | listing templates, MOTD | S→U |
| C6 | Network & storage servers | Protocol *answers* (DNS RRs, device configs) | `text/template` | per-request `Execute` on request data | U |
| C7 | Unorthodox | Interpreters, meta-templates, shell emitters, charts, tests | both | `define` recursion, `Delims`, AST walking | U |

---

## 2. C1 — Generators and scaffolders

**Definition.** The template emits source code or a file tree; the output is checked in or
compiled, never served.

**Mechanism.** `template.Must(template.New("").Funcs(fm).ParseFS(fsys, "tmpl/*.tmpl"))` at
init, `Execute` per target, then a language-specific post-pass (`go/format.Source`,
`gofmt`, `terraform fmt`), then a golden test. Generators should **never** use
`html/template`: it would escape the `<` in `Map[K, V]`-style generics and the `&` in C.

| Project | URL | Engine | Mechanism | Rating |
|---|---|---|---|---|
| oapi-codegen | https://github.com/oapi-codegen/oapi-codegen | text | embedded `*.tmpl`, user overrides via `-templates` | C |
| go-swagger | https://github.com/go-swagger/go-swagger | text | large embedded template set, Sprig-like funcs | C |
| sqlc | https://github.com/sqlc-dev/sqlc | text | per-language templates, `go/format` post-pass | C |
| mockery v3 | https://github.com/vektra/mockery | text | `template:` config selects embedded or user template | C |
| Kubebuilder | https://github.com/kubernetes-sigs/kubebuilder | text | "machinery": templates with **insertion markers** (`//+kubebuilder:scaffold:…`) so re-runs splice into existing files | S |
| cobra-cli | https://github.com/spf13/cobra-cli | text | `cobra-cli add serve` renders `cmd/serve.go` from an embedded template | C |
| protoc-gen-go-template plugins | various | text | `.proto` descriptor as data | C |
| boilr / gotemplate / kickoff | various | text | cookiecutter-style tree rendering, filenames also templated | C |
| stringer (`golang.org/x/tools/cmd/stringer`) | https://pkg.go.dev/golang.org/x/tools/cmd/stringer | **none** | builds code with `fmt.Fprintf` — the contrast case | — |

**Lessons.** (1) Template filenames are themselves templates in scaffolders
(`cmd/{{.Name}}.go`). (2) Use `Delims` when the target language uses `{{` (Helm, Actions).
(3) Determinism: sorted map keys (templates already sort map ranges), fixed timestamps in
tests.

---

## 3. C2 — Config rendering and automation

**Definition.** A long-lived process (or a one-shot CLI in CI) renders configuration from a
key/value, secret, or service-catalog source and writes it to disk; on change it re-renders
and signals or restarts a consumer.

**Mechanism.** The template is parsed once and executed many times. The FuncMap does two jobs:
it fetches data *and records what it fetched* (dependency tracking), so the runtime knows
which templates to re-render when a key changes. Writes are atomic (temp + fsync + rename),
permissions and backups are explicit, and a post-render command is run after a quiet period
(`wait`) plus a random delay (`splay`) so a fleet does not restart in lock-step.

| Project | URL | Engine | Notable | Rating |
|---|---|---|---|---|
| consul-template | https://github.com/hashicorp/consul-template | text | Dependency-tracking FuncMap (`key`, `service`, `secret`, `tree`, `ls`…), Watcher→Brain→Runner, `-exec` supervisor. MPL-2.0 (0.43.x, Sep 2026). | C |
| Nomad `template` stanza | https://developer.hashicorp.com/nomad/docs/job-specification/template | text | embeds consul-template; `change_mode`, `change_signal`, `splay`, `left/right_delimiter`, `error_on_missing_key`, `wait` | C |
| Vault Agent `template` | https://developer.hashicorp.com/vault/docs/agent-and-proxy/agent/template | text | `contents` vs `source`, `exec` mode, secret renewal re-renders | C |
| gomplate | https://github.com/hairyhenderson/gomplate | text | datasources (file, env, http, vault, consul, aws), 100+ funcs, alt delims | C |
| Helm + Sprig | https://github.com/helm/helm, https://github.com/Masterminds/sprig | text | `include`, `tpl`, `toYaml`, `nindent`, `required`; Sprig is the de-facto func library | C |
| Hugo | https://github.com/gohugoio/hugo | both | templates + pipes drive the whole SSG; `dict`, `partial`, `resources` | C |
| goreleaser | https://github.com/goreleaser/goreleaser | text | `name_template`, `{{ .Version }}`, `{{ .Os }}` in YAML values | C |
| go-task | https://github.com/go-task/task | text | templates **inside shell lines** (`cmds: ['echo {{.VAR}}']`) — a template→shell boundary, see C7 | S |
| Alertmanager | https://prometheus.io/docs/alerting/latest/notifications/ | text | notification templates with `.Alerts.Firing`, `.CommonLabels`; also console templates | C |
| Promtail / Loki | https://grafana.com/docs/loki/latest/send-data/promtail/stages/template/ | text | `template` pipeline stage with Sprig funcs | C |
| Terraform `templatefile()` | https://developer.hashicorp.com/terraform/language/functions/templatefile | **HCL template, not Go** | `${expr}` and `%{ for } … %{ endfor }`; the old `template_file` data source is deprecated | — |
| Packer | https://developer.hashicorp.com/packer | legacy JSON used Go-template-ish `{{user "x"}}`; HCL2 replaced it | — |

**Distinction worth memorising.** Terraform's templating is HCL's string-template language.
A Go template that *emits* HCL must never produce a literal `${`, or Terraform will try to
interpolate it. Formula F24 in the cookbook shows the `printf "%q"` idiom.

---

## 4. C3 — Output formatters, scraping, and logs

**Definition.** The user supplies a template at the CLI to shape structured output from an
API, container runtime, or log pipeline.

| Tool | Flag | Data | Extras |
|---|---|---|---|
| kubectl | `-o go-template`, `-o go-template-file` | the object JSON | `{{range .items}}…{{end}}` |
| docker | `--format '{{json .}}'`, `table {{.Names}}` | container/image structs | `table` prefix, `json`, `join`, `split`, `lower` |
| gh | `gh api … --template` | API JSON | `tablerow`, `tablerender`, `timeago`, `color`, `pluck`, `join`, `hyperlink`, `truncate` |
| hcloud, doctl, many | `-o template` | struct | — |
| Caddy `templates` | response bodies | request + file system | see C4 |
| Vector (VRL), lnav, goaccess | — | — | **contrast**: these *transform* data with their own languages; Go templates only *print* |

**Position.** Extract with `encoding/json`, `regexp`, `bufio.Scanner`; render with a template.
A template that parses is an anti-pattern: no error values, no backtracking, and
`regexMatch` in a template is a smell. The scraper case studies in the wild (colly + templates)
all follow this split: Go code scrapes, templates format.

---

## 5. C4 — Template-controlled web servers

**Definition.** The HTTP response body is a template; the server is a thin loop around
`ExecuteTemplate`.

### 5.1 Caddy `templates` directive (deep dive)

Caddy executes response bodies as `text/template` with a request-aware FuncMap.

- Directive: `templates [<matcher>] { mime <types…>; between <open> <close>; root <path>; extensions { … } }`
- Default MIME set: `text/plain`, `text/markdown`, `text/html`.
- Context: `.Host`, `.Cookie "name"`, `.Req` (the `*http.Request`), `.Args` (from `include`),
  `.RespHeader.Set`, `.RemoteIP`, `.Placeholder "http.request.uri"` / `ph`.
- Functions: `include` (file; args become `.Args`; **not escaped**), `import` (defines only),
  `httpInclude` (an in-memory virtual sub-request), `listFiles`, `fileStat`, `fileExists`,
  `readFile`, `markdown`, `splitFrontMatter` (YAML `---`, TOML `+++`, JSON), `env`,
  `humanize`, `maybe`, plus all of Sprig. Caddy marks the extension API as experimental.

```caddyfile
example.com {
	root * /srv
	templates
	file_server browse
}
```
```html
{{$md := (include "/posts/hello.md" | splitFrontMatter)}}
<title>{{$md.Meta.title}}</title>
{{markdown $md.Body}}
```
Rating **S**: it is `text/template`, so the author is trusted and no contextual escaping
happens — exactly the model `html/template` exists to replace. Fine for a personal site,
wrong for user-supplied content.

### 5.2 Stdlib `net/http` + `html/template` (the pattern used in CS2/CS3)

1. Parse layouts and partials once from `embed.FS`.
2. `Clone()` per page so each page can redefine `{{block "content" .}}`.
3. Execute into a `bytes.Buffer`, then `io.Copy` to the response — a mid-render error then
   never produces a half-written 200.
4. htmx: handlers return *fragments* (`{{define "preview"}}…{{end}}`) for `hx-target` swaps.

### 5.3 Others

| Project | Engine | Note |
|---|---|---|
| Gin / Echo / Fiber | html | thin wrappers; Fiber has pluggable engines |
| Gitea / Forgejo | html | large template tree, `.locale.Tr`, historical XSS findings in templates |
| Grafana | html | index page; most UI is React |
| Prometheus console templates | html | `consoles/*.html` with `query`, `graph` funcs |
| Pocketbase | html | admin UI assets + mail templates |
| Hugo server | both | SSG with live reload |
| a-h/templ | **none at runtime** | compiles `.templ` to Go; contrast case — type-safe, no `text/template` |

---

## 6. C5 — Template-operated file-transfer servers

**Finding.** "Template-operated FTP" is not an established category. Go FTP/SFTP/TFTP servers
(`fclairamb/ftpserverlib`, `goftp/server`, `pkg/sftp`, `pin/tftp`) do not use templates. What
*does* exist is templating the human-facing surfaces of file servers: directory listings,
banners/MOTD, and page chrome.

| System | Template use | Rating |
|---|---|---|
| Caddy `file_server browse [<template file>]` | The directory listing page is an overridable Go template over `{Name, Path, CanGoUp, Items[]{Name,IsDir,Size,HumanSize,ModTime,URL}, NumDirs, NumFiles, Sort, Order, Limit}` with `sortByName`, `sortByNameDirFirst` etc. The canonical unorthodox example. | U |
| rclone `serve http --template` | User-supplied Go template for listings (`.Entries`, `.Title`, `.Sort`) | U |
| MinIO / filebrowser / Gokapi | React/Vue UIs; no Go templates | — |
| gliderlabs/ssh, charmbracelet/wish | banners are strings; templating them is user code | S |

**Position for the case studies.** CS1 renders MOTD/hosts/Caddyfile from templates; CS2's
output pickup pages are, in effect, a templated file-server listing (items, sizes, hashes,
download links) published statically.

---

## 7. C6 — Template-configured network and file-storage servers

### 7.1 CoreDNS `template` plugin (deep dive)

The plugin answers DNS queries by executing Go templates per query.

```
template CLASS TYPE [ZONE...] {
    match REGEX...
    answer RR
    additional RR
    authority RR
    rcode CODE
    fallthrough [ZONES...]
}
```
- Data: `.Name`, `.Zone`, `.Class`, `.Type`, `.Match` (regex groups), `.Group` (named groups),
  `.Question`, `.Remote`, `.Meta` (metadata plugin); `parseInt` func.
- First matching regex wins; regex length is capped (10 000 chars).
- Zone match without regex match ⇒ SERVFAIL unless `fallthrough`.
- Metric `coredns_template_matches_total`. Source holds `answer []*template.Template`.

```
template IN A example {
  match "^ip-(?P<a>[0-9]*)-(?P<b>[0-9]*)-(?P<c>[0-9]*)-(?P<d>[0-9]*)[.]example[.]$"
  answer "{{ .Name }} 60 IN A {{ .Group.a }}.{{ .Group.b }}.{{ .Group.c }}.{{ .Group.d }}"
  fallthrough
}
```
Rating **U**: the template is the protocol answer. CS1 ships a `Corefile.tmpl` that renders
such blocks from KV; CS3's catalog includes the answer formula.

### 7.2 Upspin

`upspin.io` (Apache-2.0; https://github.com/upspin/upspin) is a global, secure, sharable
namespace. Packages: `upspin.io/upspin` (Client, KeyServer, DirServer, StoreServer
interfaces), `client`, `dir/*`, `store/*`, `key/*`, `pack` (`ee`, `eeintegrity`, `plain`),
`access`, `path`, `config`, `factotum`, `flags`, `subcmd`, `transports`, `errors`, `log`,
`cmd/upspin` (`shell`, `cp`, `get`, `put`, `share`, `watch`, `snapshot`, `setupserver`…),
`cmd/upspinfs` (FUSE), `cmd/cacheserver`. Templates appear only in the docs frontend.
Activity is low (last substantive commits 2025; `upspin.github.io` Jan 2026). It is an allowed
dependency for this project and is positioned as an optional phase-2 KV source for CS1
(`source.type = "upspin"`, a client that reads a directory of keys through
`upspin.io/client`). Rating as a *template* consumer: n/a; as a storage backend: viable.

### 7.3 Others

| System | Engine | Note | Rating |
|---|---|---|---|
| WireGuard / Nebula config generators | text | `[Peer]` blocks from inventory | S |
| gornir / scrapligo device configs | text | Jinja-style device configs via `text/template` | S |
| Headscale | — | YAML config; contrast | — |
| Ansible | Jinja2 | contrast | — |
| dnscontrol | — | its own DSL (JS); contrast | — |
| go-sockaddr `template` package | text | `bind_addr = "{{ GetPrivateIP }}"` — a template as a *network-query DSL* (HashiCorp) | U |

---

## 8. C7 — Unorthodox usage

| Pattern | Mechanism | Example | Risk |
|---|---|---|---|
| Template as interpreter | `define` + `template` recursion, `$x =` accumulators, `break/continue` | tree walkers, fizzbuzz-in-templates | exec depth capped at 100 000 (`maxExecDepth`); parse-time nesting of `if/range` is unbounded (golang/go#81546) |
| Meta-templates | `Delims("[[","]]")` outer template emits literal `{{ }}` | CS2's `next.tmpl`, cookbook F37 | none beyond confusion |
| Template → shell → exec | render a script, run it | go-task, CS3 `!CMD` | **quoting**: use a `shq` func; never `printf "%q"` (Go quoting ≠ shell quoting); prefer `exec.Command(args...)` |
| Templates writing CI/IssueOps artifacts | `[[ ]]` delims so `${{ }}` survives | CS3 `new cicd`, `new issueops` | input caps (25 `workflow_dispatch` inputs) |
| Templates emitting HCL/Makefiles/mkfiles/C/Go | `printf "%q"`, literal tabs, gofmt post-pass | CS1 `main.tf.tmpl`, cookbook F24/F28/F29/F32/F33 | stray `${`, tabs vs spaces |
| Templates as test fixtures | golden files rendered per case | every case study | drift — regenerate with `-update` |
| Charts | go-echarts: `html/template` strings `BaseTpl/ChartTpl/HeaderTpl/PageTpl` compiled by `render.MustTemplate` with `safeJS`/`safeHTML`/`injectInstance`; options injected via `{{ .JSON }}` | CS3 `/chart` | loads ECharts JS from `AssetsHost` — vendor for air-gap |
| Terminal UI screens | `text/template` + ANSI escapes | CS3 `sh` help/status screens | `TERM=dumb` fallback |
| Email | `html/template` + `text/template` parts via `mime/multipart` | cookbook F35 | escaping differs per part |
| Prometheus exposition | `text/template` writes `# HELP/# TYPE` lines | CS1/CS3 `/metrics` | label escaping |

### 8.1 Security classification (why rating U matters)

- `text/template` trusts the **author**. `html/template` trusts the author *and* escapes data
  contextually. Neither sandboxes the author: a template can call any exported method on the
  data and any FuncMap function.
- 2026 `html/template` advisories (fixed in Go 1.25.x/1.26.x, **not** in 1.24):
  CVE-2026-27142 (meta-refresh URL), CVE-2026-32289 (JS template literals across branches),
  CVE-2026-39823, CVE-2026-39826 (empty `<script type>`), CVE-2026-56858 (JS regexp context),
  CVE-2026-94448, CVE-2026-97030. The case studies honour the project's Go 1.24 ceiling as a
  **language level** and document the toolchain recommendation in each `SECURITY.md`.
- User-authored templates (CS3 GUI) therefore run in a **subprocess** with a minimal FuncMap,
  a 64 KiB source cap, a 1 MiB output cap and a 2 s timeout. That is the only safe shape for
  class-U systems that accept templates from the network.

---

## 9. Classification of the three case studies

| Case study | Primary class | Secondary classes | Unorthodox elements |
|---|---|---|---|
| CS1 `tmplwatch` | C2 config rendering | C6 (Corefile, WireGuard, hosts), C7 (metrics via template, HCL emission) | dependency-tracking FuncMap; Prometheus text rendered by a template; Plan 9 notes instead of signals |
| CS2 IssueOps library | C1 generators (site, items) + C4 (static web) | C5 (output pickup = templated listing), C7 (meta-template `next.tmpl`, Atom via template, wasm lab) | issue-form YAML → static form at build time; borrow ledger rendered as htmx-loadable static fragments |
| CS3 `tmplforge` | C7 (template generator, interpreter-ish) | C4 (htmx GUI), C1 (IssueOps/CI-CD generators), C3 (export) | ed/sam command TUI; parse-tree walker for fixture skeletons; sandboxed re-exec render; go-echarts latency chart |

---

## 10. Quick reference: mechanisms by API

| API | Use | Gotcha |
|---|---|---|
| `template.New(name).Funcs(fm).Parse(src)` | the canonical chain | `Funcs` **before** `Parse` |
| `ParseFS(fsys, patterns...)` + `//go:embed` | ship templates in the binary | names are base names |
| `Delims(l, r)` | alt delimiters | before `Parse`; per `*Template` |
| `Option("missingkey=error")` | fail on missing map keys | maps only; structs always fail on unknown fields |
| `Clone()` | per-page layout with overridden `block` | clone **before** adding page `define`s |
| `Lookup`, `DefinedTemplates`, `Templates()` | introspection | — |
| `AddParseTree` | programmatic trees | used by html/template internally |
| `ExecuteTemplate(w, name, data)` | pick a named template | the root name is the file name for `ParseFS` |
| `parse.Parse`, walk `*parse.ListNode` | lint, variable discovery | nodes are concrete pointer types; nil `ListNode`s exist |
| `template.HTML/JS/CSS/URL/Srcset/HTMLAttr` | mark trusted strings | bypasses escaping — audit every use |
| `ExecError` | typed execution error | `errors.As(err, &template.ExecError{})` |
