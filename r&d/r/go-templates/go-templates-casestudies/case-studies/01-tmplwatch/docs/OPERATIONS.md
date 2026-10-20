# Operations

## Runbook

| Symptom | Look at | Likely cause / action |
|---|---|---|
| `/healthz` 503 `incomplete: NAME` | `curl -s :9180/status \| jq '.templates[] \| select(.complete \| not)'` | `missing` lists absent keys: add them to the kv repo, or switch the template to `keyOrDefault`. `error` is a render error, often bad JSON in a value or a `fail` guard firing. The old file is still in place. |
| File not updating | `/status` `.dependencies`, `rendered_at`, `written_at` | Is the key under a recorded dependency? `tmplwatch_source_polls_total{result="error"}` rising means the source is failing; check the logs. |
| `polls_total{result="error"}` rising | logs: `poll github:...` | `401` bad token; `403`/`429` rate limit (tmplwatch backs off to the reset time); `404` wrong repo, ref or App not installed on the repo. |
| Command keeps failing | `tmplwatch_commands_total{result="error"}`, `/status .command_error` | The file **was** written. Fix the template or the data. To validate before reloading, point `command` at a script you own, e.g. `["/usr/local/libexec/caddy-apply", "out/Caddyfile"]` that runs `caddy validate` then `caddy reload`; tmplwatch itself never invokes a shell. |
| Child restarting | `tmplwatch_exec_restarts_total`, `/status .exec` | The child exits by itself; tmplwatch restarts it with backoff from 1s to 30s. |
| Destination has mode drift | | Fixed on the next poll; tmplwatch repairs the mode when the content already matches. |
| `.NAME.tmplwatch-*` files | | Left by a crash between create and rename; removed at next start. |

Reload templates after editing them on disk: `systemctl reload tmplwatch`
(SIGHUP), or `echo hangup >/proc/PID/note` on Plan 9.

Render one template by hand, exactly as the daemon would, without writing:

```sh
tmplwatch -c /etc/tmplwatch/tmplwatch.json -once -dry -a off
```

## Flags

| Flag | Default | |
|---|---|---|
| `-c file` | `tmplwatch.json` | configuration |
| `-once` | | render once, run commands synchronously, exit (see exit codes) |
| `-dry` | | print unified diffs and would-run commands to stdout; write nothing; exec disabled |
| `-a addr` | config `listen` | override the listener; `off` disables HTTP |
| `-pprof` | | add `/debug/pprof/` to the listener; refused unless the listener is loopback |
| `-v` | | log every poll, not only changes and errors |
| `-healthcheck` | | GET `http://ADDR/healthz` (ADDR from `-a`, default `127.0.0.1:9180`); exit 0 if 200 |
| `-version` | | print version |

## Exit codes

| Code | Meaning |
|---:|---|
| 0 | ok (daemon stopped cleanly; `-once` all complete; `-healthcheck` healthy) |
| 1 | `-once`: a fetch, render, write or command failed |
| 2 | usage: bad flags, unreadable or invalid config, template parse error, listener in use |
| 3 | `-once`: at least one template is incomplete (missing keys) |
| 4 | `-healthcheck`: not 200, or no answer |

## Signals

| Unix | Plan 9 note | Effect |
|---|---|---|
| `SIGHUP` | `hangup` | reload templates from disk and render all |
| `SIGINT`, `SIGTERM` | `interrupt` | stop: cancel timers, stop the child (`kill_signal`, then kill after `kill_timeout`), shut down HTTP |

`exec.change_signal` and `exec.kill_signal` accept `SIGHUP SIGINT SIGQUIT
SIGTERM SIGKILL SIGUSR1 SIGUSR2 SIGWINCH`, with or without the `SIG`
prefix. On Plan 9 they map to notes, and any other text is posted verbatim
as a note.

## Endpoints

All on `listen` (default `127.0.0.1:9180`). Keep it on loopback. There is
no authentication, and `/render` returns rendered configuration.

| Path | |
|---|---|
| `GET /healthz` | `200 ok`, or `503 incomplete: NAMES` while any template lacks a complete render |
| `GET /metrics` | Prometheus text format 0.0.4 |
| `GET /render` | template names, one per line |
| `GET /render?template=NAME` | last complete render, `text/plain`; 404 if none |
| `GET /status` | JSON: source version, key count, per-template state and dependencies, exec state |
| `GET /debug/vars` | expvar: memstats, cmdline, and `tmplwatch` (same as `/status`) |
| `/debug/pprof/` | only with `-pprof` |

## Metrics

| Metric | Type | Labels | |
|---|---|---|---|
| `tmplwatch_build_info` | gauge | `version` | always 1 |
| `tmplwatch_start_time_seconds` | gauge | | process start |
| `tmplwatch_renders_total` | counter | `template` | renders attempted |
| `tmplwatch_render_errors_total` | counter | (none) and `template` | renders that errored; the unlabelled series is the sum |
| `tmplwatch_render_duration_seconds` | summary | `quantile` 0.5/0.95/0.99 | over the last 1024 renders; plus `_sum`, `_count` |
| `tmplwatch_template_incomplete` | gauge | `template` | 1 while the last render was incomplete or failed |
| `tmplwatch_writes_total` | counter | `template` | destinations written (or would be, in `-dry`) |
| `tmplwatch_source_polls_total` | counter | `result` = changed, unchanged, error | |
| `tmplwatch_last_poll_timestamp_seconds` | gauge | | time of the last poll, any result |
| `tmplwatch_commands_total` | counter | `template`, `result` = ok, error | |
| `tmplwatch_exec_restarts_total` | counter | | child restarts |

## Alerts

```yaml
groups:
  - name: tmplwatch
    rules:
      - alert: TmplwatchTemplateIncomplete
        expr: max by (instance, template) (tmplwatch_template_incomplete) == 1
        for: 10m
        annotations:
          summary: "{{ $labels.template }} on {{ $labels.instance }} has not rendered for 10m; see /status"
      - alert: TmplwatchPollsFailing
        expr: rate(tmplwatch_source_polls_total{result="error"}[10m]) > 0
          and rate(tmplwatch_source_polls_total{result=~"changed|unchanged"}[10m]) == 0
        for: 10m
        annotations:
          summary: "every poll on {{ $labels.instance }} is failing"
      - alert: TmplwatchStalled
        expr: time() - tmplwatch_last_poll_timestamp_seconds > 300
        annotations:
          summary: "no poll for 5m on {{ $labels.instance }}"
      - alert: TmplwatchCommandFailing
        expr: increase(tmplwatch_commands_total{result="error"}[15m]) > 0
        annotations:
          summary: "{{ $labels.template }} command failing on {{ $labels.instance }}"
      - alert: TmplwatchChildFlapping
        expr: increase(tmplwatch_exec_restarts_total[15m]) > 3
        annotations:
          summary: "exec child restarted more than 3 times in 15m"
```

Scrape the loopback listener from a local agent, or have the shipped
`prometheus.yml.tmpl` scrape `127.0.0.1:9180` on each host.

## Logs

One line per event on stderr, `log` package format, prefix `tmplwatch: `.
Lines worth grepping: `wrote`, `waiting for keys`, `render:`, `command`,
`poll`, `exec:`, `reload`.
