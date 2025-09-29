# IssueOps Examples

Real-world examples of using IssueOps for GitHub governance.

## Example 1: Create a New Microservice Repository

### Scenario
Platform team needs a new repository for the authentication service.

### Steps

**1. Create Issue**

Navigate to: `Issues → New Issue → "Create Repository"`

Fill out the form:
```
Repository Name: auth-service-api
Workflow Filename: ci.yml
Team: platform-team
Description: Authentication and authorization microservice
Visibility: private

Features:
☑ Issues
☐ Projects  
☐ Wiki
☑ Discussions

Confirmations:
☑ Repository name not in use
☑ Approval from team lead
☑ Requires 2 platform-team approvals
```

**2. Approval Phase**

Alice (platform-team maintainer):
```
👍  [clicks thumbs up on issue]
```

Bot responds:
```
📊 Approval Status: 1/2
Approvers so far: @alice
1 more approval(s) needed from platform-team members.
```

Bob (platform-team maintainer):
```
/approve
```

Bot responds:
```
✅ Approved!

This request has been approved by 2 platform-team members:
- @alice
- @bob

The automation workflow will now process this request.
```

**3. Automation Runs**

Within seconds:
- Workflow `issueops-create-repo` triggers
- Updates `repo-yamls/platform-repos.yml`
- Creates `.github/workflows/ci.yml`
- Commits to branch `request/issue-42`
- Opens PR #43

Bot comments on issue:
```
🚀 Pull Request Created

Your repository creation request has been processed!

Pull Request: #43
Branch: request/issue-42

What's Next?
1. The Terraform plan will run automatically
2. Two reviewers must approve the PR
3. Once merged, Terraform will create your repository

[View Pull Request](https://github.com/org/repo/pull/43)
```

**4. Review PR**

Terraform plan runs automatically, comments on PR:
```
Terraform Plan 📖

+ github_repository.repos["auth-service-api"]
+ github_team_repository.repo_admin_access["auth-service-api"]
+ github_repository_file.codeowners["auth-service-api"]
+ github_repository_ruleset.main_protection["auth-service-api"]

Plan: 4 to add, 0 to change, 0 to destroy
```

Charlie and Diana review and approve.

**5. Merge & Deploy**

PR merged → `terraform-apply` runs → Repository created!

Result:
- Repository: `https://github.com/org/auth-service-api`
- Team access: `platform-team` (push), `platform-team-admins` (admin)
- Branch protection: main branch requires 2 reviews
- CODEOWNERS: `@org/platform-team @org/platform-team-pr-approvers`
- Workflow: `.github/workflows/ci.yml` ready to configure

**Total Time: ~15 minutes (approval) + ~2 minutes (automation)**

---

## Example 2: Add New Team Member

### Scenario
Eve joins the platform team and needs repository access.

### Steps

**1. Create Issue**

Navigate to: `Issues → New Issue → "Add Team Member"`

Fill out:
```
GitHub Username: eve
Team: platform-team
Role: member
Justification: New engineer joining platform team, needs access to platform repositories

Confirmations:
☑ GitHub username is correct
☑ Approval from team lead
☑ Completed onboarding
☑ Requires 2 platform-team approvals
```

**2. Approval**

Alice and Bob approve with 👍

**3. Automation**

- Updates `team-yamls/platform-team.yml`
- Adds Eve as member
- Creates PR #44

**4. Review & Merge**

Terraform plan shows:
```
~ github_team_membership.members["platform-team-eve"]
  + username = "eve"
  + role     = "member"
```

PR merged → Eve added to team → Gets access to all platform-team repositories

**Total Time: ~10 minutes**

---

## Example 3: Delete Deprecated Repository

### Scenario
Old prototype repository no longer needed, needs cleanup.

### Steps

**1. Create Issue**

Navigate to: `Issues → New Issue → "Delete Repository"`

