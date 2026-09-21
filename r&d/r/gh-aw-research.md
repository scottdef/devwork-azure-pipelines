# GitHub Agentic Workflows (gh-aw): An Engineer's Field Guide

## Table of Contents
1. Foundations
2. Design Patterns (IssueOps, CentralRepoOps, MonitorOps, ChatOps, DeterministicOps, DispatchOps, ResearchPlanAssignOps, OrchestratorOps + the rest)
3. Computer Use / Browser Automation (manual only)
4. CLI Commands Guide + Cheatsheet
5. Creation Wizard
6. Enterprise Rollout for CoolGitEnterprise / CoolGitOrg
7. Troubleshooting, Gotchas, FAQ, Quick Reference

## TL;DR
- **gh-aw is a `gh` CLI extension that compiles Markdown-plus-YAML-frontmatter workflow files into hardened GitHub Actions `.lock.yml` workflows** that run an AI agent (Copilot, Claude, Codex, Gemini, or Pi) read-only and sandboxed, then apply any GitHub writes through separate, permission-scoped "safe-output" jobs. Both the `.md` source and the `.lock.yml` are committed; the `.md` is the source of truth.
- **The security model is the product**: a read-only agent job behind a Squid egress firewall (AWF) and MCP gateway, buffered writes, an AI threat-detection gate, and compile-time scanners (actionlint/zizmor/poutine). For CoolGitEnterprise this maps cleanly onto Actions allow-lists, org/enterprise `GH_AW_*` policy variables, and fine-grained-PAT-or-GitHub-App auth.
- **Design patterns are just trigger + safe-output + token recipes.** Learn the eight focus patterns (IssueOps, CentralRepoOps, MonitorOps, ChatOps, DeterministicOps, DispatchOps, ResearchPlanAssignOps, OrchestratorOps) plus the CLI (`init/new/add-wizard/compile/run/logs/audit`) and you can build, ship, and govern the rest.

## Key Findings
- gh-aw is a collaboration among **GitHub Next, Microsoft Research, and Azure Core Upstream**, created by Don Syme and Peli de Halleux at GitHub Next; MIT-licensed with 300+ releases (repo: github/gh-aw). It is **Actions-first**: every run is a normal Actions run with native triggers, runners, logs, and spending limits.
- It is in **technical preview** (GitHub Changelog, 2026-02-13: "GitHub Agentic Workflows are now in technical preview … available in technical preview through the gh aw CLI extension").
- Built-in engines: GitHub Copilot (default), Claude Code, OpenAI Codex, Google Gemini, and Pi. Each selects via `engine:` and authenticates with its own secret or permission.
- Writes are **declared, not granted**. The agent has no write token; it emits structured `safe-outputs` requests (create-issue, add-comment, create-pull-request, push-to-pull-request-branch, dispatch-workflow, etc.) that a separate scoped job applies after a threat-detection pass.
- Workflows trigger workflows via the `dispatch-workflow` (async `workflow_dispatch`), `call-workflow` (compile-time `workflow_call` fan-out), and `dispatch-repository` (`repository_dispatch`, cross-repo) safe outputs — the basis of OrchestratorOps and CentralRepoOps.
- Computer-use: Playwright is a first-class tool (`tools.playwright`), CLI mode recommended; a manually-triggered (`workflow_dispatch`) browser workflow with an explicit `network.allowed` domain allow-list and screenshot upload is straightforward and safe.
- "go-surf" is **not** a gh-aw feature. The closest real Go project is **headzoo/surf** (`gopkg.in/headzoo/surf.v1`), a stateful programmatic browser; it can be wired in only as a custom bash/MCP step, not as a built-in.
- Enterprise governance: the `github/gh-aw@*` action must be allow-listed; `GH_AW_DEFAULT_*` and `GH_AW_POLICY_*` GitHub Actions variables percolate enterprise → org → repo; Terraform's `integrations/github` provider manages the secrets, variables, and allowed-actions policy.

## Details

### 1. Foundations

**What it is.** GitHub Agentic Workflows (`gh-aw`) lets you define AI-powered repository automation in Markdown with YAML frontmatter and run AI agents securely through GitHub Actions. The `gh aw` CLI extension compiles each agentic workflow into a standard GitHub Actions workflow. Use conventional Actions for deterministic builds/tests/deploys; per the README, "Add an agentic workflow when a task needs reasoning or interpretation, such as issue triage, pull-request review, CI failure investigation, documentation maintenance, dependency analysis, or repository reporting. GitHub Agentic Workflows complements existing CI/CD; it does not replace it."

**Anatomy.** A workflow is a `.md` file in `.github/workflows/`. Frontmatter (between `---` markers) configures `on:` (triggers), `permissions:`, `engine:`, `tools:`, `network:`, and `safe-outputs:`. The Markdown body is the natural-language task brief. `gh aw compile` validates the source and generates a `.lock.yml` — the hardened Actions workflow that embeds compiled config and loads the body at runtime. Commit both; the `.md` is the editable source of truth, the `.lock.yml` is generated and should never be hand-edited.

**Compile step.** `gh aw compile` performs schema validation, expression-safety checks (allowlisted expressions, no secrets in expressions), SHA-pins every action, and runs security scanners (actionlint incl. shellcheck/pyflakes; zizmor; poutine). Remote imports are cached under `.github/aw/imports/`. `--strict` enforces best practices: no write permissions (use safe-outputs), explicit `network:`, no wildcard domains, pinned actions, no deprecated fields.

