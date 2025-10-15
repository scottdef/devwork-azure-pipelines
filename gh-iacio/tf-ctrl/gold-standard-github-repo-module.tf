# ==============================================================================
# Gold Standard GitHub Repository Terraform Module
# Version: 1.0.0
# GitHub Provider: v6.6.0
# ==============================================================================
#
# Directory Structure:
#   modules/gold-standard-github-repository/
#   ├── main.tf           - Main resource definitions
#   ├── variables.tf      - Input variables
#   ├── outputs.tf        - Output values
#   ├── versions.tf       - Provider requirements
#   ├── README.md         - Documentation
#   └── examples/         - Usage examples
#       └── complete/
#           ├── main.tf
#           └── terraform.tfvars.example
#
# ==============================================================================

# ------------------------------------------------------------------------------
# versions.tf
# ------------------------------------------------------------------------------

terraform {
  required_version = ">= 1.5.0"

  required_providers {
    github = {
      source  = "integrations/github"
      version = "~> 6.6"
    }
  }
}

# ------------------------------------------------------------------------------
# variables.tf
# ------------------------------------------------------------------------------

# Repository Core Variables
variable "repository_name" {
  description = "Name of the repository to create"
  type        = string

  validation {
    condition     = can(regex("^[a-zA-Z0-9._-]+$", var.repository_name))
    error_message = "Repository name can only contain alphanumeric characters, periods, underscores, and hyphens."
  }
}

variable "repository_description" {
  description = "Description of the repository"
  type        = string
  default     = "Gold standard repository managed by Terraform"
}

variable "repository_homepage_url" {
  description = "URL of a page describing the project"
  type        = string
  default     = ""
}

variable "repository_visibility" {
  description = "Repository visibility (public, private, internal)"
  type        = string
  default     = "private"

  validation {
    condition     = contains(["public", "private", "internal"], var.repository_visibility)
    error_message = "Visibility must be one of: public, private, internal."
  }
}

variable "repository_topics" {
  description = "List of topics for the repository"
  type        = list(string)
  default     = ["gold-standard", "terraform-managed"]
}

variable "repository_is_template" {
  description = "Whether the repository is a template repository"
  type        = bool
  default     = false
}

variable "repository_archived" {
  description = "Whether the repository is archived"
  type        = bool
  default     = false
}

variable "repository_archive_on_destroy" {
  description = "Archive repository instead of deleting on destroy"
  type        = bool
  default     = true
}

# Repository Features
variable "has_issues" {
  description = "Enable issues for the repository"
  type        = bool
  default     = true
}

variable "has_discussions" {
  description = "Enable discussions for the repository"
  type        = bool
  default     = false
}

variable "has_projects" {
  description = "Enable projects for the repository"
  type        = bool
  default     = true
}

variable "has_wiki" {
  description = "Enable wiki for the repository"
  type        = bool
  default     = false
}

variable "has_downloads" {
  description = "Enable downloads feature"
  type        = bool
  default     = true
}

# Repository Initialization
variable "auto_init" {
  description = "Initialize repository with README"
  type        = bool
  default     = true
}

variable "gitignore_template" {
  description = "Gitignore template to use"
  type        = string
  default     = ""
}

variable "license_template" {
  description = "License template to use"
  type        = string
  default     = ""
}

# Branch Settings
variable "default_branch" {
  description = "Default branch name"
  type        = string
  default     = "main"
}

variable "delete_branch_on_merge" {
  description = "Automatically delete head branches after PR merge"
  type        = bool
  default     = true
}

# Merge Settings
variable "allow_merge_commit" {
  description = "Allow merge commits"
  type        = bool
  default     = true
}

variable "allow_squash_merge" {
  description = "Allow squash merges"
  type        = bool
  default     = true
}

variable "allow_rebase_merge" {
  description = "Allow rebase merges"
  type        = bool
  default     = false
}

variable "allow_auto_merge" {
  description = "Allow auto-merge on pull requests"
  type        = bool
  default     = false
}

variable "allow_update_branch" {
  description = "Allow updating pull request branches"
  type        = bool
  default     = true
}

variable "squash_merge_commit_title" {
  description = "Squash merge commit title (PR_TITLE or COMMIT_OR_PR_TITLE)"
  type        = string
  default     = "PR_TITLE"
}

variable "squash_merge_commit_message" {
  description = "Squash merge commit message (PR_BODY, COMMIT_MESSAGES, or BLANK)"
  type        = string
  default     = "PR_BODY"
}

