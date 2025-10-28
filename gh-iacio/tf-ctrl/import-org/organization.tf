# GitHub Organization Settings
# Import command: terraform import github_organization_settings.this <org-name>

resource "github_organization_settings" "this" {
  billing_email = var.org_billing_email

  # Email visibility
  company      = var.org_company
  email        = var.org_email
  twitter_username = try(var.org_twitter, null)
  location     = try(var.org_location, null)
  description  = try(var.org_description, null)
  
  # Feature flags
  has_organization_projects = true
  has_repository_projects   = true
  
  # Security settings
  default_repository_permission             = "read"
  members_can_create_repositories           = false
  members_can_create_public_repositories    = false
  members_can_create_private_repositories   = false
  members_can_create_internal_repositories  = false
  members_can_create_pages                  = false
  members_can_create_public_pages           = false
  members_can_create_private_pages          = false
  members_can_fork_private_repositories     = false
  
  # 2FA requirement
  web_commit_signoff_required = true
  
  # Advanced security
  advanced_security_enabled_for_new_repositories               = true
  dependabot_alerts_enabled_for_new_repositories              = true
  dependabot_security_updates_enabled_for_new_repositories    = true
  dependency_graph_enabled_for_new_repositories               = true
  secret_scanning_enabled_for_new_repositories                = true
  secret_scanning_push_protection_enabled_for_new_repositories = true
  
  # Member capabilities
  members_can_create_teams = false
  
  # Blog and name
  name = try(var.org_display_name, null)
  blog = try(var.org_blog, null)
}
