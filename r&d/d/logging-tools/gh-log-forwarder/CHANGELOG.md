# Changelog

## v1.0.0 — 2026-10-27

First release.

- `ghlog2dt` (Go ≤ 1.24, standard library only): `run`, `list`, `fetch`,
  `ship`, `checkpoint`, `token`, `schedule`, `version`.
- GitHub client: Link pagination, primary/secondary rate-limit handling,
  automatic window split above GitHub's 1000-result cap, token never
  forwarded across the job-log redirect, App JWT + installation tokens.
- Log parsing: per-line timestamps, `##[error]`/`##[warning]`/`##[debug]`
  severities, step attribution, ANSI stripping, secret redaction, line and
  log truncation, UTF-8 repair.
- Dynatrace shipper: byte/record-bounded batches, 413 and 400 bisection,
  429/502/503/504 backoff, all-stale and partial-success accounting,
  optional gzip and `X-Dynatrace-Attr`.
- Checkpoints per repository with convergent merge; pending runs;
  replay via `checkpoint rewind --forget`.
- Workflows: scheduled sharded sweep, `repository_dispatch` receiver,
  checkpoint operations, CI, release with SHA256SUMS and provenance.
- Terraform: managed cron/lookback from `interval_minutes`, Actions
  variables and secrets, `logs.ingest` token, Grail bucket, optional
  OpenPipeline pipeline and routing, optional push-mode notifiers.
