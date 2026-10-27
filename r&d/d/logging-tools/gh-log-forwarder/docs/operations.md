# Operations

## Prerequisites

| Thing | Where | Notes |
|---|---|---|
| `CoolGitOrg/gh-log-forwarder` | GitHub | private or internal (public repos lose schedules after 60 days of inactivity) |
| `issueops-autoadmin-app` | GitHub App, installed on the org | needs **Actions: read** on target repos; tokens are narrowed to `actions:read` at mint time |
| `ghlog2dt-dispatcher` | second GitHub App | push mode only: **Contents: write**, installed on `gh-log-forwarder` alone |
| Dynatrace API token for Terraform | env `DYNATRACE_API_TOKEN` | `apiTokens.read`, `apiTokens.write` (+ `settings.read/write` for OpenPipeline) |
| Dynatrace OAuth client for Terraform | env `DT_CLIENT_ID`, `DT_CLIENT_SECRET`, `DT_ACCOUNT_ID` | `storage:bucket-definitions:read/write/delete` for the bucket |
| Remote Terraform state | encrypted backend | state contains the ingest token in plain text |

## First deployment

1. **Repository.** Push this tree to `CoolGitOrg/gh-log-forwarder`.
2. **State branch.** `make bootstrap-state` (idempotent; plumbing only).
3. **Dry run.** Locally: `make dry-run REPO=CoolGitOrg/app-a`, or run the
   *forward-logs* workflow by hand with `dry_run: true` and inspect the
   `records-shard-*` artifact.
4. **Terraform.** `cd terraform && terraform apply` — renders the cron,
   sets `GHLOG_*` / `DT_URL` variables and both secrets, creates the
   `logs.ingest` token and the `ci_github_actions_logs` bucket.
5. **Route to the bucket.** Either let Terraform own logs routing
   (`manage_openpipeline_routing`, replaces the whole routing table) or add
   this entry to your central routing config, above the default route:
   - matcher: `matchesValue(log.source, "github-actions") or matchesValue(log.source, "ghlog2dt.self")`
   - target: pipeline `pipeline_ghlog2dt_github_actions`, or any pipeline
     whose storage stage assigns `ci_github_actions_logs`.
6. **Release and pin.** `git tag v1.0.0 && git push --tags`; set
   `ghlog_version = "v1.0.0"`; apply. Workflows now install that release
   and refuse it unless `SHA256SUMS` matches.
7. **Verify.** `scripts/dql.sh -f dynatrace/queries/verify-job.dql` and
   compare a job's line count with `gh run view --log --job <id> | wc -l`.
8. **Alerts.** Build Dynatrace workflows/anomaly detectors from
   `dynatrace/queries/alert-*.dql`.

## Rollout phases

| Phase | Scope | Exit criteria |
|---|---|---|
| 0 — spike (1 week) | non-prod tenant | [phase-0 checklist](phase0-spike.md) answered; batch defaults confirmed |
| 1 — dry run | 1 repo, `--dry-run` | record count matches `gh run view --log` minus dropped noise |
| 2 — live | 1 repo, 15-min cron | heartbeat steady 48 h; no stale drops; bucket routing right |
| 3 — 10 repos | matrix, 4 shards | GitHub `rate_remaining` > 10k; ingest bytes/day as estimated |
| 4 — org-wide | `target_repos = ["*"]` + excludes | per-team `dt.security_context`; cost reviewed after 2 weeks |
| 5 — push mode | latency-sensitive repos | event runs succeed; sweep finds nothing left for those repos |

## Monitoring

Every run ships one `log.source = ghlog2dt.self` record (severity `WARN`
on stale drops, partial batches or abandoned runs; `ERROR` on failures or
rejections) and appends a table to the workflow's step summary.

| Signal | Query | Alert when |
|---|---|---|
| Forwarder silent | `alert-forwarder-silent.dql` | 0 heartbeats for 3 × interval |
| Data quality | `alert-stale-or-rejected.dql` | any stale, rejected, error, or `gh_rate_remaining < 2000` |
| Duplicates | `duplicates.dql` | > 1 % extra records (crash loops) |
| Cost | `ingest-cost.dql` | ingest bytes/day above budget |

GitHub side: the *forward-logs* workflow failing is the first symptom of
an auth or rate problem; subscribe the platform team to workflow failure
notifications for the forwarder repo.

## Runbook

