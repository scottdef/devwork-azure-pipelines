# IssueOps - Infrastructure via GitHub Issues

> *"The best interface is no interface."* - Adapted for IssueOps

IssueOps brings infrastructure management into GitHub Issues. No CLI, no complex YAML, just fill out a form. The system handles approvals, automation, and PRs.

## Philosophy

**GitOps meets ChatOps meets Unix**

- Issues are the interface
- Workflows are the engine
- Git is the source of truth
- Everything is auditable
- Approvals are built-in

## How It Works

```
User creates issue → Platform team approves → Workflow runs → PR created → Review → Merge → Terraform applies
```

### The Flow

1. **Create Issue**: User fills out GitHub issue form
2. **Approval Phase**: 2 platform-team members react 👍 or comment `/approve`
3. **Automation**: Workflow parses issue, modifies YAML files, commits to branch
4. **Pull Request**: Automated PR opened for review
5. **Terraform Plan**: Plan runs automatically, posted to PR
6. **Review**: 2 reviewers approve PR
7. **Apply**: Merge triggers Terraform apply, resource created

## Creating a Repository

### Step 1: Open Issue

1. Go to **Issues** → **New Issue**
2. Select **"Create Repository"**
3. Fill out the form:
   - Repository name (lowercase, hyphens only)
   - Workflow filename (e.g., `ci.yml`)
   - Responsible team
   - Description
   - Visibility (private/internal/public)
   - Features (Issues, Projects, Wiki, etc.)

### Step 2: Wait for Approvals

The issue needs **2 approvals** from `platform-team` members.

**To approve**:
- React with 👍 (thumbs up), OR
- Comment `approved` or `/approve`

The bot tracks approvals automatically and comments status updates.

### Step 3: Automation Runs

Once approved, the workflow:
1. Parses the issue form
2. Updates `repo-yamls/{team}-repos.yml`
3. Creates empty workflow file
4. Commits to branch `request/issue-{number}`
5. Pushes branch
6. Opens PR with details

### Step 4: Review and Merge

Review the PR:
- Check the Terraform plan
- Verify YAML changes
- Ensure naming is correct

Merge when ready. Terraform will create the repository.

## Issue Templates

### Create Repository

**File**: `.github/ISSUE_TEMPLATE/create-repo.yml`

**Required Fields**:
- Repository name
- Workflow filename
- Responsible team
- Description

**Optional Fields**:
- Visibility (default: private)
- Features (Issues, Projects, Wiki, Discussions)
- Signed commits requirement

**Approval Required**: Yes (2 platform-team members)

## Workflows

### `issueops-approval.yml`

**Triggers**:
- Issue comment created
- Issue labeled

**Purpose**: Track approvals from platform-team members

**Logic**:
1. Checks if commenter is platform-team member
2. Validates approval pattern (`/approve`, `approved`)
3. Counts total approvals
4. Updates issue labels
5. Triggers create-repo workflow when threshold met

**Key Features**:
- Real-time approval tracking
- Comments status updates
- Rejects non-platform-team approvals
- Automatic workflow dispatch

### `issueops-create-repo.yml`

**Trigger**: Manual dispatch (from approval workflow)

**Purpose**: Process approved repository creation request

**Steps**:
1. Fetch and validate issue
2. Parse issue body (GitHub form format)
3. Checkout repo
4. Update YAML files
5. Create workflow file
6. Validate changes
7. Commit and push to branch
8. Create PR
9. Comment back on issue

**Error Handling**: Comments on issue if anything fails

## Approval System

### Who Can Approve?

Only members of `platform-team` can approve requests.

### How to Approve?

**Method 1**: React with 👍
```
[Click thumbs up reaction on the issue]
```

**Method 2**: Comment
```
/approve
```
or
```
approved
```

### Approval Count

The bot automatically:
- Counts approvals from platform-team members only
- Ignores duplicate approvals from same user
- Posts status updates
- Triggers workflow at threshold (2 approvals)

### Tracking

Check issue comments for status:
```
📊 Approval Status: 1/2

Approvers so far: @alice

1 more approval(s) needed from platform-team members.
```

## Configuration

### `.github/issueops-config.yml`

Central configuration for IssueOps behavior:

```yaml
approver_teams:
  - platform-team

approvals_required: 2

approval_patterns:
  - "/approve"
  - "approved"
```

### Secrets Required

