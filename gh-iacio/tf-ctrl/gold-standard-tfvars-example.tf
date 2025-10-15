# ==============================================================================
# Terraform Variables Example File
# ==============================================================================
# 
# Copy this file to terraform.tfvars and fill in your values
# 
# SECURITY WARNING: Never commit terraform.tfvars to version control!
# Add terraform.tfvars to .gitignore
#
# ==============================================================================

# ------------------------------------------------------------------------------
# GitHub Organization Configuration
# ------------------------------------------------------------------------------

github_organization = "my-organization"

# ------------------------------------------------------------------------------
# GitHub App Authentication (Recommended for Production)
# ------------------------------------------------------------------------------

github_app_id              = "123456"
github_app_installation_id = "12345678"

# Option 1: Reference PEM file
# github_app_pem_file = file("${path.module}/github-app.pem")

# Option 2: Use environment variable
# export TF_VAR_github_app_pem_file="$(cat github-app.pem)"

# Option 3: Direct content (not recommended - use for testing only)
# github_app_pem_file = <<-EOT
# -----BEGIN RSA PRIVATE KEY-----
# ...your key content...
# -----END RSA PRIVATE KEY-----
# EOT

# ------------------------------------------------------------------------------
# Alternative: Personal Access Token (PAT)
# ------------------------------------------------------------------------------
# Not used if GitHub App is configured
# Uncomment if using PAT instead of GitHub App

# github_token = "ghp_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"

# Or use environment variable:
# export GITHUB_TOKEN="ghp_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
# export TF_VAR_github_token="$GITHUB_TOKEN"

# ------------------------------------------------------------------------------
# Webhook Configuration
# ------------------------------------------------------------------------------

webhook_secret = "super-secret-webhook-validation-string"

# ------------------------------------------------------------------------------
# Deploy Keys
# ------------------------------------------------------------------------------

# Option 1: Reference SSH key file
deploy_key_public = file("~/.ssh/github_deploy_key.pub")

# Option 2: Direct content
# deploy_key_public = "ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAACAQC..."

# ------------------------------------------------------------------------------
# Repository-Specific Configuration
# ------------------------------------------------------------------------------

# Uncomment and modify as needed for specific repositories

# repository_name        = "my-awesome-service"
# repository_description = "My awesome microservice"
# repository_visibility  = "private"

# ------------------------------------------------------------------------------
# Team Configuration
# ------------------------------------------------------------------------------

# parent_team_slug = "platform-engineering"

# team_members = [
#   {
#     username = "alice"
#     role     = "maintainer"
#   },
#   {
#     username = "bob"
#     role     = "member"
#   }
# ]

# ------------------------------------------------------------------------------
# Security Settings
# ------------------------------------------------------------------------------

# vulnerability_alerts                                  = true
# security_and_analysis_secret_scanning                 = "enabled"
# security_and_analysis_secret_scanning_push_protection = "enabled"

# ------------------------------------------------------------------------------
# Environment-Specific Variables (for production/staging/dev)
# ------------------------------------------------------------------------------

# Development Environment
# environment = "dev"
# ruleset_enforcement = "evaluate"  # Test mode

# Staging Environment
# environment = "staging"
# ruleset_enforcement = "active"

# Production Environment
# environment = "prod"
# ruleset_enforcement = "active"
# ruleset_required_signatures = true

# ==============================================================================
# Best Practices
# ==============================================================================
#
# 1. Use GitHub App authentication for production
# 2. Store sensitive values in a secrets manager (Vault, AWS Secrets Manager)
# 3. Use environment variables for CI/CD pipelines
# 4. Never commit this file with real values to version control
# 5. Use different tfvars files for different environments
#
# ==============================================================================

# ==============================================================================
# Environment Variable Reference
# ==============================================================================
#
# You can set these environment variables instead of using terraform.tfvars:
#
# export TF_VAR_github_organization="my-org"
# export TF_VAR_github_app_id="123456"
# export TF_VAR_github_app_installation_id="12345678"
# export TF_VAR_github_app_pem_file="$(cat github-app.pem)"
# export TF_VAR_webhook_secret="super-secret"
# export TF_VAR_deploy_key_public="$(cat ~/.ssh/deploy_key.pub)"
#
# ==============================================================================

# ==============================================================================
# GitHub App Setup Instructions
# ==============================================================================
#
# 1. Navigate to: https://github.com/organizations/YOUR-ORG/settings/apps
# 2. Click "New GitHub App"
# 3. Set the following permissions:
#    - Repository administration: Read and write
#    - Repository contents: Read and write
#    - Organization administration: Read and write
#    - Members: Read and write
#    - Metadata: Read-only
# 4. Generate a private key and save it securely
# 5. Install the app on your organization
# 6. Note the App ID and Installation ID
#
# ==============================================================================