**Engines.**

| Engine | `engine:` | Secret or permission |
|---|---|---|
| GitHub Copilot (default) | `copilot` | `copilot-requests: write` permission (recommended, uses `${{ github.token }}`) or `COPILOT_GITHUB_TOKEN` |
| Claude Code | `claude` | `ANTHROPIC_API_KEY` or Anthropic WIF |
| OpenAI Codex | `codex` | `CODEX_API_KEY` or `OPENAI_API_KEY` (runtime tries `CODEX_API_KEY` first) |
| Google Gemini | `gemini` | `GEMINI_API_KEY` or Google WIF |
| Pi | `pi` | Copilot/Anthropic/OpenAI auth depending on `model:` |

**Tools and MCP.** Workflows expose only configured tools. `tools.github` gives read toolsets (context, repos, issues, pull_requests, actions, code_security, discussions, labels, and more; `dependabot` must be opted-in explicitly). `tools.bash` defaults to a safe command set (echo, ls, pwd, cat, head, tail, grep, wc, sort, uniq, date); use `git:*` for command families or `":*"`/`"*"` for unrestricted (review carefully). `tools.edit`, `tools.web-search`, `tools.playwright`, `tools.serena` are additional built-ins. External integrations use MCP servers; custom servers run in isolated Docker containers with per-container network allowlists and `allowed:` tool filters. `gh-aw` can itself run as an MCP server (`gh aw mcp-server`) exposing status/compile/logs/audit/etc. as tools.

**Safe-outputs.** The `safe-outputs:` block declares what the workflow may produce. When absent (or only system types configured), `create-issue` is auto-enabled with conservative defaults (`max: 1`). The catalog is large; the ones you'll use most:
- Issues/discussions: `create-issue`, `update-issue`, `close-issue`, `create-discussion`, `close-discussion`, `update-discussion`.
- PRs: `create-pull-request`, `update-pull-request`, `push-to-pull-request-branch`, `create-pull-request-review-comment`, `submit-pull-request-review`, `add-reviewer`.
- Labels/assignment: `add-labels`, `remove-labels`, `assign-to-user`, `assign-to-agent`, `set-issue-type`, `set-issue-field`.
- Orchestration: `dispatch-workflow`, `call-workflow`, `dispatch-repository`.
- Security/other: `create-code-scanning-alert`, `autofix-code-scanning-alert`, `create-check-run`, `upload-artifact` (preferred), `upload-asset`.
- System (auto): `noop` (MUST be called when no action is taken), `missing-tool`, `missing-data`.

Cross-repo writes use `target-repo` + `allowed-repos` (wildcards like `org/*` supported) plus a custom `github-token:` or `github-app:`.

**Safe-inputs (content sanitization).** Event text reaches the agent only after sanitization: `@mention` neutralization, bot-trigger neutralization (`fixes #123`), XML/HTML tag conversion, URI filtering (HTTPS only from allowed domains; others `(redacted)`), Unicode normalization, 0.5MB/65k-line limits, ANSI stripping. Access sanitized text via `${{ needs.activation.outputs.text }}` — never raw `github.event.*.body`. Integrity filtering (`min-integrity`) additionally filters GitHub content by author trust/merge status; public repos get `min-integrity: approved` automatically.

**Network / firewall.** The Agent Workflow Firewall (AWF) containerizes the agent on an internal Docker network whose only egress is a Squid proxy enforcing a domain allow-list. Configure with `network:` — `defaults` (basic infra), ecosystem bundles (`python`, `node`, `github`, `playwright`), explicit domains and wildcards, and `blocked:` (takes precedence). Same list governs URI sanitization. `gh aw domains <wf>` prints the effective allow/block set including ecosystem expansions.

**Permissions & security architecture.** Defense-in-depth over three trust layers: substrate (runner/kernel/container + AWF + API proxy + MCP gateway), configuration (schema validation, action SHA pinning, scanners, pre-activation role checks), and plan (SafeOutputs staging, threat detection, content sanitization, secret redaction). The agent job is read-only; buffered writes go to `agent_output.json`, a separate threat-detection job (AI analysis for prompt injection, secret leaks, malicious patches) emits a pass/fail verdict that gates the safe-output jobs; secret redaction runs `if: always()` over `/tmp/gh-aw` before artifact upload. Job order: pre-activation → activation (sanitize) → agent (read-only) → detection → safe-output jobs (scoped writes) → conclusion. Note the docs' own caution: the deterministic vetting "can only catch patterns that GitHub anticipated," and the MCP-gateway API key mounted into the agent is "leaked by design" — rely on substrate isolation, network policy, and staged permission separation, plus human review.

**Sandbox runtimes.** Default is Docker container isolation. Optional: gVisor (`sandbox.agent.runtime: gvisor`) or Docker sbx KVM microVM (`docker-sbx`, needs `sandbox.agent.sudo: true`, KVM, `DOCKER_USERNAME`/`DOCKER_PAT`). ARC with Docker-in-Docker uses `runner.topology: arc-dind`; gVisor/sbx are incompatible with arc-dind.

**Imports / shared components.** `imports:` pulls shared fragments (tool configs, MCP defs, prompt snippets) from local files or remote refs (`owner/repo/path@ref`). Files without an `on:` field are shared components (relaxed schema, skip compilation). `inlined-imports: true` inlines them so ruleset-required checks don't fail on runtime import resolution. APM packages are imported via `imports: - uses: shared/apm.md` (the old top-level `dependencies:` is deprecated).