| Secret | Description |
|--------|-------------|
| `GH_ADMIN_TOKEN` | PAT with repo, workflow, admin:org scopes |
| `GITHUB_TOKEN` | Built-in token (automatic) |

## Advanced Usage

### Custom Approval Patterns

Edit `issueops-approval.yml` to add more patterns:

```yaml
- name: Check if Approval Comment
  run: |
    comment="${{ github.event.comment.body }}"
    if [[ "$comment" =~ ^/(approve|approved|lgtm)$ ]]; then
      echo "is_approval=true" >> $GITHUB_OUTPUT
    fi
```

### Multiple Approver Teams

Modify to allow different teams:

```yaml
- name: Get Approver Team Members
  # Query multiple teams
  # Combine member lists
```

### Conditional Approvals

Require different approval counts based on request type:

```yaml
- name: Determine Required Approvals
  run: |
    if [[ "${{ github.event.issue.labels }}" =~ "high-risk" ]]; then
      echo "required=3" >> $GITHUB_OUTPUT
    else
      echo "required=2" >> $GITHUB_OUTPUT
    fi
```

## Helper Scripts

### `scripts/yaml-helpers.py`

Python utility for YAML manipulation:

```bash
# Add repository
python3 scripts/yaml-helpers.py add-repo platform-team '{
  "name": "my-repo",
  "description": "My awesome repo",
  "team": "platform-team",
  "visibility": "private"
}'

# List repositories
python3 scripts/yaml-helpers.py list-repos

# List repos for specific team
python3 scripts/yaml-helpers.py list-repos platform-team

# Get specific repo
python3 scripts/yaml-helpers.py get-repo my-repo

# Update repo
python3 scripts/yaml-helpers.py update-repo platform-team my-repo '{
  "description": "Updated description"
}'

# Add team member
python3 scripts/yaml-helpers.py add-member platform-team alice maintainer
```

## Troubleshooting

### Issue: Approvals not being counted

**Check**:
1. Is the approver in `platform-team`?
2. Did they use the right format? (👍 or `/approve`)
3. Check workflow logs in Actions tab

**Fix**:
```bash
# Verify team membership
gh api orgs/YOUR-ORG/teams/platform-team/members | jq '.[].login'
```

### Issue: Workflow not triggered after approval

**Check**:
1. Does issue have `approved` label?
2. Check Actions tab for failed runs
3. Verify `GH_ADMIN_TOKEN` secret exists

**Fix**:
```bash
# Manually trigger workflow
gh workflow run issueops-create-repo.yml -f issue_number=123
```

### Issue: YAML validation fails

**Check**:
1. Repository name format (lowercase, hyphens only)
2. Team exists in `team-yamls/`
3. No duplicate repository names

**Fix**:
```bash
# Validate locally
python3 scripts/validate-yaml.py
```

### Issue: PR not created

**Check**:
1. Branch already exists? Delete it:
   ```bash
   git push origin --delete request/issue-123
   ```
2. Check workflow logs for errors
3. Verify repository permissions

### Issue: Can't approve (not in platform-team)

**Solution**: Ask a platform-team member to add you:

```bash
# Platform-team maintainer runs:
python3 scripts/yaml-helpers.py add-member platform-team YOUR-USERNAME member
```

## Best Practices

### For Requesters

1. **Use clear names**: `data-pipeline-api` not `api-thing-v2`
2. **Good descriptions**: Explain what the repo is for
3. **Right team**: Choose the team that will own this repo
4. **Check duplicates**: Search existing repos first

### For Approvers

1. **Review carefully**: Check name, team, visibility
2. **Verify requester**: Do they need this repo?
3. **Check compliance**: Does it follow naming conventions?
4. **One approval per request**: Don't spam approvals

### For Platform Team

1. **Respond quickly**: Aim for <1 hour approval time
2. **Ask questions**: Comment if anything unclear
3. **Reject properly**: Close issue with explanation if denying
4. **Monitor patterns**: Look for common issues

## Security

### Approval Bypass Prevention

The system prevents bypass through:
- Label checks (must have `approved`)
- Team membership validation
- Multiple approvals required
- Audit trail in issue comments

### Token Security

Never commit tokens. Always use secrets:

```yaml
# GOOD
token: ${{ secrets.GH_ADMIN_TOKEN }}

# BAD
token: ghp_xxxxxxxxxxxxx
```

### Branch Protection

