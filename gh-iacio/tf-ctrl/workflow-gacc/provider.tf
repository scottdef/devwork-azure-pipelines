# ============================================================================
# Provider Configuration
# ============================================================================
# This file configures the GitHub Terraform provider
# Authentication via environment variables (GITHUB_TOKEN, GITHUB_OWNER)
#

terraform {
  required_version = ">= 1.5.0"

  required_providers {
    github = {
      source  = "integrations/github"
      version = "~> 6.6"
    }
  }

  # Optional: Configure remote backend
  # Uncomment and configure for team collaboration
  #
  # backend "s3" {
  #   bucket         = "your-terraform-state-bucket"
  #   key            = "github-org/terraform.tfstate"
  #   region         = "us-east-1"
  #   encrypt        = true
  #   dynamodb_table = "terraform-state-lock"
  # }
  #
  # backend "remote" {
  #   hostname     = "app.terraform.io"
  #   organization = "your-org"
  #   workspaces {
  #     name = "github-organization"
  #   }
  # }
}

# Provider configuration using environment variables
# GITHUB_TOKEN - Personal Access Token or GitHub App token
# GITHUB_OWNER - Organization name
provider "github" {
  owner = var.github_owner

  # Optional: GitHub Enterprise Server
  # base_url = "https://github.yourcompany.com"

  # Optional: App Authentication (recommended for automation)
  # app_auth {
  #   id              = var.github_app_id
  #   installation_id = var.github_app_installation_id
  #   pem_file        = var.github_app_pem_file
  # }
}

# Variable for organization name
variable "github_owner" {
  description = "GitHub organization name (defaults to GITHUB_OWNER env var)"
  type        = string
  default     = ""
}

# Optional: Variables for GitHub App authentication
# variable "github_app_id" {
#   description = "GitHub App ID"
#   type        = string
#   sensitive   = true
# }

# variable "github_app_installation_id" {
#   description = "GitHub App Installation ID"
#   type        = string
#   sensitive   = true
# }

# variable "github_app_pem_file" {
#   description = "GitHub App private key PEM file contents"
#   type        = string
#   sensitive   = true
# }
