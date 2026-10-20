variable "owner" {
  description = "GitHub organization that owns both repositories."
  type        = string
  default     = "CoolGitOrg"
}

variable "intake_repo" {
  description = "Name of the intake repository (input Pages site, issue forms, intake workflows)."
  type        = string
  default     = "issueops-intake"
}

variable "library_repo" {
  description = "Name of the library repository (output Pages site, items, ledger)."
  type        = string
  default     = "issueops-library"
}

variable "intake_visibility" {
  description = "Visibility of the intake repo: public, or internal on GitHub Enterprise Cloud. Issue forms work in both; with internal, only enterprise members can file requests."
  type        = string
  default     = "public"

  validation {
    condition     = contains(["public", "internal"], var.intake_visibility)
    error_message = "intake_visibility must be public or internal."
  }
}

variable "app_client_id" {
  description = "Client ID of the issueops-autoadmin-app GitHub App (not secret). Stored as the APP_CLIENT_ID Actions variable."
  type        = string
}

variable "app_id" {
  description = "Numeric App ID of issueops-autoadmin-app, used as the ruleset bypass actor_id. Find it on the App's settings page or with: gh api /orgs/CoolGitOrg/installations --jq '.installations[] | select(.app_slug==\"issueops-autoadmin-app\") | .app_id'"
  type        = number
}

variable "go_version" {
  description = "Go toolchain the workflows install (GO_VERSION variable). Language level stays go 1.24 in go.mod."
  type        = string
  default     = "1.24.x"
}

variable "labels" {
  description = "Issue labels created on both repositories (name => color, hex without #)."
  type        = map(object({ color = string, description = string }))
  default = {
    issueops  = { color = "5319e7", description = "Handled by IssueOps automation" }
    request   = { color = "0e8a16", description = "Library publish request" }
    borrow    = { color = "fbca04", description = "Borrow an item's next phase" }
    return    = { color = "c2e0c6", description = "Return a borrowed item" }
    accepted  = { color = "1d76db", description = "Validated and dispatched to the library" }
    invalid   = { color = "d93f0b", description = "Failed validation; edit the issue to retry" }
    published = { color = "0052cc", description = "Live on the library Pages site" }
    overdue   = { color = "b60205", description = "Loan is past its due date" }
  }
}
