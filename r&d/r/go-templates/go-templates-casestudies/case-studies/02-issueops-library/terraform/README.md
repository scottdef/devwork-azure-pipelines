# terraform: provision the two IssueOps repositories

Creates and configures `CoolGitOrg/issueops-intake` and `CoolGitOrg/issueops-library`:

| Resource | What |
|---|---|
| `github_repository.this["intake"\|"library"]` | Both repos. Issues on, wiki/projects off, squash-only merges, `pages { build_type = "workflow" }`. Intake is `public` or `internal` (`var.intake_visibility`); library is `public`. |
| `github_issue_label.this` | `issueops request borrow return accepted invalid published overdue` on both repos (16 labels). |
| `github_actions_variable.*` | `APP_CLIENT_ID` and `GO_VERSION` on both; `LIBRARY_BASE_URL` on the library. |
| `github_repository_environment.pages` + `..._deployment_policy.pages_main` | `github-pages` environment, deployable from `main` only. |
| `github_repository_ruleset.main` | `main` is PR-only for people (1 approval, stale reviews dismissed, no force push, no delete); the App bypasses so workflows can push data commits. |

Not managed here, on purpose:

- **The GitHub App** (`issueops-autoadmin-app`): created and installed in the UI (or via a
  manifest flow). Terraform has no resource for creating Apps.
- **`APP_PRIVATE_KEY`**: set with `../scripts/set-secrets.sh`. A key passed through
  `github_actions_secret` would sit in Terraform state in plain text; `gh secret set`
  encrypts it client-side with the repo's public key and nothing else keeps a copy.

## Apply order

1. **Create the App** in `CoolGitOrg` settings: Developer settings, GitHub Apps, New.
   - Name `issueops-autoadmin-app`; no webhook (uncheck "Active").
   - Repository permissions: **Contents: Read and write**, **Issues: Read and write**,
     **Metadata: Read**. Nothing else. (Pages deploys use the workflow's own
     `GITHUB_TOKEN` with `pages: write` and `id-token: write`, not the App.)
   - "Only on this account". Create, then **Generate a private key** (downloads a `.pem`).
   - Note the **Client ID** (`Iv23li...`) and the numeric **App ID**.
2. **Apply Terraform** with an org-owner token:
   ```sh
   export GITHUB_TOKEN=$(gh auth token)          # needs admin:org, repo
   cp terraform.tfvars.example terraform.tfvars  # fill app_client_id and app_id
   terraform init
   terraform plan -out tf.plan
   terraform apply tf.plan
   ```
3. **Install the App** on exactly the two repositories (org settings, GitHub Apps,
   `issueops-autoadmin-app`, Configure, "Only select repositories").
4. **Store the key**: `../scripts/set-secrets.sh ~/Downloads/issueops-autoadmin-app.*.private-key.pem`,
   then delete the local file or move it to your vault.
5. **Push the code**: push `../issueops-intake` and `../issueops-library` to their repos
   (first push through a PR, since the ruleset is active), then run each repo's `pages`
   workflow once (`gh workflow run pages --repo CoolGitOrg/issueops-intake`).

## Finding the App ID for `bypass_actors`

Repository rulesets identify an App bypass actor by `actor_type = "Integration"` and
`actor_id` = the App's **numeric App ID**: not the Client ID, not the installation ID.

```sh
# from the org's installations (needs admin:org)
gh api /orgs/CoolGitOrg/installations \
  --jq '.installations[] | select(.app_slug == "issueops-autoadmin-app") | {app_id, id}'
# or from the public App endpoint
gh api /apps/issueops-autoadmin-app --jq .id
```

Verify after apply:

```sh
gh api /repos/CoolGitOrg/issueops-library/rulesets --jq '.[].id' |
  xargs -I{} gh api /repos/CoolGitOrg/issueops-library/rulesets/{} --jq '.bypass_actors'
```

## Caveats (read before the first apply)

- **terraform was not available where this was written.** The HCL was formatted by hand to
  `terraform fmt` rules and checked by reading, not by `terraform validate`. Run
  `terraform fmt -check && terraform validate` first; `../issueops-*/scripts/preflight.sh`
  does so automatically when `terraform` is on `PATH`.
- **Confirm the `pages` block against the provider docs for the version you lock.** In
  `integrations/github` 6.x, `build_type = "workflow"` takes no `source` block; older
  releases required `source` even for workflow builds. If `plan` asks for it, add
  `source { branch = "main" }`, which is ignored for workflow builds.
- `github_repository_environment` with `custom_branch_policies = true` needs the separate
  `github_repository_environment_deployment_policy` resource (declared here) or nothing can deploy.
- **Rulesets on internal/private repos** need GitHub Team or Enterprise; on GitHub Free the
  ruleset applies only to the public library repo. Set `intake_visibility = "public"` there.
- `prevent_destroy` guards both repos: the library repo *is* the database (items and ledger).
