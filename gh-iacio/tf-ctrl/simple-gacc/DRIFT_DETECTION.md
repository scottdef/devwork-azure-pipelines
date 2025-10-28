# Organization Settings Drift Detection - Auto-Remediation

Automated hourly drift detection with **immediate auto-remediation**. Simple, direct, effective.

## Overview

This workflow:
1. ✅ Runs `terraform plan` (hourly)
2. ✅ Detects drift in organization_settings
3. ✅ Opens GitHub issue with plan output
4. ✅ **Auto-remediates immediately** via `terraform apply`
5. ✅ Comments on issue with success/failure

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│              Simple Auto-Remediation Flow                   │
└─────────────────────────────────────────────────────────────┘

   ┌──────────────┐
   │  Cron: Hourly   │
   └───────┬──────┘
           │
           ▼
   ┌──────────────────┐
   │  terraform plan  │  Exit code 2 = changes detected
   └───────┬──────────┘
           │
           ├─── No drift ──→ Close any open drift issues ──→ ✓ Done
           │
           ├─── Drift in other resources ──→ ✓ Ignore
           │
           ▼
   ┌────────────────────────┐
   │  Create GitHub Issue   │  With terraform plan output
   └───────┬────────────────┘
           │
           ▼
   ┌─────────────────────────┐
   │  terraform apply -auto  │  Immediate remediation
   └───────┬─────────────────┘
           │
           ├─── Success (exit 0) ──┐
           │                       │
           ├─── Failure (exit ≠ 0) ─┤
           │                        │
           ▼                        ▼
   ┌──────────────────┐    ┌──────────────────┐
   │  Comment: ✅     │    │  Comment: ❌     │
   │  Add label:      │    │  Add labels:     │
   │  remediation-    │    │  remediation-    │
   │  success         │    │  failed          │
   │                  │    │  priority-high   │
   └──────────────────┘    └──────────────────┘
```

## Why This Approach

**Simple > Complex**
- No data source comparison needed
- No Python scripts
- Just terraform plan → apply
- Ken Thompson would approve

**Self-Healing**
- Drift detected → Immediately fixed
- No manual intervention needed
- GitHub is always in sync
- Audit trail via issues

**Transparent**
- Full plan output in issue
- Full apply output in comment
- Clear success/failure status
- Labels for filtering

## Setup (2 Minutes)

### 1. Add Workflow File

```bash
mkdir -p .github/workflows
cp drift-detection.yml .github/workflows/
```

### 2. Add Secret

Repository Settings → Secrets and variables → Actions → New secret

**Name**: `GH_ORG_ADMIN_TOKEN`
**Value**: Your GitHub token with `admin:org` + `repo` scopes

### 3. Enable Permissions

Repository Settings → Actions → General → Workflow permissions:
- ✓ Read and write permissions
- ✓ Allow GitHub Actions to create and approve pull requests

### 4. Test

```bash
# Via GitHub CLI
gh workflow run drift-detection.yml

# Or via UI
# Actions → Organization Settings Drift Detection → Run workflow
```

## Usage

### Automatic Operation

**Schedule**: Hourly at minute 0 (`0 * * * *`)

**What happens**:
1. Workflow runs every hour
2. If drift detected → Issue created + auto-remediation
3. If no drift → Any open drift issues are closed

### Manual Triggers

**Normal run (with auto-remediation)**:
```bash
gh workflow run drift-detection.yml
```

**Plan-only (skip remediation)**:
```bash
gh workflow run drift-detection.yml -f skip_remediation=true
```

### Issue Format

**When drift detected, issue contains**:
- 🚨 Alert header
- 📊 Summary (when/where/what)
- 🔍 Full terraform plan output (collapsible)
- 🔧 Status: "Auto-remediation in progress..."

**After apply runs, comment added**:
- ✅ **Success**: Apply output, verification steps
- ❌ **Failure**: Error output, manual remediation steps

## Example Issue

### Initial Issue (Drift Detected)

```markdown
# 🚨 Organization Settings Drift Detected

**Organization**: `example-org`
**Detection Time**: Mon, 28 Oct 2024 14:00:00 GMT
**Workflow Run**: [View Details](...)

---

## 📊 Drift Summary

Terraform detected drift in organization settings.

**Auto-remediation**: ⏳ In progress...

---

## 🔍 Terraform Plan Output

