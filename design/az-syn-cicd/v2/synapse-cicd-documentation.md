# Azure Synapse CI/CD: Multi-Repository Deployment Guide

This document covers the complete CI/CD architecture for Azure Synapse Analytics across three repositories and three environments. It is organized into three sections: a quickstart for getting the system operational, a reference covering every file, secret, and workflow, and an architectural explanation of how the pieces connect and why.

---

## Part 1: Quickstart

This section takes you from zero to a working promotion pipeline. Estimated time: 45 minutes if all Azure prerequisites are already in place.

### Prerequisites checklist

Before starting, confirm you have these resources provisioned:

Three Azure Synapse workspaces: `synapse-workspace-dev`, `synapse-workspace-test`, and `synapse-workspace-prod`, each in its own resource group (`rg-synapse-dev`, `rg-synapse-test`, `rg-synapse-prod`). Each workspace must have a primary ADLS Gen2 storage account attached and a Key Vault for secrets management.

Three Azure service principals, one per environment. Each SPN needs two classes of role assignment: Azure RBAC roles (`Contributor` on its resource group, `Storage Blob Data Contributor` on its ADLS Gen2 account) and Synapse RBAC roles (`Synapse Administrator` or at minimum `Synapse Artifact Publisher` plus `Synapse Credential User`). Synapse RBAC is assigned through Synapse Studio under Manage → Access control, not through the Azure portal's IAM blade. Changes take 2–5 minutes to propagate.

Three GitHub repositories: `synapse-repo-dev`, `synapse-repo-test`, `synapse-repo-prod`, all within the same GitHub organization.

A GitHub App installed at the organization level with owner permissions. This app generates the `GH_APP_TOKEN` used for cross-repository workflow dispatch. The app needs the `actions:write` and `contents:read` permissions on all three repositories.

Git integration must already be configured on `synapse-workspace-dev`, with the collaboration branch set to `main` and the repository pointing to `synapse-repo-dev`. Test and prod workspaces remain in live mode and never connect to Git directly.

### Step 1: Populate the repositories

Copy the generated file structure into each repository. The dev repo contains 19 files, while test and prod each contain 16. The directory layout for each repo follows this pattern:

```
synapse-repo-<env>/
├── .github/workflows/
│   ├── terraform-plan.yml
│   ├── terraform-apply.yml
│   ├── synapse-deploy.yml
│   ├── drift-detection.yml
│   └── (synapse-validate.yml for dev, synapse-rollback.yml for test/prod)
├── terraform/
│   ├── main.tf
│   ├── providers.tf
│   ├── variables.tf
│   ├── synapse.tf
│   ├── outputs.tf
│   └── terraform.tfvars
├── parameters/
│   └── <env>.parameters.yaml
├── scripts/
│   ├── toggle-triggers.sh
│   └── detect-drift.sh
├── Makefile
└── .gitignore
```

The dev repo additionally contains `publish_config.json`, `template-parameters-definition.json`, `synapse-validate.yml`, and `README.md`. These files are specific to the dev repo because it is the only repo connected to Synapse Studio.

Push each repo's contents to its `main` branch.

### Step 2: Configure GitHub secrets

Each repository stores only its own environment's credentials. Run the following for each repo, substituting the correct SPN values:

