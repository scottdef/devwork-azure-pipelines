# 🏗️ Kickbutt Compliance - Architecture

**Design Philosophy**: Simple, composable, Unix-style automation

---

## 🎯 Core Principles

### 1. Query, Don't Store
- Controls don't write status files
- Status is queried from GitHub Actions API
- Single source of truth: GitHub's workflow runs

### 2. Stateless Generation
- Each update regenerates the entire status from scratch
- No state persistence needed
- Idempotent operations

### 3. Minimal Dependencies
- Standard Unix tools: bash, git, jq
- GitHub CLI (`gh`)
- No external APIs or services

### 4. Git as Deployment
- Changes committed to git
- GitHub Pages auto-deploys from `gh-pages` branch
- Atomic updates

---

## 📊 Data Flow

```
┌─────────────────────────────────────────────────────────┐
│                   Control Workflows                       │
│  Running independently every 6 hours, validating:        │
│  - Org settings                                          │
│  - Repo visibility                                       │
│  - Branch rulesets                                       │
│  - Custom roles                                          │
└─────────────┬───────────────────────────────────────────┘
              │ (Success/Failure recorded in Actions API)
              ▼
┌─────────────────────────────────────────────────────────┐
│              GitHub Actions API                          │
│  Stores workflow run history:                            │
│  - Workflow ID                                           │
│  - Run status (success/failure/pending)                  │
│  - Timestamp                                             │
│  - URL to run details                                    │
└─────────────┬───────────────────────────────────────────┘
              │
              ▼ (Queried every hour)
┌─────────────────────────────────────────────────────────┐
│           Update Trust Center Workflow                   │
│  1. Authenticate with GITHUB_TOKEN                       │
│  2. For each control workflow:                           │
│     a. Find workflow ID by filename                      │
│     b. Get latest run                                    │
│     c. Extract status, timestamp, URL                    │
│  3. Calculate overall status                             │
│  4. Generate status.json                                 │
│  5. Commit to gh-pages branch                            │
└─────────────┬───────────────────────────────────────────┘
              │
              ▼ (Git push triggers deployment)
┌─────────────────────────────────────────────────────────┐
│               GitHub Pages                               │
│  Serves static files from gh-pages:                      │
│  - index.html (main page)                                │
│  - style.css (styling)                                   │
│  - status.js (client-side logic)                         │
│  - status.json (data)                                    │
└─────────────┬───────────────────────────────────────────┘
              │
              ▼ (User visits site)
┌─────────────────────────────────────────────────────────┐
│                Web Browser                               │
│  1. Load index.html                                      │
│  2. Fetch status.json                                    │
│  3. Parse JSON                                           │
│  4. Update DOM with status indicators                    │
│  5. Format timestamps                                    │
│  6. Render control table                                 │
└─────────────────────────────────────────────────────────┘
```

---

## 🔧 Component Deep Dive

### Control Workflows

**Location**: `.github/workflows/control-*.yml`

**Purpose**: Execute security/compliance checks

**Structure**:
```yaml
name: Control - <Name>

on:
  schedule:
    - cron: '<schedule>'
  workflow_dispatch:

permissions:
  contents: read

jobs:
  validate-<thing>:
    runs-on: ubuntu-latest
    steps:
      - Authenticate with GitHub
      - Run validation checks
      - Exit 0 (success) or 1 (failure)
```

**Key Points**:
- Exit code determines pass/fail
- No need to write status files
- Logs contain detailed failure information
- Can be triggered manually via `workflow_dispatch`

**Schedule Strategy**:
- Controls run every 6 hours
- Staggered by 5 minutes to avoid overlap
- Can be adjusted per control

---

### Update Trust Center Workflow

**Location**: `.github/workflows/update-trust-center.yml`

**Purpose**: Query API and regenerate trust center

**Key Steps**:

1. **Setup Environment**
   ```bash
   # Install jq and GitHub CLI
   sudo apt-get install -y jq gh
   ```