variable "merge_commit_title" {
  description = "Merge commit title (PR_TITLE or MERGE_MESSAGE)"
  type        = string
  default     = "MERGE_MESSAGE"
}

variable "merge_commit_message" {
  description = "Merge commit message (PR_BODY, PR_TITLE, or BLANK)"
  type        = string
  default     = "PR_BODY"
}

# Security and Analysis
variable "vulnerability_alerts" {
  description = "Enable vulnerability alerts"
  type        = bool
  default     = true
}

variable "security_and_analysis_secret_scanning" {
  description = "Enable secret scanning (enabled or disabled)"
  type        = string
  default     = "enabled"
}

variable "security_and_analysis_secret_scanning_push_protection" {
  description = "Enable secret scanning push protection (enabled or disabled)"
  type        = string
  default     = "enabled"
}

variable "security_and_analysis_advanced_security" {
  description = "Enable advanced security (enabled or disabled) - requires GHAS"
  type        = string
  default     = "disabled"
}

# Template Repository
variable "template_owner" {
  description = "Owner of template repository"
  type        = string
  default     = ""
}

variable "template_repository" {
  description = "Template repository to use"
  type        = string
  default     = ""
}

variable "template_include_all_branches" {
  description = "Include all branches from template"
  type        = bool
  default     = false
}

# Pages Configuration
variable "pages_enabled" {
  description = "Enable GitHub Pages"
  type        = bool
  default     = false
}

variable "pages_source_branch" {
  description = "Branch for GitHub Pages source"
  type        = string
  default     = "gh-pages"
}

variable "pages_source_path" {
  description = "Path for GitHub Pages source (/ or /docs)"
  type        = string
  default     = "/"
}

variable "pages_cname" {
  description = "Custom domain for GitHub Pages"
  type        = string
  default     = ""
}

variable "pages_build_type" {
  description = "Build type for Pages (legacy or workflow)"
  type        = string
  default     = "legacy"
}

# Actions and Workflows - DISABLED BY DEFAULT per requirements
variable "actions_enabled" {
  description = "Enable GitHub Actions for the repository"
  type        = bool
  default     = false
}

variable "allowed_actions" {
  description = "Actions allowed (all, local_only, selected)"
  type        = string
  default     = "all"
}

variable "allowed_actions_patterns" {
  description = "Patterns of allowed actions when allowed_actions is selected"
  type        = list(string)
  default     = []
}

variable "github_owned_allowed" {
  description = "Allow GitHub-owned actions"
  type        = bool
  default     = true
}

variable "verified_allowed" {
  description = "Allow verified creator actions"
  type        = bool
  default     = true
}

variable "patterns_allowed" {
  description = "Allow specific action patterns"
  type        = list(string)
  default     = []
}

# Webhooks
variable "webhooks" {
  description = "List of webhooks to create"
  type = list(object({
    url          = string
    content_type = string
    insecure_ssl = bool
    secret       = string
    events       = list(string)
    active       = bool
  }))
  default = []
}

# Team Configuration
variable "parent_team_slug" {
  description = "Slug of the parent team"
  type        = string
  default     = "super-parent-team"
}

variable "team_name_suffix" {
  description = "Suffix for the team name (will be prefixed with repo name)"
  type        = string
  default     = "team"
}

variable "team_description" {
  description = "Description for the team"
  type        = string
  default     = "Team managing the repository"
}

variable "team_privacy" {
  description = "Team privacy level (secret or closed)"
  type        = string
  default     = "closed"
}

variable "team_create_default_maintainer" {
  description = "Add creator as maintainer"
  type        = bool
  default     = false
}

variable "team_ldap_dn" {
  description = "LDAP Distinguished Name for team sync"
  type        = string
  default     = ""
}

# Team Repository Access
variable "team_repository_permission" {
  description = "Team permission level (pull, triage, push, maintain, admin)"
  type        = string
  default     = "admin"
}

# Team Members
variable "team_members" {
  description = "List of team members with their roles"
  type = list(object({
    username = string
    role     = string # member or maintainer
  }))
  default = []
}

# Custom Role Configuration
variable "create_custom_role" {
  description = "Create organization custom role"
  type        = bool
  default     = true
}

variable "custom_role_name" {
  description = "Custom role name (will be prefixed with repo name if empty)"
  type        = string
  default     = ""
}

variable "custom_role_description" {
  description = "Custom role description"
  type        = string
  default     = "Super developer role with enhanced permissions"
}

variable "custom_role_base_role" {
  description = "Base role for custom role (read, triage, write, maintain, admin)"
  type        = string
  default     = "write"
}

