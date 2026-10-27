# Configuration

Precedence, lowest to highest: **defaults → JSON config file → environment
→ flags**. Secrets are environment-only: they never appear as flags
(visible in `ps`) and `--print-config` elides them.

```bash
ghlog2dt run --print-config          # effective configuration, secrets elided
```

## Environment

| Variable | Used by | Meaning |
|---|---|---|
| `GH_TOKEN` (fallback `GITHUB_TOKEN`) | run, list, fetch | GitHub token with Actions: read (installation token) |
| `DT_LOGS_TOKEN` | run, ship | Dynatrace token with `logs.ingest` (or platform token with `openpipeline:logs:ingest` and `DT_AUTH_SCHEME=Bearer`) |
| `DT_URL` | run, ship | `https://abc12345.live.dynatrace.com`, or an Environment ActiveGate `https://ag:9999/e/abc12345` |
| `DT_AUTH_SCHEME` | run, ship | `Api-Token` (default) or `Bearer` |
| `GHLOG_CONFIG` | all | config file (explicit: must exist) |
| `GHLOG_TARGET_REPOS` | run, list | comma/space/newline list of `owner/name`; `#` comments allowed |
| `GHLOG_LOOKBACK` | run, list | overlap window, e.g. `90m` |
| `GHLOG_STATE_DIR` | run, list, checkpoint | checkpoint directory |
| `GHLOG_CONCURRENCY` | run, list, fetch | parallel log downloads |
| `GHLOG_GZIP` | run, ship | `true` to gzip bodies (unverified, see phase-0) |
| `GITHUB_API_URL` | all GitHub calls | set automatically on runners (GHE.com / GHES aware) |
| `GITHUB_STEP_SUMMARY` | run | Markdown summary is appended when set |
| `GH_APP_ID` / `GH_APP_CLIENT_ID`, `GH_APP_PRIVATE_KEY_FILE` / `GH_APP_PRIVATE_KEY` | token | App credentials |

## Config file (`ghlog2dt.json`)

Unknown keys are an error (typos fail loudly). Every key is optional.

| Key | Default | Meaning |
|---|---|---|
| `owner` | `CoolGitOrg` | org for `token` lookups |
| `targets` | `[]` | repos to sweep |
| `all` | `false` | sweep every repo the installation token can see (archived skipped) |
| `include` / `exclude` | `[]` | case-insensitive globs on `owner/name` (`CoolGitOrg/*-archive`) |
| `workflows` | `[]` | only runs whose workflow name or path matches a glob |
| `lookback` | `90m` | overlap behind the high-water mark |
| `max_age` | `23h45m` | hard floor of the window; lines older are stale (≤ 24h) |
| `pending_ttl` | `12h` | give up on runs unfinished this long |
| `shipped_ttl` | `48h` | remember shipped job IDs this long (must be ≥ `max_age`) |
| `state_dir` | `state` | checkpoint directory |
| `github_api` | `https://api.github.com` | REST base URL |
| `github_api_version` | `2022-11-28` | `X-GitHub-Api-Version` |
| `concurrency` | `4` | parallel downloads (1–16; mind secondary limits) |
| `max_line_bytes` | `65536` | line truncation |
| `max_log_bytes` | `33554432` | job log truncation (a marker record says so) |
| `stale` | `drop` | `drop` or `rewrite` |
| `drop_noise` | `true` | drop `##[endgroup]` and blank lines |
| `strip_ansi` | `true` | remove colour escapes |
| `self_log` | `true` | ship one `log.source=ghlog2dt.self` record per run |
| `attributes` | `{"log.source":"github-actions"}` | static attributes on every record |
| `redact` | built-in list | regexps replaced by `[REDACTED]`; setting it replaces the defaults |
| `dt.url` | — | ingest environment |
| `dt.auth_scheme` | `Api-Token` | or `Bearer` |
| `dt.batch_bytes` | `4194304` | per request (Dynatrace max 10 MB) |
| `dt.batch_records` | `5000` | per request (Dynatrace max 50,000) |
| `dt.gzip` | `false` | `Content-Encoding: gzip` |
| `dt.header_attrs` | `false` | job attributes once per request in `X-Dynatrace-Attr` |
| `dt.timeout` | `60s` | per request |
| `dt.max_retries` | `5` | on 429/502/503/504 and transport errors |

