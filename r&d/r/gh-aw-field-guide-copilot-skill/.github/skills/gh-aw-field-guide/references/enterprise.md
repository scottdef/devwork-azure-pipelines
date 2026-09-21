# Enterprise rollout: allow-lists, secrets, GitHub App, governance variables, Terraform

> Source: gh-aw Engineer's Field Guide (docs as of Sept 2026). gh-aw is in technical preview; verify field names and flags against https://github.github.com/gh-aw/ before shipping.


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

