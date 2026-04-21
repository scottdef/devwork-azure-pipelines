# Azure Synapse CI/CD — GitHub Workflows Implementation

## Architecture

```
┌──────────────────────────────────────────────────────────────────────────────┐
│                           SYNAPSE-REPO-DEV                                   │
│  Branches: dev · test · prod · publish_branch · main                         │
│  Git-integrated with: synapse-workspace-dev (collab=dev)                     │
│                                                                               │
│  ① developer creates branch from prod                                         │
│  ② PR → dev        [2 approvals: db-ops — no status check required]          │
│  ③ push → dev  ──► synapse-dev-cicd-workflow (validate + deploy → dev)       │
│                    + create dev-YYYY.MM.DD.N release                          │
│                                                                               │
│  ④ PR → test       [2 approvals: db-ops + synapse-dev-cicd-workflow/validate]│
│  ⑤ push → test ──► dispatch-test-cicd.yml ──────────────────────────────────┼──►
│                                                                               │
│  ⑥ PR → prod       [2 approvals: db-ops — no status check required]          │
│  ⑦ push → prod ──► dispatch-prod-cicd.yml ──────────────────────────────────┼──►
└──────────────────────────────────────────────────────────────────────────────┘
         ▼  workflow_dispatch                         ▼  workflow_dispatch
┌─────────────────────────┐              ┌─────────────────────────────────────┐
│   SYNAPSE-REPO-TEST      │              │       SYNAPSE-REPO-PROD             │
│   Branches: test ·       │              │  Branches: prod · publish_branch    │
│   publish_branch · main  │              │  · main                             │
│                          │              │                                     │
│  Clones synapse-repo-dev │              │  Clones synapse-repo-dev prod@SHA   │
│  test@SHA                │              │                                     │
│  Deploy → test workspace │              │  Deploy → prod workspace            │
│  Create test-*.* release │              │  (requires env approval gate)       │
│                          │              │  Create prod-*.* release            │
│  PR → main checks:       │              │                                     │
│  [1 approval: db-ops     │              │  PR → main checks:                  │
│  + test-cicd/validate]   │              │  [2 approvals: db-ops               │
└─────────────────────────┘              │  + prod-cicd/validate]              │
                                          └─────────────────────────────────────┘
```

---

## File Structure

```
synapse-cicd/
├── terraform/
│   ├── main.tf            # Repos, branches, rulesets, team access
│   ├── variables.tf
│   └── outputs.tf
│
├── synapse-repo-dev/      ← commit these files INTO synapse-repo-dev
│   ├── .github/workflows/
│   │   ├── synapse-dev-cicd-workflow.yml   # validate (PR check) + deploy (push→dev)
│   │   ├── dispatch-test-cicd.yml          # triggers test workflow on push→test
│   │   └── dispatch-prod-cicd.yml          # triggers prod workflow on push→prod
│   ├── parameters/
│   │   ├── dev.parameters.yaml
│   │   ├── test.parameters.yaml
│   │   └── prod.parameters.yaml
│   ├── scripts/
│   │   ├── toggle-triggers.sh
│   │   └── create-release.sh
│   └── template-parameters-definition.json
│
├── synapse-repo-test/     ← commit these files INTO synapse-repo-test
│   ├── .github/workflows/
│   │   └── synapse-test-cicd-workflow.yml
│   └── scripts/
│       ├── toggle-triggers.sh
│       └── create-release.sh
│
└── synapse-repo-prod/     ← commit these files INTO synapse-repo-prod
    ├── .github/workflows/
    │   └── synapse-prod-cicd-workflow.yml
    └── scripts/
        ├── toggle-triggers.sh
        └── create-release.sh
```

---

## Step 1 — Azure Service Principal Setup

Run once per environment. Service principal names match your spec.

