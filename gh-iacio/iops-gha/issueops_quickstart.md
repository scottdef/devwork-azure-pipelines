# IssueOps Quick Start

Get a repository created in 5 minutes. No YAML editing required.

## For Users: Creating a Repository

### 1. Open Issue (30 seconds)

```
Issues → New Issue → "Create Repository"
```

Fill out the form:
- **Repository Name**: `my-awesome-api`
- **Workflow File**: `ci.yml`
- **Team**: `platform-team`
- **Description**: What this repo does
- **Visibility**: `private`
- Check the boxes for features you want

Click **"Submit new issue"**

### 2. Wait for Approvals (minutes to hours)

Your issue needs 2 👍 from `platform-team` members.

The bot comments status updates:
```
📊 Approval Status: 1/2
Approvers so far: @alice
1 more approval(s) needed
```

### 3. Watch Automation (seconds)

Once approved, the bot:
- ✅ Updates YAML files
- ✅ Creates branch
- ✅ Opens PR
- ✅ Posts link back to your issue

### 4. PR Gets Reviewed (minutes to hours)

Platform team reviews the PR.

You'll see Terraform plan in comments.

### 5. Merge → Repository Created! (seconds)

PR merged → Terraform runs → Repository exists!

Check: `https://github.com/YOUR-ORG/my-awesome-api`

## For Approvers: Approving Requests

### Quick Approve

Just react 👍 on the issue. Done.

### Or Comment

```
/approve
```
or
```
approved
```

### Check Status

Look for bot comment:
```
📊 Approval Status: 2/2
✅ Approved!
```

## Troubleshooting

### "Only platform-team members can approve"

→ You're not in the team. Ask a maintainer to add you.

### "Approval Status: 0/2" (nothing happening)

→ Click 👍 on the issue (not a comment)

### "Repository already exists"

→ Close issue, pick a different name, open new issue

### PR not created after approval

→ Check Actions tab for errors. Ping `@platform-team`

## Advanced

### Approve via CLI

```bash
gh issue comment 123 --body "/approve"
```

### Check who approved

```bash
gh issue view 123 --json comments \
  | jq '.comments[] | select(.body | test("/approve"))'
```

### Trigger workflow manually

```bash
gh workflow run issueops-create-repo.yml -f issue_number=123
```

## Tips

**Good Request**:
```
Name: user-service-api
Team: platform-team
Description: REST API for user management and authentication
Visibility: private
Features: Issues ✓
```

**Bad Request**:
```
Name: NewAPIThing
Team: ???
Description: api
Visibility: public
```

## Getting Help

1. Check [ISSUEOPS.md](ISSUEOPS.md) for full docs
2. Ping `@platform-team` in issue comments
3. Ask in #infrastructure on Slack
4. Check [workflow logs](../../actions)

---

**tl;dr**: Issue → 2 thumbs up → PR created → Review → Merge → Repository exists

Simple as that. 🚀
