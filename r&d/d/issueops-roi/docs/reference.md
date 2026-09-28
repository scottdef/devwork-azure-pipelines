# Reference

## Labels

| Label | Kind | Meaning |
|---|---|---|
| `issueops` | platform | Applied by every form; workflows only act on labeled issues |
| `issueops:copilot-budget`, `issueops:copilot-report`, `issueops:foundry-deploy`, `issueops:agentic-task` | type | Exactly one per issue; selects the request type |
| `issueops:invalid`, `issueops:awaiting-answers`, `issueops:validated`, `issueops:submitted`, `issueops:approved`, `issueops:executing`, `issueops:completed`, `issueops:failed`, `issueops:denied`, `issueops:cancelled` | state | Exactly one; a **projection** of the timeline state |
| `issueops:escalated` | flag | Policy escalations apply (extra approvers or a stricter environment) |

## Commands

The command must be the first token of the first non-blank line, and the comment must not be from a bot. `.approved` and `please .approve` are not commands.

| Command | Privileged | Notes |
|---|---|---|
| `.submit` | requestor | Binds the current digest |
| `.approve [note]` | approver | Latest vote per person after the current submission counts |
| `.deny <reason>` | approver | Closes as not planned |
| `.retry [note]` | approver | Only in `failed`; approvals must still be valid |
| `.answers` | requestor | `A<n>: value` lines; agentic tasks |
| `.cancel [note]` | requestor or maintainer | Closes as not planned |
| `.status`, `.help` | anyone | Informational |

## Markers

Every bot comment starts with `<!-- issueops:v1:<event> {json} -->`.

| Event | Written by | Data |
|---|---|---|
| `summary` | intake (updated in place) | `valid`, `qa_complete`, `action`, `digest` |
| `reset` | intake on reopen of a terminal request | `phase` |
| `qa-status` | `.answers` (updated in place) | `complete`, `digest` |
| `questions` | finish (needs_input) | `dynamic` (follow-up questions), `digest` |
| `submitted` | `.submit` | `actor`, `environment`, `escalations`, `digest` |
| `approval-status` | partial approval (updated in place) | `actor`, `digest` |
| `approved` | quorum, auto-approval, `.retry` | `actor`, `approvers`, `auto`, `retry`, `digest` |
| `denied`, `cancelled` | `.deny`, `.cancel` | `actor`, `reason` |
| `executing` | gate | `run`, `digest` |
| `completed`, `failed` | finish | `run`, `error`, `digest` |
| `notice` | rejections, status, help, gate refusals | `command`, `actor`, `gate` |

## Secrets and variables

| Name | Kind | Scope | Used by |
|---|---|---|---|
| `ISSUEOPS_APP_CLIENT_ID` | variable | repository | all workflows (App token) |
| `ISSUEOPS_APP_PRIVATE_KEY` | secret | repository | all workflows |
| `ISSUEOPS_RUNS_ON`, `ISSUEOPS_AKS_RUNS_ON` | variable (JSON array) | repository | runner selection |
| `AZURE_TENANT_ID`, `AZURE_SUBSCRIPTION_ID` | variable | repository | Azure login |
| `AZURE_CLIENT_ID` | variable | environments `foundry-*`, `agentic-aks`, `acr-push` | Azure login (per-environment Entra app) |
| `AKS_RESOURCE_GROUP`, `AKS_CLUSTER_NAME` | variable | `agentic-aks` | AKS context |
| `ACR_NAME` | variable | `acr-push` | runner image |
| `PUSHGATEWAY_URL` | variable | repository | reports, ops metrics |
| `PUSHGATEWAY_AUTH` | secret | repository | basic auth `user:password` (reports, ops metrics) |
| `ENTERPRISE_BILLING_PAT` | secret | `copilot-budgets-enterprise` | enterprise (cost-center) budgets |
| `COPILOT_AGENT_TOKEN` | secret | `agentic-copilot` | assign Copilot cloud agent |

## Environments