**Memory.** `tools.cache-memory` stores files at `/tmp/gh-aw/cache-memory/` behind a workflow-scoped Actions cache key (default extensions `.json/.jsonl/.txt/.md/.csv`). `tools.repo-memory` persists to a dedicated git branch (`.jsonl` uses union-merge to survive concurrent appends). Use cache-memory for ephemeral run-to-run state, repo-memory for durable audit logs.

**Concurrency.** Default per-engine group `gh-aw-{engine-id}` ensures only one agent job per engine runs at once (prevents AI resource exhaustion). Non-cancel triggers also emit `queue: max` so back-to-back triggers queue instead of displacing. `safe-outputs.concurrency-group` serializes the safe-output job when needed.

**Cost / tokens.** Spend is measured in AI Credits (AIC); **one AIC is estimated at $0.01** (billing estimate; actual varies by provider and prompt/tool/model). Cap per run with the top-level `max-ai-credits` frontmatter field. Threat detection carries its **own** budget separate from the agent: `safe-outputs.threat-detection.max-ai-credits` defaults to 400, overridable via `GH_AW_DEFAULT_DETECTION_MAX_AI_CREDITS`. A Copilot run roughly consumes two premium requests (agent + threat detection). Control cost with `skip-if-match`/`skip-if-no-match` (pre-activation, before inference), `max-turns`, smaller models, `user-rate-limit`, concurrency, and by preferring `schedule`/`workflow_dispatch` over high-volume `push`/`check_run` triggers while evaluating. `gh aw forecast` projects AIC; `gh aw logs`/`audit`/`health` report actuals. The single biggest lever is moving deterministic reads into pre-agent `steps:` (DeterministicOps) — GitHub's own measurements report −62% on Auto-Triage after doing exactly that.

**GHEC governance mapping.** For CoolGitEnterprise: allow `github/gh-aw@*` in Actions policy; set `GH_AW_DEFAULT_*` (models, AIC caps, timeouts) and `GH_AW_POLICY_*` (capability gates) as enterprise/org variables; scope engine secrets per-org; require review of `.lock.yml` diffs via CODEOWNERS; compile with `--strict` in CI.

### 2. Design Patterns

Patterns are trigger + safe-output + token recipes. gh-aw documents a large catalog under Design Patterns. Your eight focus patterns map as follows (naming notes called out where your name differs from the docs).

**IssueOps** — react to issue lifecycle (`on.issues.types: [opened,...]`), read sanitized issue text, respond via `add-comment`/`add-labels`. Use for auto-triage. Agent runs `contents: read`; comment creation happens in a separate `issues: write` job. When to use: any "on new/edited issue, reason and respond" task. Tradeoffs: event-driven cost; gate with labels to limit volume. Security: never read raw body; use `needs.activation.outputs.text`.

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

**CentralRepoOps** — a MultiRepoOps variant using one private control-plane repo. Two models: (a) a central orchestrator that dispatches per-repo worker workflows, (b) a central tracker where component repos push events for unified visibility. Use for org-wide rollouts (security patches, policy standardization) with phased adoption and a decision trail. Tradeoffs: needs cross-repo tokens; keep orchestrator permissions narrow and delegate repo-specific writes to workers. Security: use an org read token for scanning, per-repo scoped writes in workers.

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
```
Explanation: the orchestrator only reads (org read token) and dispatches; the worker holds the write token and does per-repo changes, scoped by `allowed-repos: ["CoolGitOrg/*"]`. `dispatch-workflow` validates at compile time that `dependency-upgrade-worker` exists and supports `workflow_dispatch`.

**MonitorOps** (docs: **MonitorOps**; closely related to Projects & Monitoring and DailyOps) — a scheduled workflow that inspects other workflows' logs/costs/failures via `gh aw logs`/`audit`, posts a durable report (discussion/issue), and escalates repeated failures. The reference implementation is githubnext/agentic-ops. When to use: enough workflow activity that maintainers need a rolling summary. Tradeoffs: scheduled cost; needs `actions: read`.

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

**ChatOps** — command-driven via issue/PR comments (command triggers like `/review`). Reads sanitized comment text, responds in-thread. Use for on-demand bot commands. Tradeoffs: must gate on actor permission (`roles:`) and command position. Security: command triggers run pre-activation role checks; reference `${{ needs.activation.outputs.text }}`.

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
```
Explanation: `on.command.name: review` builds the slash-command trigger; `roles:` restricts who can invoke; `submit-pull-request-review.allowed-events: [COMMENT]` prevents the bot from ever APPROVE-ing regardless of what the model outputs.