<details>
<summary>Click to expand</summary>

```hcl
Terraform will perform the following actions:

  # github_organization_settings.main will be updated in-place
  ~ resource "github_organization_settings" "main" {
      ~ default_repository_permission = "write" -> "read"
      ~ members_can_create_repositories = true -> false
    }

Plan: 0 to add, 1 to change, 0 to destroy.
```

</details>
```

### Success Comment

```markdown
## ✅ Auto-Remediation Successful

**Completed**: Mon, 28 Oct 2024 14:01:23 GMT
**Status**: Success

Drift remediated. Organization settings now match Terraform config.

### 📋 Apply Output

<details>
<summary>Click to expand</summary>

```
github_organization_settings.main: Modifying...
github_organization_settings.main: Modifications complete

Apply complete! Resources: 0 added, 1 changed, 0 destroyed.
```

</details>

### ✅ Next Steps

- Drift resolved automatically
- Settings match terraform.tfvars
- Close this issue if confirmed

**Labels**: `remediation-success`
```

### Failure Comment

```markdown
## ❌ Auto-Remediation Failed

**Completed**: Mon, 28 Oct 2024 14:01:23 GMT
**Status**: Failure
**Exit Code**: 1

Auto-remediation failed. Manual intervention required.

### 📋 Apply Output

<details>
<summary>Click to expand</summary>

```
Error: Error updating organization settings: 403 Forbidden
Token lacks admin:org permissions
```

</details>

### 🔧 Manual Remediation Required

1. Review error above
2. Fix the issue (e.g., update token permissions)
3. Run locally:
   ```bash
   make plan
   make apply
   ```

**Labels**: `remediation-failed`, `priority-high`
```

## Monitored Settings

Same 27 settings as before:
- Profile (8): billing_email, company, blog, etc.
- Permissions (6): default_permission, creation rights
- Security (6): scanning, Dependabot, Advanced Security
- Pages (3): creation permissions
- Projects (2): org/repo projects
- Other (2): forks, signoff

## Configuration

### Change Schedule

Edit workflow file:

```yaml
on:
  schedule:
    # Every 6 hours
    - cron: '0 */6 * * *'
    
    # Daily at 9 AM UTC
    - cron: '0 9 * * *'
    
    # Every 30 minutes (aggressive)
    - cron: '*/30 * * * *'
```

### Disable Auto-Remediation

**Globally**: Remove the "Terraform Apply" step from workflow

**Per-run**: Use manual trigger with flag:
```bash
gh workflow run drift-detection.yml -f skip_remediation=true
```

### Change Terraform Version

```yaml
env:
  TF_VERSION: '1.6.0'
```

## Troubleshooting

### Workflow Not Running

```bash
# Check schedule
gh workflow view drift-detection.yml

# View recent runs
gh run list --workflow=drift-detection.yml

# Enable if disabled
gh workflow enable drift-detection.yml
```

### Authentication Errors

**Error**: `403 Forbidden` or `404 Not Found`

**Fix**:
1. Verify `GH_ORG_ADMIN_TOKEN` is set
2. Check token has `admin:org` scope
3. Verify token hasn't expired
4. Test token locally:
```bash
curl -H "Authorization: token $GITHUB_TOKEN" \
  https://api.github.com/orgs/YOUR_ORG
