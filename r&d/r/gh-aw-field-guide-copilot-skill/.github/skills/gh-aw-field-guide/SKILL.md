---
name: gh-aw-field-guide
description: Author, review, debug, and govern GitHub Agentic Workflows (gh-aw) -- the Markdown-plus-frontmatter workflows that `gh aw compile` turns into hardened GitHub Actions .lock.yml files running Copilot, Claude, Codex, or Gemini agents. Use when the task mentions gh-aw, `gh aw`, agentic workflows, safe-outputs, .lock.yml, the AWF firewall, Peli's Agent Factory, githubnext/agentics, or asks for IssueOps, ChatOps, CentralRepoOps, MonitorOps, DeterministicOps, DispatchOps, ResearchPlanAssignOps, OrchestratorOps, or any "AI agent in GitHub Actions" automation -- issue triage bots, slash-command reviewers, scheduled repo reports, org-wide rollouts, Playwright browser checks. Also use it for gh aw CLI questions, add-wizard and workflow creation, engine secrets and GitHub App tokens, GH_AW_DEFAULT_*/GH_AW_POLICY_* governance, Terraform for the Actions allow-list, and failing agentic workflow runs, even when the user never says "gh-aw".
license: MIT
---

# gh-aw field guide

gh-aw compiles a Markdown file with YAML frontmatter into a GitHub Actions
workflow. The agent runs read-only behind an egress firewall. It cannot write.
It emits requests; separate, narrowly-permissioned jobs apply them after a
threat-detection pass. That separation is the whole design. Every rule below
follows from it.

gh-aw is in technical preview and changes weekly. Treat field names and flags
here as correct for September 2026 and confirm anything load-bearing against
https://github.github.com/gh-aw/ or `gh aw <cmd> --help`. Versions
0.83.3 through 0.85.3 were retired for a security issue; do not pin to them.

## Working alongside `gh aw init`

`gh aw init` installs its own dispatcher skill at
`.github/skills/agentic-workflows/SKILL.md` and a custom agent at
`.github/agents/agentic-workflows.md`. They are upstream's and track the
installed gh-aw version. This skill does not replace them. When they exist,
let them drive create, update, and debug mechanics; use this skill for the
invariants, the pattern choice, the org's conventions, and the enterprise
material they do not carry. When they disagree with this file on a field
name or flag, upstream wins: it is newer.

As the Copilot cloud agent you run inside your own firewall and a fresh
runner. `gh aw` is present only if `.github/workflows/copilot-setup-steps.yml`
installs it (`uses: github/gh-aw/actions/setup-cli@<sha>`; confirm the action
path in the gh-aw repo). If it is absent, write the `.md`, run
`scripts/aw-lint.sh`, and state in the PR body that the lock file must be
compiled by a maintainer. Do not fake one.

## Invariants

Hold these in every workflow you write or review. Each has a reason; when a
user wants to break one, explain the reason rather than refusing.

1. **The agent job is read-only.** `permissions:` holds reads only (plus
   `copilot-requests: write` for Copilot billing). Writes are declared under
   `safe-outputs:`. A write permission on the agent job hands a
   prompt-injectable process a write token.
2. **Constrain every safe output.** Set `max:`, `allowed:` labels,
   `title-prefix:`, `allowed-events:`, `allowed-repos:`. The model's output is
   untrusted; the constraint is what holds when the prompt does not.
3. **Read user text only through `${{ needs.activation.outputs.text }}`.**
   Raw `github.event.*.body` skips sanitization (mention neutralizing, URL
   redaction, size limits).
4. **Egress is an allow-list.** Start at `network: defaults`. Add ecosystems
   (`node`, `python`, `playwright`) and exact domains one at a time. No bare
   wildcards.
5. **Tell the agent to call `noop`.** A run that finishes without any safe
   output fails silently. This is the most common runtime failure.
6. **Commit the `.md` and the `.lock.yml` together. Never edit the lock.**
   Edit the source, recompile, review the lock diff.
7. **Browser and computer-use workflows are `workflow_dispatch` only**, with
   bash limited to `playwright-cli:*` and an explicit domain list.
8. **Do deterministic work deterministically.** Anything `gh`, `jq`, or a
   shell loop can do goes in `steps:` writing to `/tmp/gh-aw/agent/`. The
   agent reasons over prepared files. This is the largest cost lever.

## Choosing a pattern

Patterns are a trigger, a set of safe outputs, and a token strategy.

