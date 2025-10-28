# GitHub Organization Settings Import - Complete Package

*Built in the spirit of Rob Pike and Ken Thompson - simple, composable, powerful.*

## What You Get

This package provides everything needed to import and manage your GitHub organization settings with Terraform.

```
organization-settings/
├── organization.tf                  # Terraform resource definition
├── organization_variables.tf        # Input variables
├── import-org-settings.sh          # Import script (executable)
├── end-to-end-example.sh           # Complete workflow example (executable)
├── Makefile.org                    # Make targets to add to your Makefile
├── org-settings-workflow.yml       # GitHub Actions workflow
├── ORG_SETTINGS_README.md          # Comprehensive documentation
└── QUICK_REFERENCE.txt             # Quick reference card
```

## Quick Start (30 seconds)

```bash
# 1. Import your organization settings
./import-org-settings.sh your-org-name

# 2. Review the plan
cd terraform && terraform plan

# 3. Done! Adjust organization.tf as needed
```

## What is `github_organization_settings`?

The `github_organization_settings` Terraform resource manages:
- Organization profile (name, company, location, email)
- Default repository permissions
- Member capabilities (who can create repos, etc.)
- Security policies (2FA, commit signoff)
- Advanced security defaults for new repositories

**Critical:** Terraformer does NOT support this resource. You must manually create and import it.

## Files Explained

### Core Files

**organization.tf**
- The Terraform resource definition
- Includes recommended security-first defaults
- Edit this to match your desired organization state

**organization_variables.tf**
- Input variables for the organization resource
- Allows parameterization for different environments

### Tools & Scripts

**import-org-settings.sh** ⚡
- Imports existing GitHub organization settings into Terraform state
- Usage: `./import-org-settings.sh your-org-name`
- Simple, focused, does one thing well

**end-to-end-example.sh** 🎯
- Complete workflow from import to management
- Demonstrates every step with explanations
- Great for learning and first-time setup

**Makefile.org** 🔧
- Makefile targets for organization management
- Add these to your existing Makefile
- Includes: import-org, show-org, plan-org, apply-org, fetch-org-settings

### Automation

**org-settings-workflow.yml** ⚙️
- GitHub Actions workflow for automated management
- Supports manual triggers and auto-apply on main branch
- Includes security review checklist

### Documentation

**ORG_SETTINGS_README.md** 📖
- Comprehensive guide with examples
- Troubleshooting section
- Integration instructions

**QUICK_REFERENCE.txt** 📋
- One-page cheat sheet
- All essential commands and settings
- Print and keep handy

## Installation

### Method 1: Quick Setup

```bash
# Copy files to your project
cp organization.tf terraform/
cp organization_variables.tf terraform/
cp import-org-settings.sh ./
chmod +x import-org-settings.sh

# Import settings
./import-org-settings.sh your-org-name
```

### Method 2: Complete Integration

```bash
# Run the end-to-end example
./end-to-end-example.sh your-org-name

# This will:
# 1. Setup all files
# 2. Import settings
# 3. Generate comparison report
# 4. Guide you through next steps
```

### Method 3: GitHub Actions

```bash
# Add workflow to your repository
cp org-settings-workflow.yml .github/workflows/

# Trigger manually
gh workflow run org-settings.yml -f action=import
```

## The Import Process

### 1. Understanding the Resource ID

For `github_organization_settings`, the import ID is your **organization login name**:

```bash
terraform import github_organization_settings.this acme-corp
```

NOT the numeric organization ID. Just the slug you see in URLs.

### 2. What Happens During Import

```
┌─────────────────────────────┐
│  GitHub Organization        │
│  (Current Live Settings)    │
└─────────┬───────────────────┘
          │
          │ terraform import
          ▼
┌─────────────────────────────┐
│  Terraform State            │
│  (Imported Settings)        │
└─────────┬───────────────────┘
          │
          │ terraform plan (shows differences)
          ▼
┌─────────────────────────────┐
│  organization.tf            │
│  (Your Desired Settings)    │
└─────────────────────────────┘
```

### 3. After Import

1. Run `terraform plan` - you'll see differences
2. Edit `organization.tf` to match your desired state
3. Run `terraform plan` again
4. Run `terraform apply` when ready

