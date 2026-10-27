# Architecture

## Pieces

| Piece | What it is | Why it is shaped this way |
|---|---|---|
| `ghlog2dt` | One static Go binary, stdlib only | Runs the same on a laptop, a runner, or a cron box; nothing to install but itself |
| `forward.yml` | Scheduled sweep: plan → N shards → commit state | The authoritative path; heals any missed event |
| `forward-event.yml` | `repository_dispatch` receiver for one run | Optional latency cut from one interval to about a minute |
| `notify-forwarder.yml` | Per-target-repo `workflow_run` notifier | Only needed for push mode |
| `forwarder-state` branch | One JSON checkpoint per target repo | Durable, diffable, auditable, no extra infrastructure |
| Terraform | GitHub vars/secrets/cron + Dynatrace token/bucket/pipeline | One variable sets the frequency everywhere |

## Data flow of one sweep, per repository

```mermaid
sequenceDiagram
  autonumber
  participant W as forward.yml shard
  participant G as GitHub REST
  participant B as signed blob URL
  participant D as Dynatrace ingest
  participant S as state file
  W->>S: load owner__repo.json, prune expired IDs
  W->>G: GET /actions/runs?created=from..now (split if ≥1000)
  W->>G: GET /actions/runs/{id} for each pending run
  loop each completed run
    W->>G: GET /actions/runs/{id}/jobs?filter=all
    par up to --concurrency, in order
      W->>G: GET /actions/jobs/{job}/logs
      G-->>W: 302 Location: signed URL (Authorization dropped)
      W->>B: GET log text
    end
    W->>W: parse → records (redact, severity, step, stale rule)
    W->>D: POST /api/v2/logs/ingest (≤4 MiB, ≤5000 records)
    D-->>W: 204 / 200 partial / 400 stale / 413 split / 429 retry
    W->>W: mark job shipped once its last record's request completed
  end
  W->>S: high_water = sweep start; save atomically
```

`commit-state` then merges every shard's files into the branch.

## Correctness rules

1. **Dedupe key is the job ID.** Each attempt of a run has its own job IDs,
   and `jobs?filter=all` returns every attempt, so a re-run ships its new
   jobs once and never re-ships the old ones. Records carry
   `gh.run_attempt`.
2. **Only completed jobs ship.** A run listed while queued or in progress
   joins the *pending set*; every sweep re-polls pending runs with
   `GET /runs/{id}` until they complete or `pending_ttl` (12 h) passes. A
   completed job whose log still 404s keeps its run pending too.
3. **Checkpoint after delivery.** The shipper hands out sequence numbers:
   a job is marked shipped only when the request carrying its last record
   has completed. State is saved after the sweep. A crash between POST and
   save duplicates at most the in-flight jobs — at-least-once delivery.
   Every record has `cicd.pipeline.task.run.id` + `gh.line`, a natural
   dedupe key in DQL.
4. **The window self-heals.** A sweep lists runs created in
   `[high_water − lookback, now]`, floored at `now − max_age`. A missed
   tick just makes the next window longer; a long outage is bounded by
   Dynatrace's 24 h limit, not by the forwarder.
5. **Stale lines never reach Dynatrace.** Lines older than `max_age`
   (23h45m) are dropped and counted (`stale_dropped`), because one stale
   record would otherwise make Dynatrace reject — or partially reject —
   the batch. `--stale=rewrite` stamps them with ingest time and keeps
   the original in `gh.log.timestamp_original`.
6. **One bad record cannot poison a batch.** A 413 or a 400 splits the
   batch in half recursively; a record refused on its own is counted as
   `rejected` and skipped.
7. **Order is preserved under concurrency.** Logs are downloaded in
   parallel but consumed in job order by a single goroutine; the download
   semaphore is acquired in that same order, so it cannot deadlock and
   memory is bounded to `concurrency` logs.

### Known gap

Pull mode lists runs by `created`. A **re-run** of a run created before
the window is invisible to the sweep (GitHub offers no `updated` filter).
Push mode covers it: `workflow_run: completed` fires for re-runs with the
new `run_attempt`. Without push mode, re-runs of old runs are not
exported.

## Record schema

Dynatrace bills raw bytes received, so the schema is deliberately lean and
job-level attributes can move into one request header (`--header-attrs`).

Per line:

