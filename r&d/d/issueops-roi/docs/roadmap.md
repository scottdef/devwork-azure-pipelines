# Roadmap

## Where we are (release 1.0)

Delivered in this repository:

- Four request types, end to end:
  - Copilot AI budgets (organization, repository, user, all users, cost center)
  - Copilot usage and ROI reports (organization, teams, repositories; HTML, CSV, JSON, Grafana)
  - Foundry model deployments (Bicep through OIDC, `az` pre-flight)
  - Agentic tasks with comment Q&A on three backends (Copilot cloud agent, catalog agentic workflows, AKS runner on Foundry)
- Approval gating by team, user, organization role (owner and custom), repository permission (IssueOps or target repo), named form field and requestor. Escalations, auto-approval, distinct approvers, no self-approval, and GitHub environments as a second gate.
- Event-sourced state, digest-bound approvals, gated execution, least-privilege tokens.
- Go templates for every comment, prompt and manifest, overridable without a rebuild.
- An offline simulator; ten example scenarios run as e2e tests; ops metrics; Grafana OSS 12 dashboards.

## Phase 1: harden and adopt (0–3 months)

| # | Improvement | Why | Notes |
|---|---|---|---|
| 1.1 | **zizmor + CodeQL for Actions** in CI | Catch workflow injection and permission regressions | Add to `ci.yml`, SARIF to code scanning |
| 1.2 | **Split the GitHub App** into an *intake* App (issues, members read) and an *executor* App (billing admin, actions write) | A compromised intake path can't change billing | Two key pairs; `setup-issueops` gets an `app` input |
| 1.3 | **Approval reminders and SLAs** | Requests stall in `submitted` | Scheduled workflow: re-mention approvers after N hours, escalate to a backup team, auto-close stale `invalid` requests after 14 days |
| 1.4 | **Delegation and out-of-office** | Single-person rules block work | Registry `delegates: {user: [backup], until: date}` evaluated by `authz` |
| 1.5 | **Budget reconciliation** | ROI uses the configured credit price | Use `…/billing/ai_credit/usage` `pricePerUnit` and net amounts in the report; flag drift |
| 1.6 | **Self-service portal** | Discoverability | GitHub Pages catalog generated from `config/issueops.json` (like `issue-ops/self-service`), linking to the forms |
| 1.7 | **Project board** | Queue visibility for approvers | Add issues to an org Project with Phase, Type and Escalated fields, driven by the same decision outputs |
| 1.8 | **Chat notifications** | Approvers live in chat | Teams or Slack webhook step in `apply-decision` for `submitted`, `approved` and `failed` |
| 1.9 | **Issue-form schema check** | Catch form errors before merge | Validate `.github/ISSUE_TEMPLATE/*.yml` against the published schema in CI |
| 1.10 | **Vendored Go modules** | Air-gapped self-hosted runners | `go mod vendor` plus `-mod=vendor` in `setup-issueops` |

## Phase 2: scale (3–6 months)

| # | Improvement | Why |
|---|---|---|
| 2.1 | **Enterprise scope**: requests that target any CoolEngEnt organization, enterprise-level Copilot metrics and budgets | Multi-org rollout |
| 2.2 | **Policy impact preview on PRs**: a job that re-evaluates recent requests under the proposed registry and comments "12 requests would change approvers" | Safe policy changes |
| 2.3 | **Scheduled ROI**: recurring reports per team with trend lines and anomaly findings (spend up, output flat), sent to FinOps | Continuous ROI instead of ad hoc |
| 2.4 | **Foundry lifecycle**: watch model retirement dates and deprecations; open pre-filled upgrade requests for affected deployments | Avoid surprise retirements |
| 2.5 | **Drift detection**: compare live deployments and budgets with the last approved request; open a finding issue on drift | Approved state stays true |
| 2.6 | **Agent runner v2**: read-only repository retrieval tool, Foundry Agent Service backend, OpenTelemetry traces and token metrics to Grafana, per-run AI-credit ceiling | Better deliverables, observability |
| 2.7 | **Webhook service option**: a small Go (stdlib) service on AKS receiving App webhooks for sub-second command responses; Actions remain the execution plane | Latency at high volume |
| 2.8 | **GraphQL timeline reads** | Fewer API calls on long issues |

## Phase 3: optimize (6–12 months)

- **Budget recommendations:** from ROI trends, open pre-filled budget requests ("payments-api trending to $2,300; current budget $2,500 → keep", or "raise to $3,000").
- **Seat optimization:** reclaim idle seats with notice, reassign on request, and report on the savings.
- **Request from anywhere:** a Copilot extension, Teams or Slack command that creates the issue with the form filled in; approvals still happen on the issue.
- **Policy analytics:** approval latency and denial reasons per rule to tune thresholds.

## New IssueOps request types

