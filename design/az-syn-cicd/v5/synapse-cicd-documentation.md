# Azure Synapse CI/CD — Complete System Documentation

This document covers the full CI/CD system for Azure Synapse Analytics as designed and iterated throughout this engagement. It includes the delta deployment pipeline, promotion workflows, cross-repo synchronization, rollback, backup/restore, the divergent name problem and its solutions, the linked service strip methodology, and all supporting tooling.

---

## Table of Contents

1. System overview
2. Architecture
3. Delta deployment pipeline
4. Staging: building the ArtifactsFolder
5. Validate, strip, deploy
6. Promotion workflows
7. Promotion tracking with tags
8. Cross-repo branch synchronization
9. Rollback
10. Backup and restore
11. The divergent name problem
12. Linked service exclusion strategy
13. The dependsOn problem and fix
14. Scripts reference
15. Workflow reference
16. Secrets and permissions
17. Repository structure
18. Operational procedures

---

## 1. System overview

The system deploys Synapse artifacts (pipelines, datasets, notebooks, triggers, data flows, SQL scripts) across three environments — dev, test, prod — using incremental (delta) deployments. Only changed artifacts are deployed. Linked services are excluded from CI/CD entirely and managed through Synapse Studio UI in each workspace.

The deployment uses the `Azure/synapse-workspace-deployment` GitHub Action in a three-phase pipeline: stage changed files into a minimal ArtifactsFolder, validate to generate ARM templates, strip the templates (remove linked services and dependsOn), then deploy with `DeleteArtifactsNotInTemplate: false` for incremental updates.

Promotion tracking uses Git tags. Two moving tags (`promoted/test`, `promoted/prod`) mark the last commit successfully deployed to each environment. The delta for each promotion is computed by diffing the current branch against the corresponding tag.

---

## 2. Architecture

The system supports two repository models, chosen based on organizational requirements.

**Single-repo model** uses one repository with three long-lived branches (`dev`, `test`, `prod`). Feature branches target `dev` via PR. Promotions are PRs from `dev` → `test` and `test` → `prod`. The dev workspace is Git-connected to the `dev` branch.

**Multi-repo model** uses three repositories (`synapse-repo-dev`, `synapse-repo-test`, `synapse-repo-prod`). The dev repo is the source of truth. Cross-repo synchronization pushes branches to target repos when PRs merge. Cross-repo dispatch triggers downstream deployments.

Both models use the same delta deployment pipeline and strip methodology. The single-repo model is simpler to operate. The multi-repo model provides stronger access control boundaries and credential isolation.

The deployment flow for both models:

```
feature/* ──PR──► dev ──PR──► test ──PR──► prod
               │           │            │
          stage delta   stage delta   stage delta
          validate      validate      validate
          strip         strip         strip
          deploy        deploy        deploy
          (changed      (since last   (since last
           files)        promotion)    promotion)
```

---

## 3. Delta deployment pipeline

Every deployment follows a five-step process:

**Step 1: Diff.** Use `git diff` to identify which Synapse artifact files changed. For a PR to dev, the diff is between the PR branch and the dev branch. For a promotion to test, the diff is between the current dev HEAD and the `promoted/test` tag. For a promotion to prod, the diff is between the current test HEAD and the `promoted/prod` tag.

**Step 2: Stage.** The `stage-changed-artifacts.sh` script copies only the changed files into a staging directory, preserving the Synapse folder structure (`pipeline/`, `dataset/`, etc.). Linked service files are excluded at this step — they never enter the staging directory. Workspace config files (`publish_config.json`, `template-parameters-definition.json`) are always included if present.

**Step 3: Validate.** The `Azure/synapse-workspace-deployment` action runs with `operation: validate` against the staging directory. It generates ARM templates (`TemplateForWorkspace.json` and `TemplateParametersForWorkspace.json`) containing only the staged resources.

**Step 4: Strip.** The `strip-template.sh` script removes any linked services that leaked through the staging step and strips ALL `dependsOn` entries from the generated template. This prevents the deployment action's dependency resolver from failing on references to resources outside the delta.