variable "custom_role_permissions" {
  description = "List of permissions for custom role"
  type        = list(string)
  default = [
    "read_code",
    "write_code",
    "read_actions",
    "write_actions",
    "read_deployments",
    "write_deployments",
    "manage_pull_requests",
    "manage_issues"
  ]
}

# Environment Configuration
variable "environments" {
  description = "Repository environments configuration"
  type = map(object({
    wait_timer                     = optional(number, 0)
    can_admins_bypass              = optional(bool, true)
    prevent_self_review            = optional(bool, false)
    reviewers_teams                = optional(list(string), [])
    reviewers_users                = optional(list(string), [])
    deployment_branch_policy_type  = optional(string, "protected")
    deployment_branch_policy_patterns = optional(list(string), [])
  }))
  default = {
    nonproductive = {
      wait_timer                    = 0
      can_admins_bypass             = true
      prevent_self_review           = false
      deployment_branch_policy_type = "protected"
    }
    prod = {
      wait_timer                    = 300
      can_admins_bypass             = false
      prevent_self_review           = true
      deployment_branch_policy_type = "protected"
    }
  }
}

# Ruleset Configuration
variable "ruleset_enabled" {
  description = "Enable repository ruleset"
  type        = bool
  default     = true
}

variable "ruleset_name" {
  description = "Name of the repository ruleset"
  type        = string
  default     = "Default Branch Protection"
}

variable "ruleset_target" {
  description = "Ruleset target (branch or tag)"
  type        = string
  default     = "branch"
}

variable "ruleset_enforcement" {
  description = "Ruleset enforcement level (disabled, active, evaluate)"
  type        = string
  default     = "active"
}

variable "ruleset_bypass_actors" {
  description = "Actors who can bypass the ruleset"
  type = list(object({
    actor_id    = number
    actor_type  = string
    bypass_mode = string
  }))
  default = [
    {
      actor_id    = 5
      actor_type  = "RepositoryRole"
      bypass_mode = "always"
    }
  ]
}

variable "ruleset_ref_name_include" {
  description = "Ref name patterns to include in ruleset"
  type        = list(string)
  default     = ["~DEFAULT_BRANCH"]
}

variable "ruleset_ref_name_exclude" {
  description = "Ref name patterns to exclude from ruleset"
  type        = list(string)
  default     = []
}

variable "ruleset_deletion" {
  description = "Prevent deletion of matching refs"
  type        = bool
  default     = true
}

variable "ruleset_non_fast_forward" {
  description = "Prevent non-fast-forward pushes"
  type        = bool
  default     = true
}

variable "ruleset_required_linear_history" {
  description = "Require linear history"
  type        = bool
  default     = false
}

variable "ruleset_required_signatures" {
  description = "Require signed commits"
  type        = bool
  default     = false
}

variable "ruleset_pull_request_required_approving_review_count" {
  description = "Number of required approving reviews"
  type        = number
  default     = 2
}

variable "ruleset_pull_request_dismiss_stale_reviews_on_push" {
  description = "Dismiss stale reviews on new push"
  type        = bool
  default     = true
}

variable "ruleset_pull_request_require_code_owner_review" {
  description = "Require code owner review"
  type        = bool
  default     = true
}

variable "ruleset_pull_request_require_last_push_approval" {
  description = "Require approval of most recent push"
  type        = bool
  default     = false
}

variable "ruleset_pull_request_required_review_thread_resolution" {
  description = "Require resolution of all review threads"
  type        = bool
  default     = true
}

variable "ruleset_required_status_checks" {
  description = "List of required status check contexts"
  type        = list(string)
  default     = []
}

variable "ruleset_required_status_checks_strict" {
  description = "Require branches to be up to date before merging"
  type        = bool
  default     = true
}

# REST API Configuration
variable "rest_api_endpoint" {
  description = "REST API endpoint to call for repository metadata"
  type        = string
  default     = ""
}

variable "rest_api_method" {
  description = "HTTP method for REST API call (GET, POST, PATCH, PUT, DELETE)"
  type        = string
  default     = "PATCH"
}

variable "rest_api_body" {
  description = "Request body for REST API call"
  type        = string
  default     = ""
}

# Creator Metadata
variable "creator_username" {
  description = "Username to use for creator metadata"
  type        = string
  default     = "terraform"
}

# Collaborators
variable "collaborators" {
  description = "List of outside collaborators"
  type = list(object({
    username   = string
    permission = string
  }))
  default = []
}

