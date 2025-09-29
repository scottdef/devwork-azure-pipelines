# Variables for GitHub Governance Module

variable "github_org" {
  description = "GitHub organization name"
  type        = string
}

variable "github_token" {
  description = "GitHub personal access token with admin:org and repo scopes"
  type        = string
  sensitive   = true
}

variable "commit_email" {
  description = "Email address for automated commits (CODEOWNERS, etc)"
  type        = string
  default     = "terraform@github-actions.local"
}

variable "default_branch" {
  description = "Default branch name for new repositories"
  type        = string
  default     = "main"
}

variable "enable_vulnerability_alerts" {
  description = "Enable vulnerability alerts for all repositories"
  type        = bool
  default     = true
}