2. **Authenticate**
   ```bash
   # Use automatic GITHUB_TOKEN
   echo "${{ secrets.GITHUB_TOKEN }}" | gh auth login --with-token
   ```

3. **Execute Generator Script**
   ```bash
   # Run the bash script
   ./scripts/generate-trust-center.sh
   ```

4. **Commit to gh-pages**
   ```bash
   # Switch to gh-pages branch
   git checkout gh-pages
   # Copy new files
   cp -r trust-center/* .
   # Commit and push
   git commit -m "Update trust center"
   git push origin gh-pages
   ```

**Why This Works**:
- `GITHUB_TOKEN` has API access automatically
- Git operations are atomic
- GitHub Pages rebuilds on push
- No external dependencies

---

### Status Generator Script

**Location**: `scripts/generate-trust-center.sh`

**Purpose**: Query GitHub API and generate JSON

**Algorithm**:

```bash
for each control_workflow in CONTROL_WORKFLOWS:
  # Step 1: Find workflow ID
  workflow_id = gh api workflows | filter by filename
  
  # Step 2: Get latest run
  run_data = gh api workflows/$workflow_id/runs?per_page=1
  
  # Step 3: Extract fields
  status = run_data.conclusion or run_data.status
  timestamp = run_data.created_at
  url = run_data.html_url
  
  # Step 4: Build JSON object
  control_json = {
    name: workflow_file,
    status: status,
    last_run: timestamp,
    run_url: url
  }

# Step 5: Calculate overall status
if any control has status=failure:
  overall_status = "failed"
elif any control has status=unknown/pending:
  overall_status = "degraded"
else:
  overall_status = "operational"

# Step 6: Generate final JSON
output = {
  overall_status: overall_status,
  generated_at: current_timestamp,
  controls: [all control_json objects]
}

# Step 7: Write to file
echo $output | jq '.' > trust-center/status.json
```

**Why Bash?**:
- Simple, available everywhere
- Direct shell access to `gh` CLI
- Easy to read and debug
- No compilation needed

---

### Frontend (Trust Center)

**Location**: `trust-center/`

**Components**:

#### index.html
- Semantic HTML structure
- Minimal markup
- No framework dependencies

#### style.css
- CSS custom properties for theming
- Responsive design
- Dark theme (GitHub-inspired)
- Clean, professional aesthetics

#### status.js
- Vanilla JavaScript (no jQuery, React, etc.)
- Fetches `status.json` on page load
- Updates DOM dynamically
- Formats timestamps relative to now

**Data Flow**:
```javascript
// On page load:
1. fetch('status.json')
2. Parse JSON
3. updateOverallStatus(data)
4. updateControlsTable(data.controls)
5. formatTimestamps()

// For each control:
- Map status to icon (✅ ❌ ⏳ ❓)
- Format control name (filename → readable)
- Create clickable link to workflow run
```

---

## 🔐 Security Model

### Authentication
- Uses GitHub's automatic `GITHUB_TOKEN`
- Scoped to repository
- No need for PATs or secrets

### Permissions
```yaml
permissions:
  contents: write  # For git push to gh-pages
  actions: read    # For querying workflow API
```

### Scope
- Can only access current organization
- Can only read workflow data
- Cannot modify workflows or settings

### Public/Private
- Repository can be private
- Trust center can be public (via GitHub Pages)
- No sensitive data in generated files

---

## ⚡ Performance

### Update Frequency
- **Controls**: Every 6 hours
- **Trust Center**: Every hour
- **Cost**: ~30 workflow minutes/day (free tier: 2000/month)

### Caching
- GitHub Pages has CDN
- status.json is small (~2KB)
- No database queries

### Scalability
- Handles 100+ controls with no changes
- API queries are fast (<1s per workflow)
- Total update time: <30 seconds

---

## 🧪 Testing Strategy