# Deploy Keys
variable "deploy_keys" {
  description = "List of deploy keys"
  type = list(object({
    title     = string
    key       = string
    read_only = bool
  }))
  default = []
}

# Tags for organization
variable "additional_topics" {
  description = "Additional topics beyond defaults"
  type        = list(string)
  default     = []
}

# ------------------------------------------------------------------------------
# locals.tf
# ------------------------------------------------------------------------------

locals {
  # Combine default and additional topics
  all_topics = distinct(concat(
    var.repository_topics,
    var.additional_topics,
    ["creator:${var.creator_username}"]
  ))

  # Custom role name
  custom_role_name = var.custom_role_name != "" ? var.custom_role_name : "${var.repository_name}-superdev"

  # Team name
  team_name = "${var.repository_name}-${var.team_name_suffix}"

  # REST API configuration for adding custom metadata
  rest_api_endpoint = var.rest_api_endpoint != "" ? var.rest_api_endpoint : "repos/${data.github_organization.current.orgname}/${var.repository_name}"

  rest_api_body = var.rest_api_body != "" ? var.rest_api_body : jsonencode({
    custom_properties = {
      creator        = var.creator_username
      terraform      = "true"
      gold_standard  = "true"
      managed_by     = "terraform"
    }
  })

  # Template configuration
  use_template = var.template_owner != "" && var.template_repository != ""
}

# ------------------------------------------------------------------------------
# data.tf - Data Sources
# ------------------------------------------------------------------------------

# Get current organization details
data "github_organization" "current" {}

# Get organization teams
data "github_organization_teams" "all" {}

# Get parent team details
data "github_team" "parent" {
  slug = var.parent_team_slug
}

# Get creator user details
data "github_user" "creator" {
  username = var.creator_username
}

# Check if repository already exists
data "github_repositories" "existing" {
  query = "org:${data.github_organization.current.orgname} ${var.repository_name}"
}

# REST API to get additional repository metadata
data "github_rest_api" "repo_metadata" {
  count = var.repository_name != "" ? 1 : 0

  endpoint = "repos/${data.github_organization.current.orgname}/${var.repository_name}"

  depends_on = [github_repository.main]
}

# ------------------------------------------------------------------------------
# main.tf - Resources
# ------------------------------------------------------------------------------

# Validation: Fail if repository already exists
resource "null_resource" "validate_repo_not_exists" {
  lifecycle {
    precondition {
      condition     = length([for repo in data.github_repositories.existing.names : repo if repo == var.repository_name]) == 0
      error_message = "Repository '${var.repository_name}' already exists in organization '${data.github_organization.current.orgname}'. Please choose a different name or import the existing repository."
    }
  }
}

# Main Repository
resource "github_repository" "main" {
  name        = var.repository_name
  description = var.repository_description
  homepage_url = var.repository_homepage_url
  visibility  = var.repository_visibility

  # Topics - includes gold-standard and creator:terraform
  topics = local.all_topics

  # Template configuration
  dynamic "template" {
    for_each = local.use_template ? [1] : []
    content {
      owner                = var.template_owner
      repository           = var.template_repository
      include_all_branches = var.template_include_all_branches
    }
  }

  # Repository features
  has_issues      = var.has_issues
  has_discussions = var.has_discussions
  has_projects    = var.has_projects
  has_wiki        = var.has_wiki
  has_downloads   = var.has_downloads

  # Repository settings
  is_template            = var.repository_is_template
  archived               = var.repository_archived
  archive_on_destroy     = var.repository_archive_on_destroy
  auto_init              = var.auto_init
  gitignore_template     = var.gitignore_template
  license_template       = var.license_template

  # Branch settings
  delete_branch_on_merge = var.delete_branch_on_merge

  # Merge settings
  allow_merge_commit     = var.allow_merge_commit
  allow_squash_merge     = var.allow_squash_merge
  allow_rebase_merge     = var.allow_rebase_merge
  allow_auto_merge       = var.allow_auto_merge
  allow_update_branch    = var.allow_update_branch

  squash_merge_commit_title   = var.squash_merge_commit_title
  squash_merge_commit_message = var.squash_merge_commit_message
  merge_commit_title          = var.merge_commit_title
  merge_commit_message        = var.merge_commit_message

  # Security settings
  vulnerability_alerts = var.vulnerability_alerts

  security_and_analysis {
    secret_scanning {
      status = var.security_and_analysis_secret_scanning
    }
    secret_scanning_push_protection {
      status = var.security_and_analysis_secret_scanning_push_protection
    }
    dynamic "advanced_security" {
      for_each = var.security_and_analysis_advanced_security != "disabled" ? [1] : []
      content {
        status = var.security_and_analysis_advanced_security
      }
    }
  }

  # GitHub Pages configuration
  dynamic "pages" {
    for_each = var.pages_enabled ? [1] : []
    content {
      source {
        branch = var.pages_source_branch
        path   = var.pages_source_path
      }
      cname      = var.pages_cname != "" ? var.pages_cname : null
      build_type = var.pages_build_type
    }
  }

  lifecycle {
    ignore_changes = [
      auto_init, # Ignore after initial creation
    ]
  }

  depends_on = [null_resource.validate_repo_not_exists]
}

