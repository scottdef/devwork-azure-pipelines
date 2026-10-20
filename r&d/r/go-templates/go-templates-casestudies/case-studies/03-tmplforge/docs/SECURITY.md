# Security

## Threat model

tmplforge has two kinds of user, and they are trusted differently.

| | Operator (TUI `sh`, CLI) | Template author (web GUI) |
|---|---|---|
| Reaches tmplforge via | a shell on the host | HTTP |
| Trusted? | **Yes.** They started the process and already have a shell with its privileges. | **No.** Assume hostile templates and hostile fixture data. |
| Template execution | in-process, `funcs.All()` (includes `env`, `file`) | child process `render -sandbox`, `funcs.Safe()` |
| `!cmd`, `$EDITOR` | allowed (documented feature) | not exposed |
| Limits | opt-in (`render -sandbox`) | always on |

Assets to protect when serving the GUI:

1. **Server availability.** Do not let one template stall, crash or
   exhaust the server.
2. **Host secrets.** Keep the server's environment and files away from
   templates.
3. **Viewers of rendered output.** Stop XSS through previews.
4. **Other users' browsers.** Stop CSRF against the GUI.
5. **The token,** when one is used.

Out of scope: the confidentiality of templates themselves (the GUI stores
nothing), multi-tenant isolation between GUI users (one deployment is one
trust domain), and TLS (terminate it at a proxy).

## Controls

### Execution (assets 1 and 2)

| Threat | Control | Where | Test |
|---|---|---|---|
| Infinite / huge loop (`{{range seq 1e8}}`) | Child self-exits at `-timeout` (2 s). The parent kills at timeout + 500 ms grace (`exec.CommandContext`, `WaitDelay`) | `forge/cmdrender.go`, `forge/sandbox.go` | `TestSandboxTimeoutKillsLoop`, `TestSandboxHardKill`, `TestRenderLimits` (web) |
| Unbounded `{{template}}` recursion (stack overflow is a fatal error, not a panic) | Child `debug.SetMaxStack(32 MiB)`. The crash stays in the child and the parent reports `ErrCrashed` | `forge/cmdrender.go` | `TestSandboxCrashIsContained`, `TestCrashingChildDoesNotKillServer` |
| Memory exhaustion | Child `GOMEMLIMIT=128MiB`, systemd `MemoryMax=1G` on the unit, `-j` caps concurrent children | `forge/sandbox.go`, `deploy/systemd` | |
| Output flood | Child `LimitWriter` stops execution at 1 MiB. The parent keeps at most max+1 bytes of stdout and 64 KiB of stderr and drains the rest | `forge/render.go`, `forge/sandbox.go` | `TestSandboxOutputCap` |
| Giant template | 64 KiB cap (parent and child). Request body cap 256 KiB (`http.MaxBytesReader`) | `web/server.go`, `forge` | `TestSandboxTemplateCap`, `TestRenderLimits` (web) |
| Allocation bombs inside one function call | `repeat`/`indent` capped at 1 MiB, regex source at 1 KiB (RE2 is linear time), `seq` is a lazy iterator (costs time, not memory), YAML nesting ≤ 64, LCS table ≤ 4 M cells | `funcs`, `forge/diff.go` | `TestFuncErrors`, `TestDiffHugeFallsBack` |
| Reading host state | `Safe()` omits `env` and `file`. The child environment is `TMPLFORGE_SANDBOX=1 GOMEMLIMIT GOMAXPROCS` (+ `SOURCE_DATE_EPOCH`, `TZ`) only | `funcs`, `forge/sandbox.go` | `TestSafe`, `TestSandboxTemplateError`, `TestSandboxMinimalEnv` |
| Calling Go methods through data | Fixtures decode to `map[string]any` of plain JSON values. No value with methods reaches a template | `forge.LoadFixture` | `TestLoadFixture` |
| Too many renders at once | Semaphore of `-j` slots. Waiters give up with the request context and get 503 + `Retry-After` | `web/server.go` | `TestConcurrencyLimit` |
| Path traversal in export names | `SafeName` reduces names to `[A-Za-z0-9._-]` with no separators | `forge/export.go` | `TestSafeName`, `TestExport` (web) |

The server process parses untrusted templates (`/lint`, `/vars`) but never
executes them. Parsing has bounded cost: the input is capped at 64 KiB,
and `text/template/parse` is a linear recursive-descent parser. The
`{{template}}` walk in `vars` is memoised and depth-limited to 16.
`FuzzLint` exercises both.

### Output (asset 3)

- The `html` format executes with **html/template**: contextual escaping
  of HTML text, attributes, URLs (`javascript:` → `#ZgotmplZ`), JS and CSS.
  See `catalog/email.html.tmpl` and `catalog/htmx-partial.html.tmpl`, whose
  fixtures are hostile and whose goldens show the neutralised result.
- The GUI never inserts rendered output into the DOM. Previews are text in
  `<pre>`, escaped again by the GUI's own `html/template`.
- Generated GitHub workflows pass every untrusted value (issue bodies,
  dispatch inputs, client payloads) through `env:` and never by `${{ }}`
  interpolation inside `run:`. `TestWorkflowGolden` fails on any `run:`
  line containing `${{`.

### Browser (asset 4)

- **CSP** on every response:
  `default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; object-src 'none'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'`.
  `/chart` relaxes only `style-src` (adding `'unsafe-inline'`) for ECharts'
  tooltip styles. No inline script exists. go-echarts' inline chart script
  is served as `/chart.js`.
