# Phase 0: settle what is not documented

Run against a **non-production** Dynatrace environment. Most probes use
the binary itself (`make spike` runs `scripts/phase0-spike.sh`).

| # | Question | Why it matters | How to test | Default until answered |
|---|---|---|---|---|
| 1 | Does `/api/v2/logs/ingest` accept `Content-Encoding: gzip`? Does the 10 MB limit apply before or after decompression? | 5–10× less egress | `make spike` probe `gzip`; then a 9 MB batch gzipped | `gzip = false` |
| 2 | Does a 429 carry `Retry-After`? | backoff accuracy | flood a test tenant with parallel `ghlog2dt ship` and log response headers (`DYNATRACE`-side) | honoured if present, else exponential backoff |
| 3 | `X-Dynatrace-Attr`: exact encoding (URL-escaped pairs?), merge with body keys, and do header bytes count as billed ingest? | could halve the bill | `ghlog2dt run --repo <fixture> --header-attrs`, then `dql.sh 'fetch logs \| filter log.source=="github-actions" \| limit 5'`; compare `builtin:billing.log.ingest.usage` for equal record counts with and without | `header_attrs = false` |
| 4 | On this tenant, are `severity` and custom keys kept raw (OpenPipeline, raw data model) or flattened/lower-cased (classic pipeline)? Does `severity` populate `loglevel`? | queries and alerts use `loglevel` | look at a shipped record in Notebooks | queries assume `loglevel` |
| 5 | All-stale batch: exact 400 message; partially stale: 200 body format | the client matches the phrase `out of correct time range` | `make spike` probe `stale`; send a mixed batch | phrase match; 200 = partial |
| 6 | Platform token (`Bearer`, `openpipeline:logs:ingest`) on the `.live` URL | Terraform cannot create platform tokens yet; manual alternative | `DT_PLATFORM_INGEST_TOKEN=… make spike` | classic `logs.ingest` token |
| 7 | Job IDs are unique per attempt | dedupe key | re-run a fixture job; `ghlog2dt list` must show the new job ID | assumed (GitHub behaviour) |
| 8 | Does the signed-blob download count against the REST rate limit? | budget estimate | compare `X-RateLimit-Remaining` before/after N log downloads | counted as one call per job |
| 9 | DPL/DQL in the OpenPipeline processors (exit-code parse, token masking) | Terraform does not validate DQL | paste each into the pipeline editor preview with the `sample_data` | pipeline disabled by default |
| 10 | Grail timeseries name of the ingest billing metric | `ingest-cost.dql` | metrics browser | query marked as assumption |

## Fixture repository

Create `CoolGitOrg/ghlog2dt-fixture` with one workflow that prints known
lines, including an error, a warning, ANSI colour, a fake token, a 70 KB
line, and 30,000 lines in one step:

```yaml
name: CI
on: [workflow_dispatch, push]
jobs:
  noise:
    runs-on: ubuntu-24.04
    steps:
      - run: |
          echo "plain line"
          echo "::warning::fixture warning"
          printf '\e[36;1mcolour\e[0m\n'
          echo "token ghp_abcdefghijklmnopqrstuvwxyz0123456789 must be redacted"
          head -c 71680 /dev/zero | tr '\0' x; echo
          seq 1 30000
          echo "::error::fixture failure"; exit 2
```

Expected in Dynatrace (`verify-job.dql`): one ERROR from `::error::`, one
ERROR `Process completed with exit code 2.`, one WARN, no `ghp_` anywhere,
the 70 KB line truncated to 64 KiB with `gh.log.truncated = true`, and a
line count equal to `gh run view --log` minus `##[endgroup]` and blank
lines.

## Record the answers

Write results into this file (date, tenant version, observed behaviour)
and flip the defaults in `ghlog2dt.json` / Terraform accordingly.
