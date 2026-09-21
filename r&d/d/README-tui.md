# foundry-tui

A terminal program for the person who owns a Microsoft Foundry account.
It shows model deployments, the model catalog, quota, health, alerts, access,
GitHub workflow runs and agentic workflows; it probes models; it writes
permanent HTML records. It changes nothing itself. Every change is a
`workflow_dispatch` sent by `gh`, with inputs rendered from a Go template,
to a workflow you can read in this repository.

```
                 reads                                  writes
  az CLI ───────────────┐                    ┌──────────────── gh workflow run --json
  (JSON on stdout)      │                    │                 (JSON on stdin)
                        ▼                    │
  GitHub REST ──────► foundry-tui ───────────┤
  (net/http)            │    │               ▼
                        │    │   text/template → JSON inputs → preview → send
                        │    │
                        │    └─► audit.jsonl    hash-chained, local, append-only
                        └──────► reports/*.html + *.json + *.sha256 + index.html

  GitHub Actions (CoolGitOrg/foundry-ops)
    foundry-deploy.yml      validate → OIDC login → guard → plan|apply → commit record
                            → start foundry-report.yml → signed webhook
    foundry-report.yml      daily + on demand → foundry-tui report → commit reports/
    foundry-model-test.yml  foundry-tui test (API + Copilot CLI) → commit probe history
```

Three rules hold everywhere:

1. **One write path.** The program has no code that mutates Azure. The only
   mutation is `gh workflow run`. You see the exact payload and command before
   it is sent, and `dry_run` is `true` until you turn it off.
2. **Bounded by Foundry Owner.** The program reads the live definition of the
   Foundry Owner role and asks Azure only for what that role allows. What it
   cannot see, it says it cannot see, in the screen and in every report.
3. **No secrets.** Reads use your `az login`. Probes use a short-lived Entra
   token. Workflows use OIDC federation. No API key is read, stored or printed.

## Quick start

```sh
make deps            # once: resolves modules, writes go.sum; commit it
make check           # gofmt, vet, shellcheck, actionlint, tests under -race
make build           # bin/foundry-tui

scripts/preflight.sh                  # are az, gh and the role in place?
scripts/bootstrap-azure.sh            # prints the plan for identity + OIDC + role
scripts/bootstrap-azure.sh --apply    # does it

bin/foundry-tui scaffold -dir ../foundry-ops   # copy workflows into the ops repo
cp examples/foundry-tui.json foundry-tui.json  # edit, or use FOUNDRY_* variables
bin/foundry-tui                                # the screen
```

`go.sum` is not shipped: it must come from your module proxy, not from an
archive. `make deps` writes it; commit it; CI then builds with verified sums.

## The screen

| Key | View / action |
|---|---|
| `1` | Deployments: model, version, SKU, capacity, rate limits, state, who created and last changed it |
| `2` | Catalog: deployable models, lifecycle, retirement date, SKUs. `n` deploys the selected row |
| `3` | Quota: used / limit per quota line in the region |
| `4` | Runs: recent runs of every workflow in the ops repository |
| `5` | Agents: gh-aw workflows (`*.lock.yml`), success rate, mean duration, last run |
| `6` | Audit: the local hash chain |
| `7` | Status: account, Resource Health, fired alerts, chain state, permission matrix, skipped sections |
| `n` `s` `d` | New, scale/update, delete: form → preview → send |
| `t` `c` `T` | Probe through the API, through Copilot CLI, or in Actions |
| `R` `W` | Write a report here, or ask the report workflow to write and commit one |
| `r` `/` `?` `q` | Reload, filter, help, quit |

`-theme acme` swaps your terminal's colours for the pale yellow and pale blue of
Plan 9's acme. The HTML reports always use them.

## Commands

The screen is one face of the program. Everything it does is also a command,
so cron, Actions and pipes can use it.

```sh
foundry-tui report [-out DIR] [-fail-on high|warn]        # HTML + JSON + sha256; exit 2 on findings
foundry-tui test   [-deployment NAME] [-mode api|copilot|both] [-json]
foundry-tui dispatch -kind deploy|report|test -set k=v ... [-yes]
foundry-tui audit  [-verify]
foundry-tui perms                                         # what Foundry Owner covers
foundry-tui scaffold [-dir DIR] [-force]
```

`dispatch` without `-yes` prints the payload on stdout and the command on
stderr, and needs neither `az` nor `gh`:

```sh
$ foundry-tui dispatch -kind deploy -set action=update -set deployment=gpt-4o \
    -set model_format=OpenAI -set model_name=gpt-4o -set model_version=2024-11-20 \
    -set sku_name=GlobalStandard -set sku_capacity=80 -set reason='Q4 load'
# gh workflow run foundry-deploy.yml --repo CoolGitOrg/foundry-ops --ref main --json < payload   (sha256 b3de…)
{ "action": "update", "deployment": "gpt-4o", "dry_run": "true", … }
# not sent: add -yes to dispatch
```

## Configuration

`foundry-tui.json` in the working directory, or `$FOUNDRY_TUI_CONFIG`, or
`<user config dir>/foundry-tui/config.json`. Unknown keys are errors. Every
key is optional; [examples/foundry-tui.json](examples/foundry-tui.json) shows
the defaults.

| Key | Env override | Default |
|---|---|---|
| `subscription` | `FOUNDRY_SUBSCRIPTION` | az default |
| `resource_group` | `FOUNDRY_RESOURCE_GROUP` | `rg-coolgit-copilot` |
| `account` | `FOUNDRY_ACCOUNT` | `ais-coolgit-copilot-prod` |
| `location` | `FOUNDRY_LOCATION` | `eastus2` |
| `repo`, `ref` | `FOUNDRY_REPO`, `FOUNDRY_REF` | `CoolGitOrg/foundry-ops`, `main` |
| `github_api` | | `https://api.github.com` (`https://api.SUBDOMAIN.ghe.com` with data residency) |
| `workflows.{deploy,report,test}` | | `foundry-deploy.yml`, `foundry-report.yml`, `foundry-model-test.yml` |
| `state_dir` | `FOUNDRY_STATE_DIR` | `$XDG_STATE_HOME/foundry-tui` or `~/.local/state/foundry-tui` |
| `report_dir` | `FOUNDRY_REPORT_DIR` | `reports` |
| `template_dir` | | empty: templates embedded in the binary |
| `refresh_seconds` | | `120` (`0` disables the timer) |
| `metrics` | | read only when the role in force allows `Microsoft.Insights/metrics/read` |
| `copilot.*` | | see [docs/TESTING.md](docs/TESTING.md) |

The GitHub token comes from `GH_TOKEN`, then `GITHUB_TOKEN`, then
`gh auth token`. It is sent only to `github_api`.

## Why tview and not termloop

The brief offered termloop. termloop is a game engine: a 2D cell canvas,
entities, collisions, a frame loop, on top of termbox-go, which its author
retired. This program needs tables, forms, modals, focus and resize handling.
[tview](https://github.com/rivo/tview) on [tcell](https://github.com/gdamore/tcell)
has exactly those, and tcell's simulation screen lets `go test` drive the real
application key by key (see `internal/tui/tui_test.go`). Two dependencies, both
maintained; everything else is the standard library.

## Layout

```
cmd/foundry-tui/     main: subcommands
internal/run/        the one place processes are started; Fake for tests
internal/config/     JSON + env, validated
internal/azure/      az, read-only
internal/github/     REST, read-only
internal/perms/      Foundry Owner: live definition, embedded snapshot, matcher
internal/dispatch/   template → JSON → gh workflow run
internal/probe/      API and Copilot CLI probes, history
internal/audit/      hash-chained JSONL
internal/report/     collect → findings → HTML/JSON/sha256/index
internal/tui/        the screen
templates/           dispatch/*.json.tmpl  probe/*.tmpl  report/*.html.tmpl
.github/workflows/   the three operating workflows + ci.yml (embedded for scaffold)
scripts/             bootstrap-azure.sh  preflight.sh  verify-webhook.sh
examples/            config, gh-aw workflow, sample report
docs/                ARCHITECTURE  PERMISSIONS  WORKFLOWS  TESTING  OPERATIONS
```

## What has and has not been verified

Compiled with Go 1.22; `go vet` clean; unit tests pass under `-race`, including
an end-to-end test that drives the screen on a simulated terminal through a
dry-run delete. Workflows pass actionlint 1.7.12 (which shellchecks every `run:`
block); scripts pass shellcheck. All of that uses fakes. **Nothing here has run
against a live Azure subscription or a live GitHub repository.** Expect to
adjust: the Alerts Management `api-version`, the action major versions (pin
them to SHAs in your fork), and Copilot CLI flags, which move quickly.

## Documents

- [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md): how the pieces fit and why
- [docs/PERMISSIONS.md](docs/PERMISSIONS.md): the Foundry Owner boundary
- [docs/WORKFLOWS.md](docs/WORKFLOWS.md): inputs, identity, records, webhook
- [docs/TESTING.md](docs/TESTING.md): API and Copilot CLI probes
- [docs/OPERATIONS.md](docs/OPERATIONS.md): day-to-day runbook
