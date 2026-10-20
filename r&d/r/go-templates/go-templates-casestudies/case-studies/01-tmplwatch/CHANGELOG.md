# Changelog

All notable changes to tmplwatch. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow
[Semantic Versioning](https://semver.org/).

## [Unreleased]

## [0.1.0-beta.1] - 2026-10-19

### Added

- `tmplwatch` daemon: poll a key/value source, render `text/template` files,
  write atomically (temp, fsync, chmod, rename, optional `.bak`), skip
  identical content, repair mode drift, clean stale temp files at start.
- Sources: `github` (git trees API with `If-None-Match`, a content-addressed
  blob cache, rate-limit backoff) and `dir`.
- GitHub App authentication with a stdlib-only RS256 JWT; installation
  tokens scoped to one repository with `contents: read`.
- Dependency tracking: only templates whose recorded keys or prefixes
  changed are rendered; renders with missing keys are incomplete and
  write nothing.
- 45 template functions, a consul-template-compatible subset plus helpers,
  including stdlib `toYAML` and `toHCL` emitters and a `sockaddr` subset.
  `docs/FUNCS.md` is generated from the code, and its examples are tests.
- Commands as argv with `wait` (debounce), `splay` and timeout; exec
  supervisor with change signal or restart, and backoff.
- `-once` with exit codes, `-dry` with unified diffs, `-healthcheck`,
  `-pprof` (loopback only), SIGHUP reload. Plan 9 notes are supported.
- HTTP: `/healthz`, `/metrics` (exposition rendered by `text/template`),
  `/render`, `/status`, expvar.
- `tpl` one-shot renderer and `tmplload` open-loop load generator.
- Nine templates with golden tests; `FuzzRender`.
- Terraform for both repositories, rulesets and seed data; CI, release,
  render-check (with `repository_dispatch` chaining from the kv repository)
  and load-test workflows; Dockerfile, systemd unit, Plan 9 notes,
  preflight, load test and beta rollout scripts.

### Fixed during development

- `text/template.Clone` drops `Option("missingkey=…")`; it is now reapplied
  after every clone.
- `FuzzRender` found that a raw `count = [[ key … ]]` in the Nomad template
  could carry `${` from data; numeric fields now go through `add 0`.