**Step 5: Deploy.** The action runs with `operation: deploy` and `DeleteArtifactsNotInTemplate: false`. Only the resources in the delta template are created or updated. All other artifacts in the workspace remain untouched.

---

## 4. Staging: building the ArtifactsFolder

The staging script (`stage-changed-artifacts.sh`) is the entry point of every deployment. It accepts three arguments: the source directory (repository root), the staging directory path, and a file listing changed paths.

The script iterates each path in the changed files list and classifies it. Files under `pipeline/`, `dataset/`, `notebook/`, `trigger/`, `dataflow/`, and `sqlscript/` are copied to the staging directory. Files under `linkedService/` are skipped. Files outside Synapse artifact directories (README, Makefile, terraform/) are ignored.

The staging directory mirrors the Synapse folder structure that the deployment action expects. If three pipeline files and one dataset file changed, the staging directory contains:

```
staged/
├── pipeline/
│   ├── PipelineA.json
│   ├── PipelineB.json
│   └── PipelineC.json
├── dataset/
│   └── DatasetX.json
├── publish_config.json
└── template-parameters-definition.json
```

The `validate` operation receives this directory as its `ArtifactsFolder` and generates ARM templates containing exactly four resources — not the hundreds that would come from validating the full repository.

If a changed artifact references another artifact that didn't change (for example, a pipeline references an unchanged dataset), the referenced artifact is not in the staging directory. The validate operation may or may not resolve this reference during template generation. If validation fails with a reference error, the staging script can be extended to pull in referenced artifacts by parsing the JSON files for `referenceName` fields. In practice, the validate operation generates the template successfully for most artifact types without requiring the full dependency tree.

---

## 5. Validate, strip, deploy

The three-phase pipeline is the core of every deployment:

**Validate** uses the deployment action's built-in template generation. The action reads the artifact JSON files from the ArtifactsFolder, consolidates them into ARM-style templates, and writes `TemplateForWorkspace.json` (resource definitions) and `TemplateParametersForWorkspace.json` (parameterized values) to the `ExportedArtifacts/` directory. This is the same template generation that Synapse Studio performs when you click Publish, but it runs in CI instead of in the browser.

**Strip** processes the generated templates with `strip-template.sh`. The script removes linked service resources (type ending with `/linkedServices`) from the template. It extracts linked service names using a cascading parser that handles three name formats: ARM concat expressions (`[concat(parameters('workspaceName'), '/LS_Name')]`), simple prefixed paths (`workspace/LS_Name`), and bare names (`LS_Name`). It then removes ALL `dependsOn` entries from every remaining resource using `jq '.resources = [.resources[] | del(.dependsOn)]'`. Finally, it strips parameters whose keys match removed linked service names.

**Deploy** uses the deployment action with `operation: deploy`. The `TemplateFile` and `ParametersFile` point to the stripped templates. `OverrideArmParameters` points to the environment-specific parameter YAML (which contains only `workspaceName` since linked service properties are excluded). `DeleteArtifactsNotInTemplate: false` ensures unchanged artifacts in the workspace are preserved.

---

## 6. Promotion workflows

Three workflows handle the deployment lifecycle:

`deploy-dev.yml` triggers on PRs to the dev branch. It diffs the PR branch against dev, stages the changed files, validates, strips, and deploys to the dev workspace. A PR comment reports what was deployed.

`promote-test.yml` triggers on PRs from dev targeting the test branch. It diffs the dev branch against the `promoted/test` tag to compute the delta since the last test promotion. On first run (no tag exists), all artifact files are included. After successful deployment, it moves the `promoted/test` tag to the current dev HEAD and creates a permanent `release/test/<timestamp>` tag.

`promote-prod.yml` triggers on PRs from test targeting the prod branch. Same pattern as test — diffs against `promoted/prod`, stages the delta, deploys, moves the tag.

---

## 7. Promotion tracking with tags

Four tag patterns are used throughout the system:

| Pattern | Lifecycle | Purpose |
|---|---|---|
| `promoted/test` | Force-updated on each test deployment | Marks the dev commit last deployed to test |
| `promoted/prod` | Force-updated on each prod deployment | Marks the test commit last deployed to prod |
| `release/<env>/<timestamp>` | Permanent, never moved | Point-in-time deployment record for rollback |
| `snapshot/<env>/<timestamp>` | Permanent, never moved | Manual backup point created by users |
| `pre-restore/<env>/<timestamp>` | Permanent, never moved | Auto-created before every restore operation |

The `promoted/*` tags are the delta computation anchors. When the test promotion workflow runs, it diffs `promoted/test...HEAD` to find everything merged to dev since the last test deployment. After success, it deletes the old tag and creates a new one at the current HEAD. The `release/*` tags are permanent copies created alongside the moving tags so there's always a stable reference for rollback.

---

## 8. Cross-repo branch synchronization

For the multi-repo model, two workflows in the dev repo push branches to target repos when PRs merge.

`sync-test-repo.yml` triggers when a PR merges to the `test` branch in the dev repo. It generates a GitHub App installation token scoped to `synapse-repo-test`, then pushes the test branch to the target repo using `git push --force` with `x-access-token` authentication. The GitHub App's org admin permissions bypass branch protection on the target.

`sync-prod-repo.yml` follows the same pattern for the prod branch and `synapse-repo-prod`.

The push triggers any `push`-event workflows in the target repo, chaining the sync with downstream deployment automatically.

---

## 9. Rollback

Rollback workflows revert a workspace to a previous release tag. They use the full-template approach (not delta) with `DeleteArtifactsNotInTemplate: true` — the workspace is restored to the exact artifact state at the tag.

`rollback-test.yml` lives in the test repo. Any user with workflow run permission can trigger it. Inputs: a release tag and `ROLLBACK` confirmation. The workflow checks out the repo at the tag, validates the full artifact set, strips linked services and dependsOn, and deploys.

`rollback-prod.yml` lives in the prod repo. Adds a team membership gate — the triggering user must be an active member of `cicd-admins-team`. The workflow uses `github.rest.teams.getMembershipForUserInOrg()` with the `GH_APP_TOKEN` to verify membership before proceeding. The deployment job also requires `production` environment approval.

Both rollback workflows verify the tag exists and show available release tags if it doesn't.

---

## 10. Backup and restore

The backup/restore system provides database-style recovery for Synapse workspaces.

**Snapshot (backup)** creates a tagged recovery point from the current branch HEAD without deploying. The tag follows the pattern `snapshot/<env>/<timestamp>`. Snapshots can be created manually via workflow dispatch or on a schedule.

**Restore** deploys a branch, tag, or commit SHA to the workspace. Before deploying, it automatically creates a `pre-restore/<env>/<timestamp>` tag at the current branch HEAD as a safety net. The restore uses `DeleteArtifactsNotInTemplate: true` for a full workspace reset.

Restore accepts any Git ref as input — branch names, snapshot tags, release tags, pre-restore tags, or raw SHAs. Leaving the input blank restores from the current branch HEAD, which resets a drifted workspace to match the repository.

Every restore is reversible by restoring to the pre-restore tag.

---

## 11. The divergent name problem

When Synapse artifacts have different naming conventions across environments — `dev-db-sql01` in dev, `tst-db-sql01` in test — CI/CD deployments break. The ARM template carries literal names from dev. The deployment creates `dev-db-sql01` in test instead of `tst-db-sql01`. Pipelines referencing the test-specific name break.

Seven solutions were evaluated:

| # | Method | When to use |
|---|---|---|
| 1 | Environment-agnostic naming | New projects, willing to rename |
| 2 | Parameterize values only | Quick unblock, cosmetic mismatch OK |
| 3 | Expression-based linked services | Key Vault, storage connectors |
| 4 | Terraform templatefile() | Terraform-native teams |
| 5 | Terraform azurerm_synapse_linked_service | TF-managed infrastructure |
| 6 | Workspace sync (Go tool) | Legacy envs, force homogeneity |
| 7 | ARM template rewrite (Go tool) | Retrofit divergent names |

