# Gold Standard GitHub Repository Terraform Module

A comprehensive Terraform module for creating enterprise-grade GitHub repositories with all GitHub provider v6.6 features exposed as configurable variables.

## Features

- ✅ **Comprehensive Configuration**: Every GitHub provider v6.6 parameter exposed with sensible defaults
- ✅ **Pre-flight Validation**: Checks if repository already exists before creation
- ✅ **Gold Standard Defaults**: Repository tagged with `gold-standard`, `terraform-managed`, and `creator:terraform`
- ✅ **GitHub Actions Disabled**: Actions disabled by default but accessible by workflows
- ✅ **Multi-Environment Support**: Creates `nonproductive` and `prod` environments out of the box
- ✅ **Team Management**: Automatic team creation with configurable parent team hierarchy
- ✅ **Custom Roles**: Organization-level custom role creation with `{repo-name}-superdev` pattern
- ✅ **Advanced Security**: Secret scanning and vulnerability alerts enabled by default
- ✅ **Repository Rulesets**: Modern branch protection with comprehensive ruleset support
- ✅ **REST API Integration**: Custom metadata injection via GitHub REST API

## Prerequisites

- Terraform >= 1.5.0
- GitHub Provider ~> 6.6
- GitHub Enterprise Cloud (for custom roles and some features)
- Organization admin access or GitHub App with appropriate permissions

## Usage

### Basic Example

```hcl
module "gold_standard_repo" {
  source = "./modules/gold-standard-github-repository"

  repository_name        = "my-awesome-service"
  repository_description = "My awesome microservice"
  
  parent_team_slug = "platform-team"
  
  team_members = [
    {
      username = "alice"
      role     = "maintainer"
    },
    {
      username = "bob"
      role     = "member"
    }
  ]
}
```

### Complete Example with All Options

```hcl
module "gold_standard_repo" {
  source = "./modules/gold-standard-github-repository"

  # Repository Core
  repository_name        = "customer-api-service"
  repository_description = "Customer API microservice for enterprise platform"
  repository_homepage_url = "https://api.example.com"
  repository_visibility  = "private"
  repository_topics      = ["gold-standard", "api", "microservice"]
  
  # Repository Features
  has_issues      = true
  has_discussions = false
  has_projects    = true
  has_wiki        = false
  has_downloads   = true
  
  # Branch and Merge Settings
  default_branch         = "main"
  delete_branch_on_merge = true
  allow_merge_commit     = true
  allow_squash_merge     = true
  allow_rebase_merge     = false
  allow_auto_merge       = false
  
  # Security
  vulnerability_alerts                                    = true
  security_and_analysis_secret_scanning                   = "enabled"
  security_and_analysis_secret_scanning_push_protection   = "enabled"
  
  # Team Configuration
  parent_team_slug       = "platform-team"
  team_name_suffix       = "owners"
  team_description       = "Owners of the customer API service"
  team_privacy           = "closed"
  
  team_repository_permission = "admin"
  
  team_members = [
    {
      username = "alice"
      role     = "maintainer"
    },
    {
      username = "bob"
      role     = "member"
    },
    {
      username = "charlie"
      role     = "member"
    }
  ]
  
  # Custom Role
  create_custom_role      = true
  custom_role_name        = ""  # Will use {repo-name}-superdev pattern
  custom_role_description = "Super developer with enhanced permissions"
  custom_role_base_role   = "write"
  custom_role_permissions = [
    "read_code",
    "write_code",
    "read_actions",
    "write_actions",
    "read_deployments",
    "write_deployments",
    "manage_pull_requests",
    "manage_issues"
  ]
  
  # Environments
  environments = {
    nonproductive = {
      wait_timer                    = 0
      can_admins_bypass             = true
      prevent_self_review           = false
      deployment_branch_policy_type = "protected"
      reviewers_teams               = []
      reviewers_users               = []
    }
    staging = {
      wait_timer                    = 60
      can_admins_bypass             = true
      prevent_self_review           = false
      deployment_branch_policy_type = "protected"
      reviewers_teams               = []
      reviewers_users               = ["alice"]
    }
    prod = {
      wait_timer                    = 300
      can_admins_bypass             = false
      prevent_self_review           = true
      deployment_branch_policy_type = "protected"
      reviewers_teams               = []
      reviewers_users               = ["alice", "bob"]
    }
  }
  
  # Ruleset Configuration
  ruleset_enabled                                         = true
  ruleset_name                                            = "Main Branch Protection"
  ruleset_target                                          = "branch"
  ruleset_enforcement                                     = "active"
  ruleset_deletion                                        = true
  ruleset_non_fast_forward                                = true
  ruleset_required_linear_history                         = false
  ruleset_required_signatures                             = false
  ruleset_pull_request_required_approving_review_count    = 2
  ruleset_pull_request_dismiss_stale_reviews_on_push      = true
  ruleset_pull_request_require_code_owner_review          = true
  ruleset_pull_request_require_last_push_approval         = false
  ruleset_pull_request_required_review_thread_resolution  = true
  
  ruleset_required_status_checks = [
    "ci/build",
    "ci/test",
    "ci/security-scan"
  ]
  
  ruleset_bypass_actors = [
    {
      actor_id    = 5
      actor_type  = "RepositoryRole"  # Admin role
      bypass_mode = "always"
    }
  ]
  
  # Additional Configuration
  creator_username = "terraform"
  
  collaborators = [
    {
      username   = "external-consultant"
      permission = "triage"
    }
  ]
  
  webhooks = [
    {
      url          = "https://ci.example.com/github-webhook"
      content_type = "json"
      insecure_ssl = false
      secret       = "webhook-secret-here"
      events       = ["push", "pull_request"]
      active       = true
    }
  ]
}
```

