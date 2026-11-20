# issueops-additions

Nine CoolDeploy request types for the existing IssueOps platform in
`CoolEngOrg/issueops`, and one hand-started workflow for the sandbox.

This directory is not a repository of its own. It is what gets added to
`CoolEngOrg/issueops`:

```sh
scripts/merge-registry.sh /path/to/issueops
```

copies the forms, workflows, scripts and sandbox scenarios in, and merges the
request types and labels into `config/issueops.json` and `config/labels.json`.
Running it twice changes nothing.

## Request types

| Type | Asks for | Approval | Runs in |
| --- | --- | --- | --- |
| `cool-app-create` | a repository from a template, registered, with identities | 1 of `platform-admins` | `cool-dev` |
| `cool-deploy-promote` | the live release of dev in tst, or of tst in prd | to tst: none. To prd: 1 of `cool-release-approvers` | `cool-<env>` |
| `cool-deploy-rollback` | the standby release live again | dev, tst: none. prd: 1 of `cool-release-approvers` or `platform-admins`, who may be the requestor | `cool-<env>` |
| `cool-rolling-release` | a release moved to full traffic in stages, with an error-budget check between stages | as promote | `cool-<env>` |
| `cool-env-var` | plain settings set or removed in one environment | dev: none. tst, prd: 1 of `cool-release-approvers` | `cool-<env>` |
| `cool-domain` | a custom host name added or removed | 1 of `platform-admins` | `cool-<env>` |
| `cool-addon` | Redis, PostgreSQL, storage or an AI token budget | 1 of `platform-admins`; `large` adds FinOps; `ai` adds AI governance | `cool-<env>` |
| `cool-api-publish` | the app's OpenAPI description published through API Management | 1 of `platform-admins` | `cool-<env>` |
| `cool-app-retire` | the app removed from every environment | 1 of `cool-release-approvers` and 1 of `platform-admins`, two different people | `cool-prd` |

A rollback in prd may be approved by the person who asked for it. During an
incident the person who sees the problem is often the person who can
approve, and a second person is a delay. Everything else needs someone else.

Two request types change nothing themselves. `cool-addon` opens a pull
request on `cool-infra`, and `cool-api-publish` one on `cool-apim`; merging
those runs Terraform or the API Management scripts behind their own review.
Infrastructure stays in the repositories that own it.

## How a request is carried out

```
comment router ─▶ execute-cool-<type>.yml ─▶ execute-cool-request.yml
                                               gate ─▶ execute ─▶ finish
                                                         │
                                                         └─ scripts/cool/<type>.sh
```

The nine `execute-cool-<type>.yml` files are six lines of substance each:
they name the type, the other repositories its token may reach, and the
Azure identity it needs. The work is in `execute-cool-request.yml` and in one
script per type.

The execute job holds no deploy secret. `cool-api` accepts its GitHub OIDC
token because the token says the job is defined by a file named
`execute-cool-*` on `main` of this repository and runs in the GitHub
environment `cool-<env>`. So the environments are the lock:

| Environment | Protection |
| --- | --- |
| `cool-dev`, `cool-tst` | deployment branches: `main` only |
| `cool-prd` | `main` only; add required reviewers if you want a second gate on top of the request's own approvals |
| `cool-sbx` | `main` only; required reviewers: `platform-admins` |

and `main` is protected with code owner review, since whoever can change an
`execute-cool-*` file on `main` can change what approved requests do.

## The scripts

Each script in `scripts/cool/` reads the request from `$COOL_FIELDS`, a JSON
object of the form's fields, and writes a Markdown report to `$COOL_REPORT`.
They trust nothing in the fields: every value is matched against a pattern
before use, and reaches commands only as a quoted argument.

`cool-env-var` refuses variable names that look like secrets. A secret typed
into an issue has leaked; secrets go in the team Key Vault.

They can be run by hand against the local demo:

```sh
. ../demo/lib.sh; WORK=$(mktemp -d); build; start /path/to/a/copy/of/cool-gitops
COOL_FIELDS=$(coolctl issue fields -f examples/issues/cool-deploy-promote.md) \
COOL_TOKEN=$(token repository=CoolEngOrg/issueops environment=cool-prd \
  job_workflow_ref=CoolEngOrg/issueops/.github/workflows/execute-cool-request.yml@refs/heads/main) \
COOL_WAIT=0 scripts/cool/deploy-promote.sh
```

`DRY_RUN=1` prints the commands of the scripts that reach GitHub or Azure
(`app-create`, `addon`, `api-publish`) without running them.

## Check

```sh
make check
```

checks that forms, registry, labels, workflows, scripts and sample issues
agree, then starts a local `cool-api` and runs every script against it: the
promotions, a rolling release that completes, one that leaves its error
budget and is stopped, a rollback, and the requests that must be refused.

## What you have to adapt

The forms, the scripts and the registry entries were written against the
description of the IssueOps engine in this project, not against its source.
Four things are assumptions. Check each before the first real request.

| Assumption | Where | If it is wrong |
| --- | --- | --- |
| The `gate` action takes `issue-number` and `token`, and outputs `proceed` (`"true"` to run) and `fields` (the form fields as a JSON object, for the digest it verified) | `execute-cool-request.yml`, marked `INTEGRATION 1 of 2` | Rename the inputs and outputs. If the gate does not output the fields, add that output to it. Do not re-read the issue body in the execute job: the request must act on what was approved, and only the gate knows that. |
| The `finish` action takes `issue-number`, `token`, `outcome` (`completed` or `failed`) and `report-file` | the same file, `INTEGRATION 2 of 2` | Rename the inputs. |
| Registry entries have the keys `id`, `title`, `form`, `label`, `workflow`, `environment`, `requestors`, `rules`, `escalations`, `auto_approve`, `allow_self_approval`, `distinct_approvers`; conditions are `{field, op, value}` with `eq` and `in` | `config/issueops.cool.json` | Rename keys to match the existing entries in `config/issueops.json`. The policy in the table above is what matters. |
| Form field ids are plain (`app`, `environment`), and the engine's parser keys fields by id | `.github/ISSUE_TEMPLATE/cool-*.yml`, `config/cool-fields.json` | If the engine prefixes ids with the type, rename them in the forms and in `field`/`need` calls in the scripts. `test/check.sh` will tell you what no longer lines up. |

One thing is missing on purpose. The engine validates a request at intake
with a Go handler per type, and the handler interface is not something this
code could see. Until handlers exist, intake accepts whatever the form
accepts and the scripts refuse bad values at execution, after approval,
which works but wastes an approver's time. The patterns to port are in
`scripts/cool/lib.sh` (`RE_APP`, `RE_RELEASE`, `RE_KEY`, `RE_HOST`) and at
the top of each script; they are a few lines per type.

## Also needed in the organization

- Teams `cool-developers` (who may ask) and `cool-release-approvers` (who
  approve production changes). `platform-admins`, `finops-approvers` and
  `ai-governance` exist already.
- Variables: `COOL_API_URL`, `COOL_PROM_URL`, `COOL_TENANT_ID`,
  `COOL_SUBSCRIPTION_ID`, `COOL_PROVISION_CLIENT_ID`, `COOL_MONITOR_CLIENT_ID`,
  `COOL_PROVISION_APP_CLIENT_ID`.
- Secret: `COOL_PROVISION_APP_PRIVATE_KEY`, the key of a GitHub App that may
  create repositories and open pull requests on `cool-infra` and `cool-apim`.
  It is a different App from the IssueOps one on purpose: the IssueOps App
  comments on issues, and this one creates repositories.
- Federated credentials for the two Azure identities, with subject
  `repo:CoolEngOrg/issueops:environment:cool-<env>`. `cool-infra` creates them.