| Field | Example | Notes |
|---|---|---|
| `timestamp` | `2026-10-27T10:15:06.0000000Z` → RFC 3339 nano | from the log line; else carried from the previous line |
| `content` | `Process completed with exit code 2.` | marker stripped, ANSI stripped, secrets `[REDACTED]`, ≤ 64 KiB |
| `severity` | `ERROR` `WARN` `INFO` `DEBUG` | from `##[error]`, `##[warning]`, `##[debug]`; default INFO |
| `gh.line` | `16` | 1-based line number in the job log |
| `gh.step.number`, `gh.step.name` | `3`, `Run make build` | latest step started at or before the line (1 s resolution) |
| `gh.log.marker` | `group` `command` `error` … | only when the line had one |
| `gh.log.truncated` | `true` | line or log cut at the limit |
| `gh.log.timestamp_original` | | only with `--stale=rewrite` |

Per job (inline, or in `X-Dynatrace-Attr` with `--header-attrs`):

| Field | Source |
|---|---|
| `log.source` = `github-actions` | config `attributes` |
| `dt.security_context` = `ci` | config `attributes` |
| `vcs.repository.name` | `owner/name` |
| `vcs.ref.head.name`, `vcs.ref.head.revision` | run head branch / SHA |
| `cicd.pipeline.name` | workflow name |
| `cicd.pipeline.run.id` | run ID (string) |
| `cicd.pipeline.task.name`, `cicd.pipeline.task.run.id` | job name / job ID (string) |
| `cicd.pipeline.result` | job conclusion |
| `gh.run_attempt` | attempt number |
| `gh.event`, `gh.actor`, `gh.runner.name`, `gh.workflow.path`, `gh.job.url` | run / job |

Names follow the OpenTelemetry CI/CD and VCS semantic conventions where one
exists. One self-monitoring record per run uses `log.source =
ghlog2dt.self` and `ghlog2dt.*` counters.

## Checkpoint model

`state/<owner>__<repo>.json` on the `forwarder-state` branch:

```json
{
  "version": 1,
  "repo": "CoolGitOrg/app-a",
  "high_water": "2026-10-27T12:00:00Z",
  "pending_runs": { "8812345678": "2026-10-27T11:50:00Z" },
  "shipped_jobs": { "31200000001": "2026-10-27T12:00:03Z" },
  "updated_at": "2026-10-27T12:00:05Z"
}
```

- One file per repository: matrix shards never write the same file.
- `Merge` is max(`high_water`) and union(`pending`, `shipped`) with the
  earliest timestamps — commutative, associative, idempotent — so
  concurrent writers (shards, event jobs) converge, and a push race is
  fixed by resetting to the remote and merging again.
- `shipped_ttl` (48 h) must exceed `max_age`: a job ID is remembered for
  as long as its run could still appear in a window.
- Saves are atomic (temp file, fsync, rename). File names are
  whitelisted (`[A-Za-z0-9-]+__[A-Za-z0-9._-]+.json`); `os.Root` would
  confine writes further but `Root.Rename` only exists from Go 1.25.
- `checkpoint rewind` lowers `high_water`, which a merge cannot do, so the
  `checkpoint` workflow writes with replace semantics under the sweep's
  concurrency group.

## Why these choices

| Choice | Instead of | Reason |
|---|---|---|
| Pull sweep as source of truth | `workflow_run` only | `workflow_run` needs a file in every repo on the default branch and chains at most 3 levels; a schedule needs nothing in target repos and self-heals |
| Log Monitoring API v2 | OTLP logs | OTLP is protobuf-only on that endpoint: an encoder would break the stdlib rule and buy nothing |
| `/api/v2/logs/ingest` | `/platform/ingest/v1/…` | the platform ingest family covers events, SDLC and security events, not logs; v2 logs ingest is an OpenPipeline source |
| Classic `logs.ingest` token | platform token | creatable and rotatable in Terraform today; the endpoint also accepts `Bearer` platform tokens (`--dt-auth Bearer`) |
| JSON array batches | NDJSON body | documented, simplest for partial-success semantics |
| Git branch for state | cache / artifacts / variables | durable, auditable, mergeable, free; caches evict, artifacts expire, variables are size-capped |
| Go stdlib | go-github, cobra, SDKs | project rule; the API surface is six endpoints |