```bash
#!/usr/bin/env bash
set -euo pipefail

ORG="myorg"
SUBSCRIPTION_ID="<your-subscription-id>"

for ENV in dev test prod; do
  SPN_NAME="synapse-${ENV}-spn"
  RG_NAME="rg-synapse-${ENV}"
  WS_NAME="synapse-workspace-${ENV}"

  echo "==> Creating SPN: ${SPN_NAME}"

  # 1. Create the service principal with Contributor on the resource group
  az ad sp create-for-rbac \
    --name "$SPN_NAME" \
    --role Contributor \
    --scopes "/subscriptions/${SUBSCRIPTION_ID}/resourceGroups/${RG_NAME}" \
    --sdk-auth > "${ENV}-spn-credentials.json"

  echo "    Credentials saved to ${ENV}-spn-credentials.json"

  # 2. Assign Storage Blob Data Contributor (ADLS Gen2 access)
  SPN_OBJECT_ID=$(az ad sp list --display-name "$SPN_NAME" \
    --query "[0].id" -o tsv)

  az role assignment create \
    --role "Storage Blob Data Contributor" \
    --assignee-object-id "$SPN_OBJECT_ID" \
    --scope "/subscriptions/${SUBSCRIPTION_ID}/resourceGroups/${RG_NAME}"

  # 3. Assign Synapse RBAC — Synapse Administrator (data plane)
  #    NOTE: This must be done via Synapse REST API or Portal after workspace exists
  #    az synapse role assignment create is the correct command:
  az synapse role assignment create \
    --workspace-name "$WS_NAME" \
    --role "Synapse Administrator" \
    --assignee "$SPN_OBJECT_ID"

  echo "    SPN ${SPN_NAME} configured for ${ENV}"
done

echo ""
echo "IMPORTANT: Store each *-spn-credentials.json in the matching GitHub repo secrets."
echo "DO NOT commit these files."
rm -f dev-spn-credentials.json test-spn-credentials.json prod-spn-credentials.json
```

---

## Step 2 — GitHub App Setup

Your workflows use a GitHub App for cross-repo operations and committing releases.

**Required GitHub App permissions:**
- `Contents: Read & Write` (all 3 repos — for checkout and tag creation)
- `Actions: Read & Write` (all 3 repos — for workflow dispatch)
- `Metadata: Read` (all 3 repos)
- `Administration: Read` (optional, for ruleset API)

**Create the app:**
```bash
# Via GitHub CLI (requires owner scope)
gh api POST /orgs/{org}/apps \
  --field name="synapse-cicd-bot" \
  --field description="Synapse CI/CD cross-repo automation"

# Or create manually at: https://github.com/organizations/{org}/settings/apps
```

After creating:
1. Note the **App ID** (numeric) → store as `GH_APP_ID` variable
2. Generate a **Private Key** → store as `GH_APP_PRIVATE_KEY` secret
3. Install the app on all 3 repositories

---

## Step 3 — Terraform Infrastructure Deploy

```bash
cd terraform/

# Configure GitHub token (must be org owner or have repo admin scope)
export GITHUB_TOKEN="ghp_..."
export TF_VAR_github_org="your-org-name"

# Azure backend auth
export ARM_CLIENT_ID="..."
export ARM_CLIENT_SECRET="..."
export ARM_TENANT_ID="..."
export ARM_SUBSCRIPTION_ID="..."

terraform init
terraform plan -out=tfplan
terraform apply tfplan
```

This creates the 3 repos, all required branches, and all rulesets.

---

## Step 4 — GitHub Secrets and Variables

### synapse-repo-dev

| Type | Name | Value |
|------|------|-------|
| Secret | `AZURE_CLIENT_ID` | App ID of `synapse-dev-spn` |
| Secret | `AZURE_CLIENT_SECRET` | Client secret of `synapse-dev-spn` |
| Secret | `AZURE_TENANT_ID` | Azure AD tenant ID |
| Secret | `AZURE_SUBSCRIPTION_ID` | Azure subscription ID |
| Secret | `GH_APP_PRIVATE_KEY` | GitHub App private key (PEM) |
| Variable | `GH_APP_ID` | GitHub App numeric ID |
| Variable | `SYNAPSE_WORKSPACE_DEV` | `synapse-workspace-dev` |
| Variable | `RESOURCE_GROUP_DEV` | `rg-synapse-dev` |

### synapse-repo-test

| Type | Name | Value |
|------|------|-------|
| Secret | `AZURE_CLIENT_ID` | App ID of `synapse-test-spn` |
| Secret | `AZURE_CLIENT_SECRET` | Client secret of `synapse-test-spn` |
| Secret | `AZURE_TENANT_ID` | Azure AD tenant ID |
| Secret | `AZURE_SUBSCRIPTION_ID` | Azure subscription ID |
| Secret | `GH_APP_PRIVATE_KEY` | GitHub App private key (PEM) |
| Variable | `GH_APP_ID` | GitHub App numeric ID |
| Variable | `SYNAPSE_WORKSPACE_TEST` | `synapse-workspace-test` |
| Variable | `RESOURCE_GROUP_TEST` | `rg-synapse-test` |
| Variable | `DEV_REPO_NAME` | `synapse-repo-dev` |

### synapse-repo-prod

