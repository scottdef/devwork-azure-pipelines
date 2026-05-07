# Azure Synapse CI/CD — Multi-Repository Architecture

Three-repo deployment model for Azure Synapse Analytics. Each environment owns its infrastructure and deployment lifecycle in a dedicated repository. Cross-repo promotion uses `workflow_dispatch` via a GitHub App.

## Architecture

```
  synapse-repo-dev                synapse-repo-test              synapse-repo-prod
  ════════════════                ═════════════════              ══════════════════
  Git-connected to                Receives dispatch              Receives dispatch
  synapse-workspace-dev           from dev repo                  from test repo

  ┌─────────────────┐
  │ Synapse Studio   │
  │ Feature branches │
  └────────┬────────┘
           │ PR + merge
           ▼
  ┌─────────────────┐            ┌─────────────────┐           ┌─────────────────┐
  │ main branch     │            │ main branch     │           │ main branch     │
  │ (collaboration) │            │ (tf + params)   │           │ (tf + params)   │
  └────────┬────────┘            └────────┬────────┘           └────────┬────────┘
           │                              │                             │
     ┌─────┴──────┐                 ┌─────┴──────┐               ┌─────┴──────┐
     │ Terraform  │                 │ Terraform  │               │ Terraform  │
     │ apply dev  │                 │ apply test │               │ apply prod │
     └────────────┘                 └────────────┘               └────────────┘
           │
     ┌─────┴──────┐   dispatch    ┌──────────────┐  dispatch   ┌──────────────┐
     │ Synapse    │──────────────►│ Synapse      │────────────►│ Synapse      │
     │ deploy dev │  (GH_APP)     │ deploy test  │  (GH_APP)   │ deploy prod  │
     │ + tag      │               │ (approval)   │             │ (approval)   │
     └────────────┘               │ checkout dev │             │ checkout dev │
                                  │ @ tag        │             │ @ tag        │
                                  └──────────────┘             └──────┬───────┘
                                                                      │
                                                                 tag: prod-*
```

## Repository Roles

| Repository | Workspace | Git Connected | Artifacts Source |
|---|---|---|---|
| `synapse-repo-dev` | `synapse-workspace-dev` | Yes | Self (collaboration + workspace_publish) |
| `synapse-repo-test` | `synapse-workspace-test` | No | Cross-repo checkout of `synapse-repo-dev` @ tag |
| `synapse-repo-prod` | `synapse-workspace-prod` | No | Cross-repo checkout of `synapse-repo-dev` @ tag |

## Cross-Repo Promotion Flow

1. Developer works in Synapse Studio → commits to `synapse-repo-dev`
2. PR merges to `main` (or Publish pushes to `workspace_publish`)
3. `synapse-repo-dev` deploys to dev workspace, tags `synapse-v<timestamp>`
4. Dev workflow fires `workflow_dispatch` → `synapse-repo-test/synapse-deploy.yml`
5. Test workflow checks out `synapse-repo-dev` at the tag, deploys with test parameters
6. Test workflow fires `workflow_dispatch` → `synapse-repo-prod/synapse-deploy.yml`
7. Prod workflow checks out `synapse-repo-dev` at the tag, deploys with prod parameters
8. Prod workflow tags `prod-synapse-v<timestamp>` on the prod repo

## Secrets Per Repository

Each repo stores only its own environment's credentials:

| Secret | Description | All Repos |
|---|---|---|
| `AZURE_SUBSCRIPTION_ID` | Azure subscription | Yes |
| `AZURE_TENANT_ID` | Azure AD tenant | Yes |
| `SYNAPSE_SPN_ID` | Environment-specific SPN client ID | Yes (different value per repo) |
| `SYNAPSE_SPN_SECRET` | Environment-specific SPN secret | Yes (different value per repo) |
| `GH_APP_TOKEN` | GitHub App token (org owner) | Yes |

## Workflows Per Repository

### synapse-repo-dev

| Workflow | Trigger | Purpose |
|---|---|---|
| `terraform-plan.yml` | PR to main | Plan dev infrastructure |
| `terraform-apply.yml` | Push to main | Apply dev infrastructure |
| `synapse-validate.yml` | PR (artifact paths) | Validate Synapse artifacts |
| `synapse-deploy.yml` | Push to workspace_publish / dispatch | Deploy to dev, promote to test |
| `drift-detection.yml` | Daily 06:00 UTC | Detect manual changes in dev |

### synapse-repo-test

| Workflow | Trigger | Purpose |
|---|---|---|
| `terraform-plan.yml` | PR to main | Plan test infrastructure |
| `terraform-apply.yml` | Push to main | Apply test infrastructure |
| `synapse-deploy.yml` | Dispatch from dev | Deploy to test, promote to prod |
| `synapse-rollback.yml` | Manual dispatch | Roll back test to a tagged version |
| `drift-detection.yml` | Daily 06:00 UTC | Detect manual changes in test |

### synapse-repo-prod

| Workflow | Trigger | Purpose |
|---|---|---|
| `terraform-plan.yml` | PR to main | Plan prod infrastructure |
| `terraform-apply.yml` | Push to main | Apply prod infrastructure |
| `synapse-deploy.yml` | Dispatch from test | Deploy to prod, tag release |
| `synapse-rollback.yml` | Manual dispatch | Roll back prod to a tagged version |
| `drift-detection.yml` | Daily 06:00 UTC | Detect manual changes in prod |

## Manual Promotion

To manually promote a specific version:

```bash
# Trigger test deployment with a specific dev tag
gh workflow run synapse-deploy.yml \
  --repo <org>/synapse-repo-test \
  -f source_tag=synapse-v20260507.143022 \
  -f deploy_mode=deploy \
  -f promote_to_prod=false

# Trigger prod deployment directly
gh workflow run synapse-deploy.yml \
  --repo <org>/synapse-repo-prod \
  -f source_tag=synapse-v20260507.143022 \
  -f deploy_mode=deploy
```

## Rollback

```bash
# Roll back test
gh workflow run synapse-rollback.yml \
  --repo <org>/synapse-repo-test \
  -f target_version=synapse-v20260506.100000 \
  -f confirm=ROLLBACK

# Roll back prod
gh workflow run synapse-rollback.yml \
  --repo <org>/synapse-repo-prod \
  -f target_version=synapse-v20260506.100000 \
  -f confirm=ROLLBACK
```

## Key Design Decisions

1. **One repo per environment** — clear ownership boundaries, independent terraform state, separate access control per repo.

2. **Dev repo is the single source of truth** — only dev connects to Git. Test and prod check out dev at a specific tag, ensuring they deploy exactly what was validated.

3. **Cross-repo dispatch via GitHub App** — the `GH_APP_TOKEN` (org owner permissions) enables `workflow_dispatch` across repos. No shared runners, no shared artifacts.

4. **Tag-based promotion** — every dev deployment creates a `synapse-v*` tag. Downstream repos reference that tag. Rollback = redeploy a previous tag. Full auditability.

5. **Per-repo terraform state** — each repo maintains its own `tfstate-<env>` GitHub artifact. No shared state backend. Each environment evolves independently.
