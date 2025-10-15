# ==============================================================================
# Advanced Patterns for gold-standard-github-repository Module
# ==============================================================================
#
# This file demonstrates advanced usage patterns including:
# - Module composition
# - Dynamic repository creation from YAML
# - Multi-region team structures
# - Advanced ruleset patterns
# - Custom metadata injection
# - Workflow automation integration
#
# ==============================================================================

# ------------------------------------------------------------------------------
# Pattern 1: YAML-Driven Repository Creation (Scale to 100+ repos)
# ------------------------------------------------------------------------------

locals {
  # Read YAML files from directory
  repo_yaml_files = fileset(path.module, "configs/repositories/*.yaml")
  
  # Parse all YAML files
  repo_configs = flatten([
    for file in local.repo_yaml_files : [
      for repo in yamldecode(file("${path.module}/${file}")).repositories : merge(repo, {
        source_file = file
      })
    ]
  ])
  
  # Create map for iteration
  repositories = {
    for repo in local.repo_configs : repo.name => repo
  }
}

# Create all repositories from YAML definitions
module "yaml_repositories" {
  source   = "../../"
  for_each = local.repositories

  repository_name        = each.value.name
  repository_description = try(each.value.description, "Managed by Terraform")
  repository_visibility  = try(each.value.visibility, "private")
  repository_topics      = try(each.value.topics, ["gold-standard"])

  parent_team_slug       = try(each.value.team.parent, "super-parent-team")
  team_name_suffix       = try(each.value.team.suffix, "team")
  team_members           = try(each.value.team.members, [])

  # Environment overrides from YAML
  environments = try(each.value.environments, var.default_environments)

  # Ruleset configuration
  ruleset_enabled                                    = try(each.value.ruleset.enabled, true)
  ruleset_pull_request_required_approving_review_count = try(each.value.ruleset.required_reviews, 2)
  ruleset_required_status_checks                     = try(each.value.ruleset.status_checks, [])

  # Security settings
  security_and_analysis_secret_scanning = try(each.value.security.secret_scanning, "enabled")
  vulnerability_alerts                  = try(each.value.security.vulnerability_alerts, true)
}

# Example YAML structure (configs/repositories/platform-team.yaml):
# repositories:
#   - name: customer-api
#     description: Customer API service
#     visibility: private
#     topics: [api, customer, production]
#     team:
#       parent: platform-engineering
#       suffix: owners
#       members:
#         - username: alice
#           role: maintainer
#         - username: bob
#           role: member
#     environments:
#       nonproductive:
#         wait_timer: 0
#       prod:
#         wait_timer: 300
#     ruleset:
#       enabled: true
#       required_reviews: 3
#       status_checks: [build, test, security-scan]

# ------------------------------------------------------------------------------
# Pattern 2: Multi-Team Repository with Layered Access
# ------------------------------------------------------------------------------

# Create repository owned by primary team
module "shared_platform_repo" {
  source = "../../"

  repository_name = "shared-platform-library"
  parent_team_slug = "platform-engineering"

  team_members = [
    { username = "platform-lead", role = "maintainer" }
  ]

  # Platform team gets admin
  team_repository_permission = "admin"
}

# Grant additional teams different levels of access
resource "github_team_repository" "secondary_team_write" {
  team_id    = data.github_team.backend_team.id
  repository = module.shared_platform_repo.repository.name
  permission = "push"
}

resource "github_team_repository" "tertiary_team_read" {
  team_id    = data.github_team.frontend_team.id
  repository = module.shared_platform_repo.repository.name
  permission = "pull"
}

# Use custom role for specialized access
resource "github_team_repository" "security_team_custom" {
  team_id    = data.github_team.security.id
  repository = module.shared_platform_repo.repository.name
  permission = module.shared_platform_repo.custom_role.name

  depends_on = [module.shared_platform_repo]
}

# ------------------------------------------------------------------------------
# Pattern 3: Repository with Dynamic Environments from Locals
# ------------------------------------------------------------------------------

locals {
  # Define environment tiers
  environment_tiers = {
    development = {
      wait_timer                    = 0
      can_admins_bypass             = true
      prevent_self_review           = false
      deployment_branch_policy_type = "custom"
      branches                      = ["dev/*", "feature/*"]
    }
    staging = {
      wait_timer                    = 60
      can_admins_bypass             = true
      prevent_self_review           = false
      deployment_branch_policy_type = "custom"
      branches                      = ["staging", "release/*"]
      reviewers                     = ["staging-approver"]
    }
    production = {
      wait_timer                    = 300
      can_admins_bypass             = false
      prevent_self_review           = true
      deployment_branch_policy_type = "protected"
      reviewers                     = ["prod-approver-1", "prod-approver-2"]
    }
  }

  # Transform to module-compatible format
  environments = {
    for tier, config in local.environment_tiers : tier => {
      wait_timer                        = config.wait_timer
      can_admins_bypass                 = config.can_admins_bypass
      prevent_self_review               = config.prevent_self_review
      deployment_branch_policy_type     = config.deployment_branch_policy_type
      deployment_branch_policy_patterns = try(config.branches, [])
      reviewers_users                   = try(config.reviewers, [])
      reviewers_teams                   = []
    }
  }
}

