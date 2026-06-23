# 🚀 Kickbutt Compliance - Deployment Guide

**Simple, elegant compliance monitoring for GitHub Enterprise Cloud**

*Built in the spirit of Pike & Thompson: simple, composable, effective.*

---

## 🎯 What You're Building

A trust center website that:
- Displays real-time status of GitHub security controls
- Updates automatically every hour
- Queries GitHub Actions API for workflow status
- Deploys to GitHub Pages with zero configuration

**Architecture Philosophy**: Query, don't store. Simple scripts over complex systems.

---

## 📋 Prerequisites

### Required
- GitHub Enterprise Cloud organization
- A repository (can be private or public)
- GitHub Actions enabled
- 5 minutes of your time

### Recommended
- GitHub CLI (`gh`) installed locally for testing
- Admin access to org settings for best results

---

## 🛠️ Step 1: Create the Repository

```bash
# Create a new private repository
gh repo create kickbutt-compliance --private --clone

cd kickbutt-compliance
```

Or use the GitHub UI:
1. Go to your organization
2. Click "New repository"
3. Name it `kickbutt-compliance`
4. Make it private
5. Clone it locally

---

## 📦 Step 2: Add the Code

Copy all files from this template into your repository:

```bash
# If you have the template locally
cp -r kickbutt-compliance-template/* kickbutt-compliance/
cd kickbutt-compliance

# Directory structure should be:
# .github/workflows/
#   ├── control-org-settings.yml
#   ├── control-repo-visibility.yml
#   ├── control-repo-rulesets.yml
#   ├── control-org-custom-role.yml
#   └── update-trust-center.yml
# scripts/
#   └── generate-trust-center.sh
# trust-center/
#   ├── index.html
#   ├── style.css
#   └── status.js
# README.md
# DEPLOYMENT.md

# Make script executable
chmod +x scripts/generate-trust-center.sh

# Commit and push
git add .
git commit -m "Initial commit: Trust center setup"
git push origin main
```

---

## 🔑 Step 3: Configure Repository Settings

### Enable GitHub Actions

1. Go to **Settings** → **Actions** → **General**
2. Under "Actions permissions":
   - Select **Allow all actions and reusable workflows**
3. Under "Workflow permissions":
   - Select **Read and write permissions** ✅
   - Check **Allow GitHub Actions to create and approve pull requests** ✅
4. Click **Save**

### Enable GitHub Pages

1. Go to **Settings** → **Pages**
2. Under "Build and deployment":
   - **Source**: Deploy from a branch
   - **Branch**: `gh-pages` / `(root)`
   - If `gh-pages` doesn't exist yet, select `main` for now
3. Click **Save**

---

## 🎬 Step 4: Initial Deployment

### Run the Update Workflow Manually

1. Go to **Actions** tab
2. Click **Update Trust Center** workflow
3. Click **Run workflow** → **Run workflow**
4. Wait 1-2 minutes for completion

This will:
- Query all control workflow statuses
- Generate `status.json`
- Create `gh-pages` branch
- Deploy the trust center

### Verify Deployment

1. Go to **Settings** → **Pages**
2. You should see: "Your site is published at `https://YOUR-ORG.github.io/kickbutt-compliance/`"
3. Click the link to view your trust center
4. If `gh-pages` wasn't available before, go back and change the branch to `gh-pages` now

---

## 🧪 Step 5: Run Control Workflows

Trigger each control to populate initial data:

```bash
# Using GitHub CLI
gh workflow run control-org-settings.yml
gh workflow run control-repo-visibility.yml
gh workflow run control-repo-rulesets.yml
gh workflow run control-org-custom-role.yml

# Wait 2-3 minutes, then update the trust center
gh workflow run update-trust-center.yml
```

Or use the GitHub UI:
1. Go to **Actions** tab
2. Select each workflow from the left sidebar
3. Click **Run workflow** → **Run workflow**

---

## ⏰ Step 6: Verify Hourly Updates

The trust center will update automatically every hour. No configuration needed.

Check the schedule in `.github/workflows/update-trust-center.yml`:

```yaml
on:
  schedule:
    - cron: '0 * * * *'  # Every hour at minute 0
```

---

## 🎨 Customization

### Change Update Frequency

Edit `.github/workflows/update-trust-center.yml`:

```yaml
# Every 30 minutes
- cron: '*/30 * * * *'

# Every 4 hours
- cron: '0 */4 * * *'

# Twice daily (midnight and noon UTC)
- cron: '0 0,12 * * *'
```