| Environment | Selected when | Protection |
|---|---|---|
| `copilot-budgets` | budget ≤ $1,000, organization/repository/user/all-users | optional |
| `copilot-budgets-high` | budget > $1,000 | FinOps reviewers |
| `copilot-budgets-enterprise` | cost-center scope | enterprise billing owner |
| `copilot-reports` | usage reports | — |
| `foundry-dev`, `foundry-uat`, `foundry-prod` | Foundry account environment | prod: platform-ai reviewers |
| `agentic-copilot`, `agentic-catalog`, `agentic-aks` | agent backend | aks: platform-ai reviewers |
| `acr-push` | runner image build | — |

## Pinned actions

Pinned to commit SHAs. Dependabot (`github-actions`) proposes updates, and the version comments are kept in sync. Resolved on 2026-09-27.

| Action | Version | SHA |
|---|---|---|
| `actions/checkout` | v7.0.1 | `3d3c42e5aac5ba805825da76410c181273ba90b1` |
| `actions/setup-go` | v7.0.0 | `b7ad1dad31e06c5925ef5d2fc7ad053ef454303e` |
| `actions/setup-node` | v7.0.0 | `820762786026740c76f36085b0efc47a31fe5020` |
| `actions/upload-artifact` | v7.0.1 | `043fb46d1a93c77aae656e7c1c64a875d1fc6a0a` |
| `actions/download-artifact` | v8.0.1 | `3e5f45b2cfb9172054b4087a40e8e0b5a5461e7c` |
| `actions/create-github-app-token` | v3.2.0 | `bcd2ba49218906704ab6c1aa796996da409d3eb1` |
| `issue-ops/parser` | v5.0.0 | `cb7e2e4e5da701aad0e23e718bc4c91d442ca8ba` |
| `issue-ops/validator` | v4.0.0 | `3aec708c72ec4cb8dfca562d4addad3e78e55e26` |
| `issue-ops/labeler` | v4.0.0 | `9210b145e254445abdbfd33099941e7637d56d4e` |
| `github/command` | v2.0.3 | `3442f3fa1efe01bdb024b157083c337902d17372` |
| `peter-evans/find-comment` | v4.0.0 | `b30e6a3c0ed37e7c023ccd3f1db5c6c0b0c23aad` |
| `peter-evans/create-or-update-comment` | v5.0.0 | `e8674b075228eee787fea43ef493e45ece1004c9` |
| `azure/login` | v3.1.0 | `a641126d1b8aa4d1fa005f4f92df94a3a4c4c906` |
| `azure/setup-kubectl` | v5.1.0 | `829323503d1be3d00ca8346e5391ca0b07a9ab0d` |
| `azure/use-kubelogin` | v1.3 | `0ce7c36141aa27d4934872cf00b0120804c98a29` |
| `azure/aks-set-context` | v5.0.0 | `60623acbdcbbdcf799ad50a1adf8703874339f8b` |
| `docker/setup-buildx-action` | v4.4.1 | `f87e5991a6d7451dcb8d9637bfbc97413f497069` |
| `docker/build-push-action` | v7.4.0 | `c3c9e263c25d99ce0380d002d59b67737d91b0dc` |

The IssueOps and peter-evans actions run on Node 24. Self-hosted runners must be version 2.327.1 or later.

**Notes on action inputs used here:**

- `create-github-app-token` v3 takes `client-id` (`app-id` is deprecated) plus `permission-*` inputs to narrow the token.
- `github/command` must have `allowed_contexts: issue` for issue comments. Its `permissions` values are the legacy repository permissions `read`, `write` and `admin`.
- `issue-ops/validator` custom scripts are ESM (`export default async (field) => 'success' | 'message'`), configured in `.github/validator/config.yml`. Fields absent from a form are skipped.

## GitHub REST endpoints

API version header `2026-03-10` for billing and usage metrics. Default version elsewhere.

