# 🚀 KickButt Compliance - Complete Deployment Guide

## Executive Summary

You now have a **production-ready** GitHub Pages compliance dashboard that:
- Monitors 4 security controls via GitHub Actions workflows
- Updates hourly with real-time compliance status  
- Displays results on a dark-themed trust center website
- Runs entirely within GitHub Enterprise Cloud
- Requires **zero external dependencies**

**Repository Structure**: [View complete directory tree below](#directory-structure)

---

## 🎯 What You're Getting

### Core Components

```
1. Control Workflows (4)
   ├── control-org-settings.yml       → Validates org security configs
   ├── control-repo-visibility.yml    → Checks repo visibility policies
   ├── control-repo-rulesets.yml      → Validates branch protection rules
   └── control-org-custom-role.yml    → Audits custom role permissions

2. Automation Workflows (3)
   ├── aggregate-status.yml           → Collects control results
   ├── pages-deploy.yml               → Deploys to GitHub Pages
   └── update-compliance-site.yml     → Hourly site regeneration (NEW)

3. Site Generation Scripts
   ├── fetch-workflow-status.py       → GitHub API client
   └── generate-site.py               → Static HTML generator

4. Dashboard Website
   ├── index.html                     → Overview page
   ├── styles.css                     → Dark theme styling
   ├── status-loader.js               → Live status updates
   └── control-status.json            → Data file (auto-generated)
```

---

## 🏗️ Architecture Overview

### Data Flow

```
┌──────────────────────────────────────────────────────────────────┐
│                    EVERY HOUR (Cron Trigger)                      │
└────────────────────────┬─────────────────────────────────────────┘
                         │
         ┌───────────────┴────────────────┐
         │                                 │
         ▼                                 ▼
┌─────────────────┐              ┌─────────────────┐
│ Control 1 & 2   │              │ Control 3 & 4   │
│ Run in parallel │              │ Run in parallel │
└────────┬────────┘              └────────┬────────┘
         │                                 │
         └───────────────┬─────────────────┘
                         │ Write status files
                         ▼
                ┌─────────────────┐
                │ Aggregate Status│
                │   Workflow      │
                └────────┬────────┘
                         │ Generate JSON
                         ▼
                ┌─────────────────┐
                │  Pages Deploy   │
                │    Workflow     │
                └────────┬────────┘
                         │
                         ▼
                ┌─────────────────┐
                │   GitHub Pages  │
                │  Live Dashboard │
                └─────────────────┘
```

### Technology Stack

| Layer | Technology | Purpose |
|-------|-----------|---------|
| **Monitoring** | GitHub Actions | Run compliance checks |
| **Data Collection** | Python + GitHub API | Fetch workflow statuses |
| **Site Generation** | Python + Jinja2 | Build static HTML |
| **Hosting** | GitHub Pages | Serve dashboard |
| **Styling** | Custom CSS | Dark theme UI |
| **Updates** | JavaScript | Real-time status loading |

---

## 📋 Pre-Deployment Checklist

### Required Access

- [ ] GitHub Enterprise Cloud organization
- [ ] Admin access to organization settings
- [ ] Ability to create repositories
- [ ] Ability to enable GitHub Pages

### Repository Requirements

- [ ] Private repository (recommended) or public
- [ ] GitHub Pages enabled
- [ ] Actions workflow permissions: **Read and write**
- [ ] Allow Actions to create PRs: **Enabled**

### Optional (Enhanced Features)

- [ ] `GH_ADMIN_TOKEN` secret with `admin:org` scope
- [ ] Slack webhook for notifications (future enhancement)

---

## 🚀 Deployment Steps

### Step 1: Create Repository

```bash
# Create new repository in your organization
gh repo create <org>/kickbutt-compliance \
  --private \
  --description "Compliance Trust Center Dashboard" \
  --clone

# Navigate to the repository
cd kickbutt-compliance
```

### Step 2: Copy Files

```bash
# Copy all files from the provided kickbutt-compliance/ directory
cp -r /path/to/kickbutt-compliance/* .

# Verify structure
tree -L 2
```

Expected structure:
```
.
├── .github/
│   └── workflows/              # 7 workflow files
├── docs/
│   ├── index.html
│   ├── css/
│   ├── js/
│   └── data/                   # Auto-generated
├── scripts/
│   ├── fetch-workflow-status.py
│   └── generate-site.py
├── Makefile
├── README.md
└── SETUP.md
```

### Step 3: Enable GitHub Pages

**Option A: GitHub CLI**
```bash
gh repo edit --enable-pages --pages-branch main --pages-path /docs
```

**Option B: Web UI**
1. Go to **Settings → Pages**
2. Source: **Deploy from a branch**
3. Branch: **main**
4. Folder: **/docs**
5. Click **Save**

### Step 4: Configure Workflow Permissions

1. Go to **Settings → Actions → General**
2. Under **Workflow permissions**:
   - Select: ☑️ **Read and write permissions**
   - Enable: ☑️ **Allow GitHub Actions to create and approve pull requests**
3. Click **Save**

### Step 5: Add Secrets (Optional)

For enhanced org-level checks:

```bash
# Create a Personal Access Token with admin:org scope
# Then add it as a secret
gh secret set GH_ADMIN_TOKEN
# Paste your token when prompted
```

Or via Web UI:
1. Go to **Settings → Secrets and variables → Actions**
2. Click **New repository secret**
3. Name: `GH_ADMIN_TOKEN`
4. Value: Your PAT
5. Click **Add secret**

### Step 6: Initial Deployment

```bash
# Commit and push
git add .
git commit -m "🚀 Initial KickButt Compliance deployment"
git push origin main

# Trigger initial control runs
gh workflow run control-org-settings.yml
gh workflow run control-repo-visibility.yml
gh workflow run control-repo-rulesets.yml
gh workflow run control-org-custom-role.yml

# Wait 1-2 minutes, then aggregate
gh workflow run aggregate-status.yml

# Deploy to Pages
gh workflow run pages-deploy.yml
```

### Step 7: Verify Deployment

```bash
# Check workflow status
gh run list --limit 10

# Get your Pages URL
gh api repos/:owner/:repo/pages --jq '.html_url'

# Visit the URL
# Expected: https://<org>.github.io/kickbutt-compliance/
```

---

## 🎨 Dashboard Interface

### Overview Page

The main dashboard shows:

1. **Hero Section**
   - Title: "Compliance Trust Center"
   - Subtitle explaining purpose
   - Last updated timestamp

2. **Compliance Areas**
   - **🔐 Organization Security**
     - Organization Settings
     - Custom Roles
   
   - **📋 Repository Governance**
     - Repository Visibility
     - Repository Rulesets

3. **Control Status Table**
   - Control name and description
   - Status icon (✅ ❌ ⏳ ○)
   - Last run timestamp
   - Run duration
   - Link to workflow run

### Status Icons

| Icon | Meaning | Status |
|------|---------|--------|
| ✅ | Success | All checks passed |
| ❌ | Failure | Control found issues |
| ⏳ | Running | Check in progress |
| ○ | No Runs | Never executed |
| ⚠️ | Error | Workflow error |

### Dark Theme

- Background: `#0a0a0a`
- Surface: `#1a1a1a`
- Success: `#10b981` (Green)
- Failure: `#ef4444` (Red)
- Warning: `#f59e0b` (Amber)
- Progress: `#3b82f6` (Blue)

---

## 🔧 Configuration

### Customizing Controls

#### Adding a New Control

1. **Create workflow file**:

```yaml
# .github/workflows/control-my-check.yml
name: Control - My Check

on:
  schedule:
    - cron: '30 * * * *'  # Runs at :30 past every hour
  workflow_dispatch:

permissions:
  contents: write
  actions: read

jobs:
  validate:
    runs-on: ubuntu-latest
    steps:
      - name: Checkout
        uses: actions/checkout@v4
      
      - name: Run My Check
        run: |
          # Your validation logic
          echo "Checking something important..."
          
          # Set status based on check results
          if [ condition ]; then
            echo "status=success" >> $GITHUB_OUTPUT
          else
            echo "status=failure" >> $GITHUB_OUTPUT
            exit 1
          fi
      
      - name: Update Status File
        run: |
          mkdir -p .github/control-status
          cat > .github/control-status/control-my-check.json << EOF
          {
            "name": "control-my-check",
            "status": "${{ steps.validate.outcome }}",
            "last_run": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
            "duration": $SECONDS,
            "run_url": "${{ github.server_url }}/${{ github.repository }}/actions/runs/${{ github.run_id }}"
          }
          EOF
      
      - name: Commit Status
        run: |
          git config user.name "github-actions[bot]"
          git config user.email "github-actions[bot]@users.noreply.github.com"
          git add .github/control-status/
          git commit -m "Update control status [skip ci]" || echo "No changes"
          git push
```

2. **Update aggregate workflow**:

Edit `.github/workflows/aggregate-status.yml` and add to the controls list:

```bash
CONTROLS=$(cat << 'EOF'
control-org-settings
control-repo-visibility
control-repo-rulesets
control-org-custom-role
control-my-check        # <-- ADD THIS
EOF
)
```

3. **Update scripts**:

Edit `scripts/fetch-workflow-status.py`:

```python
WORKFLOWS = [
    "control-org-settings.yml",
    "control-repo-visibility.yml",
    "control-repo-rulesets.yml",
    "control-org-custom-role.yml",
    "control-my-check.yml"  # <-- ADD THIS
]
```

4. **Update compliance areas**:

```python
COMPLIANCE_AREAS = {
    "Organization Security": [
        "control-org-settings.yml",
        "control-org-custom-role.yml",
        "control-my-check.yml"  # <-- ADD TO AREA
    ],
    # ...
}
```

### Adjusting Update Frequency

Change cron schedules in workflow files:

```yaml
on:
  schedule:
    # Every hour at :00
    - cron: '0 * * * *'
    
    # Every 2 hours
    - cron: '0 */2 * * *'
    
    # Every 6 hours
    - cron: '0 */6 * * *'
    
    # Daily at midnight
    - cron: '0 0 * * *'
    
    # Twice daily (8 AM and 8 PM)
    - cron: '0 8,20 * * *'
```

### Customizing Compliance Areas

Edit `scripts/fetch-workflow-status.py`:

```python
COMPLIANCE_AREAS = {
    "Access Control": [
        "control-org-settings.yml",
        "control-org-custom-role.yml"
    ],
    "Security Policies": [
        "control-repo-visibility.yml",
        "control-repo-rulesets.yml"
    ],
    "Audit & Logging": [
        "control-audit-logs.yml",
        "control-webhook-logs.yml"
    ],
    "Configuration Management": [
        "control-terraform-state.yml",
        "control-iac-validation.yml"
    ]
}
```

---

## 🧪 Testing

### Local Testing

```bash
# Validate workflow syntax
make validate

# Test Python scripts locally
export GITHUB_TOKEN="your_token"
export GITHUB_REPOSITORY="org/repo"

python scripts/fetch-workflow-status.py
python scripts/generate-site.py

# Serve locally
cd docs
python3 -m http.server 8000
# Visit http://localhost:8000
```

### Manual Workflow Triggers

```bash
# Run a single control
gh workflow run control-org-settings.yml

# Check status
gh run list --workflow=control-org-settings.yml

# View logs
gh run view <run-id> --log

# Trigger full cycle
gh workflow run control-org-settings.yml && \
  sleep 60 && \
  gh workflow run aggregate-status.yml && \
  sleep 30 && \
  gh workflow run pages-deploy.yml
```

---

## 🐛 Troubleshooting

### Dashboard Not Loading

**Symptom**: Blank page or 404 error

**Solutions**:
```bash
# 1. Verify Pages is enabled
gh api repos/:owner/:repo/pages

# 2. Check deployment status
gh run list --workflow=pages-deploy.yml

# 3. Manually trigger deployment
gh workflow run pages-deploy.yml

# 4. Check status JSON exists
gh api repos/:owner/:repo/contents/docs/data/control-status.json
```

### Controls Not Running

**Symptom**: No workflow runs in Actions tab

**Solutions**:
```bash
# 1. Check cron syntax
gh workflow view control-org-settings.yml

# 2. Verify Actions are enabled
gh api repos/:owner/:repo --jq '.has_issues, .has_projects, .has_wiki'

# 3. Check workflow permissions
# Settings → Actions → General → Workflow permissions

# 4. Manually trigger
gh workflow run control-org-settings.yml --ref main
```

### Status Not Updating

**Symptom**: Dashboard shows old data

**Solutions**:
```bash
# 1. Check aggregation workflow
gh run list --workflow=aggregate-status.yml

# 2. Verify status files exist
ls -la .github/control-status/

# 3. Check for commit errors
gh run view <latest-run-id> --log | grep -i error

# 4. Clear browser cache and reload
```

### Permission Errors

**Symptom**: `Error: Resource not accessible by integration`

**Solutions**:
1. Go to **Settings → Actions → General**
2. Set **Workflow permissions** to "Read and write permissions"
3. Enable "Allow GitHub Actions to create and approve pull requests"
4. Re-run the failed workflow

### GitHub API Rate Limits

**Symptom**: `API rate limit exceeded`

**Solutions**:
```bash
# Check rate limit status
gh api rate_limit

# Increase update interval
# Edit workflow cron from '0 * * * *' to '0 */2 * * *'

# Use authenticated token
# Add GH_ADMIN_TOKEN secret with personal access token
```

---

## 🔒 Security Best Practices

### Token Management

1. **Use Fine-Grained PATs**:
   - Create at: https://github.com/settings/tokens?type=beta
   - Permissions: `admin:org` (read), `repo` (read/write)
   - Expiration: 90 days (set calendar reminder)

2. **Rotate Regularly**:
   ```bash
   # Generate new token
   # Update secret
   gh secret set GH_ADMIN_TOKEN
   ```

3. **Audit Token Usage**:
   - Settings → Developer settings → Personal access tokens
   - Review "Last used within" dates

### Branch Protection

```bash
# Protect main branch
gh api repos/:owner/:repo/branches/main/protection \
  -X PUT \
  -f required_pull_request_reviews[required_approving_review_count]=1 \
  -f enforce_admins=true \
  -f required_status_checks[strict]=true
```

### Secret Scanning

1. Enable secret scanning:
   - Settings → Code security and analysis
   - Enable "Secret scanning"
   - Enable "Push protection"

2. Review alerts:
   ```bash
   gh api repos/:owner/:repo/secret-scanning/alerts
   ```

---

## 📊 Monitoring & Maintenance

### Workflow Health Checks

```bash
# Check all recent runs
gh run list --limit 20

# Check failure rate
gh run list --workflow=control-org-settings.yml --json conclusion \
  | jq '[.[] | .conclusion] | group_by(.) | map({key: .[0], value: length}) | from_entries'

# View failure details
gh run list --workflow=control-org-settings.yml --status=failure --limit 5
```

### Weekly Maintenance

```bash
# Review failed controls
gh run list --status=failure --limit 50

# Check for workflow updates
gh workflow list

# Audit secret usage
gh secret list

# Review Pages deployment
gh api repos/:owner/:repo/pages/builds/latest
```

### Monthly Review

- [ ] Review and update control validation logic
- [ ] Check for GitHub Actions version updates
- [ ] Audit PAT expiration dates
- [ ] Review compliance area groupings
- [ ] Update documentation

---

## 🚀 Advanced Features

### Slack Notifications

Add to control workflows:

```yaml
- name: Notify on Failure
  if: failure()
  uses: slackapi/slack-github-action@v1
  with:
    webhook-url: ${{ secrets.SLACK_WEBHOOK }}
    payload: |
      {
        "text": "❌ Control ${{ github.workflow }} failed",
        "blocks": [
          {
            "type": "section",
            "text": {
              "type": "mrkdwn",
              "text": "*${{ github.workflow }}* failed\n<${{ github.server_url }}/${{ github.repository }}/actions/runs/${{ github.run_id }}|View Run>"
            }
          }
        ]
      }
```

### Custom Metrics

Track success rates over time:

```python
# In generate-site.py
def calculate_metrics(workflow_history):
    """Calculate 30-day success rate"""
    recent_runs = workflow_history[-30:]
    success_count = sum(1 for r in recent_runs if r['conclusion'] == 'success')
    return {
        'success_rate': (success_count / len(recent_runs)) * 100,
        'total_runs': len(recent_runs),
        'failures': len(recent_runs) - success_count
    }
```

### Multi-Environment Support

```yaml
# Different controls per environment
# control-prod-security.yml for production
# control-dev-security.yml for development
on:
  schedule:
    - cron: '0 * * * *'
  workflow_dispatch:
    inputs:
      environment:
        description: 'Environment to check'
        required: true
        default: 'production'
        type: choice
        options:
          - production
          - staging
          - development
```

---

## 📚 Additional Resources

### Documentation Links

- [GitHub Actions Documentation](https://docs.github.com/actions)
- [GitHub Pages Documentation](https://docs.github.com/pages)
- [GitHub REST API](https://docs.github.com/rest)
- [GitHub Enterprise Cloud](https://docs.github.com/enterprise-cloud)

### Example Control Logic

See the following for detailed validation examples:
- `control-org-settings.yml` - Organization security checks
- `control-repo-visibility.yml` - Repository visibility auditing
- `control-repo-rulesets.yml` - Branch protection validation
- `control-org-custom-role.yml` - Custom role auditing

### Community Resources

- GitHub Actions Community Forum
- GitHub Security Lab
- Awesome GitHub Actions list

---

## 📞 Support

For issues or questions:

1. **Check the troubleshooting guide** (above)
2. **Review workflow logs**: `gh run view <run-id> --log`
3. **Check GitHub status**: https://www.githubstatus.com/
4. **Open an issue** in this repository

---

## ✅ Post-Deployment Checklist

After deployment, verify:

- [ ] All 4 control workflows have run at least once
- [ ] Dashboard is accessible at GitHub Pages URL
- [ ] Status icons are displaying correctly
- [ ] Links to workflow runs are working
- [ ] Last updated timestamp is recent
- [ ] Browser console shows no errors
- [ ] Mobile view is responsive

---

## 🎉 You're Done!

Your KickButt Compliance dashboard is now live and monitoring your GitHub organization security posture 24/7.

**Next Steps**:
1. Share the dashboard URL with your team
2. Monitor control results daily
3. Add custom controls for your specific needs
4. Iterate and improve based on findings

**Dashboard URL**: `https://<org>.github.io/kickbutt-compliance/`

---

*Built with the Unix philosophy: Simple. Composable. Automated.*

**Last Updated**: November 2025
