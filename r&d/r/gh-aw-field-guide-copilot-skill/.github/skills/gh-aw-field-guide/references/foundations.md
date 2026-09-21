# Foundations: compile model, engines, tools, safe-outputs, firewall, memory, cost

> Source: gh-aw Engineer's Field Guide (docs as of Sept 2026). gh-aw is in technical preview; verify field names and flags against https://github.github.com/gh-aw/ before shipping.


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