# Repository Actions Configuration - DISABLED by default, but accessible by workflows
resource "github_actions_repository_permissions" "main" {
  count = var.actions_enabled ? 0 : 1

  repository = github_repository.main.name
  enabled    = false # Actions disabled per requirements
  allowed_actions_config {
    github_owned_allowed = var.github_owned_allowed
    patterns_allowed     = var.patterns_allowed
    verified_allowed     = var.verified_allowed
  }
}

# Repository Team
resource "github_team" "main" {
  name        = local.team_name
  description = var.team_description
  privacy     = var.team_privacy

  parent_team_id = data.github_team.parent.id

  create_default_maintainer = var.team_create_default_maintainer
  ldap_dn                  = var.team_ldap_dn != "" ? var.team_ldap_dn : null
}

# Team Repository Access
resource "github_team_repository" "main" {
  team_id    = github_team.main.id
  repository = github_repository.main.name
  permission = var.team_repository_permission
}

# Team Members
resource "github_team_membership" "members" {
  for_each = { for member in var.team_members : member.username => member }

  team_id  = github_team.main.id
  username = each.value.username
  role     = each.value.role
}

# Custom Organization Role
resource "github_organization_custom_role" "main" {
  count = var.create_custom_role ? 1 : 0

  name        = local.custom_role_name
  description = var.custom_role_description
  base_role   = var.custom_role_base_role
  permissions = var.custom_role_permissions
}

# Repository Environments
resource "github_repository_environment" "environments" {
  for_each = var.environments

  repository  = github_repository.main.name
  environment = each.key
  wait_timer  = each.value.wait_timer

  dynamic "reviewers" {
    for_each = length(each.value.reviewers_teams) > 0 || length(each.value.reviewers_users) > 0 ? [1] : []
    content {
      teams = each.value.reviewers_teams
      users = each.value.reviewers_users
    }
  }

  deployment_branch_policy {
    protected_branches     = each.value.deployment_branch_policy_type == "protected"
    custom_branch_policies = each.value.deployment_branch_policy_type == "custom"
  }

  can_admins_bypass   = each.value.can_admins_bypass
  prevent_self_review = each.value.prevent_self_review
}

# Repository Ruleset
resource "github_repository_ruleset" "main" {
  count = var.ruleset_enabled ? 1 : 0

  repository  = github_repository.main.name
  name        = var.ruleset_name
  target      = var.ruleset_target
  enforcement = var.ruleset_enforcement

  conditions {
    ref_name {
      include = var.ruleset_ref_name_include
      exclude = var.ruleset_ref_name_exclude
    }
  }

  dynamic "bypass_actors" {
    for_each = var.ruleset_bypass_actors
    content {
      actor_id    = bypass_actors.value.actor_id
      actor_type  = bypass_actors.value.actor_type
      bypass_mode = bypass_actors.value.bypass_mode
    }
  }

  rules {
    deletion                = var.ruleset_deletion
    non_fast_forward        = var.ruleset_non_fast_forward
    required_linear_history = var.ruleset_required_linear_history
    required_signatures     = var.ruleset_required_signatures

    dynamic "pull_request" {
      for_each = var.ruleset_pull_request_required_approving_review_count > 0 ? [1] : []
      content {
        required_approving_review_count   = var.ruleset_pull_request_required_approving_review_count
        dismiss_stale_reviews_on_push     = var.ruleset_pull_request_dismiss_stale_reviews_on_push
        require_code_owner_review         = var.ruleset_pull_request_require_code_owner_review
        require_last_push_approval        = var.ruleset_pull_request_require_last_push_approval
        required_review_thread_resolution = var.ruleset_pull_request_required_review_thread_resolution
      }
    }

    dynamic "required_status_checks" {
      for_each = length(var.ruleset_required_status_checks) > 0 ? [1] : []
      content {
        strict_required_status_checks_policy = var.ruleset_required_status_checks_strict

        dynamic "required_check" {
          for_each = var.ruleset_required_status_checks
          content {
            context = required_check.value
          }
        }
      }
    }
  }
}