**DeterministicOps** — combine deterministic `steps:`/`jobs:` (shell/gh/jq) with agent reasoning. Precompute data into `/tmp/gh-aw/agent/` (auto-uploaded as artifacts) or filter triggers deterministically, then let the agent reason over prepared inputs. Use for report generation, hybrid pipelines, and cost reduction (moving no-inference reads out of the LLM loop). Tradeoffs: more workflow code; but big token savings. Closely related: DataOps (deterministic extraction → agentic analysis).

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
```
Explanation: the `steps:` block does deterministic data collection with zero tokens; the agent only synthesizes. Files in `/tmp/gh-aw/agent/` are uploaded as artifacts and visible to the agent. (Keep bulky caches/venvs out of `/tmp/gh-aw/agent/`; cache siblings like `/tmp/gh-aw/python/venv` instead.)

**DispatchOps** (docs: **DispatchOps**) — trigger other workflows/repos as the primary purpose, via `dispatch-workflow` (same-repo `workflow_dispatch`, async), `call-workflow` (compile-time `workflow_call` fan-out, synchronous, preserves actor/billing), or `dispatch-repository` (`repository_dispatch`, cross-repo, experimental). Use to bridge agentic decisions into deterministic pipelines or cross-repo events. Tradeoffs: `dispatch-workflow` workers outlive the parent and need `workflow_dispatch` inputs; `call-workflow` blocks until workers finish. Security: `repository_dispatch` to another repo needs a PAT/App with `repo`/contents; `GITHUB_TOKEN`-created events do not recursively trigger further workflows except `workflow_dispatch`/`repository_dispatch`.

Complete example — SideRepoOps relay (bridge) in the main repo (`CoolGitOrg/app-service/.github/workflows/review-relay.md`):
```markdown
---
name: "Review Relay"
on:
  command:
    name: review
permissions:
  contents: read
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
```
Explanation: slash-commands cannot be handled directly from a side repo, so a thin relay in the main repo forwards via cross-repo `dispatch-workflow` using a side-repo-scoped token.

**ResearchPlanAssignOps** (docs: **ResearchPlanAssignOps**; your "ResearchPlanAssign") — a scaffolded loop that keeps humans in control while delegating research and execution. Phase 1 (Research): a scheduled agent scans the repo/ecosystem and produces a report in an issue/discussion. Phase 2 (Plan): break it into tasks (often grouped sub-issues). Phase 3 (Assign): assign to a coding agent (`assign-to-agent`) or humans. Use for dependency analysis and initiative-level automation. Tradeoffs: multi-run; needs human gates between phases.

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
```
Explanation: `create-issue.group: true` links children under a parent (up to 64), giving a plan a maintainer can triage and then assign (optionally with a follow-up `assign-to-agent` workflow).

**OrchestratorOps** — one orchestrator fans work out to worker workflows, each with scoped permissions/tools/engine. Use when a single run is too coarse (multi-repo, per-step permissions, parallelism, human review between phases). `dispatch-workflow` for async independent runs; `call-workflow` for synchronous fan-out that preserves actor attribution and billing. Pass a `tracker_id` correlation input. See the CentralRepoOps example above for a concrete orchestrator/worker pair; OrchestratorOps is the general pattern, CentralRepoOps its cross-repo control-plane specialization.

**Other documented patterns (for completeness):** BatchOps (parallel processing of large item volumes), LabelOps (`issues.types:[labeled]` + `names:` gating), MemoryOps (cache/repo memory state), MultiRepoOps (cross-repo via `target-repo`), SideRepoOps (dedicated automation repo targeting a main codebase), ProjectOps (GitHub Projects v2 boards), SpecOps (spec-driven), WorkQueueOps (sequential ordered processing), FeatureOps/Feature Grower, DataOps (deterministic extraction → analysis), DailyOps (scheduled reports/maintenance), and experimental CorrectionOps and Monitoring with Projects. TaskOps is not a distinct documented page; it maps to WorkQueueOps/BatchOps. "ProjectOps"/"DailyOps"/"SideRepoOps" cover the "MonitorOps vs ProjectOps/DailyOps" and "ResearchPlanAssign vs ResearchPlanAssignOps" naming you flagged.

**Committing/PRs from workflows.** `create-pull-request` packages the agent's commits as a git bundle uploaded as an artifact; a separate permission-controlled job checks out the base branch, applies the bundle, and pushes via the GraphQL API (signed commits) to open the PR. `push-to-pull-request-branch` pushes to an existing PR branch (auto-enables git commands: checkout/branch/switch/add/rm/commit/merge). PRs opened with `GITHUB_TOKEN` produce runs that require approval before CI — use `repository_dispatch` or a PAT-backed CI-trigger token if you need CI to run automatically.

### 3. Computer Use / Browser Automation (manual trigger only)

Playwright is the supported browser tool. CLI mode (`tools.playwright.mode: cli`) is recommended — token-efficient, no Docker overhead, reaches `localhost` directly; it installs `@playwright/cli` globally and the agent calls `playwright-cli <command>` from bash. MCP mode is deprecated (emits a compile-time warning). Domain access is controlled by `network:`; by default Playwright reaches only `localhost`/`127.0.0.1`. Add `playwright` (enables browser downloads) plus explicit domains. Chromium/Firefox/WebKit are available.

