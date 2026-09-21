# gh aw CLI guide and cheatsheet

> Source: gh-aw Engineer's Field Guide (docs as of Sept 2026). gh-aw is in technical preview; verify field names and flags against https://github.github.com/gh-aw/ before shipping.


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

