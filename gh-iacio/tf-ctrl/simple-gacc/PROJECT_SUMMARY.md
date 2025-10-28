# GitHub Organization Settings - Complete Terraform Module with Drift Detection

> *"Complexity is the enemy of security and reliability."* - Ken Thompson

## Project Overview

Complete, production-ready Infrastructure as Code solution for managing GitHub organization settings with automated drift detection. Built with Unix philosophy: simple tools that do one thing well, composable pipelines, and zero magic.

## What's Included

### 🎯 Core Terraform Module

1. **[github_org_settings.tf](github_org_settings.tf)** - Complete resource with all 27 settings
   - Every `github_organization_settings` argument
   - Fully typed variables with validation
   - Secure-by-default enterprise configuration

2. **[provider.tf](provider.tf)** - Provider configuration
   - GitHub provider v6.6+
   - Environment variable authentication
   - Optional GitHub App auth

3. **[terraform.tfvars.example](terraform.tfvars.example)** - Example configuration
   - All variables documented
   - Three scenarios (enterprise/small team/open source)

4. **[.gitignore](.gitignore)** - Proper exclusions
   - State files, secrets, artifacts

### 🛠️ CLI Workflow

5. **[Makefile](Makefile)** - Complete build system
   - Essential: `init`, `plan`, `apply`, `destroy`
   - State: `show`, `state-list`, `refresh`, `import`
   - Dev: `vars`, `graph`, `console`
   - CI/CD: `ci-validate`, `ci-plan`, `ci-apply`
   - Cleanup: `clean`, `deep-clean`

6. **[quickstart.sh](quickstart.sh)** - Interactive setup
   - Prerequisites check
   - Credential prompt
   - Terraform initialization
   - First plan generation

### 🚨 Drift Detection System

7. **[.github/workflows/drift-detection.yml](.github/workflows/drift-detection.yml)** - GitHub Actions workflow
   - Hourly automated runs
   - Deep comparison (27 settings)
   - GitHub issue creation
   - Auto-close on resolution

8. **[test-drift-detection.sh](test-drift-detection.sh)** - Local testing
   - Test drift logic locally
   - Same comparison as workflow
   - No GitHub Actions required

### 📚 Documentation

9. **[README.md](README.md)** - Complete guide
   - Quick start
   - Configuration reference
   - Troubleshooting
   - Best practices

10. **[DRIFT_DETECTION.md](DRIFT_DETECTION.md)** - Drift detection guide
    - Architecture overview
    - Setup instructions
    - Monitoring and alerts
    - Advanced configuration

11. **[EXAMPLE_DRIFT_REPORT.md](EXAMPLE_DRIFT_REPORT.md)** - Sample drift report
    - Shows what GitHub issues look like
    - Remediation examples
    - Security impact analysis

## Architecture

### Terraform Module Flow

```
terraform.tfvars → github_org_settings.tf → GitHub API
       ↓                    ↓                     ↓
   Variables          Resource Definition    Organization
                                              Settings
```

### Drift Detection Flow

```
Cron (hourly)
    ↓
terraform plan (detect changes)
    ↓
Add data source temporarily
    ↓
terraform refresh (fetch GitHub state)
    ↓
Extract both states (resource vs data source)
    ↓
Python comparison (27 settings)
    ↓
Generate markdown report
    ↓
Create/Update GitHub issue
    ↓
Cleanup
```

## Quick Start

### 1. Initial Setup (5 minutes)

```bash
# Set authentication
export GITHUB_TOKEN="ghp_your_token_here"
export GITHUB_OWNER="your-org-name"

# Run interactive setup
./quickstart.sh

# Or manual
cp terraform.tfvars.example terraform.tfvars
# Edit terraform.tfvars
make init
make plan
make apply
```

### 2. Enable Drift Detection (2 minutes)

```bash
# 1. Copy workflow to your repo
mkdir -p .github/workflows
cp .github/workflows/drift-detection.yml .github/workflows/

# 2. Add secret to repository
# GitHub UI: Settings → Secrets → New secret
# Name: GH_ORG_ADMIN_TOKEN
# Value: Your token with admin:org scope

# 3. Enable Actions write permissions
# Settings → Actions → General → Read/Write permissions

# 4. Test it
gh workflow run drift-detection.yml -f create_issue=true
```

## Monitored Settings

### Organization Profile (8)
- billing_email, company, blog, email
- twitter_username, location, name, description

### Repository Permissions (6)
- default_repository_permission
- members_can_create_* (repositories, public, private, internal)
- members_allowed_repository_creation_type

### Security (6)
- advanced_security_enabled_for_new_repositories
- secret_scanning_enabled_for_new_repositories
- secret_scanning_push_protection_enabled_for_new_repositories
- dependabot_alerts/updates_enabled_for_new_repositories
- dependency_graph_enabled_for_new_repositories

### Pages (3)
- members_can_create_pages/public_pages/private_pages

### Projects (2)
- has_organization_projects, has_repository_projects

### Other (2)
- members_can_fork_private_repositories
- web_commit_signoff_required

**Total: 27 settings monitored**

## Key Features

### Security-First Design
✅ All security features enabled by default
✅ Repository creation disabled (enforce IaC)
✅ Default permission: read-only
✅ Secret scanning with push protection
✅ Dependabot alerts and updates