Complete example (`CoolGitOrg/platform-automation/.github/workflows/browser-check.md`):
```markdown
---
name: "Manual Browser Check"
on:
  workflow_dispatch:
    inputs:
      url:
        description: "URL to inspect (must be under an allowed domain)"
        required: true
        default: "https://status.coolgitorg.example.com"
      viewport:
        description: "Viewport WxH"
        required: false
        default: "1440x900"
permissions:
  contents: read
engine: copilot
timeout_minutes: 15
tools:
  playwright:
    mode: cli
    version: "0.1.13"
  bash:
    - "playwright-cli:*"
network:
  allowed:
    - defaults
    - playwright
    - "status.coolgitorg.example.com"
    - "*.coolgitorg.example.com"
safe-outputs:
  upload-artifact:
    max-uploads: 1
    retention-days: 7
  create-issue:
    title-prefix: "[browser-check] "
    labels: [automation, ops]
    max: 1
---
# Manual Browser Check
Only inspect the domains in the network allow-list. Navigate to `${{ github.event.inputs.url }}`,
resize to `${{ github.event.inputs.viewport }}`, capture a full-page screenshot to
`/tmp/gh-aw/agent/screenshot.png`, and check for HTTP errors, console errors, and broken images.
```bash
playwright-cli browser_navigate --url "${{ github.event.inputs.url }}"
playwright-cli browser_take_screenshot --filename /tmp/gh-aw/agent/screenshot.png --full-page true
```
Upload the screenshot as an artifact and, if you find problems, open one issue summarizing them with the screenshot referenced. Otherwise call `noop`.
```
Hardening: `workflow_dispatch`-only (no automatic triggers); an explicit `network.allowed` list (Playwright cannot reach anything else — the AWF firewall drops it); `bash` restricted to `playwright-cli:*`; a `timeout_minutes` cap; screenshots via `upload-artifact` (auto-expiring, preferred over `upload-asset`); read-only permissions with writes only through safe-outputs. Pin `version:` to avoid browser-engine baseline drift.

**go-surf.** There is **no gh-aw "go-surf" tool and no verifiable project named exactly "go-surf."** The closest real Go project is **headzoo/surf** (`gopkg.in/headzoo/surf.v1`), a stateful programmatic virtual browser (cookies, history, form submission, goquery CSS selection) — not a headless-Chromium driver. Other Go browser options are chromedp and rod (DevTools Protocol) and playwright-go. gh-aw has no built-in Go browser tool, so a Go-based browser would be wired in either as (a) a bash step invoking a compiled Go binary within the AWF network allow-list, or (b) a custom MCP server (`mcp-servers:` entry running a container that exposes the Go tool). Treat that as a custom, unvetted integration: pin it, restrict `allowed:` tools, and constrain `network:`.

### 4. CLI Commands Guide + Cheatsheet

Install: `gh extension install github/gh-aw` (pin with `@v0.x.y` or a SHA; upgrade via `gh extension remove gh-aw && gh extension install github/gh-aw@vX`). Standalone installer: `curl -sL https://raw.githubusercontent.com/github/gh-aw/main/install-gh-aw.sh | bash`. In Actions use `github/gh-aw/actions/setup-cli@main`. Global flags: `-v/--verbose`, `--banner`, `--version`, `-h/--help`.

**Getting workflows.** `gh aw init` sets up a repo (`.gitattributes`, dispatcher skill `.github/skills/agentic-workflows/SKILL.md`, and for Copilot the custom agent `.github/agents/agentic-workflows.md` + MCP wiring; `--no-mcp/--no-agent/--no-skill`, `--engine`, `--codespaces`, `--create-pull-request`). `gh aw add-wizard OWNER/REPO/NAME` interactive install with secret/auth prompts. `gh aw add ./wf.md | OWNER/REPO/NAME[@ref] | https://...` non-interactive. `gh aw new [name]` scaffold or interactive. `gh aw secrets set/bootstrap`. `gh aw doctor` diagnostics.

**Building.** `gh aw compile` (flags: `--strict`, `--watch/-w`, `--validate`, `--no-emit`, `--purge`, `--fix`, `--approve`, `--zizmor`, `--actionlint`, `--poutine`, `--yamllint`, `--grype`, `--syft`, `--grant`, `--runner-guard`, `--dependabot`, `--ghes`, `--gh-aw-ref`, `--json`). `gh aw validate` = compile with all linters, no emit. `gh aw lint` runs actionlint on `.lock.yml` only. `gh aw fix` (codemods; `--write`, `--list-codemods`). `gh aw format`. `gh aw edit` (experimental frontmatter edits).

**Testing.** `gh aw run <wf>` dispatches immediately (`--push` commits+pushes+dispatches, `--repeat`, `--ref`, `--dry-run`, `--json`). `gh aw trial` tests in a temp private repo (`--host-repo`, `--logical-repo`, `--dry-run`).

**Monitoring/debug.** `gh aw list`, `gh aw status [--ref]`, `gh aw logs [wf] [-c N] [--start-date] [--json] [--train]`, `gh aw audit <run-id|url> [--parse] [--json]` (single-run report or multi-run diff), `gh aw health [--days 7|30|90] [--threshold]`, `gh aw forecast`, `gh aw outcomes <run-id>`, `gh aw models`, `gh aw checks <pr>`, `gh aw experiments`, `gh aw graders`.

**Management.** `gh aw enable/disable/remove`, `gh aw update` (3-way merge from `source:`; `--org`, `--repos`, `--create-pull-request`, `--yes`), `gh aw deploy OWNER/REPO/NAME --repo target` (roll out via PR; `--org`/`--repos`), `gh aw upgrade` (agent files + codemods; `--audit`), `gh aw env get/update` (governance variables; `--scope repo|org|ent`).

**Advanced/utility.** `gh aw mcp list|list-tools|inspect|add`, `gh aw mcp-server` (expose gh-aw as MCP tools; `--port`, `--validate-actor`), `gh aw pr transfer <url> --repo target`, `gh aw domains [wf]`, `gh aw project new`, `gh aw completion`, `gh aw version`, `gh aw hash-frontmatter`.

Debugging flow: `gh aw logs <wf>` to see tool/network/errors, then `gh aw audit <run-id>` for the rich per-run report (engine config, MCP health, safe-output summary, firewall analysis, failure analysis), then inspect the generated `.lock.yml`. Enable API debug with `DEBUG=cli:logs_github_api,cli:logs_download gh aw logs --verbose --json`.