```bash
# synapse-repo-dev
gh secret set AZURE_SUBSCRIPTION_ID --repo <org>/synapse-repo-dev --body "<subscription-id>"
gh secret set AZURE_TENANT_ID       --repo <org>/synapse-repo-dev --body "<tenant-id>"
gh secret set SYNAPSE_SPN_ID        --repo <org>/synapse-repo-dev --body "<dev-spn-client-id>"
gh secret set SYNAPSE_SPN_SECRET    --repo <org>/synapse-repo-dev --body "<dev-spn-client-secret>"
gh secret set GH_APP_TOKEN          --repo <org>/synapse-repo-dev --body "<github-app-token>"

# synapse-repo-test
gh secret set AZURE_SUBSCRIPTION_ID --repo <org>/synapse-repo-test --body "<subscription-id>"
gh secret set AZURE_TENANT_ID       --repo <org>/synapse-repo-test --body "<tenant-id>"
gh secret set SYNAPSE_SPN_ID        --repo <org>/synapse-repo-test --body "<test-spn-client-id>"
gh secret set SYNAPSE_SPN_SECRET    --repo <org>/synapse-repo-test --body "<test-spn-client-secret>"
gh secret set GH_APP_TOKEN          --repo <org>/synapse-repo-test --body "<github-app-token>"

# synapse-repo-prod
gh secret set AZURE_SUBSCRIPTION_ID --repo <org>/synapse-repo-prod --body "<subscription-id>"
gh secret set AZURE_TENANT_ID       --repo <org>/synapse-repo-prod --body "<tenant-id>"
gh secret set SYNAPSE_SPN_ID        --repo <org>/synapse-repo-prod --body "<prod-spn-client-id>"
gh secret set SYNAPSE_SPN_SECRET    --repo <org>/synapse-repo-prod --body "<prod-spn-client-secret>"
gh secret set GH_APP_TOKEN          --repo <org>/synapse-repo-prod --body "<github-app-token>"
```

The `AZURE_SUBSCRIPTION_ID`, `AZURE_TENANT_ID`, and `GH_APP_TOKEN` values are the same across all three repos. The `SYNAPSE_SPN_ID` and `SYNAPSE_SPN_SECRET` values differ — each repo stores the credentials for its own environment's service principal only.

### Step 3: Create GitHub environments

Each repo needs a GitHub environment to act as a deployment gate. For the dev repo, the environment is `dev` with no required reviewers (auto-deploy). For the test repo, create a `test` environment with at least one required reviewer. For the prod repo, create a `production` environment with at least two required reviewers.

```bash
# Dev repo — no approval gate
gh api repos/<org>/synapse-repo-dev/environments/dev --method PUT

# Test repo — configure reviewers in Settings → Environments after creation
gh api repos/<org>/synapse-repo-test/environments/test --method PUT

# Prod repo — configure reviewers in Settings → Environments after creation
gh api repos/<org>/synapse-repo-prod/environments/production --method PUT
```

After creating each environment, navigate to the repository's Settings → Environments page and add the required reviewers. You can also set deployment branch restrictions (limit to `main`) and optional wait timers.

### Step 4: Update environment-specific values

Edit `terraform/terraform.tfvars` in each repo to match your actual Azure resource names. The default values use placeholder names like `devdatalake` and `rg-synapse-dev`. Replace these with your actual storage account names, resource group names, Key Vault URLs, and Spark pool configurations.

Similarly, edit the `parameters/<env>.parameters.yaml` file in each repo to match the linked service parameter names your Synapse workspace expects. The critical fields are `workspaceName`, the Key Vault base URL, and the default storage URL.

### Step 5: Initialize Terraform

Run the initial Terraform plan for each repo to verify the configuration connects to Azure correctly:

```bash
# In each repo
cd terraform
terraform init
terraform plan
```

If the plan succeeds and shows expected resources, push a change to `terraform/` and let the workflow handle the apply. The first run will create a fresh state artifact.

### Step 6: Verify the pipeline end-to-end

Open Synapse Studio connected to `synapse-workspace-dev`. Create a simple pipeline (a `Wait` activity is fine), commit it to a feature branch, create a pull request against `main`, and merge it. The following sequence should execute automatically:

1. The `synapse-validate.yml` workflow in `synapse-repo-dev` validates the artifacts on the PR.
2. After merge, `synapse-deploy.yml` deploys to `synapse-workspace-dev` using `validateDeploy`.
3. The workflow tags the release (`synapse-v<timestamp>`) on `synapse-repo-dev`.
4. The workflow fires `workflow_dispatch` to `synapse-repo-test`.
5. After the test environment's approval gate is satisfied, `synapse-repo-test` checks out `synapse-repo-dev` at the tag and deploys to `synapse-workspace-test`.
6. The test workflow fires `workflow_dispatch` to `synapse-repo-prod`.
7. After the production approval gate, `synapse-repo-prod` deploys and tags `prod-synapse-v<timestamp>`.

If any step fails, check the workflow run logs in the failing repository's Actions tab.

### Common first-run issues

The most frequent failure is `TriggerEnabledCannotUpdate`, which means triggers were running when the deployment attempted to update them. The `toggle-triggers.sh` script handles this, but if the service principal lacks `Synapse Administrator` role, the `az synapse trigger list` command fails silently and the script skips the stop phase. Verify the SPN has the correct Synapse RBAC roles.

The second most common failure is an authentication error during cross-repo dispatch. This means the `GH_APP_TOKEN` either lacks `actions:write` permission or is not installed on the target repository. Verify the GitHub App installation in the organization settings.

If Terraform plan shows `Error: Provider produced inconsistent result` on the first run, this is normal — it occurs when the workspace data source retrieves properties not known at plan time. Running `apply` resolves it.

---

## Part 2: Reference

### Repositories

| Repository | Environment | Workspace | Git Connected | File Count |
|---|---|---|---|---|
| `synapse-repo-dev` | dev | `synapse-workspace-dev` | Yes | 19 |
| `synapse-repo-test` | test | `synapse-workspace-test` | No | 16 |
| `synapse-repo-prod` | prod | `synapse-workspace-prod` | No | 16 |

### Secrets per repository

| Secret | Description | Same Across Repos |
|---|---|---|
| `AZURE_SUBSCRIPTION_ID` | Azure subscription ID | Yes |
| `AZURE_TENANT_ID` | Azure AD tenant ID | Yes |
| `SYNAPSE_SPN_ID` | Service principal client ID for this environment | No |
| `SYNAPSE_SPN_SECRET` | Service principal client secret for this environment | No |
| `GH_APP_TOKEN` | GitHub App token with org owner permissions | Yes |

### GitHub environments

| Repository | Environment Name | Approval Required | Branch Policy |
|---|---|---|---|
| `synapse-repo-dev` | `dev` | No | `main` |
| `synapse-repo-test` | `test` | Yes (1+ reviewers) | `main` |
| `synapse-repo-prod` | `production` | Yes (2+ reviewers) | `main` |

### Azure RBAC roles per service principal

Each service principal needs roles in two separate systems. Azure RBAC controls infrastructure operations, and Synapse RBAC controls artifact operations. Missing either produces cryptic authentication failures.

| System | Role | Scope | Purpose |
|---|---|---|---|
| Azure RBAC | Contributor | Resource group | Manage Spark pools, ARM deployments |
| Azure RBAC | Storage Blob Data Contributor | ADLS Gen2 account | Read/write workspace storage |
| Synapse RBAC | Synapse Administrator | Workspace | Full artifact + trigger + RBAC control |
| Synapse RBAC | Synapse Credential User | Workspace | Use secrets in linked services |

### Workflows per repository

#### synapse-repo-dev (5 workflows)

| Workflow | Trigger | What It Does |
|---|---|---|
| `terraform-plan.yml` | PR to `main` touching `terraform/` | Runs `terraform plan`, posts output as PR comment |
| `terraform-apply.yml` | Push to `main` touching `terraform/`, or manual dispatch | Runs `terraform apply`, uploads state artifact `tfstate-dev` |
| `synapse-validate.yml` | PR to `main` touching Synapse artifact paths | Validates artifacts using `Azure/synapse-workspace-deployment@V1.9.1` in `validate` mode, uploads exported ARM templates |
| `synapse-deploy.yml` | Push to `workspace_publish`, push to `main` with artifact changes, or manual dispatch | Deploys to `synapse-workspace-dev`, tags `synapse-v<timestamp>`, fires `workflow_dispatch` to `synapse-repo-test` |
| `drift-detection.yml` | Daily at 06:00 UTC, or manual dispatch | Compares live dev workspace against ARM templates, creates GitHub issue on drift |

