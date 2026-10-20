output "repository_full_name" {
  description = "owner/name of the repository."
  value       = github_repository.tmplforge.full_name
}

output "repository_html_url" {
  description = "Web URL of the repository."
  value       = github_repository.tmplforge.html_url
}

output "ruleset_id" {
  description = "ID of the main-branch ruleset."
  value       = github_repository_ruleset.main.ruleset_id
}

output "beta_environment" {
  description = "Name of the deployment environment used by loadtest.yml."
  value       = github_repository_environment.beta.environment
}

output "go_version" {
  description = "Value of the GO_VERSION Actions variable."
  value       = github_actions_variable.go_version.value
}