Candidates, ranked by value and effort. Each reuses the engine; a new type needs a form, a registry entry, a `request.Handler`, templates, an execute workflow and a scenario (see the recipe below).

| Request type | What it does | Key APIs | Default gating | Value | Effort |
|---|---|---|---|---|---|
| **copilot-seat-request** | Grant or remove a Copilot seat for a user or team, optionally time-boxed | `POST/DELETE /orgs/{org}/copilot/billing/selected_users` / `selected_teams` | Manager (`field_users`) + auto for listed teams | High | S |
| **copilot-seat-reclaim** (scheduled + appeal) | Notify idle seat holders, reclaim after the grace period, appeal through the issue | usage-metrics users report + seat APIs | copilot-admins | High | M |
| **budget-renewal** | Renew or extend an expiring user budget from its original request | budgets API | Same as the original, auto if unchanged | Medium | S |
| **copilot-report-subscription** | Weekly or monthly scheduled ROI report for a team, delivered to a discussion or issue | as report | engineering-managers | High | S |
| **mcp-server-allowlist** | Add an MCP server to the org's Copilot MCP allow-list or registry | PR to policy repo | security + platform-ai | High | M |
| **copilot-custom-agent-publish** | Publish a custom agent or org instructions to the `.github-private` repo | PR through the Contents API | platform-ai + CODEOWNERS | Medium | S |
| **agentic-catalog-onboarding** | Propose a new `gh aw` workflow for the catalog (security review of permissions and safe outputs) | PR + review | security + platform-ai | Medium | M |
| **foundry-quota-increase** | Request regional TPM or PTU quota for a model | `Microsoft.Quota` (`az quota`), support ticket fallback | platform-ai + FinOps (PTU) | High | M |
| **foundry-deployment-retire** | Delete or resize a deployment after notifying consumers listed in the original request | Bicep / `az … deployment delete` | platform-ai | Medium | S |
| **foundry-agent-provision** | Create a Foundry Agent Service agent (instructions, tools, deployment) from a spec | Foundry Agent Service REST | platform-ai + ai-governance | High | M |
| **foundry-content-filter** | Custom RAI (content filter) policy for a deployment | `raiPolicies` resource (Bicep) | ai-governance | Medium | M |
| **model-evaluation** | Run an evaluation Job on AKS comparing deployments on a dataset; post a scorecard | AKS Job + Foundry | platform-ai | Medium | M |
| **aks-namespace-request** | Namespace with quota, LimitRange, NetworkPolicy, RBAC and PSA (Go templates, kubectl 1.30) | Kubernetes API | platform-admins | High | S |
| **grafana-access** | Grafana team or folder permissions for a squad (Grafana OSS 12 HTTP API) | Grafana API | platform-admins | Medium | S |
| **team-membership** | Join a GitHub team with maintainer approval | Teams API | team maintainers (`field_users`) | Medium | S |
| **repository-create** | Repository with rulesets, CODEOWNERS and Copilot settings baked in | Repos, rulesets | platform-admins | High | M |
| **runner-group-access** | Give a repository access to a self-hosted runner group | Actions runner-groups API | platform-admins | Low | S |
| **push-protection-bypass-review** | Structured review of secret-scanning bypass requests | Secret-scanning API | security | Medium | M |

**Suggested order:** seat request → report subscription → budget renewal → Foundry quota increase → AKS namespace → MCP allow-list → Foundry agent provision. Early items reuse existing handlers and APIs and pay back quickly; later ones widen platform coverage.

## Recipe: add a request type

1. **Form:** `.github/ISSUE_TEMPLATE/<type>.yml` with labels `issueops` and `issueops:<short>`. Prefix field ids with the type (e.g. `seat_`). Run `scripts/forms-to-json.sh`.
2. **Registry:** add a `request_types[]` entry with label, template, execute workflow and environment, requestors, approval rules, escalations, auto-approve, settings. Add the label to `config/labels.json`.
3. **Handler:** a package under `internal/<type>` implementing `request.Handler` (`Facts`, `Validate`, `Summary`, `Subject`), plus a plan and apply function. Register it in `engine.DefaultHandlers`.
4. **CLI:** an `issueops <type> plan|apply` subcommand using `verified()` (digest enforcement). Write an `outcome.json`, and add a case to `loadResult`.
5. **Templates:** `comment.completed.<type>.md.tmpl` (use `safe`, `cell` and `code`).
6. **Workflow:** copy an `execute-*.yml`: gate → execute (environment, narrowed token) → report. Add a caller job in `comment-router.yml`.
7. **Validators:** field-level custom validators in `.github/validator/config.yml`, if useful.
8. **Scenario:** `examples/<type>/scenario.json` with `expect` blocks and fixtures. Run `make test examples`.
9. **Docs:** `docs/request-types/<type>.md`, plus rows in the README and reference.