Fill out:
```
Repository Name: old-prototype
Team: platform-team
Reason: Prototype completed, production version deployed, no longer maintained

Confirmations:
☑ Backed up important data
☑ Repository not in active use
☑ Communicated to team
☑ Understand manual deletion required
☑ Team lead approval
☑ Requires 2 platform-team approvals
```

**2. Approval & Automation**

After approvals, automation:
- Removes from `repo-yamls/platform-repos.yml`
- Creates PR #45

**3. Review PR**

Terraform plan shows:
```
- github_repository.repos["old-prototype"]
- github_team_repository.repo_admin_access["old-prototype"]
- github_repository_file.codeowners["old-prototype"]

Plan: 0 to add, 0 to change, 3 to destroy
```

**4. Merge PR**

After merge, **manual step required**:
```bash
gh repo delete org/old-prototype
```

The automation removes it from Terraform but doesn't delete the actual repo for safety.

---

## Example 4: Bulk Operations

### Scenario
Creating multiple related repositories for a new project.

### Approach

Create separate issues for each repo:

**Issue #50**: Create `project-api`
**Issue #51**: Create `project-frontend`  
**Issue #52**: Create `project-worker`

All can be approved simultaneously. Each gets its own PR. Can be merged in any order.

**Benefit**: Clear audit trail, independent review, rollback individual changes.

---

## Example 5: Emergency Access

### Scenario
Production incident, engineer needs immediate repository access.

### Steps

**1. Create Issue** (standard process)

**2. Fast-track Approval**

Platform team lead can:
```bash
# Approve via CLI
gh issue comment 60 --body "/approve"

# Trigger workflow manually (if already approved)
gh workflow run issueops-add-member.yml -f issue_number=60
```

**3. Express Lane**

Once PR created:
- Skip waiting for plan
- Admin can merge immediately
- User gets access within seconds

**Use with caution**: Normal process is fast enough for most cases.

---

## Example 6: Rejected Request

### Scenario
Request doesn't meet requirements.

### Steps

**1. Request**: User creates issue for repository named "test123"

**2. Review**: Platform team identifies issues:
- Poor naming convention
- No clear purpose
- Missing team approval

**3. Rejection**:
```
Comment on issue:
"This request doesn't meet our naming conventions. 
Repository names should be descriptive. Please:
1. Use kebab-case: project-name-api
2. Provide clear description
3. Get team lead approval

Closing this issue. Please create a new one with corrected information."

[Close issue without approving]
```

**4. Cleanup**: `issueops-cleanup` workflow runs automatically:
- Deletes any branches
- Adds "closed" label
- Comments with explanation

**Result**: Clean rejection with clear feedback. User can try again.

---

## Example 7: Monitoring Dashboard

### Use Case
Platform team checks status daily.

### Command

```bash
make issueops-dashboard
```

Output:
```
═══════════════════════════════════════════════════════════
  IssueOps Dashboard
═══════════════════════════════════════════════════════════

Updated: 2025-01-15 14:30:00
Repository: org/github-governance

▶ Pending Approvals

  ⏳ #65 - [REPO] Create: ml-training-service by @frank (2025-01-15)
  ⏳ #66 - [TEAM] Add Member: grace to data-team by @hannah (2025-01-15)

  Total: 2 pending

▶ Recently Approved (Last 7 days)

  ✓ #60 - [TEAM] Add Member: eve to platform-team [CLOSED]
  ✓ #61 - [REPO] Create: auth-service-api [CLOSED]
  → #63 - [REPO] Delete: old-prototype [OPEN]

▶ Active IssueOps PRs

  #64 - [IssueOps] Delete repository: old-prototype
    Author: @github-actions | Reviews: 1

  Total: 1 active PRs

▶ Failed Requests

  ✓ No errors

▶ Statistics (Last 30 days)

  Total Requests: 15
  ├─ Create Repository: 8
  ├─ Delete Repository: 2
  └─ Add Team Member: 5

  Completion Rate: 93% (14/15)
```

---

## Example 8: CLI Workflows

### Approve Request

```bash
# View pending
make issueops-pending

# Approve specific issue
make issueops-approve ISSUE_NUM=42

# Or directly with gh
gh issue comment 42 --body "/approve"
```