#### synapse-repo-test (5 workflows)

| Workflow | Trigger | What It Does |
|---|---|---|
| `terraform-plan.yml` | PR to `main` touching `terraform/` | Runs `terraform plan` for test environment |
| `terraform-apply.yml` | Push to `main` touching `terraform/`, or manual dispatch | Runs `terraform apply`, uploads state artifact `tfstate-test` |
| `synapse-deploy.yml` | `workflow_dispatch` from `synapse-repo-dev` or manual dispatch | Checks out `synapse-repo-dev` at the source tag, deploys to `synapse-workspace-test`, fires `workflow_dispatch` to `synapse-repo-prod` |
| `synapse-rollback.yml` | Manual dispatch with tag + confirmation | Checks out `synapse-repo-dev` at specified tag, deploys to `synapse-workspace-test` |
| `drift-detection.yml` | Daily at 06:00 UTC, or manual dispatch | Compares live test workspace against ARM templates from `synapse-repo-dev`, creates issue on drift |

#### synapse-repo-prod (5 workflows)

| Workflow | Trigger | What It Does |
|---|---|---|
| `terraform-plan.yml` | PR to `main` touching `terraform/` | Runs `terraform plan` for prod environment |
| `terraform-apply.yml` | Push to `main` touching `terraform/`, or manual dispatch | Runs `terraform apply`, uploads state artifact `tfstate-prod` |
| `synapse-deploy.yml` | `workflow_dispatch` from `synapse-repo-test` or manual dispatch | Checks out `synapse-repo-dev` at the source tag, deploys to `synapse-workspace-prod`, tags `prod-synapse-v<timestamp>` on the prod repo |
| `synapse-rollback.yml` | Manual dispatch with tag + confirmation | Checks out `synapse-repo-dev` at specified tag, deploys to `synapse-workspace-prod` |
| `drift-detection.yml` | Daily at 06:00 UTC, or manual dispatch | Compares live prod workspace against ARM templates from `synapse-repo-dev`, creates issue on drift |

### Terraform files

All three repos share identical `.tf` file structure. The differentiation is in `terraform.tfvars`, which contains environment-specific values.

| File | Purpose |
|---|---|
| `main.tf` | Terraform version constraint, `azurerm` provider requirement, local backend configuration |
| `providers.tf` | Azure RM provider block authenticated via service principal variables |
| `variables.tf` | All input variables: Azure auth, environment name, workspace name, storage account, Spark pool configs, Key Vault URL, tags |
| `synapse.tf` | Data source for the existing workspace, `azurerm_synapse_spark_pool` resources (with auto-scale and auto-pause), `azurerm_synapse_managed_private_endpoint` for Key Vault connectivity |
| `outputs.tf` | Exports workspace ID, name, connectivity endpoints, Spark pool IDs, and environment name |
| `terraform.tfvars` | Environment-specific values: workspace name, resource group, storage account, Key Vault URL, Spark pool sizing, tags |

Terraform state is stored as a GitHub Actions artifact (`tfstate-dev`, `tfstate-test`, `tfstate-prod`) with 90-day retention. Each workflow run downloads the previous state before `terraform init` and uploads the updated state after `terraform apply`.

Resources explicitly excluded from Terraform management per requirements: `azurerm_synapse_firewall_rule`, `azurerm_synapse_role_assignment` (RBAC), and `azurerm_synapse_sql_pool`.

### Synapse parameter files

Each repo contains a single parameter override file used by the `Azure/synapse-workspace-deployment` action. These YAML files override the parameterized values in the ARM template generated by Synapse.