## Pre-flight Validation

The module includes built-in validation to prevent errors:

```hcl
# Automatic check for existing repositories
# Will fail with clear error message if repository already exists
```

If a repository with the same name already exists:

```
Error: Repository 'my-repo' already exists in organization 'my-org'. 
Please choose a different name or import the existing repository.
```

## Module Outputs

```hcl
# Repository details
output "repository" {
  value = module.gold_standard_repo.repository
}

# Team information
output "team" {
  value = module.gold_standard_repo.team
}

# Parent team information
output "parent_team" {
  value = module.gold_standard_repo.parent_team
}

# Custom role (if created)
output "custom_role" {
  value = module.gold_standard_repo.custom_role
}

# Environments
output "environments" {
  value = module.gold_standard_repo.environments
}

# Ruleset configuration
output "ruleset" {
  value = module.gold_standard_repo.ruleset
}

# Organization details
output "organization" {
  value = module.gold_standard_repo.organization
}
```

## Default Behaviors

### Repository Configuration
- **Visibility**: `private`
- **Topics**: `["gold-standard", "terraform-managed", "creator:terraform"]`
- **Actions**: Disabled (but accessible by workflows)
- **Security**: Secret scanning and vulnerability alerts enabled
- **Merge Strategy**: Squash and merge commits allowed, rebase disabled
- **Branch Deletion**: Automatic deletion after merge

### Team Configuration
- **Parent Team**: `super-parent-team` (configurable)
- **Team Privacy**: `closed`
- **Permission**: `admin`
- **Team Name Pattern**: `{repo-name}-team`

### Custom Role
- **Name Pattern**: `{repo-name}-superdev`
- **Base Role**: `write`
- **Enhanced Permissions**: Actions, deployments, issues, and PR management

### Environments
- **nonproductive**: No wait time, admins can bypass
- **prod**: 5-minute wait time, requires review, admins cannot bypass

### Ruleset
- **Target**: Main/default branch
- **Enforcement**: Active
- **Protection**: Deletion blocked, non-fast-forward blocked
- **Reviews**: 2 required approvals, code owner review required
- **Stale Reviews**: Dismissed on new push

## Advanced Usage

### Using with Template Repository

```hcl
module "repo_from_template" {
  source = "./modules/gold-standard-github-repository"

  repository_name = "new-service-from-template"
  
  template_owner                = "my-org"
  template_repository           = "service-template"
  template_include_all_branches = false
  
  parent_team_slug = "platform-team"
}
```

### Custom Ruleset Bypass Actors

```hcl
module "repo_with_bypass" {
  source = "./modules/gold-standard-github-repository"

  repository_name = "critical-service"
  
  parent_team_slug = "platform-team"
  
  ruleset_bypass_actors = [
    {
      actor_id    = 1
      actor_type  = "OrganizationAdmin"
      bypass_mode = "always"
    },
    {
      actor_id    = 5
      actor_type  = "RepositoryRole"
      bypass_mode = "pull_request"
    },
    {
      actor_id    = data.github_team.sre.id
      actor_type  = "Team"
      bypass_mode = "always"
    }
  ]
}
```

### Multiple Environments with Complex Policies

