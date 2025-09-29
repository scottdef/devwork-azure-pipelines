# IssueOps Integration Guide

How IssueOps integrates with the GitHub Governance Terraform system.

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                         GitHub Issues                            │
│                     (User Interface Layer)                       │
└────────────────────────┬────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Approval Workflow                             │
│              (issueops-approval.yml)                             │
│  • Tracks reactions/comments                                     │
│  • Validates platform-team membership                            │
│  • Counts approvals                                              │
│  • Triggers automation                                           │
└────────────────────────┬────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────────┐
│                 Create Repo Workflow                             │
│           (issueops-create-repo.yml)                             │
│  • Parses issue form                                             │
│  • Modifies YAML files                                           │
│  • Creates branch                                                │
│  • Opens PR                                                      │
└────────────────────────┬────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────────┐
│                      Pull Request                                │
│                 (Existing PR Workflow)                           │
│  • Terraform Plan runs                                           │
│  • Shows what will be created                                    │
│  • Requires 2 reviewers                                          │
└────────────────────────┬────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Terraform Apply                               │
│             (terraform-apply.yml)                                │
│  • Creates actual GitHub repository                              │
│  • Sets up branch protection                                     │
│  • Creates CODEOWNERS                                            │
│  • Configures teams                                              │
└─────────────────────────────────────────────────────────────────┘
```

## Data Flow

### 1. Issue Creation

User creates issue via template:

```yaml
# .github/ISSUE_TEMPLATE/create-repo.yml
→ Structured form with validation
→ Auto-labels: issueops, create-repo, pending-approval
→ Assigns: platform-team-lead
```

### 2. Approval Tracking

Workflow watches for:

```
issue_comment.created → Check if /approve or approved
issues.labeled → Update status
```

Logic:
1. Extract commenter username
2. Query GitHub API for platform-team members
3. Validate commenter is in team
4. Count unique approvals
5. Update issue with status
6. Trigger automation at threshold

### 3. Automation

Workflow dispatch with issue number:

```yaml
workflow_dispatch:
  inputs:
    issue_number: "123"
```

Steps:
1. Fetch issue via GitHub API
2. Verify `approved` label exists
3. Parse issue body (GitHub form format)
4. Validate data
5. Update YAML files
6. Create workflow file
7. Commit to branch
8. Push
9. Create PR via API
10. Comment back on issue

### 4. YAML Updates

Python script modifies files:

```python
# repo-yamls/platform-team-repos.yml
repos:
  - name: new-repo      # Added by automation
    team: platform-team
    description: "..."
```

Validation:
- Check for duplicates
- Verify team exists
- Validate repo name format
- Run full YAML validation

### 5. PR Creation

Automated PR includes:
- Clear title: `[IssueOps] Add repository: {name}`
- Detailed body with all metadata
- Links back to original issue
- `Closes #123` for auto-close

### 6. Review Process

Existing workflows handle:
- Terraform plan (auto-comment on PR)
- Status checks
- Required reviewers (2)
- Merge restrictions

### 7. Terraform Apply

On merge to main:
- Workflow triggers
- Terraform applies changes
- Repository created with:
  - Correct visibility
  - Team access configured
  - Branch protection rules
  - CODEOWNERS file
  - Default settings

## File Locations

```
github-governance/
├── .github/
│   ├── ISSUE_TEMPLATE/
│   │   └── create-repo.yml           # Issue form
│   ├── workflows/
│   │   ├── issueops-approval.yml     # Approval tracking
│   │   ├── issueops-create-repo.yml  # Repository creation
│   │   ├── issueops-cleanup.yml      # Cleanup closed issues
│   │   ├── terraform-plan.yml        # Existing
│   │   └── terraform-apply.yml       # Existing
│   └── issueops-config.yml           # Configuration
├── scripts/
│   ├── yaml-helpers.py               # YAML manipulation
│   ├── validate-yaml.py              # Existing
│   └── test-issueops.sh              # Validation
├── templates/
│   └── ci-workflow-template.yml      # Workflow template
├── repo-yamls/                       # Existing
├── team-yamls/                       # Existing
├── terraform/                        # Existing
├── ISSUEOPS.md                       # Documentation
├── ISSUEOPS-QUICKSTART.md            # Quick start
└── ISSUEOPS-INTEGRATION.md           # This file
```

## Integration Points

### With Existing Terraform

IssueOps modifies the same YAML files that Terraform reads:

```hcl
# terraform/main.tf
locals {
  repo_yaml_files = fileset(path.module, "../repo-yamls/*.yml")
  # ↑ IssueOps adds repos here
  
  repo_configs = flatten([
    for f in local.repo_yaml_files : [
      for repo in yamldecode(file("${path.module}/${f}")).repos : repo
    ]
  ])
}
```

No Terraform changes needed. It's transparent.

### With Existing Workflows

IssueOps creates PRs that trigger:

```yaml
# .github/workflows/terraform-plan.yml
on:
  pull_request:
    branches: [main]
    paths:
      - 'repo-yamls/**'  # ← Triggered by IssueOps PRs
```

Same review process. Same Terraform plan. Same merge flow.

### With Team Management

IssueOps respects team structure:

```yaml
# Issue template validates against existing teams
teams:
  - platform-team    # Must exist in team-yamls/
  - data-team
  - engineering
```