| Repository | File | Key Overrides |
|---|---|---|
| `synapse-repo-dev` | `parameters/dev.parameters.yaml` | `workspaceName: synapse-workspace-dev`, Key Vault URL for dev, storage URL for dev |
| `synapse-repo-test` | `parameters/test.parameters.yaml` | `workspaceName: synapse-workspace-test`, Key Vault URL for test, storage URL for test |
| `synapse-repo-prod` | `parameters/prod.parameters.yaml` | `workspaceName: synapse-workspace-prod`, Key Vault URL for prod, storage URL for prod |

The `template-parameters-definition.json` file in the dev repo controls what Synapse parameterizes when generating ARM templates. Connection strings are marked as `secureString` so they're overridden at deploy time rather than stored in the template.

### Scripts

Both scripts are identical across all three repos.

`toggle-triggers.sh` accepts two arguments: an action (`stop` or `start`) and a workspace name. On `stop`, it queries the workspace for all triggers in `Started` state, writes their names to `active-triggers.txt`, and stops each one. On `start`, it waits 15 seconds for the deployment to settle, then restarts each trigger listed in `active-triggers.txt` with up to 3 retries per trigger. This script is the most important safety mechanism in the entire pipeline — deploying with active triggers causes `TriggerEnabledCannotUpdate` failures.

`detect-drift.sh` accepts a workspace name and a path to `TemplateForWorkspace.json`. It extracts expected artifact names from the template using `jq` and compares them against live artifact names retrieved via `az synapse` CLI commands. It checks pipelines, triggers, datasets, and linked services. If any differences are found, it exits with a non-zero code, which the drift detection workflow uses to create a GitHub issue.

### Synapse deployment action reference

All Synapse artifact deployments use `Azure/synapse-workspace-deployment@V1.9.1`. This action supports three operations:

`validate` generates ARM templates from collaboration branch artifacts without deploying. Used in `synapse-validate.yml` during PR review.

`deploy` deploys from pre-generated ARM templates on the `workspace_publish` branch to a target workspace. Used when someone clicks Publish in Synapse Studio.

`validateDeploy` validates and deploys directly from collaboration branch artifacts, bypassing manual publish entirely. This is the recommended mode for new deployments.

The action does not support Workload Identity Federation (OIDC). Service principal authentication with `clientId` and `clientSecret` is required.

### Tag naming conventions

| Tag Pattern | Created By | Created On | Purpose |
|---|---|---|---|
| `synapse-v<YYYYMMDD.HHMMSS>` | `synapse-repo-dev` synapse-deploy workflow | `synapse-repo-dev` | Marks a dev deployment; used as the source ref for downstream repos |
| `prod-synapse-v<YYYYMMDD.HHMMSS>` | `synapse-repo-prod` synapse-deploy workflow | `synapse-repo-prod` | Marks a production deployment; references the originating dev tag |
| `infra-v<YYYYMMDD.HHMMSS>` | terraform-apply workflow (if configured) | Any repo | Marks a Terraform infrastructure change |

### CLI commands reference

Manual promotion:

```bash
# Trigger test deployment for a specific version
gh workflow run synapse-deploy.yml \
  --repo <org>/synapse-repo-test \
  -f source_tag=synapse-v20260507.143022 \
  -f deploy_mode=deploy \
  -f promote_to_prod=true

# Trigger prod deployment directly (skip the test→prod chain)
gh workflow run synapse-deploy.yml \
  --repo <org>/synapse-repo-prod \
  -f source_tag=synapse-v20260507.143022 \
  -f deploy_mode=deploy
```

Rollback:

```bash
# Roll back test to a previous version
gh workflow run synapse-rollback.yml \
  --repo <org>/synapse-repo-test \
  -f target_version=synapse-v20260506.100000 \
  -f confirm=ROLLBACK

# Roll back prod to a previous version
gh workflow run synapse-rollback.yml \
  --repo <org>/synapse-repo-prod \
  -f target_version=synapse-v20260506.100000 \
  -f confirm=ROLLBACK
```

