# Gold Standard GitHub Repository - Quick Start Guide

Get up and running with the gold-standard-github-repository module in under 5 minutes.

## Prerequisites Checklist

- [ ] Terraform >= 1.5.0 installed
- [ ] GitHub organization access
- [ ] GitHub App created (recommended) OR Personal Access Token
- [ ] Parent team exists in GitHub (default: `super-parent-team`)

## Quick Start (3 Steps)

### Step 1: Configure Provider

Create `main.tf`:

```hcl
terraform {
  required_version = ">= 1.5.0"
  
  required_providers {
    github = {
      source  = "integrations/github"
      version = "~> 6.6"
    }
  }
}

provider "github" {
  owner = "my-organization"
  
  app_auth {
    id              = var.github_app_id
    installation_id = var.github_app_installation_id
    pem_file        = var.github_app_pem_file
  }
}
```

### Step 2: Create Repository

```hcl
module "my_repo" {
  source = "path/to/gold-standard-github-repository"
  
  repository_name = "my-awesome-service"
  parent_team_slug = "platform-team"
  
  team_members = [
    {
      username = "alice"
      role     = "maintainer"
    }
  ]
}
```

### Step 3: Deploy

```bash
# Set environment variables
export TF_VAR_github_app_id="123456"
export TF_VAR_github_app_installation_id="12345678"
export TF_VAR_github_app_pem_file="$(cat github-app.pem)"

# Initialize and apply
terraform init
terraform plan
terraform apply
```

## What Gets Created?

### ✅ Repository
- Name: `my-awesome-service`
- Visibility: Private
- Topics: `gold-standard`, `terraform-managed`, `creator:terraform`
- Security: Secret scanning enabled
- Actions: Disabled (but workflow-accessible)

### ✅ Team
- Name: `my-awesome-service-team`
- Parent: `platform-team`
- Permission: Admin
- Member: alice (maintainer)

### ✅ Environments
- `nonproductive` (0 min wait, admins can bypass)
- `prod` (5 min wait, requires approval)

### ✅ Custom Role
- Name: `my-awesome-service-superdev`
- Base: Write
- Enhanced: Actions, deployments, issues

### ✅ Ruleset
- Target: Main branch
- Required: 2 approvals + code owner review
- Status checks: Configurable

## Common Patterns

### Pattern 1: Simple Repository

```hcl
module "simple" {
  source = "./modules/gold-standard-github-repository"
  
  repository_name  = "simple-service"
  parent_team_slug = "my-team"
  
  team_members = [
    { username = "alice", role = "maintainer" }
  ]
}
```

### Pattern 2: Production-Ready with Strict Controls

```hcl
module "production" {
  source = "./modules/gold-standard-github-repository"
  
  repository_name = "critical-api"
  parent_team_slug = "platform-team"
  
  # Stricter settings
  ruleset_required_signatures                        = true
  ruleset_required_linear_history                    = true
  ruleset_pull_request_required_approving_review_count = 3
  ruleset_pull_request_require_last_push_approval    = true
  
  environments = {
    nonproductive = {
      wait_timer          = 0
      can_admins_bypass   = true
      prevent_self_review = false
    }
    prod = {
      wait_timer          = 600  # 10 minutes
      can_admins_bypass   = false
      prevent_self_review = true
      reviewers_users     = ["alice", "bob", "charlie"]
    }
  }
}
```

### Pattern 3: From Template

```hcl
module "from_template" {
  source = "./modules/gold-standard-github-repository"
  
  repository_name = "new-microservice"
  
  template_owner      = "my-org"
  template_repository = "microservice-template"
  
  parent_team_slug = "backend-team"
}
```

### Pattern 4: Multiple Environments

```hcl
module "multi_env" {
  source = "./modules/gold-standard-github-repository"
  
  repository_name = "payment-service"
  parent_team_slug = "payment-team"
  
  environments = {
    dev = {
      wait_timer                        = 0
      deployment_branch_policy_type     = "custom"
      deployment_branch_policy_patterns = ["dev/*"]
    }
    staging = {
      wait_timer                        = 60
      deployment_branch_policy_type     = "custom"
      deployment_branch_policy_patterns = ["staging"]
      reviewers_users                   = ["alice"]
    }
    prod = {
      wait_timer                    = 300
      deployment_branch_policy_type = "protected"
      reviewers_users               = ["alice", "bob"]
      prevent_self_review           = true
    }
  }
}
```

## Troubleshooting

### Error: Repository Already Exists

```
Error: Repository 'my-repo' already exists in organization 'my-org'
```

**Solution**: Choose a different name or import existing repository:
```bash
terraform import 'module.my_repo.github_repository.main' "my-repo"
```

### Error: Parent Team Not Found

```
Error: parent team not found
```

**Solutions**:
1. Verify team slug: `gh api orgs/my-org/teams | jq '.[] | .slug'`
2. Change `parent_team_slug` to existing team
3. Create parent team first

### Error: Custom Role Creation Failed

