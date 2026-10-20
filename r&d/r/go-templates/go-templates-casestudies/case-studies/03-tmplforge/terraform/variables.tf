variable "owner" {
  description = "GitHub organization that owns the repository."
  type        = string
  default     = "CoolGitOrg"
}

variable "repository_name" {
  description = "Repository name."
  type        = string
  default     = "tmplforge"
}

variable "description" {
  description = "Repository description."
  type        = string
  default     = "Go template forge: ed-style TUI and htmx GUI in one Plan 9-portable binary."
}

variable "go_version" {
  description = "Go toolchain for Actions (repository variable GO_VERSION). Go 1.24 is past upstream support; raise this to a supported release when the module allows it."
  type        = string
  default     = "1.24.x"
}

variable "required_checks" {
  description = "Status check contexts required on main (job names from .github/workflows/ci.yml)."
  type        = list(string)
  default     = ["test", "preflight", "cross (linux/amd64)", "cross (plan9/amd64)"]
}

variable "required_approvals" {
  description = "Approving reviews required to merge into main."
  type        = number
  default     = 1

  validation {
    condition     = var.required_approvals >= 1 && var.required_approvals <= 6
    error_message = "required_approvals must be between 1 and 6."
  }
}

variable "beta_reviewers_team" {
  description = "Slug of the team that must approve deployments to the beta environment; empty for none."
  type        = string
  default     = "platform"
}

variable "admin_bypass_team" {
  description = "Slug of the team allowed to bypass the main ruleset in an emergency; empty for none."
  type        = string
  default     = ""
}
