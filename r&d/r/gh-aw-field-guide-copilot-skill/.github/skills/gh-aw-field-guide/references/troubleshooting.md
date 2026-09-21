# Troubleshooting, gotchas, caveats

> Source: gh-aw Engineer's Field Guide (docs as of Sept 2026). gh-aw is in technical preview; verify field names and flags against https://github.github.com/gh-aw/ before shipping.


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
