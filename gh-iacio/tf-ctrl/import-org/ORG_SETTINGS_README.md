# GitHub Organization Settings Import

Import and manage your existing GitHub organization settings with Terraform.

## The Problem

Terraformer doesn't support `github_organization_settings` - it must be manually created and imported. This resource manages:

- Organization profile (name, email, location, etc.)
- Default repository permissions
- Security policies (2FA, commit signoff, etc.)
- Feature flags (projects, pages, etc.)
- Advanced security defaults for new repositories

## Quick Start

### 1. Add the resource files to your terraform directory

```bash
cp organization.tf terraform/
cp organization_variables.tf terraform/
chmod +x import-org-settings.sh
```

### 2. Set your organization variables

Create `terraform/terraform.tfvars` or use environment variables:

```hcl
# terraform/terraform.tfvars
org_billing_email  = "billing@example.com"
org_company        = "Example Corp"
org_email          = "opensource@example.com"
org_location       = "San Francisco, CA"
org_description    = "Building the future"
org_twitter        = "example"
org_display_name   = "Example Corporation"
org_blog           = "https://blog.example.com"
```

### 3. Import existing settings

```bash
# Method 1: Using the script
./import-org-settings.sh your-org-name

# Method 2: Manual import
cd terraform
terraform import github_organization_settings.this your-org-name

# Method 3: Using make (after adding Makefile.org to your Makefile)
make import-org GITHUB_ORG=your-org-name
```

### 4. Review and adjust

```bash
cd terraform
terraform plan
```

The plan will show differences between your resource definition and actual GitHub settings. Adjust `organization.tf` to match your desired state.

### 5. Apply if needed

```bash
terraform apply
```

## Understanding the Resource

### Security Settings (Recommended Defaults)

```hcl
# Restrict default permissions - security first
default_repository_permission = "read"

# Disable repository creation by members
members_can_create_repositories          = false
members_can_create_public_repositories   = false
members_can_create_private_repositories  = false
members_can_create_internal_repositories = false

# Advanced security for all new repos
advanced_security_enabled_for_new_repositories               = true
dependabot_alerts_enabled_for_new_repositories              = true
dependabot_security_updates_enabled_for_new_repositories    = true
secret_scanning_enabled_for_new_repositories                = true
secret_scanning_push_protection_enabled_for_new_repositories = true

# Require commit signoff
web_commit_signoff_required = true
```

### Import Resource ID

For `github_organization_settings`, the import ID is simply your **organization name** (login):

```bash
terraform import github_organization_settings.this acme-corp
```

NOT the numeric organization ID. Just the slug.

## Verification

### Check current settings via API

```bash
# Full organization object
gh api /orgs/your-org | jq

# Specific settings
gh api /orgs/your-org | jq '{
  name, company, blog, location, email,
  default_repository_permission,
  members_can_create_repositories,
  web_commit_signoff_required
}'
```

### Check Terraform state

```bash
cd terraform
terraform state show github_organization_settings.this
```

### Using make targets

```bash
make show-org              # Show current state
make fetch-org-settings    # Fetch from GitHub API
make plan-org             # Plan changes
make apply-org            # Apply changes
```

## Common Adjustments

### Allow member repository creation

```hcl
members_can_create_repositories         = true
members_can_create_private_repositories = true
```

### Allow public repositories

```hcl
members_can_create_public_repositories = true
```

### Disable advanced security for new repos

```hcl
advanced_security_enabled_for_new_repositories = false
secret_scanning_enabled_for_new_repositories   = false
```

### Allow forking private repos

```hcl
members_can_fork_private_repositories = true
```

## Integration with Existing Setup

### Add to your main.tf

If you want all settings in one file:

```hcl
# terraform/main.tf
terraform {
  required_providers {
    github = {
      source  = "integrations/github"
      version = "~> 6.6"
    }
  }
}

# ... existing code ...

# Import the organization module
module "organization" {
  source = "./organization"
  
  org_billing_email = var.org_billing_email
  org_company       = var.org_company
  # ... etc
}
```

### Add to GitHub Actions workflow

```yaml
# .github/workflows/terraform-apply.yml
- name: Import Organization Settings
  if: github.event.inputs.import_org == 'true'
  run: |
    cd terraform
    terraform import github_organization_settings.this ${{ github.repository_owner }}
  continue-on-error: true
```

## Troubleshooting

### "Resource already exists"

If you accidentally apply before importing:

```bash
cd terraform
terraform state rm github_organization_settings.this
./import-org-settings.sh your-org
```

### "Error: authentication error"

Verify your token has `admin:org` scope:

```bash
gh auth status
gh auth refresh -s admin:org
```

### Settings not updating

Some settings require Enterprise Cloud or specific GitHub plans. Check your plan:

```bash
gh api /orgs/your-org | jq .plan
```

## Architecture

```
┌─────────────────────────────────────────┐
│   GitHub Organization (Live State)     │
│  ├─ Profile (name, email, location)    │
│  ├─ Security (2FA, signoff, defaults)  │
│  └─ Features (projects, pages, etc)    │
└──────────────┬──────────────────────────┘
               │
               │ import
               ▼
┌─────────────────────────────────────────┐
│   Terraform State                       │
│   github_organization_settings.this     │
└──────────────┬──────────────────────────┘
               │
               │ manages
               ▼
┌─────────────────────────────────────────┐
│   organization.tf (Desired State)       │
│  ├─ Security policies                   │
│  ├─ Default permissions                 │
│  └─ Advanced security defaults          │
└─────────────────────────────────────────┘
```

## Philosophy

> "Do one thing and do it well."
> — Doug McIlroy, Unix Philosophy

This follows the Unix way:
- **organization.tf** - Defines organization settings
- **import script** - Does the import, nothing else
- **Makefile targets** - Composable commands
- **GitHub Actions** - Automated workflows

Each piece is simple, focused, and composable.

## References

- [GitHub Terraform Provider - organization_settings](https://registry.terraform.io/providers/integrations/github/latest/docs/resources/organization_settings)
- [GitHub API - Organizations](https://docs.github.com/en/rest/orgs/orgs)
- [Terraform Import](https://www.terraform.io/cli/import)

---

*"Simplicity is the ultimate sophistication."*
