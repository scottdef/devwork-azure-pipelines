# Synapse Workspace Rollback

Manual rollback workflows for test and prod Synapse workspaces. Each workflow checks out a previous release tag, generates ARM templates from that version, strips linked services, and deploys the full template with `DeleteArtifactsNotInTemplate: true` to revert the workspace to that exact state.

## Workflows

| Workflow | Repository | Who can trigger | Authorization |
|---|---|---|---|
| `rollback-test.yml` | `synapse-repo-test` | Any user with Actions run permission | Repository-level |
| `rollback-prod.yml` | `synapse-repo-prod` | Members of `cicd-admins-team` only | Org team membership check |

## Usage

### List available release tags

```bash
# Test releases
gh api repos/<org>/synapse-repo-test/tags --paginate --jq '.[].name' | grep '^release/'

# Prod releases
gh api repos/<org>/synapse-repo-prod/tags --paginate --jq '.[].name' | grep '^release/'
```

### Trigger via CLI

```bash
# Roll back test
gh workflow run rollback-test.yml \
  --repo <org>/synapse-repo-test \
  -f release_tag=release/test/20260507.143022 \
  -f confirm=ROLLBACK

# Roll back prod (must be cicd-admins-team member)
gh workflow run rollback-prod.yml \
  --repo <org>/synapse-repo-prod \
  -f release_tag=release/prod/20260507.180000 \
  -f confirm=ROLLBACK
```

### Trigger via GitHub UI

1. Go to the repository's Actions tab
2. Select the rollback workflow
3. Click "Run workflow"
4. Enter the release tag and type `ROLLBACK` to confirm
5. Click "Run workflow"

For prod, the workflow will fail at the authorization step if you're not in `cicd-admins-team`.

## Authorization model

### Test rollback

Controlled by standard GitHub repository permissions. Any user who can trigger `workflow_dispatch` (write access to the repository) can execute the rollback. The `test` GitHub Environment's approval rules still apply — if the environment has required reviewers, they must approve before deployment proceeds.

### Prod rollback

Two-layer authorization:

1. **Team membership check**: the workflow's first job verifies the triggering user (`github.actor`) is an active member of `cicd-admins-team` via the GitHub API. This check uses `GH_APP_TOKEN` because `GITHUB_TOKEN` cannot read org team membership. If the user is not a member, the workflow fails immediately with a clear error message.

2. **Environment approval**: the deployment job runs in the `production` GitHub Environment. If the environment has required reviewers configured, they must still approve after the team check passes.

### Required secrets

| Secret | Purpose | Required by |
|---|---|---|
| `SYNAPSE_SPN_ID` | Azure service principal client ID | Both |
| `SYNAPSE_SPN_SECRET` | Azure service principal secret | Both |
| `AZURE_SUBSCRIPTION_ID` | Azure subscription | Both |
| `AZURE_TENANT_ID` | Azure AD tenant | Both |
| `GH_APP_TOKEN` | GitHub App token (org members:read) | Prod only |

### GitHub App permissions for team check

The `GH_APP_TOKEN` used in the prod workflow needs `Organization: Members: Read` permission to call `teams.getMembershipForUserInOrg`. Without this, the team membership check returns 404 for all users.

## What rollback deploys

Rollback uses `DeleteArtifactsNotInTemplate: true` with the full template from the tagged version. This means the workspace is reverted to the exact artifact state at that tag:

- Artifacts that existed at the tag are restored to their tagged versions
- Artifacts added after the tag are deleted from the workspace
- Linked services are untouched (stripped from the template)
- Triggers are stopped before deployment and restarted after

This is a true revert, not an incremental patch. The workspace matches the tag exactly (minus linked services).

## File placement

```
synapse-repo-test/                    synapse-repo-prod/
├── .github/workflows/                ├── .github/workflows/
│   └── rollback-test.yml             │   └── rollback-prod.yml
├── scripts/                          ├── scripts/
│   ├── strip-for-rollback.sh         │   ├── strip-for-rollback.sh
│   └── toggle-triggers.sh            │   └── toggle-triggers.sh
└── parameters/                       └── parameters/
    └── test.parameters.yaml              └── prod.parameters.yaml
```