Terraform creates subteams automatically:
- `{team}-admins` → Repo admins
- `{team}-workflow-admins` → CI/CD
- `{team}-pr-approvers` → Code review

## Security Model

### Authentication

```
User → GitHub OAuth → Issue Creation
Platform Team → GitHub OAuth → Approval
Workflow → GH_ADMIN_TOKEN → API Calls
Workflow → GITHUB_TOKEN → PR Comments
```

### Authorization

```
Issue Creation: Any org member
Approval: platform-team members only
PR Merge: Requires 2 reviewers
Terraform Apply: Automatic on merge
```

### Audit Trail

Complete history in:
1. Issue comments (approval tracking)
2. Git commits (YAML changes)
3. PR (review process)
4. Workflow logs (automation)
5. Terraform state (infrastructure)

## Failure Modes

### Issue Closed Before Automation

```
issueops-cleanup.yml → Deletes branch → Comments on issue
```

### Automation Fails

```
issueops-create-repo.yml → Comments error on issue → Adds error label
```

### PR Not Merged

```
Repository never created → Issue stays open → Can restart workflow
```

### Terraform Fails

```
Shown in PR comments → Fix YAML → Push to same branch → Plan reruns
```

## Customization Points

### Add New Operations

1. **Create issue template**:
```yaml
# .github/ISSUE_TEMPLATE/delete-repo.yml
name: Delete Repository
labels: ["issueops", "delete-repo", "pending-approval"]
```

2. **Create workflow**:
```yaml
# .github/workflows/issueops-delete-repo.yml
on:
  workflow_dispatch:
    inputs:
      issue_number: ...
```

3. **Update approval workflow**:
```yaml
# Check label and trigger appropriate workflow
if labels.includes('delete-repo'):
  trigger('issueops-delete-repo.yml')
```

### Change Approval Requirements

Edit `.github/issueops-config.yml`:

```yaml
approvals_required: 3  # Was 2
approver_teams:
  - platform-team
  - security-team      # Add another team
```

Update workflow to check multiple teams.

### Add Notifications

In `issueops-create-repo.yml`:

```yaml
- name: Notify Slack
  run: |
    curl -X POST ${{ secrets.SLACK_WEBHOOK }} \
      -d '{"text": "New repo request approved: ${{ github.event.issue.title }}"}'
```

### Custom Validation

In `scripts/yaml-helpers.py`:

```python
def validate_repo_name(name: str) -> bool:
    # Custom validation logic
    if not name.endswith('-api'):
        raise ValueError("Repos must end with -api")
    return True
```

## Testing

### Local Testing

```bash
# Validate setup
./scripts/test-issueops.sh

# Test YAML helpers
python3 scripts/yaml-helpers.py list-repos

# Validate issue template
python3 -c "import yaml; print(yaml.safe_load(open('.github/ISSUE_TEMPLATE/create-repo.yml')))"
```

### Integration Testing

```bash
# Create test issue
gh issue create --title "[TEST] Test Repo" \
  --label issueops,create-repo \
  --body "Testing IssueOps"

# Manually approve
gh issue comment 123 --body "/approve"

# Trigger workflow manually
gh workflow run issueops-create-repo.yml -f issue_number=123

# Check status
gh run list --workflow issueops-create-repo.yml
```

### End-to-End Testing

1. Create real issue via web UI
2. Have 2 platform-team members approve
3. Watch automation run
4. Review PR
5. Merge
6. Verify repository created

## Monitoring

### Key Metrics

```bash
# Approval time (creation to approved)
gh issue list --label approved --json createdAt,updatedAt

# Processing time (approved to PR created)
gh issue list --label pr-created --json updatedAt

# Success rate
gh run list --workflow issueops-create-repo.yml --json conclusion

# Most common requesters
gh issue list --label issueops --json author
```

### Alerts

Set up GitHub Actions notifications for:
- Workflow failures
- Long-running approvals (>24h)
- High request volume

### Dashboards

Create dashboard showing:
- Pending approvals
- Active requests
- Success/failure rates
- Average processing time

## Best Practices

### For Platform Team

1. **Respond quickly**: Check issues daily
2. **Ask questions**: Comment if unclear
3. **Validate thoroughly**: Check naming, team assignment
4. **Document decisions**: Comment why approved/rejected

### For Users

1. **Complete forms fully**: Provide good descriptions
2. **Check duplicates**: Search before requesting
3. **Follow naming**: Use consistent patterns
4. **Be patient**: Allow time for review

### For Maintainers

1. **Monitor workflow logs**: Watch for patterns
2. **Update templates**: Based on common mistakes
3. **Improve validation**: Add checks for common errors
4. **Document processes**: Keep guides current

## Troubleshooting

See [ISSUEOPS.md](ISSUEOPS.md) for detailed troubleshooting.

Quick checklist:
- [ ] Secrets configured
- [ ] platform-team exists
- [ ] Workflows enabled
- [ ] Issue templates visible
- [ ] Scripts executable
- [ ] Python dependencies installed

## Future Enhancements

Potential additions:
- [ ] More operation types (delete, archive, update)
- [ ] Scheduled cleanup of stale requests
- [ ] Auto-approval for specific users/teams
- [ ] Integration with ticketing systems
- [ ] Metrics dashboard
- [ ] Slack bot integration
- [ ] Email notifications
- [ ] Audit log export

---

**Integration Complete**: IssueOps is now the frontend for your Terraform-based GitHub governance. Issues in, infrastructure out.