module "tiered_environments_repo" {
  source = "../../"

  repository_name  = "payment-processing"
  parent_team_slug = "payment-team"

  environments = local.environments

  team_members = [
    { username = "payment-lead", role = "maintainer" }
  ]
}

# ------------------------------------------------------------------------------
# Pattern 4: Monorepo with Multiple Rulesets
# ------------------------------------------------------------------------------

module "monorepo" {
  source = "../../"

  repository_name = "monorepo-platform"
  parent_team_slug = "platform-team"

  # Main branch ruleset via module
  ruleset_enabled = true
  ruleset_name    = "Main Branch Protection"

  team_members = [
    { username = "platform-lead", role = "maintainer" }
  ]
}

# Additional rulesets for different paths
resource "github_repository_ruleset" "monorepo_services" {
  repository  = module.monorepo.repository.name
  name        = "Services Directory Protection"
  target      = "branch"
  enforcement = "active"

  conditions {
    ref_name {
      include = ["refs/heads/main"]
      exclude = []
    }
  }

  rules {
    required_status_checks {
      strict_required_status_checks_policy = true

      required_check {
        context        = "services/api/build"
        integration_id = null
      }
      required_check {
        context        = "services/worker/build"
        integration_id = null
      }
    }
  }
}

resource "github_repository_ruleset" "monorepo_infrastructure" {
  repository  = module.monorepo.repository.name
  name        = "Infrastructure Code Protection"
  target      = "branch"
  enforcement = "active"

  conditions {
    ref_name {
      include = ["refs/heads/main"]
      exclude = []
    }
  }

  rules {
    required_signatures = true

    pull_request {
      required_approving_review_count = 3
      require_code_owner_review       = true
    }

    required_status_checks {
      strict_required_status_checks_policy = true

      required_check {
        context = "terraform/plan"
      }
      required_check {
        context = "terraform/validate"
      }
    }
  }
}

# ------------------------------------------------------------------------------
# Pattern 5: Repository with Comprehensive REST API Metadata
# ------------------------------------------------------------------------------

module "metadata_rich_repo" {
  source = "../../"

  repository_name = "customer-data-api"
  parent_team_slug = "data-team"

  team_members = [
    { username = "data-lead", role = "maintainer" }
  ]

  # Custom metadata via REST API
  rest_api_endpoint = "repos/${data.github_organization.current.orgname}/customer-data-api"
  rest_api_method   = "PATCH"
  rest_api_body = jsonencode({
    custom_properties = {
      creator           = "terraform"
      cost_center       = "engineering"
      compliance_level  = "high"
      data_classification = "pii"
      backup_policy     = "daily"
      sla_tier          = "tier1"
      owner_email       = "data-team@example.com"
      project_id        = "PROJ-1234"
      terraform_module  = "gold-standard-github-repository"
      last_audit        = timestamp()
    }
  })
}

# ------------------------------------------------------------------------------
# Pattern 6: Repository Factory with Standardized Configurations
# ------------------------------------------------------------------------------

locals {
  # Repository templates by type
  repo_templates = {
    microservice = {
      topics                     = ["microservice", "api", "gold-standard"]
      has_issues                 = true
      has_wiki                   = false
      has_projects               = true
      required_reviews           = 2
      required_status_checks     = ["build", "test", "security-scan"]
      environments = {
        nonproductive = { wait_timer = 0 }
        prod         = { wait_timer = 300, prevent_self_review = true }
      }
    }
    library = {
      topics                     = ["library", "shared", "gold-standard"]
      has_issues                 = true
      has_wiki                   = true
      has_projects               = false
      required_reviews           = 1
      required_status_checks     = ["test", "lint"]
      environments = {
        nonproductive = { wait_timer = 0 }
      }
    }
    infrastructure = {
      topics                     = ["infrastructure", "terraform", "gold-standard"]
      has_issues                 = true
      has_wiki                   = false
      has_projects               = true
      required_reviews           = 3
      required_status_checks     = ["terraform-validate", "terraform-plan", "security-scan"]
      required_signatures        = true
      environments = {
        nonproductive = { wait_timer = 0 }
        prod         = { wait_timer = 600, prevent_self_review = true }
      }
    }
  }

  # Repositories to create
  repos_to_create = {
    "payment-api" = {
      type        = "microservice"
      team        = "payment-team"
      description = "Payment processing API"
    }
    "auth-library" = {
      type        = "library"
      team        = "platform-team"
      description = "Shared authentication library"
    }
    "aws-infrastructure" = {
      type        = "infrastructure"
      team        = "platform-team"
      description = "AWS infrastructure as code"
    }
  }
}

