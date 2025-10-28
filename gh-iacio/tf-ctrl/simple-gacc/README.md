# GitHub Organization Settings - Terraform Module

> *"Simplicity is the ultimate sophistication."* - Ken Thompson (probably)

Complete, production-ready Terraform configuration for managing GitHub organization settings via Infrastructure as Code. Every argument supported, sensible enterprise defaults, zero handholding.

## Features

- ✅ **Complete Coverage**: All `github_organization_settings` arguments with proper typing
- ✅ **Enterprise Defaults**: Secure-by-default configuration following least-privilege principles
- ✅ **Validated**: Input validation for enum types (permissions, creation types, etc.)
- ✅ **CLI-First**: Makefile for proper workflow management (init, plan, apply, destroy)
- ✅ **CI/CD Ready**: Separate targets for pipeline automation
- ✅ **Well-Documented**: Comprehensive comments and examples

## Prerequisites

- **Terraform**: >= 1.5
- **GitHub Provider**: ~> 6.6 (integrations/github)
- **Authentication**: GitHub Personal Access Token or GitHub App with `admin:org` scope
- **Organization**: GitHub organization (not personal account)

## Quick Start

### 1. Set Environment Variables

```bash
# Required
export GITHUB_TOKEN="ghp_your_token_here"
export GITHUB_OWNER="your-org-name"
```

### 2. Configure Variables

```bash
cp terraform.tfvars.example terraform.tfvars
# Edit terraform.tfvars with your organization details
```

### 3. Initialize and Apply

```bash
make init      # Initialize Terraform
make plan      # Review changes
make apply     # Apply configuration
```

## File Structure

```
.
├── github_org_settings.tf      # Main resource and variable definitions
├── terraform.tfvars.example    # Example configuration
├── terraform.tfvars            # Your config (gitignored)
├── Makefile                    # CLI workflow management
└── README.md                   # This file
```

## Configuration Reference

### Required Variables

| Variable | Description | Example |
|----------|-------------|---------|
| `org_billing_email` | Billing email for the organization | `billing@example.com` |

### Organization Profile (Optional)

| Variable | Description | Default |
|----------|-------------|---------|
| `org_company` | Company name | `""` |
| `org_name` | Display name | `""` |
| `org_description` | Organization description | `""` |
| `org_blog_url` | Blog URL | `""` |
| `org_email` | Public email | `""` |
| `org_twitter_username` | Twitter handle (no @) | `""` |
| `org_location` | Location | `""` |

### Repository Permissions

| Variable | Description | Default | Options |
|----------|-------------|---------|---------|
| `default_repository_permission` | Default member permission | `"read"` | read, write, admin, none |
| `members_can_create_repositories` | Allow repo creation | `false` | - |
| `members_can_create_public_repositories` | Allow public repo creation | `false` | - |
| `members_can_create_private_repositories` | Allow private repo creation | `false` | - |
| `members_can_create_internal_repositories` | Allow internal repo creation (GHEC) | `false` | - |
| `members_allowed_repository_creation_type` | Repo creation type | `"none"` | all, private, internal, none |

### Security Settings (New Repositories)

| Variable | Description | Default |
|----------|-------------|---------|
| `secret_scanning_enabled_for_new_repositories` | Enable secret scanning | `true` |
| `secret_scanning_push_protection_enabled_for_new_repositories` | Enable push protection | `true` |
| `dependabot_alerts_enabled_for_new_repositories` | Enable Dependabot alerts | `true` |
| `dependabot_security_updates_enabled_for_new_repositories` | Enable Dependabot updates | `true` |
| `dependency_graph_enabled_for_new_repositories` | Enable dependency graph | `true` |
| `advanced_security_enabled_for_new_repositories` | Enable Advanced Security (GHEC) | `false` |

### Other Settings

| Variable | Description | Default |
|----------|-------------|---------|
| `has_organization_projects` | Enable org projects | `true` |
| `has_repository_projects` | Enable repo projects | `true` |
| `members_can_create_pages` | Allow GitHub Pages | `false` |
| `members_can_fork_private_repositories` | Allow private forks | `false` |
| `web_commit_signoff_required` | Require commit signoff (DCO) | `false` |

## Security Posture

This configuration follows **secure-by-default** principles:

### ✅ Enabled by Default
- Secret scanning
- Secret scanning push protection  
- Dependabot alerts
- Dependabot security updates
- Dependency graph

### ❌ Disabled by Default
- Member repository creation (use IaC instead)
- Public repository creation
- GitHub Pages
- Private repository forks
- Advanced Security (requires GHEC license)

## Makefile Targets

### Essential Commands

```bash
make help       # Show all available targets
make init       # Initialize Terraform
make plan       # Generate execution plan
make apply      # Apply configuration
make validate   # Validate configuration
make fmt        # Format Terraform files
make destroy    # Remove from Terraform management
```

### State Management

```bash
make show       # Show current state
make state-list # List resources in state
make refresh    # Refresh state from GitHub
make import     # Import existing org settings
```

### Development

```bash
make vars       # Show variable values
make graph      # Generate dependency graph
make console    # Start Terraform console
```

### CI/CD

```bash
make ci-validate  # Run validation checks
make ci-plan      # Generate plan for review
make ci-apply     # Apply changes
```

### Cleanup

```bash
make clean       # Remove plan files
make deep-clean  # Remove .terraform directory
```

