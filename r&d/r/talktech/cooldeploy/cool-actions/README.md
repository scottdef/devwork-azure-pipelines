# cool-actions

The pipelines of CoolDeploy, kept in one place: reusable workflows that app
and platform repositories call, and the composite actions those workflows are
made of.

An app repository needs one workflow file, and `coolctl init` writes it:

```yaml
name: cool
on:
  push:
    branches: [main]
  pull_request:
    types: [opened, synchronize, reopened, closed]
permissions:
  contents: read
  id-token: write
  pull-requests: write
jobs:
  cool:
    uses: CoolEngOrg/cool-actions/.github/workflows/cool-pipeline.yml@v1
```

## Reusable workflows

| Workflow | Called by | Does |
| --- | --- | --- |
| `cool-pipeline.yml` | app repositories | routes the event to the three below |
| `cool-build.yml` | `cool-pipeline` | validates `cool.json`, tests Go code, builds the image in ACR, reports its digest |
| `cool-deploy.yml` | `cool-pipeline` | sends the digest to `cool-api` for `dev`, waits until Argo CD reports healthy |
| `cool-preview.yml` | `cool-pipeline` | puts up or removes a pull-request preview, comments its URL |
| `cool-pages.yml` | `cool-portal` | renders portal fragments from `cool-gitops`, publishes to GitHub Pages |
| `cool-terraform.yml` | `cool-infra` | plans a stack; applies it after an environment approval |
| `cool-apim-sync.yml` | `cool-apim` | dry-runs the APIM scripts; applies after an environment approval |

## Composite actions

| Action | Does |
| --- | --- |
| `setup-coolctl` | puts `coolctl` on the PATH, from the runner image or the bundled binary |
| `azure-login` | signs in to Azure with the job's OIDC token |
| `acr-build` | `az acr build`, and returns the image digest |
| `wait-healthy` | `coolctl wait` |
| `prom-gate` | asks managed Prometheus whether a release is inside its error budget |
| `pr-comment` | writes or updates one marked comment on a pull request |

Inside the reusable workflows every `uses:` names this repository in full,
`CoolEngOrg/cool-actions/<action>@v1`. A relative `./` path would be looked
up in the caller's checkout, not here.

## Why `@v1` matters more than usual

There are no deploy secrets on this platform. Two parties decide whether a
job may act, and both go by the same claim in the job's GitHub OIDC token,
`job_workflow_ref`, which names the workflow file that defines the job:

| Party | Trusts jobs defined by |
| --- | --- |
| Azure, for `az acr build` | `cool-build.yml` or `cool-pipeline.yml` of `CoolEngOrg/cool-actions`, at `@refs/tags/v1` |
| `cool-api`, for deploys to `dev` and previews | `CoolEngOrg/cool-actions/.github/workflows/*@refs/tags/v1` |

The claim names the reusable workflow that defines the job. With one
reusable workflow calling another, as `cool-pipeline` calls `cool-build`,
check on the first real run which of the two GitHub puts in the token (the
job summary of a failed `azure/login` shows the subject); Azure is set up to
accept either, and `cool-api` accepts any workflow of this repository at `v1`.

So whoever can move the tag `v1` can change what every app's pipeline does
with those rights. Protect it accordingly:

- a tag ruleset on `v*` that only the release workflow's environment can bypass;
- required reviewers on the `cool-actions-release` environment;
- branch protection on `main` with code owner review.

`release.yml` moves `v1` when a `v1.x.y` tag is pushed. A breaking change
gets `v2`, a second federated credential, and a second entry in
`cool-api`'s `COOL_ACTIONS_REF` rollout; app repositories move over one by one.

For the Azure side to match on that claim, each app repository's OIDC subject
must include it. The `cool-app-create` request sets this when it creates the
repository:

```sh
gh api --method PUT "repos/CoolEngOrg/$repo/actions/oidc/customization/sub" \
  -F use_default=false -f 'include_claim_keys[]=job_workflow_ref'
```

## Organization variables these workflows read

| Variable | Example | Used for |
| --- | --- | --- |
| `COOL_RUNS_ON` | `["self-hosted","linux","cool-linux"]` | runner labels, as a JSON array |
| `COOL_API_URL` | `http://cool-api.cool-system.svc.cluster.local` | where `coolctl` finds `cool-api` from a runner on the hub cluster |
| `COOL_REGISTRY` | `prdeus2coolopsacr001` | the platform registry |
| `COOL_TENANT_ID`, `COOL_SUBSCRIPTION_ID` | | Azure sign-in |
| `COOL_BUILD_CLIENT_ID` | | identity that may run `az acr build` |
| `COOL_INFRA_PLAN_CLIENT_ID`, `COOL_INFRA_APPLY_CLIENT_ID` | | identities for Terraform plan (reader) and apply |
| `COOL_APIM_READ_CLIENT_ID`, `COOL_APIM_WRITE_CLIENT_ID` | | identities for the APIM scripts |
| `COOL_READER_APP_CLIENT_ID` | | GitHub App that can read `cool-gitops`, for the portal build |

They are variables, not secrets: none of them lets anyone do anything
without also holding a token GitHub signed for the right workflow.

## Runners

The workflows assume self-hosted Ubuntu runners on the hub cluster, with
`az`, `jq`, `gh`, `git` and `coolctl` on the image. `cool-build` runs
`go test` on code from a pull request, so the runners must be ephemeral: one
job per pod, nothing kept. `cool-pipeline` skips pull requests from forks.

If the image does not carry `coolctl`, bundle it:

```sh
make bundle COOLCTL_SRC=../coolctl     # writes bin/coolctl-linux-{amd64,arm64} and SHA256SUMS
git add bin && git commit -m "bundle coolctl 1.0.0"
```

## Check

```sh
make check      # actionlint (with shellcheck) over the workflows, and that
                # every third-party action is pinned to a commit
```

Third-party actions are pinned to full commit SHAs with the version in a
comment. Update them deliberately, one pull request per action.
