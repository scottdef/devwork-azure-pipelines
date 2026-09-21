# Creation wizard and conversational authoring

> Source: gh-aw Engineer's Field Guide (docs as of Sept 2026). gh-aw is in technical preview; verify field names and flags against https://github.github.com/gh-aw/ before shipping.


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