| Type | Name | Value |
|------|------|-------|
| Secret | `AZURE_CLIENT_ID` | App ID of `synapse-prod-spn` |
| Secret | `AZURE_CLIENT_SECRET` | Client secret of `synapse-prod-spn` |
| Secret | `AZURE_TENANT_ID` | Azure AD tenant ID |
| Secret | `AZURE_SUBSCRIPTION_ID` | Azure subscription ID |
| Secret | `GH_APP_PRIVATE_KEY` | GitHub App private key (PEM) |
| Variable | `GH_APP_ID` | GitHub App numeric ID |
| Variable | `SYNAPSE_WORKSPACE_PROD` | `synapse-workspace-prod` |
| Variable | `RESOURCE_GROUP_PROD` | `rg-synapse-prod` |
| Variable | `DEV_REPO_NAME` | `synapse-repo-dev` |

**Set secrets via CLI:**
```bash
# Example for synapse-repo-dev
gh secret set AZURE_CLIENT_ID     --repo myorg/synapse-repo-dev --body "$(cat dev-spn.json | jq -r .clientId)"
gh secret set AZURE_CLIENT_SECRET --repo myorg/synapse-repo-dev --body "$(cat dev-spn.json | jq -r .clientSecret)"
gh secret set AZURE_TENANT_ID     --repo myorg/synapse-repo-dev --body "$(cat dev-spn.json | jq -r .tenantId)"
gh secret set AZURE_SUBSCRIPTION_ID --repo myorg/synapse-repo-dev --body "$(cat dev-spn.json | jq -r .subscriptionId)"
gh secret set GH_APP_PRIVATE_KEY  --repo myorg/synapse-repo-dev --body "$(cat synapse-cicd-bot.private-key.pem)"

gh variable set GH_APP_ID              --repo myorg/synapse-repo-dev --body "123456"
gh variable set SYNAPSE_WORKSPACE_DEV  --repo myorg/synapse-repo-dev --body "synapse-workspace-dev"
gh variable set RESOURCE_GROUP_DEV     --repo myorg/synapse-repo-dev --body "rg-synapse-dev"
```

---

## Step 5 — GitHub Environments (for approval gates)

Create these in each repo's Settings → Environments:

### synapse-repo-prod → environment: `synapse-prod`
- Required reviewers: add `db-ops` team
- Deployment branch rule: `prod` branch only

### synapse-repo-test → environment: `synapse-test`
- Optional reviewers (test is usually unattended)
- Deployment branch rule: `test` branch only

### synapse-repo-dev → environment: `synapse-dev`
- No required reviewers (auto-deploys on push to dev)

```bash
# Via API (requires admin scope on repo)
gh api --method PUT \
  repos/myorg/synapse-repo-prod/environments/synapse-prod \
  --field "wait_timer=0" \
  --field "reviewers=[{\"type\":\"Team\",\"id\":$(gh api orgs/myorg/teams/db-ops --jq .id)}]" \
  --field "deployment_branch_policy={\"protected_branches\":false,\"custom_branch_policies\":true}"
```

---

## Step 6 — Synapse Studio Git Configuration

Configure each workspace in Synapse Studio → Manage → Git configuration:

| Workspace | Repository | Collaboration Branch | Publish Branch | Root Folder |
|---|---|---|---|---|
| `synapse-workspace-dev` | `synapse-repo-dev` | `dev` | `publish_branch` | `/` |
| `synapse-workspace-test` | `synapse-repo-test` | `test` | `publish_branch` | `/` |
| `synapse-workspace-prod` | `synapse-repo-prod` | `prod` | `publish_branch` | `/` |

---

## Developer Workflow (step-by-step)