| Symptom | Likely cause | Action |
|---|---|---|
| Exit 4, "authentication" | app key rotated, app uninstalled, DT token expired/revoked | rotate: `terraform apply -replace='dynatrace_api_token.logs_ingest[0]'`; check app installation |
| Exit 5, "rate limit" | huge backlog or a noisy neighbour on the app's 15k/h budget | wait for reset (state is preserved); lower `concurrency`; split repos across more ticks |
| Exit 5, "402" | Dynatrace DPS budget exhausted | raise budget or pause the schedule; nothing retries until it is fixed |
| `stale_dropped > 0` | interval too long, an outage, or jobs longer than ~24 h | shorten `interval_minutes`; consider `stale = "rewrite"` for long jobs |
| Run stays in `pending_runs` | job log 404 (never uploaded), or run genuinely long | it is abandoned after `pending_ttl`; `checkpoint mark` its jobs to stop retries |
| A job crashes the shipper every tick | poison log | `checkpoint` workflow → `mark` with the job ID |
| Need to re-export the last N hours | data lost or mis-routed in Dynatrace | `checkpoint` workflow → `replay` `6h` (max 24h): lowers the high-water mark and forgets jobs shipped since, so the next sweep sends them again. `rewind` alone only picks up jobs never shipped |
| Workflow schedule stopped | repo inactivity (public), or the workflow file was disabled | keep repo private/internal; `gh workflow enable forward-logs` |
| `commit-state` failed to push | branch protection on `forwarder-state` | exempt `github-actions[bot]` or drop protection on that branch |
| Re-runs of old runs missing | known gap of pull mode | enable push mode for that repo |

Manual operations:

```bash
gh workflow run checkpoint -f repo=CoolGitOrg/app-a -f action=show
gh workflow run checkpoint -f repo=CoolGitOrg/app-a -f action=mark   -f value=31200000001
gh workflow run checkpoint -f repo=CoolGitOrg/app-a -f action=replay -f value=6h
gh workflow run forward-logs -f repos=CoolGitOrg/app-a -f dry_run=true
gh workflow run forward-logs -f lookback=6h          # one wider sweep after an outage
```

## Failure modes

| Failure | Detection | Built-in handling |
|---|---|---|
| Scheduled tick delayed or dropped | heartbeat gap | overlap lookback + pending set; next tick catches up |
| Job log 404 just after completion | 404 on `/logs` | 3 retries, then run stays pending |
| Signed log URL expired (~1 min) | 403/404 from blob host | the request restarts from the API URL and gets a fresh redirect |
| ≥ 1000 runs in a window | `total_count ≥ 1000` | window halved recursively |
| GitHub primary limit | `X-RateLimit-Remaining: 0` | sleeps until reset if ≤ 15 min, else exit 5 |
| GitHub secondary limit | `Retry-After` / message | waits as told, or ≥ 60 s with backoff |
| Dynatrace 413 | status | batch halved recursively; lone record → rejected |
| Dynatrace 400 (one bad record) | status | batch bisected; lone record → rejected |
| Dynatrace 429/502/503/504 | status | exponential backoff + jitter, honours `Retry-After` if sent |
| Dynatrace all-stale 400 | message | counted as `stale`, exit 3 |
| Dynatrace 402 | status | exit 5, no retry |
| Crash between POST and state push | duplicates | at-least-once; `dedupe.dql` |
| Two writers push state | non-fast-forward | reset + convergent merge + retry ×5 |
| Logs expired at GitHub | gap longer than retention | not recoverable; alert on heartbeat gaps |

## Cost and volume

Dynatrace bills log ingest on raw bytes received
(`builtin:billing.log.ingest.usage`), so attribute overhead counts.

Worked example (assumptions, not measurements; list prices — your DPS
contract governs): 1,000 jobs/day × 400 KB raw log text ≈ 0.4 GiB/day of
content. JSON envelope + per-line job attributes inflate that 2–4×:
≈ 1.2 GiB/day ingest × $0.20/GiB ≈ **$0.24/day (~$7/month)**; 35-day
retention ≈ 42 GiB × $0.0007/GiB-day ≈ $0.03/day.

Levers, biggest first:

1. `dt.header_attrs = true` — job attributes once per request instead of
   on every line (verify billing treatment in phase 0).
2. `drop_noise` (default on) and `workflows` filters for chatty pipelines.
3. Shorter bucket retention.
4. Log-based metrics in OpenPipeline for dashboards instead of repeated
   `fetch logs | summarize` scans.

GitHub side: each sweep costs about one list call per repo per tick plus
two calls per job (jobs list amortised, log redirect). At the default
15-minute cadence, 200 repos and ~1,000 jobs/hour use roughly 2,800
requests/hour of the installation's 15,000/hour.