```

### Apply Failures

**Error**: `terraform apply` failed

**Debug**:
1. Check apply output in issue comment
2. Look for common issues:
   - Token permissions
   - API rate limits
   - Invalid configuration
   - Network issues

**Fix**:
```bash
# Run locally to debug
export GITHUB_TOKEN="your-token"
export GITHUB_OWNER="your-org"
make plan
make apply
```

### Issue Not Created

**Error**: No issue created despite drift

**Check**:
1. Workflow permissions (Settings → Actions → General)
2. Verify drift in org_settings specifically:
```bash
terraform plan | grep github_organization_settings
```

## Security Considerations

### Auto-Remediation Risk

**⚠️ Warning**: Auto-remediation applies changes without human review

**Mitigations**:
- All changes are logged in GitHub issues
- Full plan/apply output captured
- Can be disabled with `skip_remediation` flag
- Terraform state provides rollback capability

### When NOT to Auto-Remediate

Consider disabling auto-remediation if:
- Organization has frequent intentional manual changes
- Multiple admins managing settings
- Testing new configurations
- High-risk environment

### Safe Auto-Remediation

Auto-remediation is safe when:
- ✓ Terraform is single source of truth
- ✓ All changes go through IaC
- ✓ GitHub org settings rarely changed manually
- ✓ Team monitors drift issues

### Audit Trail

Every remediation creates:
- GitHub issue (permanent record)
- Workflow run (detailed logs)
- Issue comments (apply output)
- Labels (success/failure status)

## Best Practices

1. **Start Conservative**: Use `skip_remediation=true` for first week
2. **Monitor Issues**: Review every auto-remediation for first month
3. **Investigate Drift**: Why did it drift? Fix root cause
4. **Document Exceptions**: If manual changes needed, update terraform.tfvars first
5. **Review Labels**: Filter by `remediation-failed` for problems

## Advanced Usage

### Slack Notifications

Add after "Create Drift Issue" step:

```yaml
- name: Notify Slack
  if: steps.check_drift.outputs.drift_detected == 'true'
  uses: slackapi/slack-github-action@v1.24.0
  with:
    payload: |
      {
        "text": "🚨 Drift detected in GitHub org settings - Auto-remediation started",
        "blocks": [{
          "type": "section",
          "text": {
            "type": "mrkdwn",
            "text": "Drift detected in *${{ github.repository_owner }}* organization settings\n\nIssue: <issue_url|View Issue>\nWorkflow: <${{ github.server_url }}/${{ github.repository }}/actions/runs/${{ github.run_id }}|View Run>"
          }
        }]
      }
  env:
    SLACK_WEBHOOK_URL: ${{ secrets.SLACK_WEBHOOK }}
```

### PagerDuty Integration

Add after "Comment on Issue - Failure" step:

```yaml
- name: Alert PagerDuty
  if: steps.apply.outputs.exitcode != '0'
  uses: PagerDuty/pagerduty-change-events-action@v2
  with:
    integration-key: ${{ secrets.PAGERDUTY_INTEGRATION_KEY }}
    summary: 'GitHub Org Settings Auto-Remediation Failed'
    severity: 'critical'
```

### Custom Labels

Modify labels in issue creation:

```yaml
labels: [
  'drift-detection',
  'organization-settings',
  'terraform',
  'auto-remediation',
  'team-platform',      # Your team
  'environment-prod'    # Your environment
]
```

## Comparison: Old vs New Workflow

| Feature | Old Workflow | New Workflow |
|---------|--------------|--------------|
| Complexity | 817 lines | 390 lines |
| Dependencies | Python, jq | None |
| Data Source | Yes | No |
| Comparison | 27 field deep dive | terraform plan |
| Remediation | Manual | Automatic |
| Speed | ~3 min | ~1 min |
| Output | Markdown report | Plan + Apply |

**Philosophy**: Simple > Complex. Direct > Clever.

## Monitoring

### View All Drift Issues

```bash
gh issue list --label drift-detection
```

### View Failed Remediations

```bash
gh issue list --label remediation-failed
```

### Check Success Rate

```bash
gh issue list --label remediation-success --state closed --json number | jq 'length'
gh issue list --label remediation-failed --state open --json number | jq 'length'
```

### Workflow Run History

```bash
# All runs
gh run list --workflow=drift-detection.yml

# Failed runs
gh run list --workflow=drift-detection.yml --status failure

# Recent run details
gh run view --log
```

## FAQ

**Q: Will this apply changes I don't want?**
A: Only if they're in your terraform.tfvars. The apply uses the plan that was generated from your configuration.

**Q: What if I make manual changes in GitHub UI?**
A: They'll be reverted within an hour. Update terraform.tfvars instead.

**Q: Can I review before applying?**
A: Check the issue before auto-remediation completes (~30 seconds). Or use `skip_remediation=true`.

**Q: What if apply fails?**
A: Workflow adds failure comment with details. Issue gets `remediation-failed` label.

**Q: How do I rollback?**
A: Update terraform.tfvars to previous values, run `make apply`.

**Q: Does this work for other resources?**
A: Workflow only triggers on `github_organization_settings.main` drift.

## License

MIT - Simple, like everything should be.

---

*"Simplicity is the ultimate sophistication."* - Leonardo da Vinci

*"Controlling complexity is the essence of computer programming."* - Brian Kernighan

**This is 50% less code doing 100% more work.**
