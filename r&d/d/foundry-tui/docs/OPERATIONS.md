# Operations

## Daily

Open the screen. View `7` first: Resource Health, fired alerts, chain state.
Then `1` for anything not `Succeeded`. The report workflow has already written
today's record at 06:17 UTC; `reports/index.html` in the ops repository lists
every record ever made.

## Change a deployment

1. `1`, select, `s` (or `n` for new, `d` to delete). Fill the form. Give a
   reason someone will understand next year.
2. **Preview.** Read the JSON and the `gh` command. Leave *Dry run* on. Send.
3. The run appears in view `4` under its `request_id`. Open the step summary:
   it shows the exact `az` command that would run.
4. Repeat with *Dry run* off. A reviewer approves the `foundry-prod`
   environment. The workflow applies the change, commits
   `records/deployments/<year>/<request_id>.json`, starts a report, and posts
   the webhook.
5. `T` to probe the new deployment from Actions; `t` and `c` from here.

From a script, the same thing:

```sh
foundry-tui dispatch -kind deploy -set action=update -set deployment=gpt-4o \
  -set model_format=OpenAI -set model_name=gpt-4o -set model_version=2024-11-20 \
  -set sku_name=GlobalStandard -set sku_capacity=80 -set dry_run=false \
  -set reason='raise capacity for Q4 load test' -yes
```

## Findings

| Severity | Rule |
|---|---|
| high | a deployment's provisioning state is not `Succeeded` |
| high | a deployed model version is past its retirement date |
| high | Resource Health is not `Available` |
| high | the audit chain does not verify |
| warn | a deployed model version retires within 90 days |
| warn | a quota line is at 80% or more |
| warn | an alert is `Fired` and not `Closed` |
| warn | a workflow's last completed run failed |
| warn | an agentic workflow succeeds under 90% over five or more runs |
| warn | the latest probe of a deployment failed |
| info | auto-upgrade is off; key authentication is on; public network access is on |

`foundry-tui report -fail-on high` exits 2 when a `high` finding exists, so
cron or a workflow can turn a finding into a red run.

## The audit chain

`<state_dir>/audit.jsonl`: one JSON record per line with `seq`, `time`, `actor`,
`kind`, `subject`, `detail`, `prev` and `hash`, where `hash` is SHA-256 over the
record with `hash` blanked and `prev` is the previous record's hash.

```sh
foundry-tui audit            # print it
foundry-tui audit -verify    # walk it; non-zero exit if broken
```

What it proves: no record was edited, removed from the middle, or reordered.
What it cannot prove alone: that nobody cut records off the **end**, or
replaced the whole file. Two things close that gap. Every report embeds the
chain head at the time it was made, and reports are committed to git, so a
truncated chain's head will not match the last committed report. And the
authoritative record of a change is not this file at all: it is the workflow
run and the signed-off commit under `records/deployments/`. The local chain
records *intent at this terminal*, including requests that never reached
GitHub.

Kinds you will see: `dispatch.request`, `dispatch.sent`, `dispatch.error`,
`probe.api`, `probe.copilot`, `report.write`.

## Verify a record

```sh
cd reports && sha256sum -c foundry-ais-coolgit-copilot-prod-20260921T061702Z.sha256
git log --show-signature -- reports/ records/
```

The `.sha256` file covers the HTML and the JSON in `sha256sum` format. The
HTML header prints the JSON's digest too, so a printed page identifies its data.

## When something fails

| Symptom | Cause, fix |
|---|---|
| `az is not signed in` | `az login`; with several subscriptions set `subscription` |
| a section says *not collected* | the action is outside Foundry Owner: see PERMISSIONS.md |
| health or alerts show an error, the rest loads | `az rest` api-version moved; adjust the constant in `internal/azure/azure.go` |
| `gh: HTTP 404` on send | the workflow file is not on `ref`, or you lack `actions: write` |
| `gh: HTTP 422 Unexpected inputs` | workflow and template disagree: re-run `scaffold`, or fix your `template_dir` |
| run never appears in view `4` | dispatch is asynchronous; `r` after a few seconds. The `request_id` is in the audit view |
| `AADSTS70021` / `AADSTS700213` in `azure/login` | the federated credential subject does not match: environment name, repo name or branch differs from `bootstrap-azure.sh` |
| `AuthorizationFailed` on apply | the identity lacks Foundry Owner on the account: re-run bootstrap |
| `InsufficientQuota` on apply | view `3`; lower `sku_capacity` or free quota |
| push rejected three times | a ruleset blocks the bot: add the app as a bypass actor and set `AUTOADMIN_APP_ID` |
| probe: `401` | token audience wrong: `token_resource` must be `https://cognitiveservices.azure.com` |
| probe: `403` | no data action at that scope, or the account's network rules exclude you |
| probe: `404 DeploymentNotFound` | deployment is still provisioning, or the name is wrong |
| probe: `429` | the deployment is saturated: that is the finding |

## Upgrading

Change Go code or templates, `make check`, commit. The report and test
workflows build from the commit they check out, so the ops repository always
runs the version it contains. After changing a workflow file here, run
`foundry-tui scaffold -dir ../foundry-ops -force` and review the diff there.
Pin the actions in your fork to commit SHAs; the shipped files use major tags
so you can see at a glance which versions they were written against.
