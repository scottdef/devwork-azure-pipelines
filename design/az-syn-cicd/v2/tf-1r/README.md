# Azure Synapse CI/CD — GitHub Actions + Terraform

Infrastructure-as-Code and artifact deployment for Azure Synapse Analytics across three environments (dev → test → prod), using GitHub Actions for CI/CD and Terraform for infrastructure management.

## Architecture

```
┌──────────────────┐     PR + Merge      ┌─────────────────────┐
│  Feature Branch   │ ──────────────────► │  Collaboration (main)│
│  (Synapse Studio) │                     │                     │
└──────────────────┘                     └────────┬────────────┘
                                                  │
                              ┌────────────────────┼────────────────────┐
                              │                    │                    │
                    Terraform (infra)     Synapse Artifacts      workspace_publish
                    Spark pools, PEs      Pipelines, notebooks   (ARM templates)
                              │                    │                    │
                              ▼                    ▼                    ▼
                    ┌──────────────────────────────────────────────────────┐
                    │              GitHub Actions Workflows                │
                    │  terraform-plan → terraform-apply (state artifacts)  │
                    │  synapse-validate → synapse-deploy (trigger mgmt)    │
                    │  synapse-rollback │ drift-detection                  │
                    └──────────┬───────────┬───────────┬──────────────────┘
                               │           │           │
                               ▼           ▼           ▼
                         ┌──────────┐ ┌──────────┐ ┌──────────┐
                         │   DEV    │ │   TEST   │ │   PROD   │
                         │ (Git)   │ │ (Live)   │ │ (Live)   │
                         └──────────┘ └──────────┘ └──────────┘
```

**Only dev connects to Git.** Test and prod receive artifacts through CI/CD only.

## Prerequisites

| Resource | Name | Notes |
|----------|------|-------|
| Synapse Workspaces | `synapse-workspace-dev`, `synapse-workspace-test`, `synapse-workspace-prod` | Pre-created |
| GitHub Repo | `synapse-repo` | This repository |
| Service Principals | 3 (one per environment) | Synapse Contributor + Synapse Admin roles |
| Git Integration | Configured on dev workspace | Collaboration branch: `main` |

## Repository Structure

```
synapse-repo/
├── .github/workflows/
│   ├── terraform-plan.yml        # PR: validate + plan terraform
│   ├── terraform-apply.yml       # Merge: apply terraform (dev→test→prod)
│   ├── synapse-validate.yml      # PR: validate synapse artifacts
│   ├── synapse-deploy.yml        # Deploy synapse artifacts (dev→test→prod)
│   ├── synapse-rollback.yml      # Manual: roll back to tagged version
│   └── drift-detection.yml       # Scheduled: detect manual changes
├── terraform/
│   ├── main.tf                   # Backend config (local, artifact-backed)
│   ├── providers.tf              # Azure RM provider
│   ├── variables.tf              # Input variables
│   ├── synapse.tf                # Workspace data source + Spark pools
│   ├── outputs.tf                # Exported values
│   └── environments/
│       ├── dev.tfvars
│       ├── test.tfvars
│       └── prod.tfvars
├── parameters/
│   ├── dev.parameters.yaml       # Synapse ARM template overrides
│   ├── test.parameters.yaml
│   └── prod.parameters.yaml
├── scripts/
│   ├── tf-state-artifact.sh      # State download/upload helper
│   ├── toggle-triggers.sh        # Stop/start triggers safely
│   └── detect-drift.sh           # Compare live vs template
├── Makefile
└── .gitignore
```

## GitHub Secrets

| Secret | Description |
|--------|-------------|
| `AZURE_SUBSCRIPTION_ID` | Azure subscription ID |
| `AZURE_TENANT_ID` | Azure AD tenant ID |
| `SYNAPSE_DEV_SPN_ID` | Dev service principal client ID |
| `SYNAPSE_DEV_SPN_SECRET` | Dev service principal secret |
| `SYNAPSE_TEST_SPN_ID` | Test service principal client ID |
| `SYNAPSE_TEST_SPN_SECRET` | Test service principal secret |
| `SYNAPSE_PROD_SPN_ID` | Prod service principal client ID |
| `SYNAPSE_PROD_SPN_SECRET` | Prod service principal secret |

Set them with `make setup-secrets` or manually:
```bash
gh secret set AZURE_SUBSCRIPTION_ID --body "<value>"
```

## GitHub Environments

Create three environments with approval gates:

| Environment | Approvers | Branch Policy |
|-------------|-----------|---------------|
| `dev` | None (auto) | `main` |
| `test` | 1+ reviewer | `main` |
| `production` | 2+ reviewers | `main` |

```bash
make setup-environments
```

## Terraform State Strategy

State is stored as a **GitHub Actions artifact** with 90-day retention. Each workflow run downloads the previous state before `terraform init` and uploads the updated state after `terraform apply`. This avoids remote backend dependencies while keeping state durable across runs.

Artifacts are named `tfstate-dev`, `tfstate-test`, `tfstate-prod`.

## Workflows

### Infrastructure (Terraform)

**`terraform-plan.yml`** — Runs on PRs modifying `terraform/`. Plans all three environments in parallel and posts results as PR comments.

**`terraform-apply.yml`** — Runs on merge to `main`. Applies sequentially: dev → test (approval) → prod (approval). Tags the commit after prod apply.

### Synapse Artifacts

**`synapse-validate.yml`** — Runs on PRs modifying Synapse artifact folders. Validates artifacts and generates ARM templates.

**`synapse-deploy.yml`** — Two trigger modes:
- Push to `workspace_publish` (traditional Publish button)
- Manual dispatch with `validateDeploy` (modern, no manual publish needed)

Deploys: dev → test (approval) → prod (approval). Tags the release after prod.

**`synapse-rollback.yml`** — Manual dispatch. Rolls back any environment to a tagged version. Requires typing `ROLLBACK` to confirm.

**`drift-detection.yml`** — Runs daily at 06:00 UTC. Compares live test/prod state against templates. Creates GitHub issues on drift.

## Local Development

```bash
# Initialize terraform for dev
make init ENV=dev

# Run a plan
make plan ENV=dev

# Apply changes
make apply ENV=dev

# Check drift in test
make drift ENV=test

# Format terraform files
make fmt
```

## Key Design Decisions

1. **Local backend + artifact persistence** — No Azure Storage or S3 dependency for state. Trade-off: no state locking (acceptable for sequential environment pipeline).

2. **Trigger management is mandatory** — `toggle-triggers.sh` runs before every deploy. Deploying with active triggers fails with `TriggerEnabledCannotUpdate`.

3. **Two deployment modes** — `deploy` (from `workspace_publish`) and `validateDeploy` (from collaboration branch). The latter removes the need for manual Publish clicks.

4. **Per-environment service principals** — Least-privilege: each SPN only has access to its own environment's resources.

5. **Tagged releases** — Every prod deployment gets a `synapse-v<timestamp>` tag for point-in-time rollback.