Cheatsheet:

| Task | Command |
|---|---|
| Install / pin | `gh extension install github/gh-aw[@vX]` |
| Set up repo | `gh aw init [--engine copilot]` |
| Add workflow (guided) | `gh aw add-wizard CoolGitOrg/some-repo/triage` |
| Add workflow (scripted) | `gh aw add githubnext/agentics/ci-doctor` |
| New from scratch | `gh aw new my-wf --engine copilot` |
| Compile all | `gh aw compile` |
| Strict + watch | `gh aw compile --strict --watch` |
| Validate (no emit) | `gh aw validate --strict` |
| Purge orphaned locks | `gh aw compile --purge` |
| Run now | `gh aw run my-wf` |
| Commit+push+run | `gh aw run my-wf --push` |
| Trial safely | `gh aw trial ./my-wf.md` |
| Status / list | `gh aw status` / `gh aw list` |
| Logs | `gh aw logs my-wf -c 10` |
| Audit a run | `gh aw audit <run-id>` |
| Diff two runs | `gh aw audit <base> <cmp>` |
| Health | `gh aw health --days 30` |
| Forecast cost | `gh aw forecast` |
| Effective domains | `gh aw domains my-wf` |
| MCP inspect | `gh aw mcp inspect my-wf` |
| Secrets bootstrap | `gh aw secrets bootstrap` |
| Governance vars | `gh aw env update org.yml --scope org --org CoolGitOrg` |
| Deploy to repo | `gh aw deploy CoolGitOrg/central/wf --repo CoolGitOrg/app` |
| Update from upstream | `gh aw update --create-pull-request` |
| Enable/disable | `gh aw enable my-wf` / `gh aw disable my-wf` |

### 5. Creation Wizard

There are several authoring paths. A literal "gh-aw-wizard" exists as a **separate browser project** (githubnext/gh-aw-wizard — a WebLLM-driven web UI that generates a prompt for a coding agent), but the primary in-CLI creation experiences are:

1. **`gh aw add-wizard OWNER/REPO/NAME`** — interactive install of an existing workflow. Checks prerequisites, prompts engine selection, walks secret/auth setup (for Copilot: choose org billing via `copilot-requests: write` or a `COPILOT_GITHUB_TOKEN` PAT), adds the `.md` + generated `.lock.yml`, and offers to add support files for agentic authoring. Example session for CoolGitOrg:
```
$ gh aw add-wizard githubnext/agentics/daily-repo-status
? Select AI engine: GitHub Copilot
? Copilot auth: Organization billing (copilot-requests: write)  [no PAT]
? Add repo support files for agentic authoring? Yes
✓ Added .github/workflows/daily-repo-status.md
✓ Compiled .github/workflows/daily-repo-status.lock.yml
? Open a pull request against CoolGitOrg/platform-automation? Yes
```

2. **`gh aw new`** (no name) — interactive scaffolding of a fresh template in `.github/workflows/`.

3. **`gh aw init`** — one-time repo setup that installs the dispatcher skill and (for Copilot) the `agentic-workflows` custom agent + MCP wiring, so a coding agent can author workflows conversationally.

4. **Conversational authoring via the `create.md` prompt / custom agent.** From Copilot Chat, the github.com agent, VS Code, or the coding agent, point the agent at `https://raw.githubusercontent.com/github/gh-aw/main/create.md` and describe intent, e.g.:
```
Create a workflow for GitHub Agentic Workflows using https://raw.githubusercontent.com/github/gh-aw/main/create.md
The purpose is to triage new issues in CoolGitOrg/app-service: label by type/priority, find duplicates, and ask clarifying questions when the description is unclear.
```
The custom agent asks clarifying questions, generates the `.md`, compiles it, and can commit. The docs' Creation Wizard page describes using the wizard with a coding agent (Claude Code / Copilot CLI) to select triggers, tools, safe outputs, and permissions, then generate the workflow. The installed agent also supports scenario evaluation without creating files (e.g., "agentic-workflows evaluate this scenario without creating files").

### 6. Enterprise Rollout for CoolGitEnterprise / CoolGitOrg

**Prerequisites.** GitHub CLI ≥ 2.45, `gh extension install github/gh-aw`, repo admin to manage secrets and enable workflows, and the `github/gh-aw@*` action allow-listed at enterprise/org.

**Allow-list the action.** Compiled `.lock.yml` files call `github/gh-aw/actions/setup@<SHA>` and other `github/gh-aw/actions/*`. In enterprise Settings → Actions → Policies choose an allowed-actions mode that permits non-enterprise actions and add `github/gh-aw@*` (or in a centralized `policies/actions.yml`, add `github/gh-aw@*` under `allowed_actions`). Also enable "Allow actions created by GitHub." Org policies override repo settings; wait for propagation.

