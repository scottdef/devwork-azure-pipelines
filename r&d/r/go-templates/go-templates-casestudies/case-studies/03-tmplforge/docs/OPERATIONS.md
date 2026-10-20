# Operations

## Build

```sh
make                 # vet, test, build bin/tmplforge and bin/forgeload
make cross           # dist/tmplforge-{linux-amd64,linux-arm64,darwin-arm64,plan9-amd64}
make release         # cross + dist/SHA256SUMS
mk cross             # on Plan 9 (rc recipes)
docker build -t tmplforge --build-arg VERSION=v0.1.0 .
```

All builds are offline (`GOFLAGS=-mod=mod GOPROXY=off`). The only
dependency is vendored. Binaries are static (`CGO_ENABLED=0`) and carry
their version in `tmplforge version`.

## Deploy

### systemd (recommended)

```sh
install -m 0755 dist/tmplforge-linux-amd64 /usr/local/bin/tmplforge
install -m 0644 deploy/systemd/tmplforge.service /etc/systemd/system/
systemctl daemon-reload && systemctl enable --now tmplforge
curl -fs http://127.0.0.1:8088/healthz
```

The unit runs as a `DynamicUser` on loopback with a read-only system,
`PrivateTmp` (the sandbox's fixture hand-off goes there), `TasksMax=256`
and `MemoryMax=1G` bounding the whole process tree including sandbox
children, a syscall allow-list, and `KillSignal=SIGINT`, which triggers a
graceful shutdown that gives in-flight renders 10 s.

Put TLS in front with a reverse proxy. `tmplforge catalog -src Caddyfile`
is a working starting point.

### Off loopback

```sh
echo "TMPLFORGE_TOKEN=$(head -c 24 /dev/urandom | base64)" > /etc/tmplforge/env
# then change ExecStart's -a to 0.0.0.0:8088
```

`serve` refuses a non-loopback address without a token of at least 16
characters and a positive `-rate`. Give users the URL `/?token=...` once.
It sets an HttpOnly cookie.

### Container

```sh
docker run --rm -p 8088:8088 -e TMPLFORGE_TOKEN=... --read-only --tmpfs /tmp tmplforge
```

The image is `scratch` plus the binary and a writable `/tmp`. Use
`--tmpfs /tmp` with `--read-only`.

### Plan 9

Copy `tmplforge-plan9-amd64` to `/amd64/bin/tmplforge`. `tmplforge sh`
works in rio and acme `win`. `serve` works too: it binds loopback by
default, the shutdown note is `interrupt`, and the sandbox re-execs
`os.Executable()` as everywhere else.

## Configuration

| Flag / env | Default | Notes |
|---|---|---|
| `-a` | `127.0.0.1:8088` | non-loopback needs `-token` and `-rate` |
| `-token` / `TMPLFORGE_TOKEN` | none | 16+ characters |
| `-rate` | 10 | per-IP req/s, burst 2× |
| `-timeout` | 2s | per render, at most 30s |
| `-j` | 2×NumCPU | concurrent sandbox children |
| `-assets DIR` | none | overlay for `/static/`, e.g. `DIR/echarts/echarts.min.js` |
| `-q` | off | suppress the request log |
| `SOURCE_DATE_EPOCH` | unset | makes `now` deterministic (passed to children) |
| `TZ` | unset | passed to children for `dateFmt` |

## Observe

`/metrics` (Prometheus text format) exposes:

| Metric | Type | Use |
|---|---|---|
| `tmplforge_build_info{version,goversion}` | gauge | inventory, toolchain audit |
| `tmplforge_http_requests_total{route,code}` | counter | traffic, error ratio |
| `tmplforge_renders_total{outcome}` | counter | `ok`, `template_error`, `timeout`, `output_limit`, `template_limit`, `crash`, `error` |
| `tmplforge_render_seconds` | histogram | SLO. Buckets include 0.15 |
| `tmplforge_sandbox_inflight` | gauge | saturation against `-j` |
| `tmplforge_rate_limited_total` | counter | abuse, or a rate set too low |
| `tmplforge_csrf_rejected_total` | counter | misconfigured proxy (stripping Origin) or attack |
| `tmplforge_auth_failed_total` | counter | scanning |

Alert rules ship in the catalog: `tmplforge catalog -src prometheus-rules.yml`.
The rendered golden is `testdata/golden/catalog/prometheus-rules.yml`, with
p99 > 150 ms for 10 m as a warning and down for 5 m as a page.

`/chart` shows p50/p95 per window from the in-memory ring buffer (last
4096 renders), useful when Prometheus is not wired up. The request log
(stderr, one line per request: method, path, status, duration) goes to the
journal under systemd.

## Runbook

| Symptom | Check | Action |
|---|---|---|
| Previews say "server busy" (503) | `tmplforge_sandbox_inflight` at `-j` | Raise `-j` if CPU allows. Look for one client hammering (`rate_limited_total`) |
| Many `timeout` outcomes | which templates (request log, user report) | Expected for runaway templates. If legitimate templates time out, raise `-timeout` (at most 30s) |
| `crash` outcomes | `journalctl -u tmplforge`, renders of deep recursion | Expected for recursive templates. Repeated crashes of simple templates point to memory pressure: check `MemoryMax` / `GOMEMLIMIT` |
| 403 on every POST behind a proxy | `csrf_rejected_total` rising | The proxy rewrites `Host` or drops `Origin`/`Sec-Fetch-Site`. Preserve `Host`, or set the upstream `Host` to the public name |
| `/chart` shows only a table | page notice | Install echarts with `scripts/fetch-echarts.sh`, then rebuild or use `-assets` |
| p99 above SLO | `render_seconds` histogram, host load | See [LOADTEST.md](LOADTEST.md), "Fallback: worker pool" |

## Upgrade and rollback

Binaries are self-contained, and the GUI holds no state. To upgrade:
replace the binary, run `systemctl restart tmplforge`, and check
`/healthz` and `tmplforge version`. Roll back the same way. Releases are
GitHub releases with `SHA256SUMS`. Verify with
`sha256sum -c --ignore-missing SHA256SUMS`.

## Repository governance

`terraform/` manages the GitHub repository: the ruleset on main (PR,
approvals, required CI checks, linear history), the `GO_VERSION` Actions
variable, and the `beta` environment that gates `loadtest.yml`. Change
the toolchain by setting `go_version` and applying, not by editing
workflows.
