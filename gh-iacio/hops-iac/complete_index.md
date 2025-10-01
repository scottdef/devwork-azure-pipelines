# Complete GitHub Governance & HEREDOC Workflows Index

*Everything you need, organized cleanly. The Unix way.*

## 📋 Quick Navigation

### Core Systems

1. **[GitHub Governance with Terraform](#github-governance)**
2. **[IssueOps System](#issueops)**  
3. **[HEREDOC Workflows](#heredoc-workflows)**

### Quick Start Guides

- [5-Minute Setup](#5-minute-setup)
- [Common Tasks](#common-tasks)
- [Troubleshooting](#troubleshooting)

---

## GitHub Governance

**Purpose**: Declarative GitHub organization management via Terraform and YAML

### Core Files

| File | Purpose |
|------|---------|
| `README.md` | Main documentation |
| `SETUP.md` | Setup instructions |
| `terraform/main.tf` | Terraform core |
| `terraform/repos.tf` | Repository management |
| `terraform/teams.tf` | Team management |
| `repo-yamls/*.yml` | Repository definitions |
| `team-yamls/*.yml` | Team definitions |

### Workflows

| Workflow | Trigger | Purpose |
|----------|---------|---------|
| `terraform-plan.yml` | PR to main | Show changes |
| `terraform-apply.yml` | PR merged | Apply changes |
| `workflow-trigger.yml` | Manual | Orchestration |

### Key Concepts

```yaml
# Define a repository
repos:
  - name: my-service
    team: platform-team
    visibility: private

# Define a team
teams:
  - name: platform-team
    ad_group: "eng-platform"  # Auto-creates 3 subteams
    members:
      - username: alice
        role: maintainer
```

### Commands

```bash
# Validate
make validate

# Plan changes
make plan

# Apply changes
make apply

# Add repo
vim repo-yamls/team-repos.yml
git commit && git push  # Opens PR

# Add team member
vim team-yamls/team.yml
git commit && git push  # Opens PR
```

### Features

- ✅ YAML-driven configuration
- ✅ Automatic subteam creation
- ✅ CODEOWNERS generation
- ✅ Branch protection rules
- ✅ Complete audit trail

---

## IssueOps

**Purpose**: Infrastructure management via GitHub Issues (no YAML editing required)

### Core Files

| File | Purpose |
|------|---------|
| `ISSUEOPS.md` | Full documentation |
| `ISSUEOPS-QUICKSTART.md` | Quick start guide |
| `ISSUEOPS-EXAMPLES.md` | Usage examples |
| `.github/ISSUE_TEMPLATE/*.yml` | Issue forms |
| `.github/workflows/issueops-*.yml` | Automation |

### Operations

| Operation | Issue Template | Workflow | Approvals |
|-----------|----------------|----------|-----------|
| Create Repo | `create-repo.yml` | `issueops-create-repo.yml` | 2 required |
| Delete Repo | `delete-repo.yml` | `issueops-delete-repo.yml` | 2 required |
| Add Member | `add-team-member.yml` | `issueops-add-member.yml` | 2 required |

### Workflows

| Workflow | Purpose |
|----------|---------|
| `issueops-approval.yml` | Track approvals (👍 or `/approve`) |
| `issueops-create-repo.yml` | Create repository automation |
| `issueops-delete-repo.yml` | Delete repository automation |
| `issueops-add-member.yml` | Add team member automation |
| `issueops-cleanup.yml` | Clean up closed issues |

### User Flow

```
1. User creates issue with form
2. Platform team approves (👍 or /approve)
3. 2 approvals → Automation runs
4. YAML updated → PR created
5. PR reviewed → Merged
6. Terraform applies → Resource created
```

### Commands

```bash
# Bootstrap IssueOps
./scripts/bootstrap-issueops.sh

# Test setup
./scripts/test-issueops.sh

# View dashboard
make issueops-dashboard

# Approve request
make issueops-approve ISSUE_NUM=42

# View pending
make issueops-pending
```

### Features

- ✅ Web-based (no CLI needed)
- ✅ Approval workflow
- ✅ Automatic PR creation
- ✅ Complete automation
- ✅ Audit trail in issues

---

## HEREDOC Workflows

**Purpose**: Generate files in repositories using bash HEREDOC scripts

### Core Files

| File | Purpose |
|------|---------|
| `HEREDOC-WORKFLOW-GUIDE.md` | Complete guide |
| `HEREDOC-CHEATSHEET.md` | Quick reference |
| `HEREDOC-WORKFLOWS-SUMMARY.md` | Summary |
| `.github/workflows/generate-*.yml` | Workflows |
| `scripts/test-heredoc-generation.sh` | Testing |

### Workflows

| Workflow | Purpose | Complexity |
|----------|---------|------------|
| `generate-files.yml` | Full-featured, any repo | Advanced |
| `generate-files-simple.yml` | Current repo only | Simple |
| `cross-repo-generate.yml` | Complete config suite | Advanced |
| `generate-docker-files.yml` | Docker setup | Practical |
| `generate-k8s-manifests.yml` | Kubernetes manifests | Practical |

### Basic Syntax

```yaml
# No variable expansion (literal)
- name: Generate
  run: |
    cat > file.txt <<'EOF'
    Content with $HOME literal
    EOF

# With variable expansion
- name: Generate
  run: |
    cat > file.txt <<EOF
    Content with $(date)
    User: $USER
    EOF
```

### Commands

```bash
# Test locally
./scripts/test-heredoc-generation.sh

# Generate in current repo
make generate-local

# Generate in remote repo
make generate-remote TARGET_REPO=org/repo

# Generate with custom workflow
./scripts/generate-heredoc-workflow.sh

# Run via CLI
gh workflow run generate-files.yml \
  -f target_repo="org/repo"
```

### Features

- ✅ Generate any file type
- ✅ Cross-repository support
- ✅ Automatic PR creation
- ✅ Template support
- ✅ Validation included

---

## 5-Minute Setup

### GitHub Governance

```bash
# 1. Clone repo
git clone <repo-url>
cd github-governance

# 2. Configure secrets
gh secret set GH_ADMIN_TOKEN
gh secret set GITHUB_ORG

# 3. Create team
vim team-yamls/my-team.yml

# 4. Create repo
vim repo-yamls/my-repos.yml

# 5. Validate and push
make validate
git add . && git commit -m "Initial setup"
git push
```

### IssueOps

```bash
# 1. Bootstrap
./scripts/bootstrap-issueops.sh

# 2. Test
./scripts/test-issueops.sh

# 3. Create issue
# Go to: Issues → New Issue → "Create Repository"

# 4. Approve (as platform-team member)
# React with 👍 or comment /approve

# 5. Watch automation
# Check Actions tab
```

### HEREDOC Workflows

```bash
# 1. Test locally
./scripts/test-heredoc-generation.sh

# 2. Run workflow
gh workflow run generate-files.yml \
  -f target_repo="your-org/your-repo"

# 3. Review PR
# Check the created PR

# 4. Merge
# Approve and merge PR
```

---

## Common Tasks

### Add a New Repository

**Via YAML (Manual)**:
```bash
vim repo-yamls/team-repos.yml
# Add repository definition
make validate
git commit && git push
# Review PR, merge
```

**Via IssueOps (Web)**:
```
Issues → New Issue → "Create Repository"
Fill form → Submit → Wait for approvals
Review PR → Merge
```

### Add Team Member

**Via YAML**:
```bash
vim team-yamls/team.yml
# Add member
make validate
git commit && git push
```

**Via IssueOps**:
```
Issues → New Issue → "Add Team Member"
Fill form → Submit → Wait for approvals
```

### Generate Configuration Files

```bash
# Docker setup
gh workflow run generate-docker-files.yml \
  -f target_repo="org/repo" \
  -f app_name="myapp" \
  -f language="node"

# Kubernetes manifests
gh workflow run generate-k8s-manifests.yml \
  -f target_repo="org/repo" \
  -f app_name="myapp" \
  -f image="myapp:latest"

# Custom files
./scripts/generate-heredoc-workflow.sh
# Follow prompts
```

### Monitor Everything

```bash
# GitHub Governance
make show
make output

# IssueOps
make issueops-dashboard
make issueops-pending

# HEREDOC Workflows
gh run list
gh run watch
```

---

## Troubleshooting

### GitHub Governance

**Problem**: Terraform plan fails

**Solution**:
```bash
# Validate YAML
make validate

# Check Terraform
cd terraform && terraform init
terraform validate

# View logs
gh run view --log
```

**Problem**: Team not found

**Solution**:
```bash
# List teams
python3 scripts/yaml-helpers.py list-teams

# Check team file exists
ls team-yamls/
```

### IssueOps

**Problem**: Approvals not counted

**Solution**:
```bash
# Check team membership
gh api orgs/ORG/teams/platform-team/members

# View workflow logs
gh run list --workflow issueops-approval.yml
```

**Problem**: Workflow not triggered

**Solution**:
```bash
# Manually trigger
gh workflow run issueops-create-repo.yml -f issue_number=42

# Check workflow permissions
gh api repos/OWNER/REPO/actions/permissions
```

### HEREDOC Workflows

**Problem**: Variables not expanding

**Solution**: Use `<<EOF` not `<<'EOF'`

**Problem**: Script not executable

**Solution**: Add `chmod +x script.sh`

**Problem**: PR not created

**Solution**:
```bash
# Check GH_ADMIN_TOKEN
gh secret list

# Check gh CLI auth
gh auth status
```

---

## File Organization

```
github-governance/
├── .github/
│   ├── ISSUE_TEMPLATE/          # IssueOps forms
│   └── workflows/               # All workflows
│       ├── terraform-*.yml      # Terraform workflows
│       ├── issueops-*.yml       # IssueOps workflows
│       └── generate-*.yml       # HEREDOC workflows
├── terraform/                   # Terraform module
│   ├── main.tf
│   ├── repos.tf
│   ├── teams.tf
│   └── modules/
├── repo-yamls/                  # Repository definitions
├── team-yamls/                  # Team definitions
├── scripts/                     # Helper scripts
│   ├── bootstrap-issueops.sh
│   ├── test-issueops.sh
│   ├── test-heredoc-generation.sh
│   ├── yaml-helpers.py
│   └── validate-yaml.py
├── templates/                   # Templates
├── Makefile                     # All commands
├── README.md                    # Main docs
├── SETUP.md                     # Setup guide
├── ISSUEOPS*.md                 # IssueOps docs
├── HEREDOC*.md                  # HEREDOC docs
└── INDEX.md                     # This file
```

---

## Documentation Map

### Getting Started
- `README.md` - Start here
- `SETUP.md` - Setup instructions
- `INDEX.md` - This file

### GitHub Governance
- `README.md` - Core concepts
- `SETUP.md` - Initial setup
- `terraform/` - Module docs

### IssueOps
- `ISSUEOPS-QUICKSTART.md` - 5-minute guide
- `ISSUEOPS.md` - Complete docs
- `ISSUEOPS-EXAMPLES.md` - Real examples
- `ISSUEOPS-INTEGRATION.md` - Architecture
- `ISSUEOPS-COMPLETE.md` - Implementation checklist

### HEREDOC Workflows
- `HEREDOC-CHEATSHEET.md` - Quick reference
- `HEREDOC-WORKFLOW-GUIDE.md` - Complete guide
- `HEREDOC-WORKFLOWS-SUMMARY.md` - Overview

---

## Command Reference

### Make Targets

```bash
# GitHub Governance
make validate           # Validate YAML
make plan              # Terraform plan
make apply             # Terraform apply
make fmt               # Format Terraform

# IssueOps
make issueops-bootstrap    # Setup IssueOps
make issueops-test         # Test IssueOps
make issueops-dashboard    # View dashboard
make issueops-approve      # Approve request

# HEREDOC
make generate-local        # Test locally
make generate-remote       # Trigger workflow
make test-heredoc          # Test syntax

# General
make help                  # Show all commands
```

### gh CLI Commands

```bash
# Workflows
gh workflow list
gh workflow run WORKFLOW.yml
gh run list
gh run view RUN_ID

# Issues (IssueOps)
gh issue create
gh issue list --label issueops
gh issue comment NUM --body "/approve"

# Pull Requests
gh pr list
gh pr view NUM
gh pr merge NUM
```

---

## Best Practices

### General
1. ✅ Test locally before CI
2. ✅ Validate before committing
3. ✅ Use descriptive commit messages
4. ✅ Review PRs carefully
5. ✅ Document changes

### GitHub Governance
1. ✅ One repo per YAML file (team-based)
2. ✅ Use consistent naming
3. ✅ Always validate YAML
4. ✅ Review Terraform plans
5. ✅ Keep teams organized

### IssueOps
1. ✅ Fill forms completely
2. ✅ Wait for approvals
3. ✅ Review generated PRs
4. ✅ Close issues when done
5. ✅ Monitor dashboard

### HEREDOC Workflows
1. ✅ Use `<<'EOF'` for literal
2. ✅ Use `<<EOF` for expansion
3. ✅ Always chmod +x scripts
4. ✅ Validate generated files
5. ✅ Test locally first

---

## Philosophy

This system embodies the Unix philosophy:

1. **Do one thing well**
   - Terraform manages GitHub
   - IssueOps provides interface
   - HEREDOC generates files

2. **Compose tools**
   - YAML → Terraform → GitHub
   - Issues → Workflows → PRs
   - HEREDOC → Git → PRs

3. **Text as interface**
   - Everything is code
   - Everything is versioned
   - Everything is reviewable

4. **Simplicity**
   - No complex frameworks
   - No enterprise bloat
   - Just clean code

---

## Getting Help

1. **Check docs** - Start with README.md
2. **Run tests** - Use test scripts
3. **View logs** - `gh run view --log`
4. **Ask team** - Ping @platform-team
5. **Create issue** - Describe problem

---

## Contributing

1. Fork repository
2. Create feature branch
3. Make changes
4. Test thoroughly
5. Submit PR with clear description

---

**Built with the spirit of Rob Pike and Ken Thompson**: Simple tools, powerful composition, text as the universal interface.

**Everything is code. Everything is versioned. Everything is auditable.**

*Ship it.* 🚀