**Secrets/tokens per engine.**
- Copilot: prefer `copilot-requests: write` permission (uses `${{ github.token }}`, no PAT, requires org Copilot with centralized billing). Alternative `COPILOT_GITHUB_TOKEN` must be a fine-grained PAT (Account → Copilot Requests: Read), user-owned; GitHub Apps and OAuth (`gho_`) tokens are rejected (the run fails early on a `gho_` prefix).
- Claude: `ANTHROPIC_API_KEY` (or Anthropic WIF via `engine.auth`). `CLAUDE_CODE_OAUTH_TOKEN` is silently ignored — the run then fails with an unhelpful Claude CLI auth error, which is your signal to switch credentials.
- Codex: `CODEX_API_KEY` or `OPENAI_API_KEY` (runtime tries `CODEX_API_KEY` first).
- Gemini: `GEMINI_API_KEY` (or Google WIF).
- `GH_AW_GITHUB_TOKEN`: optional "magic" fallback token for GitHub operations outside inference (fallback chain: custom `github-token` → `GH_AW_GITHUB_TOKEN` → `GITHUB_TOKEN`). Fine-grained PAT; not a substitute for Copilot inference auth.
- Project safe-outputs need a write-capable PAT or GitHub App (default `GITHUB_TOKEN` lacks Projects v2 access); the ProjectOps pattern uses separate read/write project tokens (verify the exact variable names on the Authentication (Projects) doc page before wiring).

Use `gh aw secrets bootstrap` to detect missing secrets and `gh aw secrets set NAME` to create them.

**GitHub App vs PAT.** For short-lived, least-privilege tokens, configure `safe-outputs.github-app` (`client-id` from a `vars.APP_ID`, `private-key` from `secrets.APP_PRIVATE_KEY`, optional `owner`/`repositories`). Tokens are minted with permissions matching the specific safe-output operation and revoked at workflow end (even on failure). Per-handler `github-app:` overrides mint a dedicated token for that handler only (e.g., `add-comment` with only `issues: write` while `dispatch-workflow` uses an app with `actions: write`). `ignore-if-missing: true` falls back to `GH_AW_GITHUB_TOKEN`/`GITHUB_TOKEN` (e.g., fork PRs). Copilot inference still requires a PAT, not an App.

```yaml
safe-outputs:
  github-app:
    client-id: ${{ vars.APP_ID }}
    private-key: ${{ secrets.APP_PRIVATE_KEY }}
    owner: "CoolGitOrg"
    repositories: ["app-service", "platform-automation"]
  create-issue:
```

**Central repo strategy (CentralRepoOps).** Keep agentic workflows in a private control-plane repo (e.g., `CoolGitOrg/central-control`). Read with an org-scoped read token; delegate writes to per-repo workers with narrowly scoped tokens/Apps and `allowed-repos: ["CoolGitOrg/*"]`. Roll out with `gh aw deploy ... --org CoolGitOrg --repos '*-service'`.

**Governance variables.** `gh aw env get/update` manages `GH_AW_DEFAULT_*` (models, AIC caps, timeouts) at `--scope repo|org|ent`. Precedence: frontmatter → repo var → org var → enterprise var → compiler fallback. Some defaults (`GH_AW_DEFAULT_MAX_TURNS`, `..._DETECTION_MODEL`, `..._UTC`, `..._MAX_TURN_CACHE_MISSES`) are compile-time — pass them into the CI compile step env. Policy gates `GH_AW_POLICY_*` are boolean capability switches read at runtime; e.g., `GH_AW_POLICY_ALLOW_CREATE_PULL_REQUEST=false` blocks all PR creation org-wide without recompiling (the safe-outputs server refuses to start for any workflow with `create-pull-request` configured).

```yaml
# org-defaults.yml → gh aw env update org-defaults.yml --scope org --org CoolGitOrg
default_max_ai_credits: "5M"
default_max_daily_ai_credits: "15M"
default_max_turns: "12"
default_timeout_minutes: "30"
default_model_copilot: "gpt-5-mini"
default_detection_model: "gpt-5.5-mini"
```