### Local Testing
```bash
# Test generator script
export GITHUB_REPOSITORY="org/repo"
./scripts/generate-trust-center.sh

# Test frontend
cd trust-center
cp status.json.example status.json
python3 -m http.server 8000
# Visit http://localhost:8000
```

### CI Testing
- Workflow syntax validated by GitHub
- Can add `actionlint` for stricter validation
- Generator script runs in real workflow environment

### Manual Testing
```bash
# Trigger workflows manually
make run-controls
make update

# Check results
make status
```

---

## 🔄 Extension Points

### Adding Controls
1. Create workflow: `.github/workflows/control-new.yml`
2. Add to array: `CONTROL_WORKFLOWS` in generator script
3. That's it!

### Custom Styling
- Edit CSS custom properties in `style.css`
- No build step needed
- Changes reflect immediately

### Alternative Output Formats
- Modify generator script to output different format
- Add new templates (e.g., Markdown, PDF)
- Keep JSON as canonical format

### Notifications
Add to update workflow:
```yaml
- name: Notify on failure
  if: contains(needs.*.result, 'failure')
  run: |
    # Send to Slack, email, etc.
```

---

## 📊 Comparison to Alternatives

### vs. Writing Status Files
**Status Files Approach**:
```
❌ Each control writes a file
❌ Need to commit from each workflow
❌ Merge conflicts possible
❌ Stale data if control doesn't run
```

**API Query Approach (This)**:
```
✅ Query once, get all statuses
✅ Single commit point
✅ No merge conflicts
✅ Always fresh data from source
```

### vs. External Dashboard Tools
**External Tools** (Datadog, StatusPage.io):
```
❌ Monthly cost
❌ Data leaves GitHub
❌ Complex setup
❌ Vendor lock-in
```

**This Approach**:
```
✅ Free (within GitHub Actions limits)
✅ All data in GitHub
✅ 5-minute setup
✅ Simple bash/HTML/CSS
```

---

## 🎯 Design Decisions

### Why Bash Instead of Python?
- **Simpler**: Fewer dependencies
- **Lighter**: No runtime installation
- **More Unix-y**: Direct tool composition
- **Faster**: No startup time

### Why Static Site Instead of App?
- **Simpler**: No server needed
- **Faster**: CDN-delivered HTML
- **Cheaper**: Free GitHub Pages
- **More Reliable**: No database/backend

### Why Hourly Instead of Real-Time?
- **Sufficient**: Controls run every 6 hours
- **Efficient**: Reduces API calls
- **Simple**: Cron scheduling
- **Reliable**: No webhooks to maintain

---

## 💡 Future Enhancements

### Possible Additions
1. **Historical trends**: Store status snapshots
2. **Multi-org support**: Query multiple organizations
3. **Custom metrics**: Add business-specific KPIs
4. **Alerts**: Slack/email on failure
5. **SLA tracking**: Uptime percentages
6. **PDF exports**: Generate compliance reports

### Maintaining Simplicity
Any enhancement should follow these rules:
- ✅ Keep bash as primary language
- ✅ No databases
- ✅ No external APIs
- ✅ Single-file changes when possible
- ✅ Clear documentation

---

## 📚 References

### Documentation
- [GitHub Actions Workflow Syntax](https://docs.github.com/actions/reference/workflow-syntax-for-github-actions)
- [GitHub REST API](https://docs.github.com/rest)
- [GitHub CLI Manual](https://cli.github.com/manual/)
- [GitHub Pages](https://docs.github.com/pages)

### Inspiration
- [Vanta Trust Centers](https://www.vanta.com/trust)
- [Anthropic Trust Center](https://trust.anthropic.com/)
- [Unix Philosophy](https://en.wikipedia.org/wiki/Unix_philosophy)
- [The Art of Unix Programming](http://www.catb.org/esr/writings/taoup/html/)

---

**Built with ❤️ in the spirit of Pike and Thompson**

*"Simple, composable tools that do one thing well."*
