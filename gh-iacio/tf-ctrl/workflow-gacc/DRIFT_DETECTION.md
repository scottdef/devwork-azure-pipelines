# Organization Settings Drift Detection Workflow

Automated hourly detection and reporting of configuration drift between Terraform-managed organization settings and actual GitHub organization state.

## Overview

This workflow:
1. ✅ Runs hourly via cron schedule
2. ✅ Detects changes via `terraform plan`
3. ✅ Fetches current GitHub state via data source
4. ✅ Performs deep argument-by-argument comparison
5. ✅ Creates detailed GitHub issue with drift report
6. ✅ Auto-closes issue when drift is resolved

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                    Drift Detection Flow                     │
└─────────────────────────────────────────────────────────────┘

   ┌──────────────┐
   │  Cron: 0 * * * *  │  (Hourly at minute 0)
   └───────┬──────┘
           │
           ▼
   ┌──────────────┐
   │ Terraform Init  │
   └───────┬──────┘
           │
           ▼
   ┌──────────────┐
   │ Terraform Plan  │  (Detect changes)
   └───────┬──────┘
           │
           ├─── No Changes ──→ ✓ Exit (close issue if open)
           │
           ├─── Changes but not org_settings ──→ ✓ Exit
           │
           ▼
   ┌──────────────────────────────┐
   │  Add Data Source Temporarily │  (github_organization_settings)
   └───────┬──────────────────────┘
           │
           ▼
   ┌──────────────────────────────┐
   │  Terraform Refresh           │  (Fetch current GitHub state)
   └───────┬──────────────────────┘
           │
           ▼
   ┌──────────────────────────────┐
   │  Extract States              │
   │  - Resource (terraform.tfvars)│
   │  - Data Source (GitHub API)  │
   └───────┬──────────────────────┘
           │
           ▼
   ┌──────────────────────────────┐
   │  Python Comparison Script    │  (compare_drift.py)
   │  - Compare all arguments     │
   │  - Categorize differences    │
   │  - Generate markdown report  │
   └───────┬──────────────────────┘
           │
           ├─── No Drift ──→ ✓ Exit (close issue if open)
           │
           ▼
   ┌──────────────────────────────┐
   │  Create/Update GitHub Issue  │
   │  - Detailed drift report     │
   │  - Remediation instructions  │
   │  - Labels: drift-detection   │
   └──────────────────────────────┘
```

## Setup

### 1. Required Secrets

Add to repository secrets (Settings → Secrets and variables → Actions):

```
GH_ORG_ADMIN_TOKEN
```

**Token Requirements**:
- Type: Personal Access Token (classic) or Fine-grained PAT
- Scopes: `admin:org`, `repo`
- Organization: Must have admin access

**Creating the token**:
```bash
# Navigate to: https://github.com/settings/tokens/new
# Select scopes:
#   - admin:org (full)
#   - repo (full)
# Click "Generate token"
# Copy and add to repository secrets as GH_ORG_ADMIN_TOKEN
```

### 2. Workflow File Location

Place workflow file at:
```
.github/workflows/drift-detection.yml
```

### 3. Enable Actions

1. Go to repository Settings → Actions → General
2. Under "Workflow permissions":
   - Select "Read and write permissions"
   - Enable "Allow GitHub Actions to create and approve pull requests"

### 4. Test the Workflow

#### Manual Trigger (Recommended for Testing)

```bash
# Via GitHub UI:
# Actions → Organization Settings Drift Detection → Run workflow

# Via GitHub CLI:
gh workflow run "drift-detection.yml"

# With test mode (creates issue even without drift):
gh workflow run "drift-detection.yml" -f create_issue=true
```

#### Verify Setup

```bash
# Check workflow runs
gh run list --workflow=drift-detection.yml