Terraform operations:

```bash
# Trigger Terraform apply manually for any repo
gh workflow run terraform-apply.yml --repo <org>/synapse-repo-dev

# Run drift detection manually
gh workflow run drift-detection.yml --repo <org>/synapse-repo-test
```

List available tags for rollback:

```bash
# List all release tags on the dev repo, newest first
git -C synapse-repo-dev tag -l "synapse-v*" --sort=-version:refname

# List production tags
git -C synapse-repo-prod tag -l "prod-synapse-v*" --sort=-version:refname
```

---

## Part 3: Architecture and Process Flow

### The core problem

Azure Synapse Analytics stores workspace artifacts — pipelines, notebooks, datasets, linked services, triggers, data flows, and SQL scripts — as individual JSON files when connected to Git. When you click Publish in Synapse Studio, it consolidates all those individual files into ARM-style templates on a special `workspace_publish` branch. Deploying to a downstream environment means taking those consolidated templates, swapping out environment-specific parameter values (workspace name, Key Vault URL, storage account URL), and applying them to the target workspace.

The fundamental constraint is that only one workspace connects to Git. Test and prod workspaces stay in live mode and receive artifacts exclusively through CI/CD. This means the dev repo is the single source of truth for all Synapse artifacts, and every downstream deployment is a parameterized projection of what dev has published.

### Why three repositories instead of one

A single-repository model with multi-environment workflows is simpler to set up but creates several problems at enterprise scale.

Access control becomes coarse. A single repo means everyone who can trigger a workflow can potentially trigger any environment's deployment. With three repos, you can restrict `synapse-repo-prod` access to a smaller group while keeping `synapse-repo-dev` open to the full data engineering team.

Blast radius is contained. If the dev repo's secrets are compromised, the attacker gains access to the dev workspace only — they never see test or prod credentials because those live in separate repositories. Each repo stores only `SYNAPSE_SPN_ID` and `SYNAPSE_SPN_SECRET` for its own environment.

Terraform state isolation is natural. Each repo maintains its own state artifact. There is no risk of one environment's state file accidentally being overwritten by another environment's workflow run. State locking is unnecessary because each repo's pipeline is sequential within its own environment.

Audit trails are clearer. Each repo's workflow run history shows exactly what was deployed to that environment and when. Cross-repo dispatch creates an explicit link (the `source_tag` input) that connects a prod deployment back to the exact dev commit that produced the artifacts.

The trade-off is operational complexity — you have 15 workflow files across three repos instead of 6 in one repo, and you need a GitHub App for cross-repo dispatch. For teams managing Synapse at scale with strict change control requirements, this trade-off is worthwhile.

### The two artifact formats

Synapse has two fundamentally different artifact representations, and the CI/CD pipeline must handle both.

The collaboration branch (`main`) in `synapse-repo-dev` holds individual JSON files organized into folders: `pipeline/`, `dataset/`, `linkedService/`, `notebook/`, `trigger/`, `dataflow/`, and `sqlscript/`. Each artifact is a separate file. These files are human-readable, PR-reviewable, and diff-friendly. Developers interact with these files through Synapse Studio, which commits changes to Git on their behalf.

The `workspace_publish` branch holds consolidated ARM-style templates — exactly two files under a folder named after the workspace: `TemplateForWorkspace.json` (all artifacts merged into one template) and `TemplateParametersForWorkspace.json` (parameterized values that differ per environment). These files are machine-generated when someone clicks Publish in Synapse Studio and should never be edited manually.

The `deploy` mode uses the ARM templates from `workspace_publish`. The `validateDeploy` mode uses the individual JSON files from the collaboration branch and generates templates at deploy time, bypassing the manual Publish step entirely.

