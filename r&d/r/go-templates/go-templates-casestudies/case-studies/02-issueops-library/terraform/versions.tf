terraform {
  required_version = ">= 1.6"

  required_providers {
    github = {
      source  = "integrations/github"
      version = "~> 6.13"
    }
  }
}

# Authenticate with GITHUB_TOKEN (an org owner's token) or GITHUB_APP_*
# variables; nothing secret lives in this directory.
provider "github" {
  owner = var.owner
}