# Create repositories with template-based configuration
module "factory_repos" {
  source   = "../../"
  for_each = local.repos_to_create

  repository_name        = each.key
  repository_description = each.value.description
  repository_topics      = local.repo_templates[each.value.type].topics

  parent_team_slug = each.value.team

  has_issues   = local.repo_templates[each.value.type].has_issues
  has_wiki     = local.repo_templates[each.value.type].has_wiki
  has_projects = local.repo_templates[each.value.type].has_projects

  ruleset_pull_request_required_approving_review_count = local.repo_templates[each.value.type].required_reviews
  ruleset_required_status_checks                       = local.repo_templates[each.value.type].required_status_checks
  ruleset_required_signatures                          = try(local.repo_templates[each.value.type].required_signatures, false)

  environments = local.repo_templates[each.value.type].environments

  team_members = [] # Add from data source or variables
}

# ------------------------------------------------------------------------------
# Pattern 7: Blue/Green Repository Strategy
# ------------------------------------------------------------------------------

module "service_blue" {
  source = "../../"

  repository_name  = "customer-api-blue"
  parent_team_slug = "platform-team"

  repository_topics = ["blue", "active", "production"]

  team_members = [
    { username = "ops-lead", role = "maintainer" }
  ]

  environments = {
    prod = {
      wait_timer          = 300
      prevent_self_review = true
    }
  }
}

module "service_green" {
  source = "../../"

  repository_name  = "customer-api-green"
  parent_team_slug = "platform-team"

  repository_topics = ["green", "standby", "production"]

  team_members = [
    { username = "ops-lead", role = "maintainer" }
  ]

  environments = {
    prod = {
      wait_timer          = 300
      prevent_self_review = true
    }
  }
}

# ------------------------------------------------------------------------------
# Pattern 8: Repository with Time-Based Ruleset Enforcement
# ------------------------------------------------------------------------------

locals {
  # Enable strict enforcement during business hours
  is_business_hours = timeadd(timestamp(), "0h") # Implement time logic
  enforcement_mode  = local.is_business_hours ? "active" : "evaluate"
}

module "time_sensitive_repo" {
  source = "../../"

  repository_name  = "trading-platform"
  parent_team_slug = "trading-team"

  ruleset_enforcement = local.enforcement_mode

  team_members = [
    { username = "trader-lead", role = "maintainer" }
  ]
}

# ------------------------------------------------------------------------------
# Pattern 9: Webhook Integration with External Systems
# ------------------------------------------------------------------------------

module "integrated_repo" {
  source = "../../"

  repository_name  = "ci-integrated-service"
  parent_team_slug = "devops-team"

  webhooks = [
    {
      url          = "https://jenkins.example.com/github-webhook/"
      content_type = "json"
      insecure_ssl = false
      secret       = var.jenkins_webhook_secret
      events       = ["push", "pull_request"]
      active       = true
    },
    {
      url          = "https://sonarqube.example.com/api/github/webhook"
      content_type = "json"
      insecure_ssl = false
      secret       = var.sonar_webhook_secret
      events       = ["push", "pull_request"]
      active       = true
    },
    {
      url          = "https://slack.example.com/webhooks/github"
      content_type = "json"
      insecure_ssl = false
      secret       = var.slack_webhook_secret
      events       = ["push", "pull_request", "release", "issues"]
      active       = true
    }
  ]

  team_members = [
    { username = "devops-lead", role = "maintainer" }
  ]
}

# ------------------------------------------------------------------------------
# Pattern 10: Repository with Automated CODEOWNERS Generation
# ------------------------------------------------------------------------------

module "codeowners_repo" {
  source = "../../"

  repository_name  = "platform-core"
  parent_team_slug = "platform-team"

  team_members = [
    { username = "platform-lead", role = "maintainer" }
  ]
}

