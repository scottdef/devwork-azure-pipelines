variable "owner" {
  description = "Organization that owns both repositories."
  type        = string
  default     = "CoolGitOrg"
}

variable "app_client_id" {
  description = "Client ID of the issueops-autoadmin-app GitHub App (e.g. Iv23li...). Not a secret."
  type        = string

  validation {
    condition     = length(var.app_client_id) > 0
    error_message = "app_client_id must not be empty."
  }
}

variable "visibility" {
  description = "Visibility of both repositories: internal (enterprise-wide) or private."
  type        = string
  default     = "internal"

  validation {
    condition     = contains(["internal", "private"], var.visibility)
    error_message = "visibility must be internal or private."
  }
}

variable "go_version" {
  description = "Go toolchain CI uses (repository variable GO_VERSION)."
  type        = string
  default     = "1.24.x"
}

variable "required_checks" {
  description = "Status checks required on main in the tmplwatch repository."
  type        = list(string)
  default     = ["test", "render-check"]
}