```hcl
module "repo_multi_env" {
  source = "./modules/gold-standard-github-repository"

  repository_name = "multi-env-service"
  parent_team_slug = "platform-team"
  
  environments = {
    dev = {
      wait_timer                    = 0
      can_admins_bypass             = true
      prevent_self_review           = false
      deployment_branch_policy_type = "custom"
      deployment_branch_policy_patterns = ["dev/*", "feature/*"]
    }
    staging = {
      wait_timer                    = 60
      can_admins_bypass             = true
      prevent_self_review           = false
      deployment_branch_policy_type = "custom"
      deployment_branch_policy_patterns = ["staging", "release/*"]
      reviewers_users               = ["alice"]
    }
    prod = {
      wait_timer                    = 300
      can_admins_bypass             = false
      prevent_self_review           = true
      deployment_branch_policy_type = "protected"
      reviewers_teams               = [data.github_team.sre.slug]
      reviewers_users               = ["alice", "bob"]
    }
  }
}
```

## GitHub Provider Configuration

The module requires the GitHub provider to be configured at the root level:

```hcl
terraform {
  required_providers {
    github = {
      source  = "integrations/github"
      version = "~> 6.6"
    }
  }
}

provider "github" {
  owner = "my-organization"
  
  # Option 1: GitHub App (Recommended)
  app_auth {
    id              = var.github_app_id
    installation_id = var.github_app_installation_id
    pem_file        = var.github_app_pem_file
  }
  
  # Option 2: Personal Access Token
  # token = var.github_token
}
```

## Input Variables Reference

See [variables.tf](./variables.tf) for complete variable documentation.

### Key Variables

| Variable | Type | Default | Description |
|----------|------|---------|-------------|
| `repository_name` | `string` | - | **Required** - Name of the repository |
| `parent_team_slug` | `string` | `"super-parent-team"` | Slug of parent team |
| `team_repository_permission` | `string` | `"admin"` | Team permission level |
| `actions_enabled` | `bool` | `false` | Enable GitHub Actions |
| `environments` | `map(object)` | See defaults | Environment configurations |
| `create_custom_role` | `bool` | `true` | Create custom organization role |
| `ruleset_enabled` | `bool` | `true` | Enable repository ruleset |

## Terraform State Management

When using this module, ensure proper state management:

```hcl
terraform {
  backend "s3" {
    bucket         = "terraform-state"
    key            = "github/repositories/my-repo.tfstate"
    region         = "us-east-1"
    encrypt        = true
    dynamodb_table = "terraform-state-lock"
  }
}
```

## Importing Existing Repositories

If you need to import an existing repository:

```bash
# Import the repository
terraform import 'module.gold_standard_repo.github_repository.main' "my-existing-repo"

# Import the team
terraform import 'module.gold_standard_repo.github_team.main' "12345678"

# Import team repository access
terraform import 'module.gold_standard_repo.github_team_repository.main' "12345678:my-existing-repo"
```

## Troubleshooting

### Repository Already Exists

If you see:
```
Error: Repository 'my-repo' already exists
```

Either:
1. Choose a different repository name
2. Import the existing repository (see Importing section above)
3. Delete the existing repository first (⚠️ destructive)

### Parent Team Not Found

If you see:
```
Error: parent team not found
```

Ensure:
1. Parent team slug is correct
2. Parent team exists in the organization
3. Terraform has permission to read team information

### Custom Role Creation Fails

Custom roles require GitHub Enterprise Cloud. If creation fails:
1. Verify you're on Enterprise Cloud
2. Set `create_custom_role = false` if not needed
3. Check permissions for creating organization custom roles

## Security Considerations

- 🔒 GitHub Actions are **disabled by default** per gold-standard requirements
- 🔒 Secret scanning and push protection **enabled by default**
- 🔒 Vulnerability alerts **enabled by default**
- 🔒 Repository visibility defaults to **private**
- 🔒 Branch protection via rulesets enforced on main branch
- 🔒 Minimum 2 required approving reviews
- 🔒 Code owner review required by default

## Contributing

When contributing to this module:

1. Maintain backwards compatibility
2. Add tests for new features
3. Update documentation
4. Follow Terraform best practices
5. Ensure all GitHub provider v6.6 features remain accessible

## License

MIT License - See LICENSE file for details

## Authors

Created for enterprise GitHub repository management with Terraform.

## Changelog

### v1.0.0 (2025-01-15)
- Initial release
- GitHub provider v6.6 support
- Comprehensive variable coverage
- Gold standard defaults
- Pre-flight validation
- Multi-environment support
- Custom role creation
- Repository ruleset support