### Cross-repository promotion flow

The promotion chain works through a sequence of `workflow_dispatch` calls, with each repository acting autonomously within its own environment while receiving instructions from upstream.

When a developer merges a pull request to `main` in `synapse-repo-dev`, the `synapse-deploy.yml` workflow triggers. It deploys the artifacts to `synapse-workspace-dev` using either `deploy` or `validateDeploy` mode depending on whether the trigger was a push to `workspace_publish` or a push to `main`. After a successful deployment, the workflow creates a git tag on `synapse-repo-dev` with the format `synapse-v<YYYYMMDD.HHMMSS>`. This tag serves as an immutable pointer to the exact commit and artifact state that was deployed.

The workflow's second job, `promote-to-test`, uses `actions/github-script@v7` to call `github.rest.actions.createWorkflowDispatch()` against `synapse-repo-test`. It passes three inputs: `source_tag` (the tag just created), `source_repo` (the full org/repo path), and `deploy_mode` (whether to use ARM templates or validateDeploy). The `GH_APP_TOKEN` authenticates this cross-repo call — it must have `actions:write` permission on the target repository.

When `synapse-repo-test` receives the dispatch, its `synapse-deploy.yml` workflow runs. The first step checks out `synapse-repo-test` itself to get the test-specific parameter file and scripts. The second step checks out `synapse-repo-dev` at the specified tag to get the ARM templates (or collaboration branch artifacts for validateDeploy mode). The workflow then stops triggers on `synapse-workspace-test`, deploys with test parameter overrides, and restarts triggers.

After a successful test deployment, the same cross-repo dispatch pattern fires toward `synapse-repo-prod`. The prod workflow follows the identical pattern — checkout self for parameters, checkout dev at the tag for templates, stop triggers, deploy, start triggers — but adds a final step that tags the prod repo with `prod-synapse-v<timestamp>`.

The approval gates are implemented through GitHub environments. When a workflow job references `environment: test`, GitHub pauses execution and waits for the configured reviewers to approve. This happens after the dispatch is received but before any deployment steps execute. The reviewer can see the workflow run's summary (which includes the source tag and deploy mode) before approving.

### Trigger management

Synapse triggers are scheduled or event-based processes that run pipelines. If a deployment attempts to update a trigger definition while the trigger is actively running, the deployment fails with `TriggerEnabledCannotUpdate`. This is the single most common CI/CD failure in Synapse environments.

The `toggle-triggers.sh` script addresses this by executing a stop-deploy-start sequence around every deployment. Before deployment, the script queries the workspace for all triggers in `Started` state, saves their names, and stops each one. After deployment, it waits 15 seconds for the deployment to settle, then restarts each previously-active trigger with retry logic (up to 3 attempts with 5-second backoff).

The `if: always()` condition on the trigger restart step ensures triggers are restarted even if the deployment fails. This prevents a deployment failure from leaving the workspace in a degraded state with all triggers permanently stopped.

### Terraform state management

Each repository uses Terraform's local backend, with state persisted as a GitHub Actions artifact. This avoids any dependency on Azure Storage, AWS S3, or Terraform Cloud for state management.

Before each Terraform operation, the workflow downloads the most recent artifact matching the pattern `tfstate-<env>` using `gh run download`. If no artifact exists (first run), the workflow proceeds with a fresh state. After `terraform apply` completes, the updated state file is uploaded as a new artifact with `overwrite: true` and 90-day retention.

The trade-off is the absence of state locking. If two workflow runs for the same environment execute concurrently, the second one may read stale state. This is mitigated by two factors: the environment-level approval gates serialize deployments for test and prod, and the `concurrency` setting on the dev terraform workflows cancels in-progress runs when a new one starts.

### Drift detection

There is no native drift detection for Synapse workspaces. The `detect-drift.sh` script implements a basic comparison by extracting artifact names from the ARM template and comparing them against the live workspace state.