# View specific run
gh run view <run-id> --log
```

## Usage

### Automatic Operation

Once enabled, the workflow runs hourly:
- **Schedule**: `0 * * * *` (every hour at minute 0)
- **Timezone**: UTC
- **First run**: Next hour mark after workflow is merged

### Manual Triggers

**Test without changes**:
```bash
gh workflow run drift-detection.yml -f create_issue=true
```

**Normal run**:
```bash
gh workflow run drift-detection.yml
```

### Issue Management

**When drift is detected**:
- Issue is created with title: "🚨 Organization Settings Drift Detected"
- Labels: `drift-detection`, `organization-settings`, `terraform`
- Body contains detailed drift report

**Subsequent runs**:
- If drift persists: Issue is updated with timestamp
- If drift is resolved: Issue is closed automatically

**Manual issue handling**:
```bash
# List drift issues
gh issue list --label drift-detection

# View specific issue
gh issue view <issue-number>

# Close manually if remediated
gh issue close <issue-number> --comment "Manually remediated via terraform apply"
```

## Drift Report Format

The generated issue contains:

### 📊 Drift Summary Table
Quick overview of all drifted settings

### 🔍 Detailed Analysis
Organized by category:
- Profile Information
- Repository Permissions
- Pages Settings
- Project Settings
- Security Settings
- Other Settings

Each drift shows:
```diff
- Terraform Config: expected_value
+ GitHub Actual:    actual_value
```

### 🔧 Remediation Options

**Option 1: Apply Terraform (Recommended)**
```bash
make plan
make apply
```

**Option 2: Update Terraform Config**
Shows exact `terraform.tfvars` changes needed to match GitHub

## Monitored Settings

### Profile Information (8)
- `billing_email`
- `company`
- `blog`
- `email`
- `twitter_username`
- `location`
- `name`
- `description`

### Repository Permissions (6)
- `default_repository_permission`
- `members_can_create_repositories`
- `members_can_create_public_repositories`
- `members_can_create_private_repositories`
- `members_can_create_internal_repositories`
- `members_allowed_repository_creation_type`

### Pages Settings (3)
- `members_can_create_pages`
- `members_can_create_public_pages`
- `members_can_create_private_pages`

### Project Settings (2)
- `has_organization_projects`
- `has_repository_projects`

### Security Settings (6)
- `advanced_security_enabled_for_new_repositories`
- `secret_scanning_enabled_for_new_repositories`
- `secret_scanning_push_protection_enabled_for_new_repositories`
- `dependabot_alerts_enabled_for_new_repositories`
- `dependabot_security_updates_enabled_for_new_repositories`
- `dependency_graph_enabled_for_new_repositories`

### Other Settings (2)
- `members_can_fork_private_repositories`
- `web_commit_signoff_required`

**Total: 27 settings monitored**

## Troubleshooting

### Workflow Not Running

**Check schedule**:
```bash
# View next scheduled run time
gh workflow view drift-detection.yml
```

**Verify cron syntax**:
```bash
# Test cron expression at: https://crontab.guru/#0_*_*_*_*
```

**Check workflow status**:
```bash
# Ensure workflow is enabled
gh workflow list
gh workflow enable drift-detection.yml
```

### Authentication Errors

**Error**: `Resource not found (404)`

**Solutions**:
1. Verify `GH_ORG_ADMIN_TOKEN` is set correctly
2. Ensure token has `admin:org` scope
3. Check token hasn't expired
4. Verify organization name in `GITHUB_OWNER`

**Test token locally**:
```bash
export GITHUB_TOKEN="your-token"
curl -H "Authorization: token $GITHUB_TOKEN" \
  https://api.github.com/orgs/YOUR_ORG
```

### Permission Errors

**Error**: `refused to allow a GitHub Actions workflow to create or approve pull requests`

**Solution**: Enable in Settings → Actions → General → Workflow permissions

### Python Script Errors

**Error**: `compare_drift.py failed`

**Debug locally**:
```bash
# Download workflow artifacts
gh run download <run-id>