### Add More Controls

1. **Create new workflow** in `.github/workflows/control-your-check.yml`
2. **Add to the list** in `scripts/generate-trust-center.sh`:

```bash
CONTROL_WORKFLOWS=(
    "control-org-settings.yml"
    "control-repo-visibility.yml"
    "control-repo-rulesets.yml"
    "control-org-custom-role.yml"
    "control-your-check.yml"  # Add this
)
```

3. Commit and push

### Customize Styling

Edit `trust-center/style.css` to change colors, fonts, layout, etc.

The CSS uses CSS variables for easy theming:

```css
:root {
    --primary-bg: #0d1117;
    --success: #238636;
    --error: #da3633;
    /* ... */
}
```

---

## 🔍 Troubleshooting

### "No runs found" for controls

**Problem**: Trust center shows "unknown" status for all controls

**Solution**:
```bash
# Manually trigger each control once
gh workflow run control-org-settings.yml
gh workflow run control-repo-visibility.yml
gh workflow run control-repo-rulesets.yml
gh workflow run control-org-custom-role.yml

# Wait 2 minutes, then update
gh workflow run update-trust-center.yml
```

### GitHub Pages not deploying

**Problem**: Trust center URL shows 404

**Solutions**:
1. Check **Settings** → **Pages** - ensure `gh-pages` branch is selected
2. Run the update workflow manually once to create `gh-pages`
3. Wait 1-2 minutes after workflow completes
4. Check **Actions** tab for any failed deployments

### Workflows failing with authentication errors

**Problem**: Workflows fail with "Resource not accessible by integration"

**Solution**:
1. Go to **Settings** → **Actions** → **General**
2. Ensure "Read and write permissions" is selected
3. Re-run failed workflows

### Control workflows always fail

**Problem**: A specific control always shows "failure" status

**Solution**:
1. Click the "View Run →" link in the trust center
2. Review the workflow logs
3. The control might be correctly identifying a real compliance issue
4. Fix the underlying issue or adjust the control's validation logic

---

## 🧪 Local Testing

### Test the Status Generator Script

```bash
# Set environment variables
export GITHUB_REPOSITORY_OWNER="your-org"
export GITHUB_REPOSITORY="your-org/kickbutt-compliance"
export GH_TOKEN="ghp_your_token_here"

# Authenticate gh CLI
echo "$GH_TOKEN" | gh auth login --with-token

# Run the script
./scripts/generate-trust-center.sh

# Check the output
cat trust-center/status.json | jq '.'
```

### Serve the Trust Center Locally

```bash
cd trust-center
python3 -m http.server 8000

# Visit: http://localhost:8000
```

---

## 📊 Understanding Control Status

### Status Values

- **✅ success**: Control passed all checks
- **❌ failure**: Control found compliance issues
- **⏳ pending**: Workflow is currently running
- **❓ unknown**: No workflow run found

### Overall Status

- **Operational**: All controls passing
- **Degraded**: Some controls are pending or unknown
- **Failed**: One or more controls failed

---

## 🔒 Security Best Practices

### 1. Keep Repository Private

The trust center can be public, but keep the repository private to protect workflow logic and tokens.

### 2. Use GITHUB_TOKEN

The workflows use the automatic `GITHUB_TOKEN` which has scoped permissions. This is secure by default.

### 3. Review Workflow Logs

Regularly check workflow runs for any suspicious activity.

### 4. Enable Branch Protection

Protect the `main` branch to prevent unauthorized changes:

```bash
# Using GitHub CLI
gh api -X PUT /repos/{owner}/{repo}/branches/main/protection \
  -f required_pull_request_reviews[required_approving_review_count]=1 \
  -f enforce_admins=true
```

---

## 🎉 You're Done!

Your trust center is now live and will update automatically every hour.

**Next Steps**:
1. Share the trust center URL with stakeholders
2. Monitor control statuses regularly
3. Adjust control logic as needed
4. Add more controls for comprehensive coverage

---

## 📚 Additional Resources

- [GitHub Actions Documentation](https://docs.github.com/actions)
- [GitHub Pages Documentation](https://docs.github.com/pages)
- [GitHub API Documentation](https://docs.github.com/rest)
- [GitHub CLI Documentation](https://cli.github.com/manual/)

---

**Questions?** Open an issue in this repository.

**Built with ❤️ in the Unix tradition: do one thing and do it well.**
