terraform {
  required_version = ">= 1.5.0"
  
  required_providers {
    github = {
      source  = "integrations/github"
      version = "~> 6.0"
    }
  }

  backend "s3" {
    # Configure your backend - S3, GCS, or Terraform Cloud
    # bucket = "your-terraform-state"
    # key    = "github-governance/terraform.tfstate"
    # region = "us-east-1"
  }
}

# Load all YAML files
locals {
  # Read all repo YAML files
  repo_yaml_files = fileset(path.module, "../repo-yamls/*.yml")
  repo_configs = flatten([
    for f in local.repo_yaml_files : [
      for repo in try(yamldecode(file("${path.module}/${f}")).repos, []) : merge(repo, {
        source_file = f
      })
    ]
  ])

  # Read all team YAML files
  team_yaml_files = fileset(path.module, "../team-yamls/*.yml")
  team_configs = flatten([
    for f in local.team_yaml_files : try(yamldecode(file("${path.module}/${f}")).teams, [])
  ])

  # Create map of repos by name
  repos = {
    for repo in local.repo_configs : repo.name => repo
  }

  # Create map of teams by name
  teams = {
    for team in local.team_configs : team.name => team
  }

  # Generate subteams for teams with AD groups
  subteams = flatten([
    for team_name, team in local.teams : [
      for subteam_suffix in ["admins", "workflow-admins", "pr-approvers"] : {
        name        = "${team_name}-${subteam_suffix}"
        parent_name = team_name
        description = "${subteam_suffix} for ${team_name}"
        privacy     = "closed"
      }
    ] if try(team.ad_group, null) != null
  ])

  subteams_map = {
    for st in local.subteams : st.name => st
  }

  # Merge teams and subteams
  all_teams = merge(local.teams, local.subteams_map)
}