### Enterprise-Grade
✅ Complete argument coverage (27 settings)
✅ Input validation on enums
✅ Remote state backend examples
✅ GitHub App authentication support
✅ Hourly drift detection

### Developer-Friendly
✅ Makefile for common operations
✅ Interactive quickstart script
✅ Local drift testing
✅ Comprehensive documentation
✅ Example configurations

### Production-Ready
✅ CI/CD integration examples
✅ Automated drift detection
✅ Issue-based alerting
✅ Auto-resolution tracking
✅ Audit trail via issues

## File Tree

```
.
├── .github/
│   └── workflows/
│       └── drift-detection.yml        # Hourly drift detection workflow
├── github_org_settings.tf             # Main Terraform resource
├── provider.tf                        # Provider configuration
├── terraform.tfvars.example           # Example configuration
├── .gitignore                         # Git exclusions
├── Makefile                           # CLI workflow automation
├── quickstart.sh                      # Interactive setup
├── test-drift-detection.sh            # Local drift testing
├── README.md                          # Main documentation
├── DRIFT_DETECTION.md                 # Drift detection guide
└── EXAMPLE_DRIFT_REPORT.md            # Sample drift report
```

## Common Workflows

### Daily Operations

```bash
# Check for drift locally
./test-drift-detection.sh

# Review configuration
make vars

# Check state
make state-list
make show
```

### Making Changes

```bash
# Edit configuration
vim terraform.tfvars

# Review changes
make plan

# Apply changes
make apply

# Verify
./test-drift-detection.sh
```

### Troubleshooting

```bash
# Import existing org settings
make import

# Refresh state from GitHub
make refresh

# View dependency graph
make graph

# Start interactive console
make console
```

### CI/CD Pipeline

```bash
# In GitHub Actions
make ci-validate  # Validate + format check
make ci-plan      # Generate plan for review
make ci-apply     # Apply changes
```

## Security Considerations

### Secrets Management
- ✅ Never commit `GITHUB_TOKEN` to repository
- ✅ Use GitHub Secrets for Actions
- ✅ Rotate tokens regularly (set calendar reminders)
- ✅ Minimum scope: `admin:org` + `repo`

### State Protection
- ✅ Use remote state backend (S3, Terraform Cloud)
- ✅ Enable state locking (DynamoDB)
- ✅ Encrypt state at rest
- ✅ Never commit `.tfstate` files

### Drift Detection
- ✅ Hourly monitoring catches unauthorized changes
- ✅ Issue-based alerts for visibility
- ✅ Audit trail via GitHub issues
- ✅ Auto-resolution tracking

### Least Privilege
- ✅ Repository creation disabled by default
- ✅ Default permission: read-only
- ✅ Explicit grants via teams/IaC
- ✅ No public repo creation without approval

## Advanced Usage

### Multi-Organization

Create separate directories:
```
terraform/
├── org-1/
│   ├── terraform.tfvars
│   └── backend.tf
├── org-2/
│   ├── terraform.tfvars
│   └── backend.tf
└── shared/
    ├── github_org_settings.tf
    └── provider.tf
```

### GitHub Enterprise Server

Update `provider.tf`:
```hcl
provider "github" {
  owner    = var.github_owner
  base_url = "https://github.yourcompany.com"
}
```

### Auto-Remediation

Add to drift detection workflow (use with caution):
```yaml
- name: Auto-Apply
  if: steps.compare.outputs.has_drift == 'true'
  run: terraform apply -auto-approve
```

## Prerequisites

- **Terraform**: >= 1.5.0
- **GitHub Provider**: ~> 6.6 (integrations/github)
- **Python**: >= 3.7 (for drift detection)
- **Make**: Any version
- **jq**: For JSON parsing
- **GitHub CLI** (optional): For testing workflows

## License

MIT - Do whatever you want with it. No warranty.

## Philosophy

This project follows Unix philosophy:
1. **Do one thing well**: Manage organization settings, nothing else
2. **Composable**: Works with standard tools (make, terraform, gh)
3. **Text-based**: All config in text files, version controlled
4. **Automation**: CI/CD friendly, no manual clicks
5. **Transparency**: Full audit trail, no hidden magic

Built with the precision of C, the simplicity of Plan 9, and the pragmatism of Go.

---

## What You Should Do Next

### Immediate (5 minutes)
1. Run `./quickstart.sh`
2. Review generated plan
3. Apply configuration

### Today (30 minutes)
1. Copy workflow to `.github/workflows/`
2. Add `GH_ORG_ADMIN_TOKEN` secret
3. Enable Actions permissions
4. Test drift detection

### This Week
1. Set up remote state backend
2. Configure CI/CD pipeline
3. Document organization-specific settings
4. Train team on workflow

### Ongoing
1. Review drift detection issues
2. Keep terraform.tfvars updated
3. Rotate GitHub tokens quarterly
4. Review audit logs for unauthorized changes

---

*"Talk is cheap. Show me the code."* - Linus Torvalds

*"Simplicity is prerequisite for reliability."* - Edsger W. Dijkstra

*This is working software. Not slides. Not promises. Working. Software.*
