# Workflows

Four files in `.github/workflows/`, embedded in the binary and copied out by
`foundry-tui scaffold`. They are flat bash: every `run:` block starts with
`set -euo pipefail`, inputs reach bash only through `env:` (there is no
`${{ }}` inside any script), and actionlint with shellcheck passes clean.

## Setup, once

| What | Where | Set by |
|---|---|---|
| `AZURE_CLIENT_ID`, `AZURE_TENANT_ID`, `AZURE_SUBSCRIPTION_ID` | repository variables | `bootstrap-azure.sh` |
| `FOUNDRY_ACCOUNT`, `FOUNDRY_RESOURCE_GROUP`, `FOUNDRY_LOCATION` | repository variables | `bootstrap-azure.sh` |
| environments `foundry-plan`, `foundry-prod` | repository | `bootstrap-azure.sh`; add **required reviewers** to `foundry-prod` by hand |
| `AUTOADMIN_APP_ID` (variable), `AUTOADMIN_APP_PRIVATE_KEY` (secret) | optional | you: lets `issueops-autoadmin-app` make the commits |
| `FOUNDRY_WEBHOOK_URL`, `FOUNDRY_WEBHOOK_SECRET` | optional secrets | you |

These are ids, not credentials. Azure trusts the repository through OIDC
federation; there is no client secret to rotate or leak.

**Why the app token matters.** A push made with `GITHUB_TOKEN` does not start
other workflows, and cannot pass a branch ruleset that requires a bypass
actor. With the app token the record commits are attributable to the app, can
be allowed through the ruleset, and do trigger `workflow_run` listeners such as
the gh-aw triage example. Without it everything still works; `foundry-deploy`
starts the report workflow explicitly with `gh workflow run` for this reason.

## foundry-deploy.yml

The only thing that changes Foundry.

| Input | Meaning |
|---|---|
| `action` | `create`, `update` or `delete` |
| `deployment` | deployment name |
| `model_format`, `model_name`, `model_version` | e.g. `OpenAI`, `gpt-4o`, `2024-11-20` |
| `sku_name`, `sku_capacity` | e.g. `GlobalStandard`, `50` (thousands of tokens per minute) |
| `dry_run` | `true` plans only; default `true` |
| `request_id` | correlation id; appears in the run name and the record file name |
| `reason` | 8–200 characters; goes into the record and the commit message |

Ten inputs, the historical `workflow_dispatch` ceiling (25 on github.com since
December 2025).

Steps: validate every input against a regular expression → optional app token →
checkout → `azure/login` with OIDC → read current state → **guard** (`create`
must not exist, `update` and `delete` must) → print the exact `az` command to
the step summary → stop here if `dry_run` → run it → write
`records/deployments/<year>/<request_id>.json` with `before` and `after` →
`git commit -s`, push with three rebase retries → start `foundry-report.yml` →
POST the record to the webhook, if one is configured.

`environment:` is `foundry-plan` when `dry_run` is true and `foundry-prod`
otherwise. Reviewers on `foundry-prod` approve real changes; dry runs flow
freely so people use them. `concurrency` is keyed on the deployment name and
never cancels, so two changes to one deployment queue instead of racing.

### The webhook

The body is the change record. The header
`X-Foundry-Signature-256: sha256=<hex>` is HMAC-SHA256 of the raw body with
`FOUNDRY_WEBHOOK_SECRET`. A receiver checks it like this (save the body byte for
byte; a trailing newline changes the digest):

```sh
FOUNDRY_WEBHOOK_SECRET=… scripts/verify-webhook.sh body.json "sha256=ab12…"
```

Point it at a `repository_dispatch` relay, a chat hook, or a CMDB. A receiver
that itself dispatches a workflow completes the chain: workflow → workflow →
webhook → workflow.

## foundry-report.yml

Daily at 06:17 UTC and on demand. Builds `foundry-tui` from the repository at
the checked-out commit, runs `foundry-tui report` with
`FOUNDRY_STATE_DIR=records/state`, commits `reports/` and `records/state/`,
and uploads the same files as an artifact kept 90 days. The git history of
`reports/` is the long-term record; the artifact is a convenience.

## foundry-model-test.yml

Inputs `deployment` (empty for all chat deployments), `mode`
(`api`, `copilot`, `both`), `request_id`, `reason`. Installs `@github/copilot`
with npm when the mode needs it, runs `foundry-tui test -json`, writes a table
to the step summary, appends to `records/state/probes.jsonl` and commits. The
job fails if any probe fails, so a schedule on this workflow is a synthetic
monitor whose alarm is a red run.

## ci.yml

`gofmt -l`, `go vet`, `go test -race`, actionlint. No Azure, no secrets.

## Records layout in the ops repository

```
records/deployments/2026/ftui-20260921T141503Z-9f2c1a.json   one per applied change
records/state/audit.jsonl                                    the workflows' own audit chain
records/state/probes.jsonl                                   probe history from Actions
reports/foundry-<account>-<UTC>.html|.json|.sha256           one triple per report
reports/index.html                                           regenerated each time
```

Protect `records/` and `reports/` with CODEOWNERS and a ruleset that lets only
the app (or `github-actions[bot]`) push to them.

## Agentic workflows

The program treats any workflow whose path ends in `.lock.yml` as agentic:
that is what `gh aw compile` emits from a `.md` source. View `5` and the
"Agentic workflows" report section show state, runs, passes, failures, success
rate, mean duration and last run for each; a success rate under 90% across five
or more completed runs is a `warn` finding. For token spend and per-run
forensics use gh-aw's own tools, which read data this program does not:
`gh aw status`, `gh aw logs`, `gh aw audit <run-id>`, `gh aw health`.

[`examples/agentic/foundry-record-triage.md`](../examples/agentic/foundry-record-triage.md)
is a small agentic consumer of the reports: after each `foundry-report` run it
reads the newest JSON record and opens or updates an issue per `high` finding.
It holds read-only permissions; its writes are `safe-outputs` capped at three
issues. Install it with:

```sh
cp examples/agentic/foundry-record-triage.md .github/workflows/
gh aw compile --strict foundry-record-triage
git add .github/workflows/foundry-record-triage.md .github/workflows/foundry-record-triage.lock.yml
git commit -s -m "aw: foundry record triage"
```

It deliberately may not dispatch `foundry-deploy.yml`. Agents report; people
with a reviewer behind them change Foundry.
