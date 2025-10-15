# ==============================================================================
# Complete Example: Gold Standard GitHub Repository Module
# ==============================================================================
# 
# Directory: examples/complete/
#
# This example demonstrates all features of the gold-standard-github-repository
# module including team management, custom roles, environments, and rulesets.
#
# ==============================================================================

# ------------------------------------------------------------------------------
# Provider Configuration
# ------------------------------------------------------------------------------

terraform {
  required_version = ">= 1.5.0"

  required_providers {
    github = {
      source  = "integrations/github"
      version = "~> 6.6"
    }
  }

  # Optional: Configure remote state
  # backend "s3" {
  #   bucket = "terraform-state-github"
  #   key    = "repositories/customer-api.tfstate"
  #   region = "us-east-1"
  # }
}

provider "github" {
  owner = var.github_organization

  # GitHub App Authentication (Recommended)
  app_auth {
    id              = var.github_app_id
    installation_id = var.github_app_installation_id
    pem_file        = var.github_app_pem_file
  }

  # Alternative: Personal Access Token
  # token = var.github_token
}

# ------------------------------------------------------------------------------
# Variables
# ------------------------------------------------------------------------------

variable "github_organization" {
  description = "GitHub organization name"
  type        = string
}

variable "github_app_id" {
  description = "GitHub App ID"
  type        = string
  default     = ""
}

variable "github_app_installation_id" {
  description = "GitHub App Installation ID"
  type        = string
  default     = ""
}

variable "github_app_pem_file" {
  description = "GitHub App PEM file contents"
  type        = string
  sensitive   = true
  default     = ""
}

# ------------------------------------------------------------------------------
# Data Sources - Get existing teams
# ------------------------------------------------------------------------------

data "github_team" "sre" {
  slug = "sre-team"
}

data "github_team" "security" {
  slug = "security-team"
}

# ------------------------------------------------------------------------------
# Example 1: Basic Repository with Minimal Configuration
# ------------------------------------------------------------------------------

module "simple_repo" {
  source = "../../"

  repository_name        = "simple-service"
  repository_description = "A simple microservice"

  parent_team_slug = "platform-engineering"

  team_members = [
    {
      username = "alice"
      role     = "maintainer"
    }
  ]
}

# ------------------------------------------------------------------------------
# Example 2: Full-Featured Repository with All Options
# ------------------------------------------------------------------------------

module "customer_api" {
  source = "../../"

  # Repository Core Configuration
  repository_name         = "customer-api-service"
  repository_description  = "Customer API microservice for enterprise platform"
  repository_homepage_url = "https://api.example.com/customer"
  repository_visibility   = "private"

  # Topics including gold-standard (default) plus custom
  repository_topics = ["gold-standard", "api", "microservice", "customer"]
  additional_topics = ["production", "critical"]

  # Repository Features
  has_issues      = true
  has_discussions = false
  has_projects    = true
  has_wiki        = false
  has_downloads   = true

  # Repository Initialization
  auto_init          = true
  gitignore_template = "Go"
  license_template   = "mit"

  # Branch Settings
  default_branch         = "main"
  delete_branch_on_merge = true

  # Merge Strategy Configuration
  allow_merge_commit  = true
  allow_squash_merge  = true
  allow_rebase_merge  = false
  allow_auto_merge    = false
  allow_update_branch = true

  squash_merge_commit_title   = "PR_TITLE"
  squash_merge_commit_message = "PR_BODY"
  merge_commit_title          = "MERGE_MESSAGE"
  merge_commit_message        = "PR_BODY"

  # Security Configuration
  vulnerability_alerts                                  = true
  security_and_analysis_secret_scanning                 = "enabled"
  security_and_analysis_secret_scanning_push_protection = "enabled"
  security_and_analysis_advanced_security               = "disabled"

  # GitHub Actions - Disabled but workflow-accessible
  actions_enabled           = false
  github_owned_allowed      = true
  verified_allowed          = true
  patterns_allowed          = ["my-org/*"]

  # Team Configuration
  parent_team_slug               = "platform-engineering"
  team_name_suffix               = "owners"
  team_description               = "Owners of the customer API service"
  team_privacy                   = "closed"
  team_create_default_maintainer = false

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