```
Error: Custom roles require GitHub Enterprise Cloud
```

**Solution**: Disable custom role creation:
```hcl
create_custom_role = false
```

### Error: Insufficient Permissions

```
Error: Resource not accessible by integration
```

**Solutions**:
1. Verify GitHub App permissions:
   - Repository administration: Read & Write
   - Organization administration: Read & Write
2. Reinstall GitHub App if needed
3. Check PAT scopes: `admin:org`, `repo`, `workflow`

## Configuration Tips

### Minimal Configuration
```hcl
# Just 3 required values
repository_name  = "my-repo"
parent_team_slug = "my-team"
team_members     = [{ username = "alice", role = "maintainer" }]
```

### Disable Optional Features
```hcl
# Disable what you don't need
create_custom_role = false
ruleset_enabled    = false
actions_enabled    = false  # Already default
```

### Override Defaults
```hcl
# Change any default
repository_visibility = "internal"
team_repository_permission = "maintain"
ruleset_pull_request_required_approving_review_count = 1
```

## GitHub App Setup (Recommended)

### 1. Create GitHub App

Navigate to: `https://github.com/organizations/YOUR-ORG/settings/apps/new`

**Required Permissions**:
- Repository administration: Read & Write
- Repository contents: Read & Write
- Repository metadata: Read-only
- Organization administration: Read & Write
- Organization members: Read & Write

### 2. Generate Private Key

1. Click "Generate a private key"
2. Save the `.pem` file securely
3. Never commit it to version control

### 3. Install App

1. Click "Install App"
2. Select your organization
3. Choose "All repositories" or specific repos
4. Note the Installation ID from URL

### 4. Configure Terraform

```bash
export TF_VAR_github_app_id="YOUR_APP_ID"
export TF_VAR_github_app_installation_id="YOUR_INSTALLATION_ID"
export TF_VAR_github_app_pem_file="$(cat /path/to/app.pem)"
```

## Alternative: Personal Access Token

### 1. Generate Token

Navigate to: `https://github.com/settings/tokens/new`

**Required Scopes**:
- `repo` (full control)
- `admin:org` (full control)
- `workflow` (if using Actions)

### 2. Configure Provider

```hcl
provider "github" {
  owner = "my-organization"
  token = var.github_token
}
```

```bash
export GITHUB_TOKEN="ghp_xxxxxxxxxxxx"
export TF_VAR_github_token="$GITHUB_TOKEN"
```

## Testing Before Production

### 1. Dry Run

```bash
terraform plan -out=tfplan
terraform show tfplan | less
```

### 2. Test in Sandbox Org

```hcl
provider "github" {
  owner = "my-sandbox-org"  # Use test org
}
```

### 3. Use Evaluate Mode

```hcl
ruleset_enforcement = "evaluate"  # Logs violations, doesn't block
```

### 4. Create Then Destroy

```bash
terraform apply
# Verify in GitHub UI
terraform destroy
```

## Best Practices

### 1. Use Remote State
```hcl
terraform {
  backend "s3" {
    bucket = "terraform-state"
    key    = "github/repos/my-repo.tfstate"
  }
}
```

### 2. Separate Per Repository
```
repos/
├── customer-api/
│   ├── main.tf
│   └── terraform.tfvars
├── payment-service/
│   ├── main.tf
│   └── terraform.tfvars
└── shared/
    └── github-app-credentials.auto.tfvars
```

### 3. Use Workspaces
```bash
terraform workspace new dev
terraform workspace new staging
terraform workspace new prod
```

### 4. Secure Credentials
```bash
# Store in secret manager
aws secretsmanager get-secret-value \
  --secret-id github-app-pem \
  --query SecretString \
  --output text > /tmp/github-app.pem

export TF_VAR_github_app_pem_file="$(cat /tmp/github-app.pem)"
rm /tmp/github-app.pem
```

## Next Steps

1. ✅ Review [Complete Example](./examples/complete/main.tf)
2. ✅ Read [Full Documentation](./README.md)
3. ✅ Check [Variables Reference](./variables.tf)
4. ✅ Explore [Advanced Patterns](./docs/advanced-patterns.md)

## Getting Help

- Check [Troubleshooting Section](#troubleshooting)
- Review [GitHub Provider Docs](https://registry.terraform.io/providers/integrations/github/latest/docs)
- Open an issue in the repository

## Common Commands Reference

```bash
# Initialize
terraform init

# Validate syntax
terraform validate

# Format code
terraform fmt -recursive

# Plan changes
terraform plan

# Apply changes
terraform apply

# Show current state
terraform show

# List resources
terraform state list

# View specific resource
terraform state show 'module.my_repo.github_repository.main'

# Import existing resource
terraform import 'module.my_repo.github_repository.main' "repo-name"

# Destroy everything
terraform destroy

# Target specific resource
terraform apply -target='module.my_repo.github_repository.main'
```

---

**Ready to create your first gold-standard repository?** Start with the [Simple Repository pattern](#pattern-1-simple-repository) above!
