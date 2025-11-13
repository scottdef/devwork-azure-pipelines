# 🚀 Quick Start - Kickbutt Compliance

Get your trust center running in 5 minutes!

## Step 1: Copy Files to Your Repo

```bash
# Clone or create your repo
gh repo create kickbutt-compliance --private --clone
cd kickbutt-compliance

# Copy all files from this package
cp -r /path/to/kickbutt-compliance-package/* .

# Commit
git add .
git commit -m "Initial: Trust center setup"
git push origin main
```

## Step 2: Configure GitHub Actions

1. **Settings** → **Actions** → **General**
   - Workflow permissions: ✅ **Read and write**
   - ✅ **Allow GitHub Actions to create and approve pull requests**

## Step 3: Enable GitHub Pages

1. **Settings** → **Pages**
   - Source: **Deploy from a branch**
   - Branch: **gh-pages** / `(root)`
   - Click **Save**

## Step 4: Deploy

```bash
# Trigger the update workflow
gh workflow run update-trust-center.yml

# Wait 1-2 minutes, then visit:
# https://YOUR-ORG.github.io/kickbutt-compliance/
```

## That's It! 🎉

Your trust center will now update automatically every hour.

## Optional: Run Controls

```bash
# Run all controls to populate data
make run-controls

# Wait 2-3 minutes, then update
make update
```

## Need Help?

- 📖 [Full Deployment Guide](DEPLOYMENT.md)
- 🏗️ [Architecture Documentation](ARCHITECTURE.md)
- 🐛 Open an issue if you hit problems

---

**Live URL**: `https://YOUR-ORG.github.io/kickbutt-compliance/`