### Check Status

```bash
# Full dashboard
make issueops-dashboard

# Just statistics
make issueops-stats

# Just errors
make issueops-errors

# Live monitoring (updates every 30s)
make issueops-watch
```

### Create Request via CLI

```bash
# Opens web browser to create issue
make issueops-create

# Or use gh directly
gh issue create \
  --title "[REPO] Create: my-new-service" \
  --label "issueops,create-repo,pending-approval" \
  --body-file request.md
```

---

## Example 9: Integration with Slack

### Setup

Add to workflow:
```yaml
- name: Notify Slack
  run: |
    curl -X POST ${{ secrets.SLACK_WEBHOOK }} \
      -d '{
        "text": "New repo request: ${{ github.event.issue.title }}",
        "channel": "#infrastructure",
        "username": "IssueOps Bot"
      }'
```

### Flow

1. User creates issue
2. Slack notification: "New repo request from @user"
3. Team reviews in Slack
4. Quick approve: Click notification → Open issue → React 👍
5. Slack update: "Request approved and processing"
6. Final notification: "Repository created: auth-service-api"

---

## Example 10: Audit Trail

### Scenario
Need to audit who created a repository and when.

### Query

```bash
# Find issue for specific repo
gh issue list --label create-repo --search "auth-service-api" --state closed

# View full history
gh issue view 42

# See approvals
gh api repos/org/repo/issues/42/comments \
  | jq '.[] | select(.body | contains("/approve")) | .user.login'

# Find associated PR
gh pr list --search "closes:#42"

# View Terraform apply
gh run list --workflow terraform-apply.yml
```

### Result
Complete audit trail:
- Who requested: @user (issue creator)
- Who approved: @alice, @bob (comments)
- When approved: timestamps in issue
- What changed: PR diff
- When created: Terraform apply timestamp

---

## Tips & Tricks

### 1. Batch Approvals

Approve multiple at once:
```bash
for issue in 42 43 44; do
  gh issue comment $issue --body "/approve"
done
```

### 2. Template Responses

Save common responses:
```bash
# reject-naming.txt
This doesn't follow our naming convention.
Please use: service-name-type
Example: auth-service-api

# Close and respond
gh issue close 50 --comment "$(cat reject-naming.txt)"
```

### 3. Scheduled Reports

Add to cron:
```bash
# Daily summary at 9am
0 9 * * * cd /path/to/repo && make issueops-dashboard | mail -s "IssueOps Daily Summary" team@company.com
```

### 4. Quick Filters

```bash
# Your requests
gh issue list --label issueops --author @me

# Urgent requests (labeled)
gh issue list --label issueops,urgent --state open

# Requests older than 2 days
gh issue list --label pending-approval \
  --search "created:<$(date -d '2 days ago' +%Y-%m-%d)"
```

### 5. Metrics

```bash
# Average approval time
gh issue list --label approved --json createdAt,closedAt \
  | jq '.[] | (.closedAt - .createdAt) / 3600' \
  | awk '{sum+=$1; count++} END {print sum/count " hours"}'

# Busiest day
gh issue list --label issueops --json createdAt \
  | jq -r '.[].createdAt | split("T")[0]' \
  | sort | uniq -c | sort -rn | head -1
```

---

## Common Patterns

### Pattern 1: New Project Setup

1. Create team (`add-team` - future feature)
2. Add team members (multiple `add-member` issues)
3. Create repositories (multiple `create-repo` issues)
4. Configure CI/CD in the repos

### Pattern 2: Team Restructure

1. Create new subteam
2. Add members to new subteam
3. Update repository team assignments
4. Remove members from old team

### Pattern 3: Deprecation

1. Archive repositories (mark in issue)
2. Delete from YAML (`delete-repo`)
3. Update documentation
4. Notify stakeholders

---

**Remember**: IssueOps is about making infrastructure accessible. Use it for what it's good at—simple, common operations. For complex changes, edit YAML directly.
