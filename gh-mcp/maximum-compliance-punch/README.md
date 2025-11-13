# 🛡️ Kickbutt Compliance

**Enterprise GitHub Security & Compliance Trust Center**

A real-time compliance monitoring dashboard for GitHub Enterprise Cloud that queries workflow statuses and displays them on a beautiful, automatically-updating static site.

*Built in the spirit of Rob Pike and Ken Thompson: simple, composable, and effective.*

---

## 🎯 What Is This?

Kickbutt Compliance is a **trust center** (like [Vanta](https://www.vanta.com/trust) or [Anthropic's Trust Center](https://trust.anthropic.com/)) for your GitHub organization. It:

- ✅ Runs security control checks via GitHub Actions workflows
- 🔄 Updates automatically every hour
- 📊 Displays real-time status on a GitHub Pages site  
- 🎨 Uses a clean, professional Vanta-inspired design
- 🔧 Requires zero external dependencies

---

## 🏗️ Architecture

```
┌──────────────────────────────────────────┐
│     Hourly Cron Schedule (0 * * * *)     │
└─────────────────┬────────────────────────┘
                  │
                  ▼
┌──────────────────────────────────────────┐
│   Control Workflows (Run independently)   │
│   • control-org-settings.yml      (6h)   │
│   • control-repo-visibility.yml   (6h)   │
│   • control-repo-rulesets.yml     (6h)   │
│   • control-org-custom-role.yml   (6h)   │
└──────────────────────────────────────────┘
                  │
                  ▼
┌──────────────────────────────────────────┐
│    Update Trust Center Workflow (1h)     │
│   1. Query GitHub Actions API            │
│   2. Get latest run for each control     │
│   3. Generate status.json                │
│   4. Update gh-pages branch              │
└─────────────────┬────────────────────────┘
                  │
                  ▼
┌──────────────────────────────────────────┐
│         GitHub Pages (gh-pages)          │
│   • Serves trust-center/index.html       │
│   • Loads status.json via JavaScript     │
│   • Updates UI with control statuses     │
└──────────────────────────────────────────┘
```

**Key Principle**: Query, don't store. The trust center queries the GitHub API directly for workflow statuses rather than having each workflow write status files.

---

## 🚀 Quick Start

### 1. Clone or Create Repository

```bash
gh repo create kickbutt-compliance --private --clone
cd kickbutt-compliance
```

### 2. Copy Files

Copy all files from this template into your repo:
- `.github/workflows/` - All workflow files
- `scripts/` - Status generator script
- `trust-center/` - HTML, CSS, JS for the site

### 3. Configure GitHub Actions

**Settings** → **Actions** → **General**:
- ✅ Workflow permissions: **Read and write**
- ✅ Allow GitHub Actions to create and approve pull requests

### 4. Enable GitHub Pages

**Settings** → **Pages**:
- Source: **Deploy from a branch**
- Branch: **gh-pages** / `(root)`

### 5. Initial Deployment

```bash
# Run update workflow manually
gh workflow run update-trust-center.yml

# Wait 1-2 minutes, then visit:
# https://YOUR-ORG.github.io/kickbutt-compliance/
```

**📖 Full setup instructions**: See [DEPLOYMENT.md](DEPLOYMENT.md)

---

## 📊 Control Workflows

### 🔐 Organization Settings
**File**: `control-org-settings.yml`  
**Checks**:
- Two-factor authentication requirement
- Default repository permissions
- Member repository creation settings

**Schedule**: Every 6 hours

### 👁️ Repository Visibility
**File**: `control-repo-visibility.yml`  
**Checks**:
- Public vs private repository ratio
- Sensitive repositories are private
- Repository visibility compliance

**Schedule**: Every 6 hours

### 📋 Repository Rulesets
**File**: `control-repo-rulesets.yml`  
**Checks**:
- Organization-level rulesets
- Branch protection on critical repos
- Required status checks enforcement

**Schedule**: Every 6 hours

### 👤 Custom Roles
**File**: `control-org-custom-role.yml`  
**Checks**:
- Custom role definitions
- Recommended roles exist
- No overly permissive roles

**Schedule**: Every 6 hours

---

## 🎨 Features

### Dynamic Status Dashboard

- **Overall Status Badge**: Shows operational/degraded/failed state
- **Control Status Table**: Individual workflow status with icons
- **Last Updated Timestamps**: Shows how fresh the data is
- **Direct Links**: Click through to workflow runs for details

### Automatic Updates

- Runs every hour via GitHub Actions cron
- No manual intervention required
- Git commits updates to gh-pages branch

### Clean, Professional Design

- Dark theme inspired by Vanta
- Responsive layout (mobile-friendly)
- Icons and visual indicators (✅ ❌ ⏳ ❓)
- Smooth animations and transitions

---

## 🔧 Customization

### Change Update Frequency

Edit `.github/workflows/update-trust-center.yml`:

```yaml
on:
  schedule:
    - cron: '0 * * * *'     # Every hour
    - cron: '*/30 * * * *'  # Every 30 minutes
    - cron: '0 */4 * * *'   # Every 4 hours
```

### Add More Controls

1. Create new workflow: `.github/workflows/control-your-check.yml`
2. Add to control list in `scripts/generate-trust-center.sh`:

```bash
CONTROL_WORKFLOWS=(
    "control-org-settings.yml"
    "control-repo-visibility.yml"
    "control-repo-rulesets.yml"
    "control-org-custom-role.yml"
    "control-your-check.yml"  # ← Add this
)
```

### Customize Styling

Edit `trust-center/style.css` - uses CSS variables:

```css
:root {
    --primary-bg: #0d1117;
    --success: #238636;
    --error: #da3633;
    /* Change these to customize colors */
}
```

---

## 📁 Project Structure

```
kickbutt-compliance/
├── .github/
│   └── workflows/
│       ├── control-org-settings.yml      # Check org settings
│       ├── control-repo-visibility.yml    # Check repo visibility
│       ├── control-repo-rulesets.yml      # Check rulesets
│       ├── control-org-custom-role.yml    # Check custom roles
│       └── update-trust-center.yml        # Update the site (hourly)
├── scripts/
│   └── generate-trust-center.sh           # Query API & generate JSON
├── trust-center/
│   ├── index.html                         # Main page
│   ├── style.css                          # Styling
│   ├── status.js                          # Load & display status
│   └── status.json                        # Generated status data
├── README.md                              # This file
└── DEPLOYMENT.md                          # Detailed setup guide
```

---

## 🛠️ How It Works

### The Pipeline

1. **Control workflows run** (every 6 hours) and perform compliance checks
2. **Update workflow runs** (every hour) and:
   - Uses `gh` CLI to query GitHub Actions API
   - Gets the latest run for each control workflow
   - Extracts status, timestamp, and run URL
   - Generates `trust-center/status.json`
3. **Git commits to gh-pages** branch
4. **GitHub Pages** serves the updated site
5. **JavaScript loads status.json** and updates the UI

### The Script

`scripts/generate-trust-center.sh` is a simple bash script that:

```bash
# For each control workflow:
for workflow in "${CONTROL_WORKFLOWS[@]}"; do
  # 1. Get workflow ID from filename
  workflow_id=$(gh api "/repos/${ORG}/${REPO}/actions/workflows" ...)
  
  # 2. Get latest run
  run_data=$(gh api ".../workflows/${workflow_id}/runs?per_page=1" ...)
  
  # 3. Extract status, timestamp, URL
  
  # 4. Build JSON object
done

# Generate final status.json
echo "$final_json" > trust-center/status.json
```

Simple, composable, Unix-style. No fancy frameworks or dependencies.

---

## 🔍 Troubleshooting

### Controls showing "unknown"

Run each control manually once:

```bash
gh workflow run control-org-settings.yml
gh workflow run control-repo-visibility.yml
gh workflow run control-repo-rulesets.yml
gh workflow run control-org-custom-role.yml
```

### GitHub Pages not updating

1. Check that `gh-pages` branch exists
2. Verify **Settings** → **Pages** is set to `gh-pages` branch
3. Check workflow runs in **Actions** tab for errors

### Workflows failing

1. Verify **Settings** → **Actions** has "Read and write" permissions
2. Check workflow logs for specific errors
3. See [DEPLOYMENT.md](DEPLOYMENT.md) troubleshooting section

---

## 🔒 Security

- Uses GitHub's automatic `GITHUB_TOKEN` (scoped, secure)
- No external API calls or third-party services
- All data stays within GitHub
- Repository can be private while site is public

---

## 📚 Tech Stack

- **Backend**: Bash, GitHub CLI (`gh`), `jq`
- **Frontend**: HTML, CSS, Vanilla JavaScript
- **Platform**: GitHub Actions, GitHub Pages
- **No external dependencies**

Simple tools, simple stack. It just works.

---

## 💡 Philosophy

This project embodies the Unix philosophy:

1. **Do one thing well**: Monitor GitHub compliance
2. **Composable**: Uses standard tools (bash, git, jq, gh)
3. **Text-based**: Data in JSON, config in YAML
4. **No magic**: Clear, readable code
5. **Minimal dependencies**: Works with GitHub and standard Unix tools

As Pike and Thompson would say: *"Perfection is achieved not when there is nothing more to add, but when there is nothing left to take away."*

---

## 🤝 Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Test thoroughly
5. Submit a pull request

---

## 📄 License

MIT License - Use freely, attribute generously.

---

## 🆘 Support

- 📖 [Deployment Guide](DEPLOYMENT.md)
- 🐛 [Open an Issue](../../issues)
- 💬 Ask questions in Discussions

---

## 🎯 Roadmap

- [ ] Add more control workflows (SSH keys, webhooks, secrets)
- [ ] Support for multiple orgs
- [ ] Compliance reporting exports
- [ ] Slack/email notifications
- [ ] Historical trend graphs

---

**Built with ❤️ by DevOps engineers who appreciate simplicity**

*"The best code is no code. The second best is simple code."*

---

**Live Demo**: 🔗 `https://YOUR-ORG.github.io/kickbutt-compliance/`
