terraform {
  required_version = ">= 1.6"

  required_providers {
    github = {
      source  = "integrations/github"
      version = "~> 6.13"
    }
  }
}

# Authenticate as the issueops-autoadmin-app GitHub App in CI, or with
# GITHUB_TOKEN locally. The owner is the organization, not a user.
provider "github" {
  owner = var.owner
}
