# Load testing

Two different questions, two different tests:

| Question | Test | Bottleneck |
|---|---|---|
| Can readers hammer the catalog? | `scripts/loadtest.sh`: constant-rate GETs against Pages | GitHub's CDN (Fastly). Effectively never us. |
| How fast does a request become a page, and how many per hour? | `scripts/beta-run.sh`: real issues end to end | Actions queueing + the serialized Pages deploy |

## Read load: `scripts/loadtest.sh`

```sh
BASE=https://coolgitorg.github.io/issueops-library/ RATE=50 DURATION=2m scripts/loadtest.sh
scripts/loadtest.sh -local          # same, against site-sample/ served on 127.0.0.1
```

It uses **vegeta** if installed, then **hey**, then the repo's own **`cmd/siteload`**
(standard library only). All three request the same three paths in rotation:

- `/` (the catalog: the largest page, with an inline ledger table)
- `/items/payments-api/caddyfile/42/` (one item page; override with `ITEM=`)
- `/ledger/open.html` (the htmx fragment every catalog view refreshes)

`siteload` is an *open-model* generator: it sends on a fixed ticker regardless of how
long responses take, so slow responses show up as latency rather than as a lower
request rate (the coordinated-omission trap of closed-loop tools). It reports p50, p90,
p99, max, the status code histogram, and p99 per path, then exits 1 if p99 exceeds
`-max-p99` (default 1.5 s) or errors exceed `-max-err` (default 1%).

### Targets

| Metric | Target | Why |
|---|---|---|
| p99, catalog/item/fragment | < 1.5 s at 50 req/s | a global CDN edge hit is ~20-80 ms; misses go to origin |
| errors | < 1% | Pages returns 429 or 503 under abuse-level load |
| achieved rate | ≥ 98% of requested | otherwise the generator, not the site, is the limit |

### A reference run (local)

Against `site-sample/` served by `siteload -serve` on the build machine:

```
target    http://127.0.0.1:8798/ (3 paths) at 50 req/s for 5s
requests  250 (50.0/s achieved), 0 errors, 1019527 bytes
latency   p50 700µs  p90 900µs  p99 1.3ms  max 4.7ms
status    200 x 250
```

That measures the generator and the HTML size (about 4 KB per request), not GitHub. Run
the real thing from outside GitHub's network before declaring a number.

### Etiquette

GitHub Pages is a shared service with a soft limit of 100 GB/month bandwidth and
abuse protection. 50 req/s for 2 minutes is 6,000 requests and ~25 MB: fine. Do not run
sustained high-rate tests against Pages; if you need more, test `-local` or a copy of
`site-sample/` on your own static host.

## Write path: request to page

The write path is bounded by GitHub, not by our binaries (rendering an item and
rebuilding the site takes tens of milliseconds; `go test` measures that). Per request:

| Stage | Typical |
|---|---|
| Issue opened → intake job starts | 5-30 s (runner queue) |
| build + parse + validate + token + dispatch | 30-60 s (mostly `setup-go` and `go build`) |
| dispatch → publish job starts | 5-30 s |
| apply (build, render, commit, push) | 30-60 s |
| deploy (upload + deploy-pages) | 30-120 s, serialized |
| CDN refresh → notify sees the new `library.json` | 0-60 s |
| **total** | **about 2-5 minutes** |

Throughput: deploys are serialized, but each deploy publishes everything committed before
it starts, so a burst of N requests costs about one deploy for the burst, not N.
Apply runs in parallel and resolves push races by re-applying (`apply-event.sh`, up to 5
attempts with jittered backoff). A practical ceiling is set by the API budget, below.

### API budget per request

| Run | Calls |
|---|---|
| intake | token mint 1, dispatch 1, comment 1, labels 1-2 |
| library apply | token mint 1, git fetch/push 2-3 |
| library notify | token mint 1, comment 1, label 1, close 1-2 |
| **total** | **~12** |

An App installation token gets at least 5,000 requests per hour (more on Enterprise
Cloud), so the REST budget allows hundreds of requests per hour. The tighter limits are
GitHub's **secondary** limits on content creation (issues and comments): keep writes at
least 1 s apart and avoid bursts of more than a few dozen comments per minute. `dispatch`
paces itself at 1 s; `beta-run.sh` opens one issue every 10 s by default.

Measure it: [BETA.md](BETA.md) and `scripts/beta-run.sh -n 10 -i 10`.
