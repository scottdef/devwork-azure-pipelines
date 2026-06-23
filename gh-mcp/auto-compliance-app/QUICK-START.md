# ⚡ KickButt Compliance - 5-Minute Quick Start

Get your compliance dashboard live in 5 minutes.

---

## Prerequisites

```bash
# Required
- GitHub Enterprise Cloud org
- GitHub CLI installed: gh --version
- Admin access to create repos

# Verify
gh auth status
gh auth login  # If needed
```

---

## 🚀 Quick Deploy

### 1. Create & Setup (60 seconds)

```bash
# Create repository
gh repo create <YOUR-ORG>/kickbutt-compliance \
  --private \
  --description "Compliance Dashboard" \
  --clone

cd kickbutt-compliance

# Copy files
cp -r /path/to/kickbutt-compliance/* .

# Push
git add .
git commit -m "🚀 Initial deployment"
git push origin main
```

### 2. Enable Pages (30 seconds)

```bash
# Enable GitHub Pages
gh repo edit --enable-pages --pages-branch main --pages-path /docs

# Configure Actions
gh api -X PATCH repos/:owner/:repo \
  -f allow_actions=all \
  -f default_workflow_permissions=read_write \
  -f can_create_pull_request=true
```

### 3. Trigger Workflows (90 seconds)

```bash
# Run all controls
for workflow in control-org-settings control-repo-visibility \
                control-repo-rulesets control-org-custom-role; do
  gh workflow run ${workflow}.yml
done

# Wait 60 seconds
sleep 60

# Aggregate results
gh workflow run aggregate-status.yml

# Wait 30 seconds  
sleep 30

# Deploy to Pages
gh workflow run pages-deploy.yml
```

### 4. Access Dashboard (10 seconds)

```bash
# Get your URL
gh api repos/:owner/:repo/pages --jq '.html_url'

# Open in browser
open $(gh api repos/:owner/:repo/pages --jq '.html_url')
```

---

## ✅ Verify It Works

```bash
# Check workflow runs
gh run list --limit 10

# Check Pages deployment
gh api repos/:owner/:repo/pages/builds/latest --jq '.status'

# Should see: "built"
```

---

## 🎯 One-Liner Deploy

```bash
# COPY THIS ENTIRE COMMAND
gh repo create <YOUR-ORG>/kickbutt-compliance --private --clone && \
cd kickbutt-compliance && \
cp -r /path/to/kickbutt-compliance/* . && \
git add . && git commit -m "🚀 Deploy" && git push && \
gh repo edit --enable-pages --pages-branch main --pages-path /docs && \
for w in control-org-settings control-repo-visibility control-repo-rulesets control-org-custom-role; do gh workflow run ${w}.yml; done && \
sleep 60 && gh workflow run aggregate-status.yml && sleep 30 && \
gh workflow run pages-deploy.yml && \
echo "✅ Dashboard will be live at: $(gh api repos/:owner/:repo/pages --jq '.html_url')"
```

---

## 🔧 Optional: Add Admin Token

For enhanced org-level checks:

```bash
# Create token at: https://github.com/settings/tokens
# Scopes needed: admin:org (read)

# Add as secret
gh secret set GH_ADMIN_TOKEN
# Paste token when prompted
```

---

## 📊 Dashboard Features

Your live dashboard shows:

- ✅ **4 Security Controls**
  - Organization Settings
  - Repository Visibility  
  - Repository Rulesets
  - Custom Roles

- 🔄 **Auto-Updates**
  - Runs every hour
  - Real-time status

- 📈 **Compliance Areas**
  - Organization Security
  - Repository Governance

---

## 🎨 What You'll See

```
┌─────────────────────────────────────────┐
│     🛡️ Compliance Trust Center          │
│                                          │
│  Last updated: 2025-11-13 14:30 UTC     │
└─────────────────────────────────────────┘

┌─────────────────────────────────────────┐
│  🔐 Organization Security      ✅        │
│    • Organization Settings     ✅        │
│    • Custom Roles              ✅        │
└─────────────────────────────────────────┘

┌─────────────────────────────────────────┐
│  📋 Repository Governance      ✅        │
│    • Repository Visibility     ✅        │
│    • Repository Rulesets       ✅        │
└─────────────────────────────────────────┘
```

---

## 🐛 Quick Troubleshooting

### Dashboard shows blank page?

```bash
# Check Pages status
gh api repos/:owner/:repo/pages

# Redeploy
gh workflow run pages-deploy.yml
```

### Controls not showing?

```bash
# Re-run controls
gh workflow run control-org-settings.yml
sleep 60
gh workflow run aggregate-status.yml
```

### Permission denied errors?

1. Go to: Settings → Actions → General
2. Enable: "Read and write permissions"
3. Enable: "Allow GitHub Actions to create and approve pull requests"

---

## 📚 Next Steps

1. ✅ **Verify Dashboard**: Check all controls are green
2. 🔧 **Customize**: Add your own controls (see DEPLOYMENT-GUIDE.md)
3. 📊 **Monitor**: Review daily, address any red status
4. 🚀 **Share**: Give URL to your team

---

## 🆘 Need Help?

See the complete guide:
```bash
cat DEPLOYMENT-GUIDE.md
cat README.md
```

Or check workflow logs:
```bash
gh run list
gh run view <run-id> --log
```

---

**That's it! Your compliance dashboard is live. 🎉**

*Built in the spirit of Unix: Simple. Fast. Effective.*