- **CSRF.** Every non-GET request must show same-origin evidence, in this
  order:
  1. `Sec-Fetch-Site: same-origin|none` is accepted, and
     `cross-site|same-site` is rejected. Browsers set this header and page
     script cannot forge it.
  2. Failing that, `Origin` must equal `http(s)://Host`.
  3. Failing that, `HX-Request: true` is accepted. A cross-origin page
     cannot add a custom header without a CORS preflight, and this server
     answers none.

  Requests with none of the three (bare `curl`) are rejected with 403.
  `TestCSRF` covers each case.
- `X-Frame-Options: DENY`, `X-Content-Type-Options: nosniff`,
  `Referrer-Policy: no-referrer`, and same-origin COOP/CORP.
- htmx is configured with `allowEval:false`, `allowScriptTags:false`,
  `selfRequestsOnly:true`.

### Exposure and authentication (asset 5)

- The server binds `127.0.0.1:8088` by default.
- **Any non-loopback `-a` requires `-token` (at least 16 characters) and
  `-rate > 0`**, and `serve` refuses to start otherwise (`CheckConfig`).
- The token is accepted as `Authorization: Bearer T`, as the
  `tmplforge_token` cookie (HttpOnly, SameSite=Strict, Secure under TLS),
  or once as `GET /?token=T`, which sets the cookie. Comparison is
  constant-time. Prefer the header or cookie: query strings end up in
  proxy logs.
- **Rate limit.** A per-client-IP token bucket, written in this
  repository (`web/ratelimit.go`, no `x/time`) with burst 2×rate. Memory
  is bounded at 16384 buckets with sweeping. The key is the TCP peer
  address. `X-Forwarded-For` is ignored on purpose, because behind a
  proxy the limit belongs at the proxy. `/healthz` is exempt.
- Server timeouts: ReadHeader 5 s, Read 10 s, Write 15 s, Idle 60 s,
  header bytes 16 KiB.

## Toolchain: Go 1.24 is past upstream support

The module declares `go 1.24`, as the case-study build conventions
require, and CI uses `${{ vars.GO_VERSION || '1.24.x' }}`. Upstream
supports the two newest major releases. As of this release (2026-10-19)
those are Go 1.27 (released 2026-08-19) and Go 1.26, so **both 1.24
(end of support 2026-02-10, last patch 1.24.13) and 1.25 (end of support
2026-08-19) receive no further security fixes** ([endoflife.date/go](https://endoflife.date/go)).

Several 2026 security releases fixed `html/template`, the package this
tool relies on to make HTML previews safe. Those fixes shipped only for
the then-supported 1.25 and 1.26 lines, never for 1.24:

| CVE | Fixed in | Issue |
|---|---|---|
| CVE-2026-27142 | 1.25.8, 1.26.1 (2026-03) | URLs inserted into `<meta http-equiv="refresh" content=...>` were not escaped ([record](https://fieldguide.lutrasecurity.com/CVE-2026-27142)) |
| CVE-2026-32289 | 1.25.9, 1.26.2 (2026-04) | context not tracked across branches and brace depth in JS template literals ([announcement](https://groups.google.com/g/golang-announce/c/0uYbvbPZRWU)) |
| CVE-2026-39826 | 1.25.10, 1.26.3 (2026-05-07) | escaper bypass with an empty or whitespace `type` attribute on `<script>` ([announcement](https://groups.google.com/g/golang-announce/c/qcCIEXso47M)) |
| CVE-2026-39823 | 1.25.10, 1.26.3 (2026-05-07) | whitespace around `=` in a meta `content` URL bypassed the CVE-2026-27142 fix |

These are template-author-side bugs: they need a template containing the
vulnerable construct. In the GUI the template author is the untrusted
party, so they apply directly to the html preview. The process sandbox
limits what a successful escape can reach (the preview is shown as text,
never as live DOM), but the escaping guarantee documented above is only
as good as the toolchain.

**Recommendation.** Build release binaries with a supported toolchain
(Go 1.26.x or 1.27.x at the latest patch) by setting the `GO_VERSION`
repository variable (`terraform/variables.tf: go_version`). The code uses
no 1.25+ APIs and builds unchanged. Keep `go 1.24` in `go.mod` as the
minimum language level until the collection's conventions move. Run
`govulncheck ./...` in CI once network access allows it. It is not
vendored here because the build is offline.

## Vendored dependencies and provenance

| Component | Version | Where | Provenance | License |
|---|---|---|---|---|
| go-echarts | v2.7.3, commit a154e551b39d | `third_party/go-echarts` | `third_party/go-echarts/VENDOR.md`. Tests, docs and go.sum removed, go.mod reduced to the module line, no source changes. Consumed with `replace`, so `GOPROXY=off` builds work | Apache-2.0 |
| htmx | 2.0.11 | `internal/web/static/htmx.min.js` | `SHA256SUMS` alongside, checked by `preflight.sh`. `scripts/fetch-htmx.sh` re-fetches from the npm registry and verifies the same SHA-256 | 0BSD |
| ECharts | 5.6.0 (not vendored) | operator-installed at `static/echarts/` | `scripts/fetch-echarts.sh` pins the npm tarball's sha256 and sha512 SRI and the `echarts.min.js` sha256 | Apache-2.0 |

go-echarts runs only server-side, to build chart options. It never sees
untrusted input: the chart data is our own latency samples.

## Reporting

Report vulnerabilities privately through GitHub Security Advisories on
`CoolGitOrg/tmplforge`.
