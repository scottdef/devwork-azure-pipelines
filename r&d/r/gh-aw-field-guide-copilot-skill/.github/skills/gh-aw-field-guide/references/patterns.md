# Design patterns with complete example workflows

> Source: gh-aw Engineer's Field Guide (docs as of Sept 2026). gh-aw is in technical preview; verify field names and flags against https://github.github.com/gh-aw/ before shipping.

## Contents (grep for `^## <name>` to jump)
- IssueOps
- CentralRepoOps
- MonitorOps
- ChatOps
- DeterministicOps
- DispatchOps
- ResearchPlanAssignOps
- OrchestratorOps
- Other documented patterns
- Committing and PRs from workflows



Patterns are trigger + safe-output + token recipes. gh-aw documents a large catalog under Design Patterns. Your eight focus patterns map as follows (naming notes called out where your name differs from the docs).

## IssueOps

IssueOps — react to issue lifecycle (`on.issues.types: [opened,...]`), read sanitized issue text, respond via `add-comment`/`add-labels`. Use for auto-triage. Agent runs `contents: read`; comment creation happens in a separate `issues: write` job. When to use: any "on new/edited issue, reason and respond" task. Tradeoffs: event-driven cost; gate with labels to limit volume. Security: never read raw body; use `needs.activation.outputs.text`.