```bash
# ── STEP 1: Create feature branch from prod ────────────────────────────────
git clone git@github.com:myorg/synapse-repo-dev.git
cd synapse-repo-dev
git checkout prod
git pull origin prod
git checkout -b feature/add-sales-pipeline

# ── STEP 2: Work in Synapse Studio ────────────────────────────────────────
# Synapse Studio (dev workspace) shows your feature branch.
# Create/edit pipelines, datasets, linked services, notebooks.
# Commit changes via Synapse Studio UI (not git directly).

# ── STEP 3: PR feature → dev ───────────────────────────────────────────────
gh pr create \
  --base dev \
  --head feature/add-sales-pipeline \
  --title "Add sales aggregation pipeline" \
  --body "Adds daily sales roll-up from raw to curated zone."
# → Get 2 approvals from db-ops team (no required status check on dev)
# → Merge

# ── AUTO: push to dev triggers synapse-dev-cicd-workflow ──────────────────
# → validate job: validates artifacts against synapse-workspace-dev
# → deploy job: validateDeploy to synapse-workspace-dev
# → creates release: dev-2026.04.14.42

# ── STEP 4: PR dev → test ─────────────────────────────────────────────────
gh pr create \
  --repo myorg/synapse-repo-dev \
  --base test \
  --head dev \
  --title "Promote dev → test [dev-2026.04.14.42]" \
  --body "Promoting dev release dev-2026.04.14.42 to test."
# → synapse-dev-cicd-workflow/validate runs automatically as PR check
# → Get 2 approvals from db-ops team
# → Merge

# ── AUTO: push to test triggers dispatch-test-cicd.yml ───────────────────
# → dispatches synapse-test-cicd-workflow in synapse-repo-test
# → clones synapse-repo-dev@<sha>
# → validateDeploy to synapse-workspace-test
# → creates release: test-2026.04.14.43

# ── STEP 5: PR test → prod (both in synapse-repo-dev) ────────────────────
gh pr create \
  --repo myorg/synapse-repo-dev \
  --base prod \
  --head test \
  --title "Promote test → prod [test-2026.04.14.43]" \
  --body "Promoting test release test-2026.04.14.43 to production."
# → Get 2 approvals from db-ops team (no required status check on prod)
# → Merge

# ── AUTO: push to prod triggers dispatch-prod-cicd.yml ───────────────────
# → dispatches synapse-prod-cicd-workflow in synapse-repo-prod
# → waits for synapse-prod environment approval gate
# → clones synapse-repo-dev@<sha>
# → validateDeploy to synapse-workspace-prod
# → creates release: prod-2026.04.14.44
```

---

## Release Naming Convention

```
<env>-YYYY.MM.DD.<run_number>

dev-2026.04.14.42    # 42nd workflow run on 2026-04-14 in synapse-repo-dev
test-2026.04.14.43   # 43rd workflow run in synapse-repo-test
prod-2026.04.14.44   # 44th workflow run in synapse-repo-prod
```

Each release includes:
- Git tag on the target repo
- GitHub Release with deployment metadata (source SHA, triggering actor, workflow run link)
- Exported ARM templates as release artifacts (90-day retention for prod)

---

## Ruleset Status Check Names (must match exactly)

These are the exact strings to configure in GitHub Rulesets for required status checks:

| Repo | Branch | Required Check |
|---|---|---|
| `synapse-repo-dev` | `test` | `synapse-dev-cicd-workflow / validate` |
| `synapse-repo-test` | `main` | `synapse-test-cicd-workflow / validate` |
| `synapse-repo-prod` | `main` | `synapse-prod-cicd-workflow / validate` |

The format is `<workflow-name> / <job-name>`. The workflow `name:` field and job `name:` field in the YAML must match exactly.

---

## Rollback

To redeploy any prior release to test or prod:

```bash
# List available releases
gh release list --repo myorg/synapse-repo-test --limit 20
gh release list --repo myorg/synapse-repo-prod --limit 20

# Manual rollback: dispatch the workflow with a prior SHA
# Get the source SHA from the release notes, then:
gh workflow run synapse-test-cicd-workflow.yml \
  --repo myorg/synapse-repo-test \
  --field source_sha="<sha-from-release-notes>" \
  --field source_branch="test" \
  --field dev_release="rollback-to-test-2026.04.12.31" \
  --field triggered_by="$(gh api user --jq .login)"
```

---

## Key Design Decisions

**Why `validateDeploy` instead of `deploy` from `publish_branch`?**
`validateDeploy` reads the raw JSON artifact files directly from the checked-out source, eliminating the manual "Publish" step in Synapse Studio. The test and prod deployments pull artifacts from `synapse-repo-dev` at an exact commit SHA, ensuring identical code across environments.

**Why separate repos for test and prod?**
Isolated repositories give each environment its own: RBAC (who can see/run workflows), secrets scope, release history, branch protections, and audit trail. A compromise in the dev repo cannot propagate credentials from prod.

**Why a GitHub App instead of PAT?**
GitHub Apps generate scoped installation tokens with automatic expiry (1 hour). Cross-repo operations (`workflow_dispatch` to other repos, checking out private repos) require org-level token scope that a per-repo `GITHUB_TOKEN` cannot provide.

**Why stop/start triggers before every deployment?**
The Synapse API returns `TriggerEnabledCannotUpdate` if any artifact a running trigger references is being modified. This is the single most common cause of Synapse CI/CD failures. The `toggle-triggers.sh` script captures which triggers were running, stops them with state verification, and restarts exactly those triggers post-deployment with retry logic.
