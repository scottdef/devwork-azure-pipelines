# terraform: tmplforge repository governance

Manages the `CoolGitOrg/tmplforge` repository with the
[`integrations/github`](https://registry.terraform.io/providers/integrations/github/latest)
provider (`~> 6.13`, Terraform `>= 1.6`):

| Resource | Purpose |
|---|---|
| `github_repository.tmplforge` | internal repo, squash-only, auto-delete branches, topics |
| `github_repository_ruleset.main` | default branch: PR with approvals, last-push approval, linear history, no force-push/delete, required CI checks |
| `github_actions_variable.go_version` | `GO_VERSION` read by every workflow (`${{ vars.GO_VERSION \|\| '1.24.x' }}`) |
| `github_repository_environment.beta` | `beta` environment for `loadtest.yml`, protected branches only, optional team reviewers |
| `github_actions_environment_variable.beta_loadtest_rate` | `LOADTEST_RATE` default for the beta environment |

## Use

```sh
export GITHUB_TOKEN=...            # or GitHub App credentials (see below)
terraform init
terraform plan -var go_version=1.26.x
terraform apply
```

In CI, authenticate as the `issueops-autoadmin-app` GitHub App
(`APP_CLIENT_ID` variable, `APP_PRIVATE_KEY` secret) via
`actions/create-github-app-token@v3` and pass the token as `GITHUB_TOKEN`.

## Notes

- `required_checks` must match job names in `.github/workflows/ci.yml`; a
  check that never reports blocks every merge.
- Raising `go_version` is the supported way to lift the toolchain in CI
  without editing workflows (see `docs/SECURITY.md`, toolchain support).
- The build environment for this case study has no `terraform` binary; the
  files are hand-formatted to `terraform fmt` style and
  `scripts/preflight.sh` runs `fmt -check` and `validate` when one exists.
