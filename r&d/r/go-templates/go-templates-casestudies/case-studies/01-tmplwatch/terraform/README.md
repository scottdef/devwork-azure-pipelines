# terraform

Creates the two repositories tmplwatch needs in `CoolGitOrg`:

| Resource | |
|---|---|
| `github_repository.tmplwatch` | the code (this repository) |
| `github_repository.tmplwatch_kv` | the key/value data: one file per key under `kv/` |
| `github_repository_ruleset.main` (both) | protect the default branch: no deletion or force-push, PR with 1 approval, linear history |
| `github_repository_ruleset.checks` | required checks `test` and `render-check` on tmplwatch |
| `github_repository_file.kv_seed` | `README.md`, `kv/motd/text`, `kv/site/env`, `.github/workflows/notify.yml` |
| `github_actions_variable.app_client_id` (both) | `APP_CLIENT_ID` for `actions/create-github-app-token@v3` |
| `github_actions_variable.go_version` | `GO_VERSION` read by every workflow |

## Apply order

1. **Install the App in the UI first.** `issueops-autoadmin-app` must be
   installed on `CoolGitOrg` with access to `tmplwatch` and `tmplwatch-kv`
   (or all repositories), and with **Contents: read & write** and
   **Metadata: read** (the dispatch API needs `contents: write` on the
   target). The provider cannot create installations or grant App
   permissions; that is a UI step for an org owner, and it needs doing
   once. If the repositories do not exist yet, install it on "All
   repositories", or come back and add them after step 3.
2. Authenticate the provider as the App installation (or with a token
   from an org owner):

   ```sh
   export GITHUB_APP_ID=123456                    # numeric App ID, not the client ID
   export GITHUB_APP_INSTALLATION_ID=7891011
   export GITHUB_APP_PEM_FILE="$(cat issueops-autoadmin-app.pem)"
   ```

3. Apply:

   ```sh
   terraform init
   terraform apply -var app_client_id=Iv23liXXXXXXXXXXXXXX
   ```

   Terraform creates the repositories, then commits the seed files to
   `main`, then creates the rulesets (they `depends_on` the seed files, so
   the first apply can still push to `main`).
4. **Secrets are not in Terraform**, so they stay out of state. Set the App
   private key on both repositories:

   ```sh
   gh secret set APP_PRIVATE_KEY -R CoolGitOrg/tmplwatch    < issueops-autoadmin-app.pem
   gh secret set APP_PRIVATE_KEY -R CoolGitOrg/tmplwatch-kv < issueops-autoadmin-app.pem
   ```

5. Push this code to `CoolGitOrg/tmplwatch` through a pull request, because
   the ruleset is now active.

Seed files are created once. Afterwards `kv/` belongs to the kv
repository's pull requests, and `lifecycle.ignore_changes` keeps Terraform
from fighting them.

## Variables

| Name | Default | |
|---|---|---|
| `owner` | `CoolGitOrg` | organization |
| `app_client_id` | (required) | the App's client ID (`Iv23li…`), not secret |
| `visibility` | `internal` | `internal` or `private` |
| `go_version` | `1.24.x` | CI toolchain; see `docs/SECURITY.md` about raising it |
| `required_checks` | `["test", "render-check"]` | required status checks on tmplwatch |

## Outputs

`tmplwatch_full_name`, `tmplwatch_kv_full_name` (use this as `source.repo`),
and `tmplwatch_clone_urls` / `tmplwatch_kv_clone_urls` (`ssh`, `http`).

## Validation here

`terraform` is not installed in the build environment where this was
written. The HCL is hand-formatted to `terraform fmt` rules, and CI's
`terraform` job runs `fmt -check`, `init -backend=false` and `validate`.