| The user wants | Pattern | Trigger | Typical safe outputs |
|---|---|---|---|
| React to new or edited issues | IssueOps | `issues` | add-comment, add-labels |
| `/command` in a comment | ChatOps | `command:` + `roles:` | add-comment, submit-pull-request-review |
| Rolling health or cost report | MonitorOps | `schedule` | create-discussion, create-issue |
| Shell gathers, agent interprets | DeterministicOps | any, with `steps:` | update-release, create-issue |
| Kick another workflow or repo | DispatchOps | any | dispatch-workflow, call-workflow, dispatch-repository |
| One brain, many scoped workers | OrchestratorOps | schedule or dispatch | dispatch-workflow / call-workflow |
| Org-wide change from one control repo | CentralRepoOps | schedule or dispatch | dispatch-workflow, then create-pull-request with `target-repo` |
| Research, then plan, then hand off | ResearchPlanAssignOps | `schedule` | create-issue with `group: true`, assign-to-agent |

[references/patterns.md](references/patterns.md) has a complete, linted example for each, plus the
other documented patterns (LabelOps, SideRepoOps, ProjectOps, DailyOps, ...).
Grep `^## <Name>` to jump. User vocabulary drifts: "MonitorOps" may mean
DailyOps or Projects monitoring; "TaskOps" is WorkQueueOps or BatchOps. Map
to the documented name and say so.

## Writing a workflow

1. Settle four things first: trigger, what it may write, which engine, which
   hosts it must reach. If the conversation does not answer one, ask once.
2. Start from [assets/workflow-template.md](assets/workflow-template.md) or the nearest example in
   `references/patterns.md`. Use the user's real org and repo names.
3. Write the body as a brief to a capable colleague: the task, the inputs,
   the limits, and when to do nothing (`noop`). Short imperative sentences.
4. Give cross-repo writes `target-repo`, `allowed-repos`, and a
   `github-token:` or `github-app:`. The default `GITHUB_TOKEN` cannot leave
   the repo and cannot touch Projects v2.
5. Run `bash .github/skills/gh-aw-field-guide/scripts/aw-lint.sh path/to/workflow.md`
   (adjust the prefix if installed under `~/.copilot/skills`). It checks the invariants the
   compiler cannot see (raw event text, missing `noop`, playwright on a
   non-manual trigger) and a few it can. Fix every FAIL; explain every WARN
   you keep.
6. Compile and test in the terminal: `gh aw compile --strict <name>`, then
   `gh aw run <name>` or `gh aw trial ./<name>.md`. If `gh aw` is missing,
   `gh extension install github/gh-aw`. Never hand-write a `.lock.yml`; only
   the compiler produces one. Stage the `.md` and the `.lock.yml` in the same
   commit.
7. After each example, explain the frontmatter section by section. Users of
   this skill review these files; they need the why.

## Reviewing a workflow

Run `scripts/aw-lint.sh` on it, then read for what it cannot judge: safe-output
limits loose for the task, `bash` wider than needed, `roles:` missing on a
command trigger, a schedule that burns credits with no `skip-if-match` gate,
tokens broader than the repos they touch. Report findings most dangerous
first.

## Debugging a run

`gh aw logs <wf>`, then `gh aw audit <run-id>`, then the `.lock.yml`. The
audit's firewall section names every blocked domain. Match the symptom in
`references/troubleshooting.md` before theorizing: most failures are a
missing `noop`, a blocked domain, a rejected token type (`gho_` for Copilot;
`CLAUDE_CODE_OAUTH_TOKEN` is ignored), or `github/gh-aw@*` missing from the
Actions allow-list.

## Reference files

Read only what the task needs.

| File | Read it when |
|---|---|
| [references/foundations.md](references/foundations.md) | Explaining how gh-aw works; engines, tools, MCP, safe-output catalog, sanitization, firewall, sandbox runtimes, imports, memory, concurrency, cost |
| [references/patterns.md](references/patterns.md) | Writing or choosing a pattern; workflows triggering workflows; commits and PRs from workflows |
| [references/browser-automation.md](references/browser-automation.md) | Playwright or computer-use; anyone asks about "go-surf" (not a real gh-aw tool; see file) |
| [references/cli.md](references/cli.md) | Any `gh aw` command, flag, or the cheatsheet |
| [references/wizard.md](references/wizard.md) | `add-wizard`, `gh aw new`, `gh aw init`, `create.md` conversational authoring, githubnext/gh-aw-wizard |
| [references/enterprise.md](references/enterprise.md) | Actions allow-list, secrets per engine, GitHub App vs PAT, `GH_AW_DEFAULT_*` / `GH_AW_POLICY_*`, CODEOWNERS, Terraform |
| [references/troubleshooting.md](references/troubleshooting.md) | A run failed; known gotchas; caveats to state |

## Output

Give workflows as complete files in fenced blocks labelled with their path
(`.github/workflows/<name>.md`), then the explanation, then the commands to
compile and run. For enterprise questions, give the Terraform or
`gh aw env` snippet, not prose about it. Where the docs were uncertain
(project-token variable names, `dispatch-repository`), say so.
