# Scaffolding around the workflow: repo, labels, approver team, environment, variable.
# The app's private key is NOT here; it would land in state. Set it once:
#   gh secret set ISSUEOPS_APP_PRIVATE_KEY -R CoolGitOrg/issueops-copilot-budget < key.pem
terraform {
  required_version = ">= 1.6"
  required_providers {
    github = { source = "integrations/github", version = "~> 6.6" }
  }
}

provider "github" {
  owner = "CoolGitOrg"
}

variable "app_client_id" {
  type        = string
  description = "Client ID of issueops-autoadmin-app"
}

variable "approvers" {
  type        = set(string)
  description = "Logins in issueops-bureaucraticops"
}

locals {
  labels = {
    "copilot-budget-request" = "1d76db"
    "budget:pending"         = "fbca04"
    "budget:approved"        = "0e8a16"
    "budget:denied"          = "b60205"
    "budget:rejected"        = "6a737d"
    "budget:failed"          = "d93f0b"
  }
}

resource "github_repository" "ops" {
  name                 = "issueops-copilot-budget"
  description          = "IssueOps: approval-gated Copilot AI-credit budget increases"
  visibility           = "internal" # every org member can open an issue; only teams pass validation
  has_issues           = true
  vulnerability_alerts = true
}

resource "github_issue_label" "l" {
  for_each   = local.labels
  repository = github_repository.ops.name
  name       = each.key
  color      = each.value
}

resource "github_team" "approvers" {
  name    = "issueops-bureaucraticops"
  privacy = "closed" # closed, not secret: a secret team cannot be @mentioned
}

resource "github_team_membership" "approvers" {
  for_each = var.approvers
  team_id  = github_team.approvers.id
  username = each.value
}

resource "github_repository_environment" "budget" {
  repository  = github_repository.ops.name
  environment = "copilot-budget"
  deployment_branch_policy {
    protected_branches     = true
    custom_branch_policies = false
  }
}

resource "github_actions_variable" "client_id" {
  repository    = github_repository.ops.name
  variable_name = "ISSUEOPS_APP_CLIENT_ID"
  value         = var.app_client_id
}

# The workflows and scripts are the control plane. Changing them changes who gets money.
resource "github_branch_protection" "main" {
  repository_id  = github_repository.ops.node_id
  pattern        = "main"
  enforce_admins = true
  required_pull_request_reviews {
    required_approving_review_count = 1
    require_code_owner_reviews      = true
  }
}
