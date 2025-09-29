provider "github" {
  owner = var.github_org
  token = var.github_token
}

# Variables
variable "github_org" {
  description = "GitHub organization name"
  type        = string
}

variable "github_token" {
  description = "GitHub token with admin:org and repo permissions"
  type        = string
  sensitive   = true
}

variable "commit_email" {
  description = "Email for automated commits"
  type        = string
  default     = "terraform@github-actions.local"
}