Complete example (`CoolGitOrg/platform-automation/.github/workflows/issue-triage.md`):
```markdown
---
name: "Issue Triage"
on:
  issues:
    types: [opened, reopened]
permissions:
  contents: read
  issues: read
engine: copilot
network: defaults
tools:
  github:
    toolsets: [issues, repos]
safe-outputs:
  add-comment:
    max: 1
  add-labels:
    allowed: [bug, enhancement, question, needs-triage, area/api, area/ui]
    max: 3
---
# Issue Triage
Triage this issue for CoolGitOrg:
"${{ needs.activation.outputs.text }}"
1. Classify type (bug/enhancement/question) and add the matching label plus at most one `area/*` label.
2. If the description lacks repro steps or version info, post one concise comment asking for exactly what's missing.
3. If it duplicates an existing open issue, say so and link it.
If no action is warranted, call the `noop` tool with a reason.
```
Section-by-section: `on.issues.types` fires on open/reopen. `permissions` are read-only — the agent cannot write. `engine: copilot` uses org Copilot billing (add `copilot-requests: write` or a `COPILOT_GITHUB_TOKEN`). `network: defaults` keeps egress minimal. `tools.github.toolsets` grants read access to issues/repos. `safe-outputs.add-comment`/`add-labels` are the only writes; `allowed:` constrains labels (defense against prompt injection). The body references sanitized text and mandates `noop` to avoid silent completion.

## CentralRepoOps

CentralRepoOps — a MultiRepoOps variant using one private control-plane repo. Two models: (a) a central orchestrator that dispatches per-repo worker workflows, (b) a central tracker where component repos push events for unified visibility. Use for org-wide rollouts (security patches, policy standardization) with phased adoption and a decision trail. Tradeoffs: needs cross-repo tokens; keep orchestrator permissions narrow and delegate repo-specific writes to workers. Security: use an org read token for scanning, per-repo scoped writes in workers.

Complete example — orchestrator (`CoolGitOrg/central-control/.github/workflows/rollout-orchestrator.md`):
```markdown
---
name: "Rollout Orchestrator"
on:
  schedule:
    - cron: "0 9 * * 1"   # weekly Monday 09:00 UTC
  workflow_dispatch:
permissions:
  contents: read
engine: copilot
network: defaults
tools:
  github:
    github-token: ${{ secrets.GH_AW_READ_ORG_TOKEN }}
    toolsets: [repos]
safe-outputs:
  dispatch-workflow:
    workflows: [dependency-upgrade-worker]
    max: 5
---
# Rollout Orchestrator
List CoolGitOrg repositories that depend on the deprecated `internal-sdk` v1.
Categorize by complexity, prioritize a pilot wave of at most 5, and dispatch
`dependency-upgrade-worker` for each with input `target_repo` set to the repo slug.
Summarize the candidates, the wave you chose, and your rationale.
If no action is warranted, call the `noop` tool with a reason.
```
Worker (`CoolGitOrg/central-control/.github/workflows/dependency-upgrade-worker.md`):
```markdown
---
name: "Dependency Upgrade Worker"
on:
  workflow_dispatch:
    inputs:
      target_repo:
        description: "Target repository (CoolGitOrg/name)"
        required: true
permissions:
  contents: read
engine: copilot
network: [defaults, node]
tools:
  github:
    github-token: ${{ secrets.GH_AW_WRITE_REPO_TOKEN }}
    toolsets: [repos, pull_requests]
  edit:
safe-outputs:
  create-pull-request:
    target-repo: ${{ github.event.inputs.target_repo }}
    allowed-repos: ["CoolGitOrg/*"]
    github-token: ${{ secrets.GH_AW_WRITE_REPO_TOKEN }}
    title-prefix: "[sdk-upgrade] "
    labels: [automation, dependencies]
---
# Dependency Upgrade Worker
Analyze ${{ github.event.inputs.target_repo }}, upgrade `internal-sdk` to v2,
fix any breaking call sites, and open a pull request explaining what changed and why.
If no action is warranted, call the `noop` tool with a reason.
```
Explanation: the orchestrator only reads (org read token) and dispatches; the worker holds the write token and does per-repo changes, scoped by `allowed-repos: ["CoolGitOrg/*"]`. `dispatch-workflow` validates at compile time that `dependency-upgrade-worker` exists and supports `workflow_dispatch`.

## MonitorOps

MonitorOps (docs: **MonitorOps**; closely related to Projects & Monitoring and DailyOps) — a scheduled workflow that inspects other workflows' logs/costs/failures via `gh aw logs`/`audit`, posts a durable report (discussion/issue), and escalates repeated failures. The reference implementation is githubnext/agentic-ops. When to use: enough workflow activity that maintainers need a rolling summary. Tradeoffs: scheduled cost; needs `actions: read`.

Complete example (`CoolGitOrg/platform-automation/.github/workflows/aw-monitor.md`):
```markdown
---
name: "Agentic Workflow Monitor"
on:
  schedule:
    - cron: "0 7 * * *"   # daily 07:00 UTC
permissions:
  contents: read
  actions: read
engine: copilot
network: defaults
tools:
  agentic-workflows:
safe-outputs:
  create-discussion:
    category: "reports"
    title-prefix: "[aw-monitor] "
    close-older-issues: true
  create-issue:
    title-prefix: "[aw-alert] "
    labels: [automation, reliability]
    max: 3
---
# Agentic Workflow Monitor
Using the agentic-workflows tools, review the last 24h of agentic workflow runs in this repository.
Summarize per-workflow success rate, AIC spend, and notable failures into a single discussion in the "reports" category.
For any workflow failing 3+ times in the window, open one issue with the run URLs and the likely cause.
If everything is healthy, call `noop`.
```
Explanation: `actions: read` + the `agentic-workflows` MCP tool let the agent read run logs; reporting goes to a discussion, escalation to issues capped at 3.

## ChatOps

ChatOps — command-driven via issue/PR comments (command triggers like `/review`). Reads sanitized comment text, responds in-thread. Use for on-demand bot commands. Tradeoffs: must gate on actor permission (`roles:`) and command position. Security: command triggers run pre-activation role checks; reference `${{ needs.activation.outputs.text }}`.

Complete example (`CoolGitOrg/app-service/.github/workflows/review-command.md`):
```markdown
---
name: "Review Command"
on:
  command:
    name: review
permissions:
  contents: read
  pull-requests: read
engine: copilot
network: defaults
roles: [admin, maintain, write]
tools:
  github:
    toolsets: [pull_requests, repos]
safe-outputs:
  add-comment:
    max: 1
  submit-pull-request-review:
    allowed-events: [COMMENT]
---
# Review Command
A maintainer invoked `/review` on this pull request:
"${{ needs.activation.outputs.text }}"
Review the PR diff for correctness, security, and style. Post a single review (event COMMENT)
with specific, actionable feedback. Do not approve or request changes.
If no action is warranted, call the `noop` tool with a reason.
```
Explanation: `on.command.name: review` builds the slash-command trigger; `roles:` restricts who can invoke; `submit-pull-request-review.allowed-events: [COMMENT]` prevents the bot from ever APPROVE-ing regardless of what the model outputs.

## DeterministicOps

DeterministicOps — combine deterministic `steps:`/`jobs:` (shell/gh/jq) with agent reasoning. Precompute data into `/tmp/gh-aw/agent/` (auto-uploaded as artifacts) or filter triggers deterministically, then let the agent reason over prepared inputs. Use for report generation, hybrid pipelines, and cost reduction (moving no-inference reads out of the LLM loop). Tradeoffs: more workflow code; but big token savings. Closely related: DataOps (deterministic extraction → agentic analysis).

Complete example (`CoolGitOrg/platform-automation/.github/workflows/release-highlights.md`):
```markdown
---
name: "Release Highlights"
on:
  release:
    types: [published]
permissions:
  contents: read
engine: copilot
network: defaults
steps:
  - name: Collect release data
    env:
      GH_TOKEN: ${{ github.token }}
    run: |
      mkdir -p /tmp/gh-aw/agent
      gh pr list --state merged --limit 100 \
        --json number,title,author,mergedAt,labels > /tmp/gh-aw/agent/prs.json
safe-outputs:
  update-release:
---
# Release Highlights
Read `/tmp/gh-aw/agent/prs.json`. Write concise, user-facing release highlights
grouped by theme (features, fixes, breaking changes) and update the release body.
If no action is warranted, call the `noop` tool with a reason.
```
Explanation: the `steps:` block does deterministic data collection with zero tokens; the agent only synthesizes. Files in `/tmp/gh-aw/agent/` are uploaded as artifacts and visible to the agent. (Keep bulky caches/venvs out of `/tmp/gh-aw/agent/`; cache siblings like `/tmp/gh-aw/python/venv` instead.)

## DispatchOps

DispatchOps (docs: **DispatchOps**) — trigger other workflows/repos as the primary purpose, via `dispatch-workflow` (same-repo `workflow_dispatch`, async), `call-workflow` (compile-time `workflow_call` fan-out, synchronous, preserves actor/billing), or `dispatch-repository` (`repository_dispatch`, cross-repo, experimental). Use to bridge agentic decisions into deterministic pipelines or cross-repo events. Tradeoffs: `dispatch-workflow` workers outlive the parent and need `workflow_dispatch` inputs; `call-workflow` blocks until workers finish. Security: `repository_dispatch` to another repo needs a PAT/App with `repo`/contents; `GITHUB_TOKEN`-created events do not recursively trigger further workflows except `workflow_dispatch`/`repository_dispatch`.

Complete example — SideRepoOps relay (bridge) in the main repo (`CoolGitOrg/app-service/.github/workflows/review-relay.md`):
```markdown
---
name: "Review Relay"
on:
  command:
    name: review
permissions:
  contents: read
engine: copilot
network: defaults
safe-outputs:
  github-token: ${{ secrets.GH_AW_SIDE_REPO_TOKEN }}
  dispatch-workflow:
    target-repo: "CoolGitOrg/automation-repo"
    allowed-repos: ["CoolGitOrg/automation-repo"]
    workflows: [review]
    max: 1
---
# Review Relay
Forward this `/review` command to CoolGitOrg/automation-repo for processing,
passing the PR number and sanitized comment body as inputs.
If no action is warranted, call the `noop` tool with a reason.
```
Explanation: slash-commands cannot be handled directly from a side repo, so a thin relay in the main repo forwards via cross-repo `dispatch-workflow` using a side-repo-scoped token.

## ResearchPlanAssignOps

ResearchPlanAssignOps (docs: **ResearchPlanAssignOps**; your "ResearchPlanAssign") — a scaffolded loop that keeps humans in control while delegating research and execution. Phase 1 (Research): a scheduled agent scans the repo/ecosystem and produces a report in an issue/discussion. Phase 2 (Plan): break it into tasks (often grouped sub-issues). Phase 3 (Assign): assign to a coding agent (`assign-to-agent`) or humans. Use for dependency analysis and initiative-level automation. Tradeoffs: multi-run; needs human gates between phases.

Complete example (`CoolGitOrg/platform-automation/.github/workflows/dep-research.md`):
```markdown
---
name: "Dependency Research"
on:
  schedule:
    - cron: "0 6 * * 1"
permissions:
  contents: read
engine: copilot
network: [defaults, node, python]
tools:
  github:
    toolsets: [repos]
  web-search:
safe-outputs:
  create-issue:
    title-prefix: "[dep-research] "
    labels: [dependencies, research]
    group: true
    max: 6
---
# Dependency Research
Scan CoolGitOrg/app-service dependencies for outdated or vulnerable packages.
Produce a parent issue summarizing findings, then create one sub-issue per actionable upgrade
(grouped under the parent). For each, note risk, effort, and a suggested owner.
Assign nothing automatically — leave prioritization to maintainers.
If no action is warranted, call the `noop` tool with a reason.
```
Explanation: `create-issue.group: true` links children under a parent (up to 64), giving a plan a maintainer can triage and then assign (optionally with a follow-up `assign-to-agent` workflow).

## OrchestratorOps

OrchestratorOps — one orchestrator fans work out to worker workflows, each with scoped permissions/tools/engine. Use when a single run is too coarse (multi-repo, per-step permissions, parallelism, human review between phases). `dispatch-workflow` for async independent runs; `call-workflow` for synchronous fan-out that preserves actor attribution and billing. Pass a `tracker_id` correlation input. See the CentralRepoOps example above for a concrete orchestrator/worker pair; OrchestratorOps is the general pattern, CentralRepoOps its cross-repo control-plane specialization.

## Other documented patterns

 BatchOps (parallel processing of large item volumes), LabelOps (`issues.types:[labeled]` + `names:` gating), MemoryOps (cache/repo memory state), MultiRepoOps (cross-repo via `target-repo`), SideRepoOps (dedicated automation repo targeting a main codebase), ProjectOps (GitHub Projects v2 boards), SpecOps (spec-driven), WorkQueueOps (sequential ordered processing), FeatureOps/Feature Grower, DataOps (deterministic extraction → analysis), DailyOps (scheduled reports/maintenance), and experimental CorrectionOps and Monitoring with Projects. TaskOps is not a distinct documented page; it maps to WorkQueueOps/BatchOps. "ProjectOps"/"DailyOps"/"SideRepoOps" cover the "MonitorOps vs ProjectOps/DailyOps" and "ResearchPlanAssign vs ResearchPlanAssignOps" naming you flagged.

## Committing and PRs from workflows

 `create-pull-request` packages the agent's commits as a git bundle uploaded as an artifact; a separate permission-controlled job checks out the base branch, applies the bundle, and pushes via the GraphQL API (signed commits) to open the PR. `push-to-pull-request-branch` pushes to an existing PR branch (auto-enables git commands: checkout/branch/switch/add/rm/commit/merge). PRs opened with `GITHUB_TOKEN` produce runs that require approval before CI — use `repository_dispatch` or a PAT-backed CI-trigger token if you need CI to run automatically.