The drift detection workflow runs daily at 06:00 UTC on a cron schedule. For `synapse-repo-dev`, it checks out its own `workspace_publish` branch for the template. For test and prod repos, it performs a cross-repo checkout of `synapse-repo-dev`'s `workspace_publish` branch using the `GH_APP_TOKEN`. If drift is detected (artifact names differ between template and live), the workflow creates a GitHub issue with the diff output and labels it `drift`.

Drift occurs when someone makes manual changes in Synapse Studio on a test or prod workspace, bypassing the CI/CD pipeline. The drift detection workflow catches this and creates a visible record, but it does not automatically remediate — the team must decide whether to redeploy from the pipeline (restoring the expected state) or incorporate the manual changes into the dev repo.

### Rollback

Rollback is a deployment of a previous version, not a Git revert. The rollback workflow in each downstream repo accepts a dev repo tag as input, checks out `synapse-repo-dev` at that tag, and deploys the ARM templates from that point in time with the current environment's parameter overrides.

The confirmation mechanism (`confirm: ROLLBACK`) prevents accidental rollbacks. The workflow validates that the specified tag exists on `synapse-repo-dev` before proceeding.

Rollback restores the Synapse artifacts (pipelines, notebooks, datasets, linked services, triggers, data flows, SQL scripts) to the state captured by the tag. It does not roll back Terraform-managed infrastructure (Spark pools, managed private endpoints) — those are managed through their own terraform-apply workflow and are not affected by Synapse artifact deployments.

### What Terraform manages vs. what the Synapse deployment action manages

This system has two distinct deployment pipelines that operate independently.

Terraform manages the infrastructure layer: Spark pool creation and sizing (auto-scale ranges, node sizes, Spark versions, auto-pause delays), managed private endpoints (Key Vault connectivity), and workspace data source lookups. These are defined in `synapse.tf` and parameterized through `terraform.tfvars`. Changes to Spark pool configurations flow through the `terraform-plan.yml` and `terraform-apply.yml` workflows.

The Synapse deployment action manages the artifact layer: pipelines, notebooks, datasets, linked services, triggers, data flows, and SQL scripts. These are defined as JSON files in the dev repo (or as consolidated ARM templates on `workspace_publish`) and parameterized through `<env>.parameters.yaml`. Changes to Synapse artifacts flow through `synapse-validate.yml` and `synapse-deploy.yml`.

The two layers can be deployed independently. Adding a new Spark pool does not trigger a Synapse artifact deployment, and adding a new pipeline does not trigger Terraform. Both can also be deployed in the same commit if the changes touch both `terraform/` files and Synapse artifact folders.

Resources explicitly excluded from both layers (per requirements): firewall rules, RBAC assignments, and SQL pools. These are assumed to be managed outside this system, either manually or through a separate IaC pipeline.

### Security model

The multi-repo architecture implements a defense-in-depth approach to credentials isolation.

Each repo stores only the service principal credentials for its own environment. An attacker who compromises `synapse-repo-test`'s secrets can only access `synapse-workspace-test` — they cannot see or use the dev or prod service principal credentials.

The `GH_APP_TOKEN` is the one credential shared across all three repos. It has no Azure permissions — it only authorizes GitHub API calls for cross-repo workflow dispatch. If this token is compromised, an attacker can trigger workflows but cannot authenticate to Azure without the per-repo SPN secrets.

The Synapse deployment action does not support OIDC/Workload Identity Federation. All Azure authentication uses client ID + client secret. The `creds` format for `azure/login@v2` passes these as a JSON string constructed inline in the workflow, avoiding the need for a combined `AZURE_CREDENTIALS` secret that would expose all four values (subscription, tenant, client ID, client secret) together.

Approval gates on test and production environments ensure that no deployment reaches downstream workspaces without human review. The reviewer sees the source tag, deploy mode, and workflow summary before approving.
