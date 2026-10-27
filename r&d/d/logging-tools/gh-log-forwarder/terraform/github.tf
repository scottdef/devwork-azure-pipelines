locals {
  # One knob, two consequences: the cron line and the lookback window.
  # Same formula as `ghlog2dt schedule` (cmd/ghlog2dt/schedule.go).
  offset   = var.minute_offset % var.interval_minutes
  cron     = "${join(",", [for m in range(local.offset, 60, var.interval_minutes) : tostring(m)])} * * * *"
  lookback = "${max(2 * var.interval_minutes, 60) + 30}m"

  # The workflow file in this repo is the source; only the line tagged
  # `# ghlog2dt:cron` is rewritten, so the committed file stays valid YAML
  # before and after.
  forward_src = file("${path.module}/../.github/workflows/forward.yml")
  forward_yml = replace(
    local.forward_src,
    "/(?m)^(\\s*-\\s*cron:\\s*)\"[^\"]*\"(\\s*#\\s*ghlog2dt:cron.*)$/",
    "$${1}\"${local.cron}\"$${2}",
  )

  notify_src = file("${path.module}/../templates/notify-forwarder.yml")

  forwarder_vars = {
    GHLOG_TARGET_REPOS = join(",", var.target_repos)
    GHLOG_LOOKBACK     = local.lookback
    GHLOG_SHARDS       = tostring(var.shards)
    GHLOG_VERSION      = var.ghlog_version
    GHLOG_APP_ID       = var.app_id
    DT_URL             = var.dt_env_url
  }

  dt_logs_token = var.manage_dt_token ? one(dynatrace_api_token.logs_ingest[*].token) : var.dt_logs_token
}

check "cron_line_rendered" {
  assert {
    condition     = strcontains(local.forward_yml, "\"${local.cron}\"")
    error_message = "forward.yml has no `- cron: \"...\" # ghlog2dt:cron` line to manage."
  }
}

check "push_repos_are_targets" {
  assert {
    condition = join(",", var.target_repos) == "*" || alltrue([
      for name in keys(var.push_mode_repos) : contains([for r in var.target_repos : lower(r)], lower("${var.github_owner}/${name}"))
    ])
    error_message = "Every push_mode_repos entry must also be in target_repos; forward-event.yml rejects other repositories."
  }
}

# ------------------------------------------------------------ forwarder

resource "github_repository_file" "forward_workflow" {
  count = var.manage_forward_workflow ? 1 : 0

  repository          = var.forwarder_repo
  file                = ".github/workflows/forward.yml"
  content             = local.forward_yml
  commit_message      = "ci(ghlog2dt): export every ${var.interval_minutes}m (cron ${local.cron})"
  overwrite_on_create = true
  # No commit_author/commit_email: the commit is attributed to, and
  # verified as, the GitHub App.
}

resource "github_actions_variable" "forwarder" {
  for_each = local.forwarder_vars

  repository    = var.forwarder_repo
  variable_name = each.key
  value         = each.value
}

resource "github_actions_secret" "dt_logs_token" {
  repository  = var.forwarder_repo
  secret_name = "DT_LOGS_TOKEN"
  value       = local.dt_logs_token

  lifecycle {
    precondition {
      condition     = var.manage_dt_token || var.dt_logs_token != ""
      error_message = "Set manage_dt_token = true, or pass dt_logs_token (a token with logs.ingest only)."
    }
  }
}

resource "github_actions_secret" "app_private_key" {
  repository  = var.forwarder_repo
  secret_name = "GHLOG_APP_PRIVATE_KEY"
  value       = var.app_private_key
}

# ------------------------------------------------------ push mode (opt-in)

resource "github_repository_file" "notify_forwarder" {
  for_each = var.push_mode_repos

  repository = each.key
  file       = ".github/workflows/notify-log-forwarder.yml"

  content = replace(
    local.notify_src,
    "/(?m)^(\\s*workflows:\\s*)\\[[^\\]]*\\](\\s*#\\s*ghlog2dt:workflows.*)$/",
    "$${1}${jsonencode(each.value)}$${2}",
  )

  commit_message      = "ci(ghlog2dt): notify the log forwarder when ${join(", ", each.value)} complete"
  overwrite_on_create = true

  lifecycle {
    precondition {
      condition     = var.dispatch_app_id != "" && var.dispatch_app_private_key != ""
      error_message = "Push mode needs dispatch_app_id and dispatch_app_private_key: a separate app with Contents: write on the forwarder repo only."
    }
  }
}

resource "github_actions_variable" "notify_app_id" {
  for_each = var.push_mode_repos

  repository    = each.key
  variable_name = "GHLOG_APP_ID"
  value         = var.dispatch_app_id
}

resource "github_actions_variable" "notify_forwarder_repo" {
  for_each = var.push_mode_repos

  repository    = each.key
  variable_name = "GHLOG_FORWARDER_REPO"
  value         = var.forwarder_repo
}

resource "github_actions_secret" "notify_app_private_key" {
  for_each = var.push_mode_repos

  repository  = each.key
  secret_name = "GHLOG_APP_PRIVATE_KEY"
  value       = var.dispatch_app_private_key
}