## Security Recommendations

The included `organization.tf` has security-first defaults:

```hcl
✓ default_repository_permission = "read"
✓ members_can_create_repositories = false
✓ members_can_create_public_repositories = false
✓ web_commit_signoff_required = true
✓ advanced_security_enabled_for_new_repositories = true
✓ secret_scanning_enabled_for_new_repositories = true
```

Review and adjust based on your organization's needs.

## Integration with Existing Setup

### Add to Makefile

```makefile
include Makefile.org

# Now you have:
# - make import-org
# - make show-org
# - make plan-org
# - make apply-org
```

### Add to GitHub Actions

The workflow supports:
- Manual triggers (import, plan, apply)
- Auto-apply on push to main
- PR comments with plan output
- Security review checklist

## Verification

### Check Current Settings

```bash
# Via GitHub API
gh api /orgs/your-org | jq

# Via Terraform
cd terraform && terraform state show github_organization_settings.this

# Via Make
make fetch-org-settings
```

### Compare GitHub vs Terraform

```bash
# Show differences
cd terraform && terraform plan -target=github_organization_settings.this
```

## Common Workflows

### Initial Setup

```bash
./import-org-settings.sh acme-corp
cd terraform
terraform plan
# Edit organization.tf as needed
terraform apply
```

### Update Settings

```bash
# Edit organization.tf
vim terraform/organization.tf

# Review changes
make plan-org

# Apply changes
make apply-org
```

### Automated via GitHub Actions

```bash
# Edit and commit
git add terraform/organization.tf
git commit -m "Update org settings"
git push

# Workflow automatically runs terraform plan
# Merge PR to apply changes
```

## Troubleshooting

### "Error: Resource already exists in state"

```bash
terraform state rm github_organization_settings.this
./import-org-settings.sh your-org-name
```

### "Error: authentication error"

```bash
gh auth status
gh auth refresh -s admin:org
```

### "Settings not taking effect"

Some settings require:
- GitHub Enterprise Cloud
- Organization owner permissions
- Specific GitHub plans

Check your plan:
```bash
gh api /orgs/your-org | jq .plan
```

## Architecture Philosophy

Following Unix philosophy:
- **Do one thing well** - Each file has a single, clear purpose
- **Composable** - Scripts and Make targets work together
- **Transparent** - Plain text, readable configuration
- **Automatable** - CLI-first, GitHub Actions-ready

This is infrastructure as it should be: simple, understandable, maintainable.

## Required Permissions

### GitHub Token

```bash
# Required scopes
admin:org       # Read and write organization settings
read:org        # Read organization metadata
```

### Organization Role

You must be an **organization owner** to:
- Import organization settings
- Modify organization settings
- Apply changes

## What's NOT Supported

Terraformer doesn't support these newer GitHub features:
- ❌ `github_organization_settings` (this package!)
- ❌ `github_organization_ruleset`
- ❌ `github_repository_ruleset`
- ❌ `github_organization_custom_role`

All must be manually created and imported.

## Next Steps

1. **Read QUICK_REFERENCE.txt** - Get familiar with commands
2. **Run end-to-end-example.sh** - See the full workflow
3. **Review ORG_SETTINGS_README.md** - Deep dive into details
4. **Import your settings** - `./import-org-settings.sh your-org`
5. **Add to CI/CD** - Install org-settings-workflow.yml

## Philosophy

> "Simplicity is the ultimate sophistication."

These tools follow the Plan 9 and Unix philosophy:
- Small, focused components
- Text-based configuration
- Composable through pipes and Make
- Automation-friendly
- Human-readable

Rob Pike and Ken Thompson would approve.

## Support

- Documentation: See ORG_SETTINGS_README.md
- Quick ref: See QUICK_REFERENCE.txt
- Examples: Run end-to-end-example.sh
- API docs: https://registry.terraform.io/providers/integrations/github/latest/docs/resources/organization_settings

## License

Use freely. Build amazing things. Keep it simple.

---

*"This is the Unix philosophy: Write programs that do one thing and do it well."*
*— Doug McIlroy*
