# Azure Synapse CI/CD — Terraform + GitHub Actions

Multi-environment Synapse deployment (dev → test → prod) with Terraform infrastructure management, GitHub artifact-based state persistence, and workflow-triggered-from-workflow orchestration.

## Architecture

```
Feature Branch ──PR──► main ──push──► 02-deploy-dev ──auto──► 03-promote-test ──manual──► 04-promote-prod
                                          │                          │                          │
                                    synapse-workspace-dev    synapse-workspace-test     synapse-workspace-prod
                                    synapse-repo-dev         synapse-repo-test          synapse-repo-prod
                                    (Git-linked)             (Live mode)                (Live mode)
```

Only DEV connects to Git. Test/prod receive artifacts exclusively through CI/CD.
Terraform state lives as GitHub artifacts — no external backend required.

## Workflows

| # | Name | Trigger | Purpose |
|---|------|---------|---------|
| 01 | Validate | PR → main | Validate Synapse artifacts, post to PR |
| 02 | Deploy DEV | Push → main | validateDeploy, auto-triggers 03 |
| 03 | Promote TEST | Dispatch | Sync repos, deploy with test params |
| 04 | Promote PROD | Manual + approval | Requires `DEPLOY` confirm + env reviewers |
| 05 | TF Plan | PR (terraform/) | Download state artifact → plan → PR comment |
| 06 | TF Apply | Push (terraform/) | Download state → apply → upload state |
| 07 | Drift | Cron 06:00 UTC | Compare live vs repo, create issues |
| 08 | Rollback | Manual | Checkout tag → redeploy |
| 09 | Orchestrator | Manual | Dispatch any workflow, webhook, repo sync |

## Secrets

| Secret | Description |
|--------|-------------|
| `SYNAPSE_DEV_SPN_ID` | Dev SPN client ID |
| `SYNAPSE_DEV_SPN_SECRET` | Dev SPN client secret |
| `SYNAPSE_TEST_SPN_ID` | Test SPN client ID |
| `SYNAPSE_TEST_SPN_SECRET` | Test SPN client secret |
| `SYNAPSE_PROD_SPN_ID` | Prod SPN client ID |
| `SYNAPSE_PROD_SPN_SECRET` | Prod SPN client secret |
| `AZURE_TENANT_ID` | Azure AD tenant |
| `AZURE_SUBSCRIPTION_ID` | Azure subscription |
| `GH_APP_TOKEN` | GitHub App token (org owner) |

## Variables

| Variable | Example |
|----------|---------|
| `GITHUB_ORG` | `my-org` |
| `SYNAPSE_WORKSPACE_DEV` | `synapse-workspace-dev` |
| `SYNAPSE_WORKSPACE_TEST` | `synapse-workspace-test` |
| `SYNAPSE_WORKSPACE_PROD` | `synapse-workspace-prod` |
| `RESOURCE_GROUP_DEV` | `rg-synapse-dev` |
| `RESOURCE_GROUP_TEST` | `rg-synapse-test` |
| `RESOURCE_GROUP_PROD` | `rg-synapse-prod` |

## State Management

Each TF workflow downloads `terraform-state-{env}` artifact → works locally → uploads new state.
Timestamped backups go to `terraform-state-backup-{env}-{N}` (365-day retention).

```bash
make tf/state-download ENV=dev    # pull state locally
make tf/state-list ENV=dev        # list artifacts
```

## Quick Start

```bash
make validate                     # check artifact JSON
make tf/plan ENV=dev              # plan infrastructure
make tf/apply ENV=dev             # apply infrastructure
make dispatch/full                # trigger full pipeline
```
