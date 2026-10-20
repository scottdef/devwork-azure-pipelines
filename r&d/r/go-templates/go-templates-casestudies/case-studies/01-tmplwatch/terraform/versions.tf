terraform {
  required_version = ">= 1.6"

  required_providers {
    github = {
      source  = "integrations/github"
      version = "~> 6.13"
    }
  }
}

# Authenticate as the issueops-autoadmin-app installation in CoolGitOrg:
#   export GITHUB_APP_ID=...            (numeric App ID)
#   export GITHUB_APP_INSTALLATION_ID=...
#   export GITHUB_APP_PEM_FILE="$(cat app.pem)"
# or set GITHUB_TOKEN for a quick local run.
provider "github" {
  owner = var.owner
}
