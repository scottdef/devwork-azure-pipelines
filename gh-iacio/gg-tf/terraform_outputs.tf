# Outputs for GitHub Governance Module

output "repositories" {
  description = "Map of all managed repositories"
  value = {
    for name, repo in github_repository.repos : name => {
      id          = repo.id
      full_name   = repo.full_name
      html_url    = repo.html_url
      clone_url   = repo.http_clone_url
      visibility  = repo.visibility
      team        = try(local.repos[name].team, null)
    }
  }
}

output "teams" {
  description = "Map of all managed teams"
  value = {
    for name, team in github_team.teams : name => {
      id          = team.id
      slug        = team.slug
      name        = team.name
      description = team.description
      members_count = team.members_count
    }
  }
}

output "repository_count" {
  description = "Total number of managed repositories"
  value       = length(github_repository.repos)
}

output "team_count" {
  description = "Total number of managed teams"
  value       = length(github_team.teams)
}

output "subteams" {
  description = "List of automatically created subteams"
  value       = [for st in local.subteams : st.name]
}

output "summary" {
  description = "Summary of managed resources"
  value = {
    repositories = length(github_repository.repos)
    teams        = length(local.teams)
    subteams     = length(local.subteams)
    total_teams  = length(github_team.teams)
  }
}