**Review of `.lock.yml`, CODEOWNERS, strict mode.** Commit both `.md` and `.lock.yml`; never hand-edit the lock — recompile. Require review of lock diffs and add a CODEOWNERS entry covering `.github/workflows/` so changes to high-impact automation need owner approval. Compile with `--strict` in CI (no write permissions, explicit network, no wildcard domains, pinned actions). Use repository/org rulesets to require reviewer-workflow checks (keep `name:` and job names stable; set `inlined-imports: true` so ruleset-required checks don't fail on runtime imports). GHES/GHEC data residency: set `ghes: true` in `.github/workflows/aw.json` (auto-detected by `gh aw init`).

**Terraform (`integrations/github` provider).** Manage the platform with:
- `github_actions_organization_secret` / `github_actions_secret` — engine secrets.
- `github_actions_organization_variable` / `github_actions_variable` — `GH_AW_DEFAULT_*` / `GH_AW_POLICY_*`.
- `github_actions_organization_permissions` — allowed-actions policy; put `github/gh-aw@*` in `allowed_actions_config.patterns_allowed`.
- `github_repository_file` — manage the `.md`/`.lock.yml` if you must, though git + `gh aw deploy` is usually better; there is no first-class "workflow" resource.

```hcl
resource "github_actions_organization_permissions" "coolgitorg" {
  allowed_actions      = "selected"
  enabled_repositories = "all"
  allowed_actions_config {
    github_owned_allowed = true
    verified_allowed     = false
    patterns_allowed     = ["actions/*", "github/gh-aw@*"]
  }
}

resource "github_actions_organization_variable" "block_prs" {
  variable_name = "GH_AW_POLICY_ALLOW_CREATE_PULL_REQUEST"
  visibility    = "all"
  value         = "false"
}

resource "github_actions_organization_secret" "anthropic" {
  secret_name     = "ANTHROPIC_API_KEY"
  visibility      = "private"
  plaintext_value = var.anthropic_api_key
}
```

### 7. Troubleshooting, Gotchas, FAQ, Quick Reference

- **Silent completion.** If the agent finishes without calling any safe-output tool, the run fails silently — always instruct it to call `noop` with a reason when no action is needed. This is the #1 runtime failure mode.
- **"action ... is not allowed."** Allow-list `github/gh-aw@*` at enterprise/org and enable GitHub-created actions.
- **Copilot auth failures.** OAuth (`gho_`) tokens are rejected; use a fine-grained PAT or `copilot-requests: write`. `CLAUDE_CODE_OAUTH_TOKEN` is silently ignored — use `ANTHROPIC_API_KEY`.
- **Firewall blocked domains.** The audit report's Firewall Analysis lists every blocked domain; add needed domains/ecosystems to `network.allowed` and recompile. Start from `defaults` and add incrementally.
- **Projects fail with "Resource not accessible."** Default `GITHUB_TOKEN` can't touch Projects v2 — supply a PAT/App with Projects: Read & Write.
- **PRs don't trigger CI.** `GITHUB_TOKEN`-created PRs require approval; use `repository_dispatch` or a PAT-backed CI-trigger token.
- **`add-comment` on PRs missing permission.** A known compiler bug historically omitted `pull-requests: write` from the safe_outputs job for `add-comment`; verify the generated lock and file an issue/patch if needed.
- **Missing tool.** The agent reports via `missing-tool`; check `tools:` and toolset opt-ins (e.g., `dependabot` is not in `all`).
- **Concurrency drops.** repo-memory fan-out can lose writes under heavy parallel dispatch; prefer bounded fan-out and cache-memory union-merge for `.jsonl`.
- **Cost creep.** Move deterministic reads into `steps:`, cap `max-turns` and `max-ai-credits`, use smaller models via `GH_AW_DEFAULT_MODEL_*`, and gate with `skip-if-match`.
- **Debug fast.** Hand a coding agent the run URL (`agentic-workflows debug <run-url>`), or run `gh aw audit <run-id>` then read the `.lock.yml`.

Quick reference: `.md` source → `gh aw compile` → `.lock.yml`; agent is read-only; writes via safe-outputs; egress via `network.allowed`; governance via `GH_AW_DEFAULT_*`/`GH_AW_POLICY_*`; observability via `logs`/`audit`/`health`.

## Recommendations
1. **Pilot in a side repo first.** Stand up `CoolGitOrg/central-control` (private), install the extension, run `gh aw init`, and add one IssueOps and one MonitorOps workflow with `engine: copilot` + `copilot-requests: write`. Benchmark: <2–3 min/run, clean `gh aw audit` firewall section, zero safe-output rejections. If green after a week, proceed.
2. **Lock down governance before scaling.** Allow-list `github/gh-aw@*` via Terraform `github_actions_organization_permissions`; set enterprise `GH_AW_DEFAULT_*` (AIC caps like `default_max_ai_credits: "5M"`, small default models) and `GH_AW_POLICY_ALLOW_CREATE_PULL_REQUEST=false` until you've reviewed PR-creating workflows; add CODEOWNERS on `.github/workflows/`; require `--strict` compile in CI. Threshold to relax the PR policy: a reviewed, staged (`staged: true`) run history showing no bad writes.
3. **Standardize auth on GitHub Apps for writes, PAT only for Copilot.** Configure `safe-outputs.github-app` for least-privilege short-lived tokens; keep a single `GH_AW_GITHUB_TOKEN` fallback. Rotate quarterly.
4. **Adopt OrchestratorOps/CentralRepoOps for org-wide work**, DeterministicOps to cut cost, and keep browser/computer-use workflows `workflow_dispatch`-only with tight `network.allowed`. Roll out with `gh aw deploy --org`.
5. **Operate on data.** Schedule a MonitorOps workflow (or githubnext/agentic-ops) and watch `gh aw health --days 30`; if success rate drops below 90% or AIC spikes >100% run-over-run in `gh aw audit` diff, disable the offending workflow and investigate.

## Caveats
- gh-aw is in **technical preview** and moving fast (300+ releases). A security vulnerability was found in versions ">= 0.83.3, < 0.85.4 and, as a result, those releases were retired as a pre-emptive measure" (github/gh-aw README). Verify exact frontmatter fields, safe-output names, and CLI flags against the current docs and repo before shipping — details here reflect docs as of September 2026 and will drift.
- Project-token variable names (e.g., `GH_AW_READ_PROJECT_TOKEN`/`GH_AW_WRITE_PROJECT_TOKEN`) and `GH_AW_AGENT_TOKEN`/`GH_AW_GITHUB_MCP_SERVER_TOKEN` appear in pattern/tooling docs and community skills but not always on the main Authentication page; confirm the exact names on the Authentication (Projects) page before wiring Projects workflows.
- AIC is a best-effort estimate ($0.01/AIC per the cost docs) and may differ from the provider's final bill; verify actual charges in the billing dashboard. Threat-detection carries its own separate budget (`max-ai-credits` default 400).
- The security model reduces but does not eliminate risk: threat detection is deterministic/AI-best-effort and catches anticipated patterns; the MCP-gateway key is "leaked by design." Human review of safe-output diffs remains mandatory.
- "go-surf" could not be verified as a real tool/library; treat any Go browser integration as a custom, unvetted MCP/bash addition.
- Terraform resource names are from the official `integrations/github` provider registry, not the gh-aw docs; gh-aw does not itself document Terraform usage, and there is no first-class Terraform resource for a workflow file.