# Run script locally
python3 compare_drift.py

# Check JSON files
cat resource_state.json | jq .
cat datasource_state.json | jq .
```

### Issue Creation Failures

**Error**: `Could not create issue`

**Solutions**:
1. Check repository permissions (Actions needs `issues: write`)
2. Verify labels exist or workflow can create them
3. Check GitHub rate limits:
```bash
curl -H "Authorization: token $GITHUB_TOKEN" \
  https://api.github.com/rate_limit
```

## Configuration

### Change Schedule

Edit cron expression in workflow:

```yaml
on:
  schedule:
    # Run every 6 hours
    - cron: '0 */6 * * *'
    
    # Run daily at 9 AM UTC
    - cron: '0 9 * * *'
    
    # Run every 30 minutes
    - cron: '*/30 * * * *'
```

### Customize Working Directory

If Terraform files are in subdirectory:

```yaml
env:
  WORKING_DIR: 'terraform/github-org'
```

### Change Terraform Version

```yaml
env:
  TF_VERSION: '1.6.0'
```

### Disable Auto-Close

Remove or comment out the "Close Drift Issue" step if you want manual resolution only.

## Best Practices

1. **Start with daily runs**: Use `0 9 * * *` initially, increase to hourly after validation
2. **Monitor token expiration**: Set calendar reminders to rotate tokens
3. **Review issues promptly**: Drift indicates manual changes that bypass IaC
4. **Use manual triggers**: Test workflow after changes with `-f create_issue=true`
5. **Keep state in sync**: Regular `terraform refresh` prevents false positives
6. **Document exceptions**: If manual changes are intentional, update terraform.tfvars

## Advanced Usage

### Integration with Slack

Add notification step:

```yaml
- name: Notify Slack
  if: steps.compare.outputs.has_drift == 'true'
  uses: slackapi/slack-github-action@v1.24.0
  with:
    payload: |
      {
        "text": "🚨 GitHub Org Drift Detected - See issue #${{ steps.create_issue.outputs.issue-number }}"
      }
  env:
    SLACK_WEBHOOK_URL: ${{ secrets.SLACK_WEBHOOK }}
```

### Auto-Remediation (Use with Caution)

Add auto-apply step:

```yaml
- name: Auto-Remediate Drift
  if: steps.compare.outputs.has_drift == 'true' && github.ref == 'refs/heads/main'
  working-directory: ${{ env.WORKING_DIR }}
  env:
    GITHUB_TOKEN: ${{ secrets.GH_ORG_ADMIN_TOKEN }}
    GITHUB_OWNER: ${{ github.repository_owner }}
  run: |
    terraform apply -auto-approve
```

**⚠️ Warning**: Only enable auto-remediation after extensive testing

## Monitoring

### Track Drift Frequency

```bash
# Count drift issues created
gh issue list --label drift-detection --state all --json number | jq 'length'

# View drift history
gh issue list --label drift-detection --state all --json number,title,createdAt
```

### Workflow Health

```bash
# Check success rate
gh run list --workflow=drift-detection.yml --json conclusion | \
  jq 'group_by(.conclusion) | map({conclusion: .[0].conclusion, count: length})'

# Recent failures
gh run list --workflow=drift-detection.yml --json conclusion,createdAt | \
  jq '.[] | select(.conclusion == "failure")'
```

## Security Considerations

1. **Token Security**: Never commit `GH_ORG_ADMIN_TOKEN` to repository
2. **Least Privilege**: Token only needs `admin:org` + `repo`, not `admin:enterprise`
3. **Audit Logs**: Review GitHub audit log for unexpected changes
4. **State Files**: Workflow never commits state files
5. **Temporary Files**: All artifacts cleaned up after run

## License

MIT - Do whatever you want. No warranty.

---

*Built with the precision of Plan 9, the reliability of Unix, and the simplicity of Go.*
