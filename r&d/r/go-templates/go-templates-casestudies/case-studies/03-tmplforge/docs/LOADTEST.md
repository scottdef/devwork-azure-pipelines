# Load test

## SLO

**`POST /render` p99 < 150 ms at 20 requests/second sustained for 5
minutes, with 100% 2xx, measured at the client and including the sandbox
process spawn.**

Every render forks and execs a child (see [DESIGN.md](DESIGN.md#2-trust-model-and-the-sandbox)).
That spawn dominates the cost, so the SLO is stated with it included.

## Plan

| | |
|---|---|
| Tool | [vegeta](https://github.com/tsenart/vegeta) when installed (open-loop, constant rate), else `cmd/forgeload` (stdlib, also open-loop) |
| Target | `tmplforge serve` on loopback, `-rate 0` (the limiter is not under test), default `-j` |
| Request | `POST /render` with `HX-Request: true`, form `template=<catalog/motd.txt.tmpl>&fixture=<its fixture>&format=txt` |
| Rate / duration | 20 req/s for 5 min (6000 requests) |
| Pass | p99 < 150 ms, success = 100%, zero transport errors |
| Also watch | `tmplforge_sandbox_inflight` (should hover near rate × latency, about 0.2), `tmplforge_renders_total{outcome!="ok"}` (should be 0), RSS of the server (flat) |

The motd template exercises printf, dateFmt, plural, with, range and
indent, which is representative of a real preview, and it renders in well
under a millisecond. The measurement is therefore the sandbox's fixed
cost, which is what the SLO is about.

### Running it

```sh
make loadtest                              # 20 req/s, 5 m, starts its own server
RATE=50 DURATION=1m scripts/loadtest.sh    # heavier, shorter
TARGET=https://forge.example TOKEN=... scripts/loadtest.sh   # an existing deployment
```

With vegeta, `scripts/loadtest.sh` runs the equivalent of:

```sh
printf 'template=%s&fixture=%s&format=txt' "$(jq -sRr @uri < catalog/motd.txt.tmpl)" \
    "$(jq -sRr @uri < catalog/fixtures/motd.txt.json)" > body
echo "POST http://127.0.0.1:18088/render" |
  vegeta attack -rate=20 -duration=5m -body=body \
    -header 'Content-Type: application/x-www-form-urlencoded' -header 'HX-Request: true' |
  tee results.bin | vegeta report
vegeta report -type=json results.bin > loadtest.json
```

Without vegeta it runs `bin/forgeload -r 20 -d 5m -p99 150 -json` and
renders the JSON through `scripts/loadtest-report.md.tmpl` with
`tmplforge render`. That is the same report the `loadtest` workflow
writes to `$GITHUB_STEP_SUMMARY`. `hey` is supported with `USE_HEY=1`, but
it is closed-loop (each worker waits for its response), so it hides
queueing delay and is not the default.

### Why open-loop

A closed-loop tool slows down when the server slows down, so the
latencies it reports omit the requests that would have queued. This is
coordinated omission. vegeta and forgeload fire on a ticker regardless of
outstanding requests, so server slowness shows up as latency.

## Results in this build environment

These numbers come from `scripts/loadtest.sh` and `forgeload` against a
loopback server in the case-study sandbox (2 vCPU, Go 1.24.7, linux/amd64, default `-j 4`).
These were short runs. Run the full five minutes on the target hardware.

| Run | Requests | 2xx | p50 | p95 | p99 | max |
|---|---:|---:|---:|---:|---:|---:|
| 20 req/s × 10 s | 200 | 100% | 7.3 ms | 9.6 ms | 11.4 ms | 15.0 ms |
| 20 req/s × 5 s | 100 | 100% | 6.7 ms | 8.1 ms | 8.5 ms | 9.5 ms |
| 20 req/s × 3 s | 60 | 100% | 6.4 ms | 8.4 ms | 10 ms | 10 ms |

The p99 is about 13× under the SLO. The headroom is wide enough that the
5-minute run is a regression gate, not a capacity test. For capacity,
raise `RATE` until p99 crosses 150 ms. On this box the limit is set by
fork/exec throughput, roughly NumCPU / 7 ms.

## Fallback: worker pool

If spawn cost ever breaks the SLO (slow `fork` on a large-RSS parent, a
container runtime with expensive exec, or Plan 9 on slow hardware), the
plan is a **pre-forked worker pool** that keeps the process boundary:

1. At start, `serve` launches `-j` long-lived children:
   `tmplforge render -sandbox -worker`.
2. The protocol over each child's stdin/stdout is a netstring-framed
   request (`format`, `delims`, `template`, `fixture`) and response
   (`status-tag`, `output`). It stays text, in keeping with everything
   else.
3. A worker renders **one** request at a time under the same caps. After
   each request it exits if its heap grew past a threshold, or after N
   requests (default 100), and the pool replaces it. Recycling bounds
   leaks and limits how much one hostile template can learn about the
   previous one.
4. A timeout, cap breach or crash kills the worker. The parent replaces
   it and returns the same classified error as today.
5. `Sandbox` becomes an interface with two implementations, `Exec`
   (today's) and `Pool`, selected by `serve -pool`. The tests run against
   both.

Expected effect: removes about 5 ms of fork/exec from the median and most
of the tail. The cost is a more complex supervisor and state shared
between consecutive renders in one worker, which is why it is the
fallback and not the default.

Cheaper mitigations to try first:

- Raise `-j` if `tmplforge_sandbox_inflight` is pinned at the cap and
  latency is queueing rather than execution.
- Run behind a cache keyed by (template, fixture, format) hash for
  repeated previews. htmx's `keyup changed` already suppresses identical
  consecutive requests.
- Lower `GOMAXPROCS` in the child (already 2). Runtime start-up cost
  scales mildly with it.