| Purpose | Endpoint |
|---|---|
| Budgets (org) | `GET/POST /organizations/{org}/settings/billing/budgets`, `PATCH …/budgets/{id}` |
| Budgets (enterprise) | `GET/POST /enterprises/{enterprise}/settings/billing/budgets`, `PATCH …/{id}` |
| AI-credit usage (client implemented, reconciliation planned) | `GET /organizations/{org}/settings/billing/ai_credit/usage` |
| Copilot usage metrics | `GET /orgs/{org}/copilot/metrics/reports/{users-1-day,user-teams-1-day,repos-1-day,organization-1-day}?day=` |
| Copilot seats | `GET /orgs/{org}/copilot/billing` |
| Output | `GET /search/issues` (merged PRs, reviews), `GET /repos/{o}/{r}/commits`, `GET /repos/{o}/{r}/stats/code_frequency` |
| Authorization | `GET /orgs/{org}/teams/{slug}/memberships/{user}`, `GET /orgs/{org}/memberships/{user}`, `GET /orgs/{org}/organization-roles[/{id}/users\|teams]`, `GET /repos/{o}/{r}/collaborators/{user}/permission` |
| Issues | `GET /repos/{o}/{r}/issues/{n}`, `…/comments`, `PATCH …/issues/{n}` (close), labels |
| Agents | `POST /repos/{o}/{r}/issues` (assignee `copilot-swe-agent[bot]`, `agent_assignment`), `POST /agents/repos/{o}/{r}/tasks`, `POST /repos/{o}/{r}/actions/workflows/{wf}/dispatches` |

## Azure commands

`az cognitiveservices account list-models`, `az cognitiveservices usage list -l`, `az cognitiveservices account deployment list|show`, `az cognitiveservices account show`, `az deployment group what-if|create`, `az aks get-credentials` (via `aks-set-context`), `az acr login`.

## Registry schema (`config/issueops.json`)

```text
version, enterprise, organization, repository, platform_label, docs_url
pricing: { ai_credit_usd, seat_monthly_usd, seat_plan, note }
request_types[]:
  id, name, description, template, label
  execute: { workflow, environment }        # environment may contain {fact} placeholders
  close_on_complete, qa
  requestors?: { target_repo_field?, any_of: Selector }
  approval: { require_submit, allow_self_approval, distinct_approvers, target_repo_field?,
              rules: Rule[], escalations: [{ name, when: Condition, rules: Rule[], environment? }],
              auto_approve?: Condition }
  settings: type-specific (see request-type docs)
Rule      = { name, min, any_of: Selector }
Selector  = { teams?, users?, org_roles?, repo_permissions?, target_repo_permissions?, field_users?, requestor? }
Condition = { all?: Predicate[], any?: Predicate[] };  Predicate = { field, op, value? }
```

`config.Load` validates the registry:

- unique ids, labels and templates
- type labels prefixed with the platform label
- `.yml` templates and an execute workflow per type
- rules with `min ≥ 1` and non-empty selectors, with known repository permissions
- known operators, and valid regexes for `matches`
- `target_repo_permissions` only with a `target_repo_field`

`issueops check-forms` then checks every field the registry references against the issue forms.

## Mapping from the research guide

| Research guide | This implementation |
|---|---|
| `cmd/copilot-roi` | `issueops report build` (`internal/copilot`) |
| `bicep/model-deployment.bicep` | `deploy/bicep/model-deployment.bicep` |
| `APP_ID` / `APP_PRIVATE_KEY` | `ISSUEOPS_APP_CLIENT_ID` (client ID, per `create-github-app-token` v3) / `ISSUEOPS_APP_PRIVATE_KEY` |
| `production-foundry` environment | `foundry-dev`, `foundry-uat`, `foundry-prod` (selected from the Foundry account) |
| `issueops:ready` label | `issueops:validated` (Q&A complete) |
| `issueops-approvers` team | Per-domain teams (`copilot-admins`, `finops-approvers`, `platform-ai`, `ai-governance`) |
| Single execute workflow per type | Gate → execute → report jobs, with a digest hand-off |