## Import Existing Organization

If your organization already exists and has settings configured:

```bash
make import
make plan     # See what would change
```

## Authentication Methods

### Personal Access Token (PAT)

```bash
export GITHUB_TOKEN="ghp_xxxxxxxxxxxx"
export GITHUB_OWNER="your-org"
```

**Required Scopes**: `admin:org`, `repo`

### GitHub App (Recommended for Automation)

```hcl
provider "github" {
  owner = var.github_owner
  
  app_auth {
    id              = var.github_app_id
    installation_id = var.github_app_installation_id
    pem_file        = var.github_app_pem_file
  }
}
```

**Required Permissions**: Organization administration (read/write)

## CI/CD Integration

### GitHub Actions Example

```yaml
name: Terraform Organization Settings
on:
  pull_request:
    paths: ['terraform/**']
  push:
    branches: [main]

jobs:
  terraform:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      
      - uses: hashicorp/setup-terraform@v3
        with:
          terraform_version: 1.5.0
      
      - name: Terraform Plan
        if: github.event_name == 'pull_request'
        env:
          GITHUB_TOKEN: ${{ secrets.ORG_ADMIN_TOKEN }}
          GITHUB_OWNER: ${{ github.repository_owner }}
        run: make ci-plan
      
      - name: Terraform Apply
        if: github.ref == 'refs/heads/main'
        env:
          GITHUB_TOKEN: ${{ secrets.ORG_ADMIN_TOKEN }}
          GITHUB_OWNER: ${{ github.repository_owner }}
        run: make ci-apply
```

## Configuration Examples

### Example 1: Maximum Security (Enterprise)

```hcl
default_repository_permission               = "read"
members_can_create_repositories             = false
members_allowed_repository_creation_type    = "none"
members_can_fork_private_repositories       = false
secret_scanning_push_protection_enabled_for_new_repositories = true
advanced_security_enabled_for_new_repositories = true
```

### Example 2: Small Team (More Permissive)

```hcl
default_repository_permission               = "write"
members_can_create_private_repositories     = true
members_allowed_repository_creation_type    = "private"
has_repository_projects                     = true
```

### Example 3: Open Source Organization

```hcl
default_repository_permission               = "read"
members_can_create_public_repositories      = true
members_can_create_pages                    = true
members_can_create_public_pages             = true
members_allowed_repository_creation_type    = "all"
```

## Gotchas & Known Issues

### Internal Repositories
- `members_can_create_internal_repositories` requires GitHub Enterprise Cloud
- Setting to `true` without GHEC license will cause errors

### Advanced Security
- `advanced_security_enabled_for_new_repositories` requires GitHub Enterprise Cloud with Advanced Security license
- Free/Team plans should keep this `false`

### Org vs Repo Settings
- This resource manages **organization-level defaults**
- Individual repository settings can override these defaults
- Use `github_repository` resource for repo-specific configuration

### Import Behavior
- Importing existing org settings may show changes on first plan
- Review carefully before applying
- Some settings may not match GitHub's API defaults exactly

## Troubleshooting

### Error: "This resource can only be used in the context of an organization"

**Solution**: Set `GITHUB_OWNER` to organization name (not username)

```bash
export GITHUB_OWNER="your-org-name"  # Not your username
```

### Error: "Resource not found" (404)

**Solutions**:
1. Token lacks `admin:org` scope
2. `GITHUB_OWNER` is incorrect
3. Using user account instead of organization

### State Drift

If settings change outside Terraform:

```bash
make refresh  # Sync state with GitHub
make plan     # See differences
```

## Drift Detection

Automated hourly detection of configuration drift between Terraform and GitHub.

### Quick Setup

1. **Add workflow file**: `.github/workflows/drift-detection.yml` (included)
2. **Add secret**: `GH_ORG_ADMIN_TOKEN` with `admin:org` scope
3. **Enable Actions**: Repository Settings → Actions → General → Read/Write permissions

### What It Does

- ✅ Runs hourly via cron
- ✅ Detects changes via `terraform plan`
- ✅ Compares all 27 organization settings
- ✅ Creates detailed GitHub issue with drift report
- ✅ Auto-closes issue when drift resolved

### Manual Testing

```bash
# Test locally
./test-drift-detection.sh

# Test via GitHub Actions
gh workflow run drift-detection.yml -f create_issue=true
```

See [DRIFT_DETECTION.md](DRIFT_DETECTION.md) for complete documentation.

## Best Practices

1. **Use IaC for Everything**: Disable member repo creation, manage via Terraform
2. **Least Privilege**: Start with `read` permission, grant explicit access via teams
3. **Security First**: Enable all scanning features by default
4. **Version Control**: Store configuration (not secrets) in git
5. **Review Plans**: Always run `make plan` before `make apply`
6. **GitOps**: Use pull requests for changes, apply via CI/CD
7. **State Backend**: Use remote state (S3, Terraform Cloud) for team collaboration
8. **Drift Detection**: Enable hourly drift detection to catch unauthorized changes

## License

MIT - Do whatever you want with it. No warranty. Don't blame us if GitHub breaks.

## Contributing

Found a bug? Missing an argument? PR it.

Keep it simple. Keep it clean. Make Ken and Rob proud.

---

*Built with the spirit of Plan 9, the pragmatism of Go, and the wisdom of Unix.*