  # Custom Organization Role
  create_custom_role      = true
  custom_role_name        = "" # Will use customer-api-service-superdev
  custom_role_description = "Super developer with enhanced permissions for customer API"
  custom_role_base_role   = "write"
  custom_role_permissions = [
    "read_code",
    "write_code",
    "read_actions",
    "write_actions",
    "read_deployments",
    "write_deployments",
    "manage_pull_requests",
    "manage_issues",
    "manage_settings_wiki",
    "edit_repo_metadata"
  ]

  # Repository Environments
  environments = {
    dev = {
      wait_timer                        = 0
      can_admins_bypass                 = true
      prevent_self_review               = false
      deployment_branch_policy_type     = "custom"
      deployment_branch_policy_patterns = ["dev/*", "feature/*"]
      reviewers_teams                   = []
      reviewers_users                   = []
    }
    staging = {
      wait_timer                        = 60
      can_admins_bypass                 = true
      prevent_self_review               = false
      deployment_branch_policy_type     = "custom"
      deployment_branch_policy_patterns = ["staging", "main"]
      reviewers_teams                   = []
      reviewers_users                   = ["alice"]
    }
    nonproductive = {
      wait_timer                    = 0
      can_admins_bypass             = true
      prevent_self_review           = false
      deployment_branch_policy_type = "protected"
      reviewers_teams               = []
      reviewers_users               = []
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

  # Repository Ruleset
  ruleset_enabled     = true
  ruleset_name        = "Main Branch Protection"
  ruleset_target      = "branch"
  ruleset_enforcement = "active"

  ruleset_ref_name_include = ["~DEFAULT_BRANCH"]
  ruleset_ref_name_exclude = []

  # Ruleset Rules
  ruleset_deletion                = true
  ruleset_non_fast_forward        = true
  ruleset_required_linear_history = false
  ruleset_required_signatures     = false

  # Pull Request Rules
  ruleset_pull_request_required_approving_review_count   = 2
  ruleset_pull_request_dismiss_stale_reviews_on_push     = true
  ruleset_pull_request_require_code_owner_review         = true
  ruleset_pull_request_require_last_push_approval        = false
  ruleset_pull_request_required_review_thread_resolution = true

  # Required Status Checks
  ruleset_required_status_checks        = ["ci/build", "ci/test", "ci/security-scan", "ci/lint"]
  ruleset_required_status_checks_strict = true

  # Bypass Actors
  ruleset_bypass_actors = [
    {
      actor_id    = 1
      actor_type  = "OrganizationAdmin"
      bypass_mode = "always"
    },
    {
      actor_id    = 5
      actor_type  = "RepositoryRole" # Admin
      bypass_mode = "pull_request"
    },
    {
      actor_id    = data.github_team.sre.id
      actor_type  = "Team"
      bypass_mode = "always"
    }
  ]

  # REST API Custom Metadata
  rest_api_endpoint = ""
  rest_api_method   = "PATCH"
  rest_api_body     = ""

  # Creator Information
  creator_username = "terraform"

  # Outside Collaborators
  collaborators = [
    {
      username   = "external-consultant"
      permission = "triage"
    },
    {
      username   = "contractor-bob"
      permission = "pull"
    }
  ]

  # Webhooks
  webhooks = [
    {
      url          = "https://ci.example.com/github-webhook"
      content_type = "json"
      insecure_ssl = false
      secret       = var.webhook_secret
      events       = ["push", "pull_request", "release"]
      active       = true
    }
  ]

  # Deploy Keys
  deploy_keys = [
    {
      title     = "CI/CD Deploy Key"
      key       = var.deploy_key_public
      read_only = true
    }
  ]
}

# ------------------------------------------------------------------------------
# Example 3: Repository from Template
# ------------------------------------------------------------------------------

module "service_from_template" {
  source = "../../"

  repository_name        = "payment-service"
  repository_description = "Payment processing service from template"

  # Template Configuration
  template_owner                = var.github_organization
  template_repository           = "microservice-template"
  template_include_all_branches = false

  parent_team_slug = "payment-team"

  team_members = [
    {
      username = "dave"
      role     = "maintainer"
    }
  ]
}

# ------------------------------------------------------------------------------
# Example 4: Repository with Custom Environments Only
# ------------------------------------------------------------------------------

module "data_pipeline" {
  source = "../../"

  repository_name        = "data-pipeline-etl"
  repository_description = "ETL data pipeline"
  repository_visibility  = "internal"

  parent_team_slug = "data-engineering"

  # Override default environments
  environments = {
    development = {
      wait_timer                    = 0
      can_admins_bypass             = true
      prevent_self_review           = false
      deployment_branch_policy_type = "custom"
      deployment_branch_policy_patterns = ["dev/*"]
    }
    production = {
      wait_timer                    = 600 # 10 minutes
      can_admins_bypass             = false
      prevent_self_review           = true
      deployment_branch_policy_type = "protected"
      reviewers_teams               = [data.github_team.security.slug]
      reviewers_users               = ["alice"]
    }
  }

  # Stricter ruleset for data pipeline
  ruleset_required_signatures                            = true
  ruleset_required_linear_history                        = true
  ruleset_pull_request_required_approving_review_count   = 3
  ruleset_pull_request_require_last_push_approval        = true
}

# ------------------------------------------------------------------------------
# Example 5: Public Open Source Repository
# ------------------------------------------------------------------------------

module "open_source_lib" {
  source = "../../"

  repository_name        = "awesome-go-library"
  repository_description = "An awesome open source Go library"
  repository_homepage_url = "https://awesome-lib.dev"
  repository_visibility  = "public"

  # Open source features
  has_issues      = true
  has_discussions = true
  has_wiki        = true
  has_projects    = true

  license_template   = "apache-2.0"
  gitignore_template = "Go"

  # GitHub Pages for documentation
  pages_enabled      = true
  pages_source_branch = "gh-pages"
  pages_source_path  = "/"
  pages_cname        = "docs.awesome-lib.dev"
  pages_build_type   = "workflow"

  # Less restrictive settings for open source
  parent_team_slug = "open-source-maintainers"

  ruleset_pull_request_required_approving_review_count = 1
  ruleset_required_status_checks                       = ["ci/test"]

  # No custom role for public repos
  create_custom_role = false
}

# ------------------------------------------------------------------------------
# Webhook Secret Variable
# ------------------------------------------------------------------------------

variable "webhook_secret" {
  description = "Secret for webhook validation"
  type        = string
  sensitive   = true
  default     = ""
}

variable "deploy_key_public" {
  description = "Public SSH key for deployments"
  type        = string
  sensitive   = true
  default     = ""
}

# ------------------------------------------------------------------------------
# Outputs
# ------------------------------------------------------------------------------

output "simple_repo" {
  description = "Simple repository details"
  value = {
    name     = module.simple_repo.repository.name
    html_url = module.simple_repo.repository.html_url
    team     = module.simple_repo.team.name
  }
}

output "customer_api" {
  description = "Customer API repository details"
  value = {
    name         = module.customer_api.repository.name
    html_url     = module.customer_api.repository.html_url
    team         = module.customer_api.team.name
    custom_role  = module.customer_api.custom_role
    environments = keys(module.customer_api.environments)
    ruleset      = module.customer_api.ruleset
  }
}

output "service_from_template" {
  description = "Service from template details"
  value = {
    name     = module.service_from_template.repository.name
    html_url = module.service_from_template.repository.html_url
  }
}

output "data_pipeline" {
  description = "Data pipeline repository details"
  value = {
    name         = module.data_pipeline.repository.name
    environments = keys(module.data_pipeline.environments)
  }
}

output "open_source_lib" {
  description = "Open source library details"
  value = {
    name       = module.open_source_lib.repository.name
    html_url   = module.open_source_lib.repository.html_url
    visibility = module.open_source_lib.repository.visibility
  }
}

# ------------------------------------------------------------------------------
# Usage Instructions
# ------------------------------------------------------------------------------

# 1. Copy this file to your project
# 2. Create terraform.tfvars:
#
#    github_organization        = "my-org"
#    github_app_id              = "123456"
#    github_app_installation_id = "12345678"
#    github_app_pem_file        = file("path/to/app.pem")
#    webhook_secret             = "super-secret-webhook"
#    deploy_key_public          = file("~/.ssh/deploy_key.pub")
#
# 3. Initialize Terraform:
#    terraform init
#
# 4. Review the plan:
#    terraform plan
#
# 5. Apply the configuration:
#    terraform apply
#
# 6. View outputs:
#    terraform output
