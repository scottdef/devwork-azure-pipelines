# ============================================================================
# GitHub Organization Settings - Complete Resource Block
# Provider: integrations/github ~> 6.6
# ============================================================================
#
# Rob Pike + Ken Thompson would approve of this simplicity
# Every argument, properly typed, sensible enterprise defaults
#

# ============================================================================
# Variables
# ============================================================================

variable "org_billing_email" {
  description = "Billing email address for the organization"
  type        = string
}

variable "org_company" {
  description = "Company name displayed on organization profile"
  type        = string
  default     = ""
}

variable "org_blog_url" {
  description = "Blog URL for the organization"
  type        = string
  default     = ""
}

variable "org_email" {
  description = "Public email address for the organization"
  type        = string
  default     = ""
}

variable "org_twitter_username" {
  description = "Twitter username for the organization (without @)"
  type        = string
  default     = ""
}

variable "org_location" {
  description = "Organization location"
  type        = string
  default     = ""
}

variable "org_name" {
  description = "Display name for the organization"
  type        = string
  default     = ""
}

variable "org_description" {
  description = "Organization description"
  type        = string
  default     = ""
}

# Repository Creation & Permissions
variable "default_repository_permission" {
  description = "Default permission level members have for organization repositories (read, write, admin, none)"
  type        = string
  default     = "read"
  validation {
    condition     = contains(["read", "write", "admin", "none"], var.default_repository_permission)
    error_message = "Must be one of: read, write, admin, none"
  }
}

variable "members_can_create_repositories" {
  description = "Whether members can create repositories"
  type        = bool
  default     = false
}

variable "members_can_create_public_repositories" {
  description = "Whether members can create public repositories"
  type        = bool
  default     = false
}

variable "members_can_create_private_repositories" {
  description = "Whether members can create private repositories"
  type        = bool
  default     = false
}

variable "members_can_create_internal_repositories" {
  description = "Whether members can create internal repositories (GitHub Enterprise Cloud only)"
  type        = bool
  default     = false
}

variable "members_allowed_repository_creation_type" {
  description = "Specifies which types of repositories members can create (all, private, none)"
  type        = string
  default     = "none"
  validation {
    condition     = contains(["all", "private", "internal", "none"], var.members_allowed_repository_creation_type)
    error_message = "Must be one of: all, private, internal, none"
  }
}

# Pages Settings
variable "members_can_create_pages" {
  description = "Whether members can create GitHub Pages sites"
  type        = bool
  default     = false
}

variable "members_can_create_public_pages" {
  description = "Whether members can create public GitHub Pages sites"
  type        = bool
  default     = false
}

variable "members_can_create_private_pages" {
  description = "Whether members can create private GitHub Pages sites"
  type        = bool
  default     = false
}

# Fork Settings
variable "members_can_fork_private_repositories" {
  description = "Whether members can fork private organization repositories"
  type        = bool
  default     = false
}

# Commit Settings
variable "web_commit_signoff_required" {
  description = "Whether contributors are required to sign off on web-based commits"
  type        = bool
  default     = false
}

# Project Settings
variable "has_organization_projects" {
  description = "Whether organization projects are enabled"
  type        = bool
  default     = true
}

variable "has_repository_projects" {
  description = "Whether repository projects are enabled"
  type        = bool
  default     = true
}

# Advanced Security & Dependency Settings
variable "advanced_security_enabled_for_new_repositories" {
  description = "Enable GitHub Advanced Security for new repositories (requires GitHub Enterprise Cloud)"
  type        = bool
  default     = false
}

variable "secret_scanning_enabled_for_new_repositories" {
  description = "Enable secret scanning for new repositories"
  type        = bool
  default     = true
}

variable "secret_scanning_push_protection_enabled_for_new_repositories" {
  description = "Enable secret scanning push protection for new repositories"
  type        = bool
  default     = true
}

variable "dependabot_alerts_enabled_for_new_repositories" {
  description = "Enable Dependabot alerts for new repositories"
  type        = bool
  default     = true
}

variable "dependabot_security_updates_enabled_for_new_repositories" {
  description = "Enable Dependabot security updates for new repositories"
  type        = bool
  default     = true
}

variable "dependency_graph_enabled_for_new_repositories" {
  description = "Enable dependency graph for new repositories"
  type        = bool
  default     = true
}

# ============================================================================
# Resource Block
# ============================================================================

resource "github_organization_settings" "main" {
  # Basic Organization Info
  billing_email    = var.org_billing_email
  company          = var.org_company
  blog             = var.org_blog_url
  email            = var.org_email
  twitter_username = var.org_twitter_username
  location         = var.org_location
  name             = var.org_name
  description      = var.org_description

  # Project Settings
  has_organization_projects = var.has_organization_projects
  has_repository_projects   = var.has_repository_projects

  # Member Repository Creation Permissions
  default_repository_permission               = var.default_repository_permission
  members_can_create_repositories             = var.members_can_create_repositories
  members_can_create_public_repositories      = var.members_can_create_public_repositories
  members_can_create_private_repositories     = var.members_can_create_private_repositories
  members_can_create_internal_repositories    = var.members_can_create_internal_repositories
  members_allowed_repository_creation_type    = var.members_allowed_repository_creation_type

  # Pages Permissions
  members_can_create_pages         = var.members_can_create_pages
  members_can_create_public_pages  = var.members_can_create_public_pages
  members_can_create_private_pages = var.members_can_create_private_pages

  # Fork & Commit Settings
  members_can_fork_private_repositories = var.members_can_fork_private_repositories
  web_commit_signoff_required           = var.web_commit_signoff_required

  # Security Settings for New Repositories
  advanced_security_enabled_for_new_repositories                = var.advanced_security_enabled_for_new_repositories
  secret_scanning_enabled_for_new_repositories                  = var.secret_scanning_enabled_for_new_repositories
  secret_scanning_push_protection_enabled_for_new_repositories  = var.secret_scanning_push_protection_enabled_for_new_repositories
  dependabot_alerts_enabled_for_new_repositories                = var.dependabot_alerts_enabled_for_new_repositories
  dependabot_security_updates_enabled_for_new_repositories      = var.dependabot_security_updates_enabled_for_new_repositories
  dependency_graph_enabled_for_new_repositories                 = var.dependency_graph_enabled_for_new_repositories
}

# ============================================================================
# Outputs
# ============================================================================

output "organization_id" {
  description = "GitHub organization ID"
  value       = github_organization_settings.main.id
}

output "organization_settings" {
  description = "Complete organization settings object"
  value       = github_organization_settings.main
  sensitive   = true
}