# REST API Call for Custom Metadata
resource "github_rest_api" "repository_metadata" {
  count = var.rest_api_endpoint != "" || var.rest_api_body != "" ? 1 : 0

  endpoint = local.rest_api_endpoint
  method   = var.rest_api_method
  body     = local.rest_api_body

  depends_on = [github_repository.main]
}

# Repository Webhooks
resource "github_repository_webhook" "webhooks" {
  for_each = { for idx, webhook in var.webhooks : idx => webhook }

  repository = github_repository.main.name
  active     = each.value.active
  events     = each.value.events

  configuration {
    url          = each.value.url
    content_type = each.value.content_type
    insecure_ssl = each.value.insecure_ssl
    secret       = each.value.secret
  }
}

# Repository Collaborators
resource "github_repository_collaborator" "collaborators" {
  for_each = { for collaborator in var.collaborators : collaborator.username => collaborator }

  repository = github_repository.main.name
  username   = each.value.username
  permission = each.value.permission
}

# Deploy Keys
resource "github_repository_deploy_key" "deploy_keys" {
  for_each = { for idx, key in var.deploy_keys : idx => key }

  repository = github_repository.main.name
  title      = each.value.title
  key        = each.value.key
  read_only  = each.value.read_only
}

# ------------------------------------------------------------------------------
# outputs.tf
# ------------------------------------------------------------------------------

output "repository" {
  description = "Repository details"
  value = {
    id                = github_repository.main.id
    node_id           = github_repository.main.node_id
    name              = github_repository.main.name
    full_name         = github_repository.main.full_name
    description       = github_repository.main.description
    html_url          = github_repository.main.html_url
    ssh_clone_url     = github_repository.main.ssh_clone_url
    http_clone_url    = github_repository.main.http_clone_url
    git_clone_url     = github_repository.main.git_clone_url
    svn_url           = github_repository.main.svn_url
    default_branch    = github_repository.main.default_branch
    primary_language  = github_repository.main.primary_language
    topics            = github_repository.main.topics
    visibility        = github_repository.main.visibility
  }
}

output "team" {
  description = "Team details"
  value = {
    id          = github_team.main.id
    node_id     = github_team.main.node_id
    name        = github_team.main.name
    slug        = github_team.main.slug
    description = github_team.main.description
    privacy     = github_team.main.privacy
  }
}

output "parent_team" {
  description = "Parent team details"
  value = {
    id          = data.github_team.parent.id
    name        = data.github_team.parent.name
    slug        = data.github_team.parent.slug
    description = data.github_team.parent.description
  }
}

output "custom_role" {
  description = "Custom role details"
  value = var.create_custom_role ? {
    id          = github_organization_custom_role.main[0].id
    name        = github_organization_custom_role.main[0].name
    description = github_organization_custom_role.main[0].description
    base_role   = github_organization_custom_role.main[0].base_role
    permissions = github_organization_custom_role.main[0].permissions
  } : null
}

output "environments" {
  description = "Repository environments"
  value = {
    for env_name, env in github_repository_environment.environments :
    env_name => {
      id          = env.id
      environment = env.environment
    }
  }
}

output "ruleset" {
  description = "Repository ruleset details"
  value = var.ruleset_enabled ? {
    id          = github_repository_ruleset.main[0].id
    node_id     = github_repository_ruleset.main[0].node_id
    name        = github_repository_ruleset.main[0].name
    target      = github_repository_ruleset.main[0].target
    enforcement = github_repository_ruleset.main[0].enforcement
  } : null
}

output "creator_user" {
  description = "Creator user details"
  value = {
    id       = data.github_user.creator.id
    login    = data.github_user.creator.login
    name     = data.github_user.creator.name
    email    = data.github_user.creator.email
  }
}

output "organization" {
  description = "Organization details"
  value = {
    id       = data.github_organization.current.id
    login    = data.github_organization.current.login
    name     = data.github_organization.current.name
    node_id  = data.github_organization.current.node_id
  }
}

output "repository_metadata" {
  description = "Additional repository metadata from REST API"
  value       = length(data.github_rest_api.repo_metadata) > 0 ? jsondecode(data.github_rest_api.repo_metadata[0].body) : null
  sensitive   = true
}
