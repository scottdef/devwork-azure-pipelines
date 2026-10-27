# ---------------------------------------------------------------- GitHub

variable "github_owner" {
  description = "Organization that owns the forwarder and the target repositories."
  type        = string
  default     = "CoolGitOrg"
}

variable "forwarder_repo" {
  description = "Repository (name only) that runs ghlog2dt."
  type        = string
  default     = "gh-log-forwarder"
}

variable "target_repos" {
  description = "Repositories (owner/name) whose Actions logs are exported. [\"*\"] means every repository the app installation can see, filtered by ghlog2dt.json include/exclude."
  type        = list(string)
  default     = ["CoolGitOrg/app-a", "CoolGitOrg/app-b"]

  validation {
    condition = (
      join(",", var.target_repos) == "*" ||
      (length(var.target_repos) > 0 && alltrue([for r in var.target_repos : can(regex("^[A-Za-z0-9][A-Za-z0-9-]{0,38}/[A-Za-z0-9._-]{1,100}$", r))]))
    )
    error_message = "target_repos must be [\"*\"] or a non-empty list of owner/name."
  }
}

variable "interval_minutes" {
  description = "Export frequency. Must divide 60; GitHub's floor is 5. Keep it short: Dynatrace refuses logs older than 24h."
  type        = number
  default     = 15

  validation {
    condition     = contains([5, 6, 10, 12, 15, 20, 30, 60], var.interval_minutes)
    error_message = "interval_minutes must be one of 5, 6, 10, 12, 15, 20, 30, 60."
  }
}

variable "minute_offset" {
  description = "Minute offset within each interval; keeps ticks off :00, when GitHub's scheduler is busiest and may delay or drop runs."
  type        = number
  default     = 7

  validation {
    condition     = var.minute_offset >= 0 && var.minute_offset < 60
    error_message = "minute_offset must be in [0, 60)."
  }
}

variable "shards" {
  description = "Parallel matrix jobs per sweep (repositories are spread round-robin)."
  type        = number
  default     = 4

  validation {
    condition     = var.shards >= 1 && var.shards <= 32
    error_message = "shards must be in [1, 32]."
  }
}

variable "ghlog_version" {
  description = "ghlog2dt release the workflows install (vX.Y.Z, SHA256-verified), or \"source\" to build from the checked-out code."
  type        = string
  default     = "source"

  validation {
    condition     = var.ghlog_version == "source" || can(regex("^v[0-9]+\\.[0-9]+\\.[0-9]+(-[0-9A-Za-z.]+)?$", var.ghlog_version))
    error_message = "ghlog_version must be \"source\" or vX.Y.Z."
  }
}

variable "manage_forward_workflow" {
  description = "Commit .github/workflows/forward.yml with the cron line rendered from interval_minutes. Apply from the main branch only."
  type        = bool
  default     = true
}

variable "app_id" {
  description = "Client ID (or numeric app ID) of the GitHub App the forwarder uses to read Actions logs (issueops-autoadmin-app). Tokens are narrowed to actions:read at mint time."
  type        = string
}

variable "app_private_key" {
  description = "PEM private key of that app, stored as the forwarder repo secret GHLOG_APP_PRIVATE_KEY. Supply via TF_VAR_app_private_key."
  type        = string
  sensitive   = true
}

# ---------------------------------------------------------- push mode

variable "push_mode_repos" {
  description = "Optional low-latency mode: repo name (no owner) => workflow names whose completion triggers an immediate export. Installs notify-log-forwarder.yml in each."
  type        = map(list(string))
  default     = {}

  validation {
    condition = alltrue([
      for name, wfs in var.push_mode_repos :
      can(regex("^[A-Za-z0-9._-]{1,100}$", name)) && length(wfs) > 0 && !contains(wfs, "notify-log-forwarder")
    ])
    error_message = "Keys are repository names without owner; each needs at least one workflow name, and never notify-log-forwarder itself."
  }
}

variable "dispatch_app_id" {
  description = "Push mode only: a SEPARATE minimal GitHub App (Contents: write) installed on the forwarder repo alone. Its key is copied into every push-mode repo, so it must not be issueops-autoadmin-app."
  type        = string
  default     = ""
}

variable "dispatch_app_private_key" {
  description = "Push mode only: PEM key of the dispatch app (TF_VAR_dispatch_app_private_key)."
  type        = string
  default     = ""
  sensitive   = true
}

# ------------------------------------------------------------ Dynatrace

variable "dt_env_url" {
  description = "Dynatrace environment URL for ingest, e.g. https://abc12345.live.dynatrace.com (or an Environment ActiveGate .../e/<env-id>)."
  type        = string
  default     = "https://abc12345.live.dynatrace.com"

  validation {
    condition     = can(regex("^https://", var.dt_env_url))
    error_message = "dt_env_url must be https."
  }
}

variable "manage_dt_token" {
  description = "Create the logs.ingest API token with Terraform (needs apiTokens.read/write). If false, pass dt_logs_token."
  type        = bool
  default     = true
}

variable "dt_logs_token" {
  description = "Existing token with only logs.ingest, used when manage_dt_token = false (TF_VAR_dt_logs_token)."
  type        = string
  default     = ""
  sensitive   = true
}

variable "dt_token_expiration" {
  description = "Optional expiry for the managed ingest token (ISO 8601, e.g. 2027-01-31T00:00:00.000Z). Rotate with terraform apply -replace."
  type        = string
  default     = null
}

variable "manage_dt_bucket" {
  description = "Create a dedicated Grail bucket for build logs (needs the OAuth client env vars)."
  type        = bool
  default     = true
}

variable "dt_bucket_name" {
  description = "Grail bucket for GitHub Actions logs."
  type        = string
  default     = "ci_github_actions_logs"
}

variable "dt_bucket_retention_days" {
  description = "Retention of the build-log bucket, in days."
  type        = number
  default     = 35
}

variable "manage_openpipeline_pipeline" {
  description = "Create an OpenPipeline logs pipeline that stores these logs in the bucket and extracts the exit code. Review the processors in the pipeline preview before enabling in production."
  type        = bool
  default     = false
}

variable "manage_openpipeline_routing" {
  description = "DANGER: the logs routing resource REPLACES the environment's whole logs routing table. Enable only if this module is the single owner of logs routing; otherwise add the route from the README to your central routing config."
  type        = bool
  default     = false
}

variable "dt_security_context" {
  description = "dt.security_context value stamped on every record, for IAM policies on who may read build logs."
  type        = string
  default     = "ci"
}