# Generate CODEOWNERS file based on repository structure
resource "github_repository_file" "codeowners" {
  repository = module.codeowners_repo.repository.name
  branch     = "main"
  file       = ".github/CODEOWNERS"

  content = templatefile("${path.module}/templates/CODEOWNERS.tftpl", {
    org           = data.github_organization.current.orgname
    platform_team = module.codeowners_repo.team.slug
    backend_team  = data.github_team.backend_team.slug
    frontend_team = data.github_team.frontend_team.slug
    security_team = data.github_team.security.slug
  })

  commit_message      = "Update CODEOWNERS [skip ci]"
  commit_author       = "Terraform Automation"
  commit_email        = "terraform@example.com"
  overwrite_on_create = true

  depends_on = [module.codeowners_repo]
}

# CODEOWNERS template (templates/CODEOWNERS.tftpl):
# * @${org}/${platform_team}
# /backend/** @${org}/${backend_team}
# /frontend/** @${org}/${frontend_team}
# /security/** @${org}/${security_team}
# /.github/** @${org}/${platform_team}

# ------------------------------------------------------------------------------
# Pattern 11: Cross-Repository Dependencies
# ------------------------------------------------------------------------------

# Main service repository
module "api_service" {
  source = "../../"

  repository_name  = "api-service"
  parent_team_slug = "backend-team"

  team_members = [
    { username = "backend-lead", role = "maintainer" }
  ]
}

# Documentation repository with access to API service team
module "api_docs" {
  source = "../../"

  repository_name  = "api-service-docs"
  parent_team_slug = "backend-team"

  repository_topics = concat(
    ["documentation"],
    module.api_service.repository.topics
  )

  team_members = [
    { username = "backend-lead", role = "maintainer" },
    { username = "tech-writer", role = "member" }
  ]
}

# Link repositories with topics
resource "github_repository_file" "api_service_link" {
  repository = module.api_service.repository.name
  branch     = "main"
  file       = ".github/RELATED_REPOS.md"

  content = <<-EOT
    # Related Repositories
    
    - Documentation: [${module.api_docs.repository.name}](${module.api_docs.repository.html_url})
    - Team: [${module.api_service.team.name}](https://github.com/orgs/${data.github_organization.current.orgname}/teams/${module.api_service.team.slug})
  EOT

  commit_message = "Add related repositories documentation"
}

# ------------------------------------------------------------------------------
# Pattern 12: Repository with Compliance Audit Trail
# ------------------------------------------------------------------------------

module "compliant_repo" {
  source = "../../"

  repository_name  = "financial-reporting"
  parent_team_slug = "finance-team"

  # Maximum security settings for compliance
  vulnerability_alerts                                  = true
  security_and_analysis_secret_scanning                 = "enabled"
  security_and_analysis_secret_scanning_push_protection = "enabled"
  security_and_analysis_advanced_security               = "enabled"

  # Strictest ruleset
  ruleset_enabled                                        = true
  ruleset_required_signatures                            = true
  ruleset_required_linear_history                        = true
  ruleset_deletion                                       = true
  ruleset_non_fast_forward                               = true
  ruleset_pull_request_required_approving_review_count   = 3
  ruleset_pull_request_require_code_owner_review         = true
  ruleset_pull_request_require_last_push_approval        = true
  ruleset_pull_request_required_review_thread_resolution = true

  # Compliance metadata
  rest_api_body = jsonencode({
    custom_properties = {
      compliance_frameworks = ["SOX", "PCI-DSS", "GDPR"]
      data_classification   = "confidential"
      audit_required        = "true"
      retention_years       = "7"
      encryption_required   = "true"
      backup_frequency      = "hourly"
    }
  })

  team_members = [
    { username = "compliance-lead", role = "maintainer" }
  ]
}

# ==============================================================================
# Supporting Data Sources
# ==============================================================================

data "github_organization" "current" {}

data "github_team" "backend_team" {
  slug = "backend-engineering"
}

data "github_team" "frontend_team" {
  slug = "frontend-engineering"
}

data "github_team" "security" {
  slug = "security-team"
}

# ==============================================================================
# Variables for Advanced Patterns
# ==============================================================================

variable "default_environments" {
  description = "Default environment configuration"
  type        = any
  default = {
    nonproductive = {
      wait_timer          = 0
      can_admins_bypass   = true
      prevent_self_review = false
    }
    prod = {
      wait_timer          = 300
      can_admins_bypass   = false
      prevent_self_review = true
    }
  }
}

variable "jenkins_webhook_secret" {
  description = "Jenkins webhook secret"
  type        = string
  sensitive   = true
  default     = ""
}

variable "sonar_webhook_secret" {
  description = "SonarQube webhook secret"
  type        = string
  sensitive   = true
  default     = ""
}

variable "slack_webhook_secret" {
  description = "Slack webhook secret"
  type        = string
  sensitive   = true
  default     = ""
}

# ==============================================================================
# End of Advanced Patterns
# ==============================================================================
