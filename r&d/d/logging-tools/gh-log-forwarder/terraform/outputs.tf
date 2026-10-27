output "schedule" {
  description = "Rendered export cadence."
  value = {
    interval_minutes = var.interval_minutes
    cron             = local.cron
    lookback         = local.lookback
  }
}

output "forwarder_repository" {
  description = "Repository running the forwarder."
  value       = "${var.github_owner}/${var.forwarder_repo}"
}

output "target_repositories" {
  description = "Repositories exported (\"*\" = every repo the app can see)."
  value       = var.target_repos
}

output "push_mode_repositories" {
  description = "Repositories with the notify-log-forwarder workflow installed."
  value       = keys(var.push_mode_repos)
}

output "dynatrace_ingest_token_id" {
  description = "ID (not secret) of the managed logs.ingest token."
  value       = one(dynatrace_api_token.logs_ingest[*].id)
}

output "dynatrace_bucket" {
  description = "Grail bucket for build logs."
  value       = var.manage_dt_bucket ? var.dt_bucket_name : null
}