**Method 1** (rename everything to environment-agnostic names) is the correct long-term solution with zero ongoing cost. **Method 6** (workspace sync) copies missing artifacts across workspaces to create homogeneity. **Method 7** (ARM template rewrite) uses a pattern file of match/replace pairs to transform the template before deployment.

The system as built uses a different approach: **linked service exclusion**. Linked services are stripped from every deployment. Each workspace maintains its own linked services through Synapse Studio UI. This eliminates the divergent name problem entirely for linked services — they never appear in the ARM template, so there are no names to mismatch.

---

## 12. Linked service exclusion strategy

Linked services are excluded at two levels:

**Staging level.** The `stage-changed-artifacts.sh` script skips all files under `linkedService/` when building the ArtifactsFolder. The validate operation never sees linked service definitions, so the generated ARM template never contains linked service resources.

**Template level.** The `strip-template.sh` script removes any linked service resources that appear in the template (defensive, in case staging didn't catch them) and removes linked service parameters from the parameters file.

With linked services excluded and `DeleteArtifactsNotInTemplate: false`, the deployment action treats linked services as outside its scope. It never creates, updates, or deletes them. Each workspace's linked services are configured independently through Synapse Studio with environment-appropriate names and connection strings.

The `template-parameters-definition.json` file in the repo should have its `Microsoft.Synapse/workspaces/linkedServices` block removed so the validate operation doesn't parameterize linked service properties at all.

The trade-off: new linked services must be created manually in each workspace through Synapse Studio before merging pipelines that reference them.

---

## 13. The dependsOn problem and fix

The `Azure/synapse-workspace-deployment` action builds a dependency graph from `dependsOn` entries in the ARM template before deploying. If any entry references a resource not in the template, the action fails with "Could not figure out full dependency model."

This affects both the linked service strip approach and the delta deployment approach. When linked services are stripped, data flows and datasets that declare `dependsOn` entries referencing those linked services have dangling references. When the template is filtered to a delta, `dependsOn` entries referencing unchanged (and therefore absent) resources also dangle.

The fix is to strip ALL `dependsOn` entries from the template:

```bash
jq '.resources = [.resources[] | del(.dependsOn)]' "$TEMPLATE" > tmp && mv tmp "$TEMPLATE"
```

This is safe because `DeleteArtifactsNotInTemplate: false` means the workspace already contains all dependencies. The deployment action only needs to create or update the resources in the template — deployment ordering between them is irrelevant since their dependencies already exist.

For rollback and restore (which use `DeleteArtifactsNotInTemplate: true`), dependsOn is also stripped. The full template contains all non-linked-service resources, so there are no missing inter-resource dependencies. The linked service references in dependsOn are the only dangling ones, and stripping them all is simpler than selectively filtering.

---

## 14. Scripts reference

### stage-changed-artifacts.sh

Builds a minimal ArtifactsFolder from a list of changed files.

```
Usage: ./scripts/stage-changed-artifacts.sh <source-dir> <staging-dir> <changed-files-list>
```

Inputs: repository root, staging directory path, file with one changed path per line (from `git diff`). Copies files from `pipeline/`, `dataset/`, `notebook/`, `trigger/`, `dataflow/`, `sqlscript/` to the staging directory. Excludes `linkedService/`. Includes workspace config files. Outputs `staged_count` and `skip_deploy` to `$GITHUB_OUTPUT`.

### strip-template.sh

Cleans a generated ARM template for deployment.

```
Usage: ./scripts/strip-template.sh <template.json> [params.json]
```

Strips linked service resources, all dependsOn entries, and linked service parameters. Outputs `resource_count` to `$GITHUB_OUTPUT`.

### strip-for-rollback.sh

Same as `strip-template.sh` but used by rollback and restore workflows. Identical functionality — exists as a separate file for clarity in workflow references.

### toggle-triggers.sh

Stops and starts Synapse triggers around deployments.

```
Usage: ./scripts/toggle-triggers.sh <stop|start> <workspace-name>
```

On `stop`: lists triggers in `Started` state, saves names, stops each one. On `start`: waits 15 seconds, restarts each saved trigger with 3 retries.

### sync-branch-to-repo.sh

Pushes a branch from the dev repo to a target repo (multi-repo model).

```
Usage: ./scripts/sync-branch-to-repo.sh <branch> <target-repo> <token> [target-branch]
```

Uses `x-access-token` authentication with the GitHub App token to bypass branch protection.

### Name extraction (jq helper)

Used across all strip scripts to extract artifact names from ARM template resource name fields. Handles three formats:

```
[concat(parameters('workspaceName'), '/ArtifactName')]  → ArtifactName
workspace/ArtifactName                                   → ArtifactName
ArtifactName                                             → ArtifactName
```

The jq expression tries a regex match for concat expressions first, falls back to `split("/") | last` for prefixed paths, and uses the raw string for bare names.

---

## 15. Workflow reference

### Delta deployment (single-repo model)

| Workflow | Trigger | Scope | Target |
|---|---|---|---|
| `deploy-dev.yml` | PR to `dev` | Files changed in PR | dev workspace |
| `promote-test.yml` | PR to `test` | Changes since `promoted/test` | test workspace |
| `promote-prod.yml` | PR to `prod` | Changes since `promoted/prod` | prod workspace |

### Cross-repo sync (multi-repo model)

| Workflow | Trigger | Action |
|---|---|---|
| `sync-test-repo.yml` | PR merge to `test` in dev repo | Push test branch to synapse-repo-test |
| `sync-prod-repo.yml` | PR merge to `prod` in dev repo | Push prod branch to synapse-repo-prod |

### Rollback

| Workflow | Repository | Authorization | Deployment mode |
|---|---|---|---|
| `rollback-test.yml` | synapse-repo-test | Workflow run permission | Full (DeleteNotInTemplate: true) |
| `rollback-prod.yml` | synapse-repo-prod | cicd-admins-team membership | Full (DeleteNotInTemplate: true) |

### Backup and restore

| Workflow | Repository | Operations |
|---|---|---|
| `workspace-restore-dev.yml` | synapse-repo-dev | snapshot, restore |
| `workspace-restore-test.yml` | synapse-repo-test | snapshot, restore |
| `workspace-restore-prod.yml` | synapse-repo-prod | snapshot, restore |

---

## 16. Secrets and permissions

### Repository secrets

| Secret | Description | Used by |
|---|---|---|
| `AZURE_SUBSCRIPTION_ID` | Azure subscription | All deploy workflows |
| `AZURE_TENANT_ID` | Azure AD tenant | All deploy workflows |
| `SYNAPSE_SPN_ID` | Service principal client ID (per environment) | All deploy workflows |
| `SYNAPSE_SPN_SECRET` | Service principal client secret (per environment) | All deploy workflows |
| `GH_APP_TOKEN` | GitHub App token (org admin) | Cross-repo sync, prod rollback team check |
| `GH_APP_ID` | GitHub App numeric ID | Cross-repo sync (runtime token generation) |
| `GH_APP_PRIVATE_KEY` | GitHub App PEM private key | Cross-repo sync (runtime token generation) |

### Azure service principal roles

Each service principal needs roles in both Azure RBAC and Synapse RBAC:

| System | Role | Scope |
|---|---|---|
| Azure RBAC | Contributor | Resource group |
| Azure RBAC | Storage Blob Data Contributor | ADLS Gen2 account |
| Synapse RBAC | Synapse Administrator | Workspace |

Synapse RBAC is assigned through Synapse Studio (Manage → Access control), not the Azure portal.

### GitHub App permissions

| Permission | Level | Purpose |
|---|---|---|
| Contents | Read & Write | Push to target repos |
| Actions | Write | Cross-repo workflow dispatch |
| Administration | Read & Write | Bypass branch protection |
| Organization: Members | Read | Team membership check for prod rollback |

---

## 17. Repository structure

### Single-repo model

```
synapse-repo/
├── .github/workflows/
│   ├── deploy-dev.yml
│   ├── promote-test.yml
│   ├── promote-prod.yml
│   ├── rollback-test.yml          (optional)
│   ├── rollback-prod.yml          (optional)
│   └── workspace-restore-dev.yml  (optional)
├── scripts/
│   ├── stage-changed-artifacts.sh
│   ├── strip-template.sh
│   ├── strip-for-rollback.sh
│   └── toggle-triggers.sh
├── parameters/
│   ├── dev.parameters.yaml
│   ├── test.parameters.yaml
│   └── prod.parameters.yaml
├── pipeline/
├── dataset/
├── linkedService/
├── notebook/
├── trigger/
├── dataflow/
├── sqlscript/
├── publish_config.json
└── template-parameters-definition.json
```

### Multi-repo model

Each repo has the same `scripts/` and `parameters/` structure. The dev repo additionally has cross-repo sync workflows and the Synapse artifact directories. Test and prod repos have rollback and restore workflows.

---

## 18. Operational procedures

### First-time setup

1. Create three Synapse workspaces. Connect the dev workspace to the `dev` branch via Git integration.
2. Create service principals for each environment with the required roles.
3. Set repository secrets.
4. Create GitHub environments (`dev`, `test`, `production`) with appropriate approval reviewers.
5. Remove the `Microsoft.Synapse/workspaces/linkedServices` block from `template-parameters-definition.json`.
6. Create linked services in each workspace through Synapse Studio.
7. Push the workflows and scripts to the repository.

### Daily development

1. Developer creates a feature branch from `dev`.
2. Developer works in Synapse Studio (connected to dev workspace).
3. Developer creates a PR targeting `dev`.
4. The `deploy-dev.yml` workflow stages the changed files, validates, strips, and deploys the delta to the dev workspace.
5. Developer reviews the deployment result in the workflow summary and in Synapse Studio.
6. PR is merged to `dev`.

### Promoting to test

1. Create a PR from `dev` targeting `test`.
2. The `promote-test.yml` workflow computes the delta since the last test promotion, stages, validates, strips, and deploys.
3. Reviewer approves the test environment gate.
4. Deployment completes. The `promoted/test` tag moves forward. A permanent `release/test/<timestamp>` tag is created.
5. PR is merged to `test`.

### Promoting to prod

Same process as test, targeting the `prod` branch. The `production` environment gate requires additional reviewers.

### Adding a new linked service

1. Create the linked service in the dev workspace through Synapse Studio.
2. Create the same linked service in test and prod workspaces through Synapse Studio (with environment-appropriate connection strings).
3. Verify the linked service works in each workspace.
4. Now merge pipeline changes that reference the new linked service.

### Rolling back

```bash
# List available release tags
gh api repos/<org>/<repo>/tags --jq '.[].name' | grep '^release/'

# Trigger rollback
gh workflow run rollback-test.yml -f release_tag=release/test/20260604.143022 -f confirm=ROLLBACK
```

### Backing up and restoring

```bash
# Create a snapshot (backup)
gh workflow run workspace-restore-dev.yml -f operation=snapshot

# Restore to current branch state
gh workflow run workspace-restore-dev.yml -f operation=restore -f confirm=RESTORE

# Restore to a specific snapshot
gh workflow run workspace-restore-dev.yml -f operation=restore -f restore_ref=snapshot/dev/20260604.143022 -f confirm=RESTORE
```

### Handling validate failures

If the validate operation fails because a changed artifact references another artifact not in the staging directory, there are two options. First, extend `stage-changed-artifacts.sh` to parse artifact JSON files for `referenceName` fields and include referenced artifacts in the staging directory. Second, fall back to the full-repo validate approach (pass the entire repository as ArtifactsFolder and filter the ARM template after generation). The staging approach is preferred for speed and clarity; the full-repo approach is the fallback for complex dependency trees.

### Cleaning orphaned artifacts

Delta deployments with `DeleteArtifactsNotInTemplate: false` never delete artifacts from the workspace. If an artifact is removed from the repository, it remains in the workspace until manually deleted. Periodically review the workspace in Synapse Studio and remove artifacts that no longer exist in the repository. Alternatively, run a full restore from the current branch to reset the workspace: this uses `DeleteArtifactsNotInTemplate: true` and removes any artifact not in the branch.