The automation creates branches but can't merge. Requires:
- PR approval (2 reviewers)
- Passing Terraform plan
- Manual merge

## Extending IssueOps

### Add New Operations

1. Create issue template: `.github/ISSUE_TEMPLATE/your-operation.yml`
2. Create workflow: `.github/workflows/issueops-your-operation.yml`
3. Update approval workflow to trigger new workflow
4. Document in this file

### Example: Delete Repository

```yaml
# .github/ISSUE_TEMPLATE/delete-repo.yml
name: Delete Repository
description: Request repository deletion
labels: ["issueops", "delete-repo", "pending-approval"]
body:
  - type: input
    id: repo_name
    attributes:
      label: Repository Name
    validations:
      required: true
```

### Example: Add Team Member

```yaml
# .github/ISSUE_TEMPLATE/add-member.yml
name: Add Team Member
description: Add member to team
labels: ["issueops", "add-member", "pending-approval"]
body:
  - type: input
    id: username
    attributes:
      label: GitHub Username
  - type: dropdown
    id: team
    attributes:
      label: Team
```

## Metrics and Monitoring

### Track IssueOps Usage

```bash
# Count IssueOps issues this month
gh issue list --label issueops --state all \
  --created "$(date -d '1 month ago' +%Y-%m-%d)" \
  --json number,title,state \
  --jq 'length'

# Average approval time
gh issue list --label approved --state all \
  --json createdAt,comments \
  --jq '.[] | [.createdAt, (.comments[]? | select(.body | test("/approve")) | .createdAt)] | .[1] - .[0]'

# Most active approvers
gh issue list --label approved --state all \
  --json comments \
  --jq '.[] | .comments[] | select(.body | test("/approve")) | .author.login' \
  | sort | uniq -c | sort -rn
```

### Workflow Success Rate

Check in Actions tab or via CLI:

```bash
gh run list --workflow issueops-create-repo.yml \
  --json conclusion \
  --jq 'group_by(.conclusion) | map({conclusion: .[0].conclusion, count: length})'
```

## FAQ

**Q: Can I approve my own request?**  
A: No. Approvals must come from different platform-team members.

**Q: How long does approval take?**  
A: Usually < 1 hour during business hours. Check #infrastructure on Slack.

**Q: Can I cancel a request?**  
A: Yes. Close the issue and comment why.

**Q: What if I made a mistake in the form?**  
A: Close the issue and create a new one. Don't try to edit.

**Q: Can I request multiple repos at once?**  
A: No. One issue per repository. But you can open multiple issues.

**Q: Why do I need 2 approvals?**  
A: Safety. Two sets of eyes prevent mistakes.

**Q: Can the approval count be changed?**  
A: Yes. Edit `issueops-config.yml` and change `approvals_required`.

**Q: What happens if Terraform fails?**  
A: The PR shows the error. Fix the YAML and push to the branch.

## Integration with Existing Workflows

### Slack Notifications

Add to workflow:

```yaml
- name: Notify Slack
  run: |
    curl -X POST ${{ secrets.SLACK_WEBHOOK }} \
      -H 'Content-Type: application/json' \
      -d '{
        "text": "New repo request: ${{ github.event.issue.title }}",
        "blocks": [{
          "type": "section",
          "text": {"type": "mrkdwn", "text": "*New Repository Request*\n<${{ github.event.issue.html_url }}|View Issue>"}
        }]
      }'
```

### Jira Integration

Create Jira ticket for each request:

```yaml
- name: Create Jira Ticket
  run: |
    curl -X POST https://your-domain.atlassian.net/rest/api/2/issue \
      -u "${{ secrets.JIRA_EMAIL }}:${{ secrets.JIRA_API_TOKEN }}" \
      -H 'Content-Type: application/json' \
      -d '{
        "fields": {
          "project": {"key": "INFRA"},
          "summary": "${{ github.event.issue.title }}",
          "issuetype": {"name": "Task"}
        }
      }'
```

## Roadmap

Future IssueOps operations:

- [ ] Delete repository
- [ ] Archive repository
- [ ] Add team member
- [ ] Remove team member
- [ ] Update repository settings
- [ ] Create team
- [ ] Update branch protection
- [ ] Request infrastructure access

---

**Remember**: IssueOps is about making infrastructure accessible. The form is the interface. The workflow is the implementation. Keep it simple.

*"The best interface is the one you don't notice."*