See [`examples/ghlog2dt.full.json`](../examples/ghlog2dt.full.json).

## Flags

`ghlog2dt <command> -h` lists them. The important ones for `run`:

| Flag | Meaning |
|---|---|
| `--repo owner/name` | repeatable or comma separated; replaces file/env targets |
| `--all`, `--include`, `--exclude`, `--workflow` | targeting |
| `--run-id N [--run-attempt K]` | event mode: one run (one repo), high-water untouched |
| `--dry-run [--records-out F]` | write records as NDJSON, ship nothing, save no state |
| `--summary F` | JSON summary destination (`-` = stdout) |
| `--fail-on-stale` | exit 3 when any line was too old |
| `--lookback`, `--max-age`, `--stale`, `--concurrency` | as config |
| `--dt-url`, `--dt-auth`, `--gzip`, `--header-attrs`, `--batch-bytes`, `--batch-records` | as config |
| `--attr key=value` | extra static attribute; repeatable |
| `--log-format json\|text`, `--log-level` | stderr logging |

## Exit status

| Code | Meaning | Workflow wrapper (`run-forwarder.sh`) |
|---|---|---|
| 0 | success | success |
| 1 | some repository failed | fail |
| 2 | usage or configuration | fail |
| 3 | partial: Dynatrace 200-partial, rejected or all-stale batches | **warning**, success |
| 4 | authentication (GitHub 401, Dynatrace 401/403) | fail |
| 5 | GitHub rate limit resets > 15 min away, or Dynatrace 402 | fail; state preserved |
| 6 | checkpoint read/write | fail |

Authentication, budget and cancellation stop a multi-repo run at once;
other per-repository errors are recorded and the run moves on.

## Frequency: one knob

`interval_minutes` ∈ {5, 6, 10, 12, 15, 20, 30, 60} decides:

- the cron line (`minute_offset` keeps ticks off `:00`, when GitHub's
  scheduler is busiest and may delay or drop runs), and
- `GHLOG_LOOKBACK = max(2 × interval, 60m) + 30m`.

| Interval | Cron (offset 7) | Lookback | Typical latency | Notes |
|---|---|---|---|---|
| 5 | `2,7,12,…,57 * * * *` | 90m | 5–20 min | most runner minutes, most state commits |
| **15** | `7,22,37,52 * * * *` | 90m | 15–30 min | default |
| 30 | `7,37 * * * *` | 90m | 30–45 min | |
| 60 | `7 * * * *` | 150m | 60–90 min | upper end of comfort |

Longer than an hour is not offered: one delayed or dropped tick would then
push lines toward Dynatrace's 24 h limit. For sub-interval latency use
push mode rather than a shorter cron.

The workflow's `schedule` cannot read Actions variables, so the cron is
rendered into the file. Terraform and `ghlog2dt schedule --rewrite` touch
only the line tagged `# ghlog2dt:cron`.

## Targeting recipes

```bash
ghlog2dt run --repo CoolGitOrg/app-a                                # one
ghlog2dt run --repo CoolGitOrg/app-a,CoolGitOrg/app-b               # several
GHLOG_TARGET_REPOS="$(cat examples/targets.txt)" ghlog2dt run       # from a file
ghlog2dt run --all --exclude 'CoolGitOrg/*-archive'                 # everything the app sees
ghlog2dt run --repo CoolGitOrg/app-a --workflow CI --workflow 'Release*'
```

In the workflows, `GHLOG_TARGET_REPOS = "*"` runs a single `--all` shard;
a list is spread round-robin over `GHLOG_SHARDS` matrix jobs.
