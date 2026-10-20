# Load test

tmplwatch's hot path is not HTTP. It renders a few times a day. The HTTP
surface still matters: Prometheus scrapes `/metrics`, health checkers hit
`/healthz`, and tooling reads `/render`. None of that may disturb rendering.
This document states the targets, the method, and a measured baseline.

## Targets

| Endpoint | Load | Duration | Pass |
|---|---|---|---|
| `GET /render?template=Caddyfile` | 50 req/s | 5 min | p99 < 50 ms, **zero** errors or drops |
| `GET /metrics` | 10 req/s, concurrently | 5 min | p99 < 100 ms, zero errors |
| rendering | KV changes during the run | | `tmplwatch_render_errors_total` unchanged |

Fifty requests per second is about 100 times the realistic rate: a few
scrapers, a health check every 10 seconds, and occasional humans. Ten
`/metrics` per second exercises the `text/template` exposition and the
quantile sort, which are the only per-request work beyond a map lookup.

## Method

```sh
make build
bin/tmplwatch -c config/testdata.json -a 127.0.0.1:9180 &
scripts/loadtest.sh http://127.0.0.1:9180      # RATE=50 DURATION=5m by default
bin/tpl -t scripts/loadtest-summary.md.tmpl -d dist/loadtest/report.json
```

`scripts/loadtest.sh` uses [vegeta] when it is installed and otherwise
`cmd/tmplload`. Both are **open-loop**: requests are issued on schedule
whatever the response time, so a slow server shows up as latency, not as a
lower request rate. `tmplload` measures latency from each request's
*scheduled* start (avoiding coordinated omission), and counts a request as
*dropped* if every worker is busy and the backlog exceeds one second. Both
attacks run at the same time. The script normalises vegeta's JSON to
tmplload's shape, writes `dist/loadtest/report.json`, and exits non-zero
if a target is missed. The `loadtest` workflow does the same on a GitHub
runner and renders the job summary with `tpl`, a Go template, from that
JSON.

## Baseline

Measured during development (go1.24.7, linux/amd64, a shared container,
`tmplload`, 15s run, all nine testdata templates loaded):

| Endpoint | Rate | Requests | Errors | p50 ms | p95 ms | p99 ms |
|---|---:|---:|---:|---:|---:|---:|
| `/render` | 50/s | 750 | 0 | 1.20 | 1.84 | 4.46 |
| `/metrics` | 10/s | 150 | 0 | 1.39 | 2.43 | 3.53 |

Both are an order of magnitude inside the targets. A render of all nine
templates took 0.15 ms at p50 (`tmplwatch_render_duration_seconds`). Run
the full five-minute test on the target hardware before promoting a
release; the numbers above set expectations and are not a substitute.

## Beta plan

1. **Stage** (`scripts/beta-rollout.sh beta-1 beta-2`): copy the release
   binary to one or two hosts, run `tmplwatch -once -dry` against the live
   destinations and read the diff, then render to a scratch directory and
   run the validator (`caddy validate ...`). Nothing changes on the host.
2. **Shadow, 24h**: run the new version with `"dry": true` next to the old
   one, on a different `listen` port. Compare `/status` SHA-256 values with
   the files on disk; they must match.
3. **Canary, 48h** (`scripts/beta-rollout.sh -apply beta-1`): switch one
   host. Watch `tmplwatch_template_incomplete`,
   `tmplwatch_source_polls_total{result="error"}`,
   `tmplwatch_commands_total{result="error"}`, and the application's own
   error rate. Run `loadtest.yml` against it once.
4. **Second host**, then the fleet in batches of 25%. Rollback is the
   previous symlink in `/usr/local/lib/tmplwatch/` and `systemctl restart
   tmplwatch`; configuration files are untouched by a rollback.

Exit criteria for beta: two weeks with no incomplete template other than
those caused by data, no command failures caused by tmplwatch, and the
GitHub API budget below 10% (`tmplwatch_source_polls_total` with
`result="changed"` approximates spent requests; `unchanged` is free).

[vegeta]: https://github.com/tsenart/vegeta
