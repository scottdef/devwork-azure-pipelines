# Repository creation
resource "github_repository" "repos" {
  for_each = local.repos

  name        = each.value.name
  description = try(each.value.description, "Managed by Terraform")
  visibility  = try(each.value.visibility, "private")

  # Default settings
  has_issues      = try(each.value.has_issues, true)
  has_projects    = try(each.value.has_projects, false)
  has_wiki        = try(each.value.has_wiki, false)
  has_downloads   = try(each.value.has_downloads, true)
  has_discussions = try(each.value.has_discussions, false)

  # Auto init with README
  auto_init = try(each.value.auto_init, true)

  # Merge settings - enforce squash
  allow_merge_commit     = false
  allow_squash_merge     = true
  allow_rebase_merge     = false
  allow_auto_merge       = try(each.value.allow_auto_merge, false)
  delete_branch_on_merge = true
  squash_merge_commit_title   = "PR_TITLE"
  squash_merge_commit_message = "PR_BODY"

  # Template if specified
  template {
    owner      = try(each.value.template_owner, var.github_org)
    repository = try(each.value.template_repo, null)
  }

  # Archive settings
  archived           = try(each.value.archived, false)
  archive_on_destroy = false

  vulnerability_alerts = true

  lifecycle {
    prevent_destroy = false
  }
}

# Repository team access - admin team
resource "github_team_repository" "repo_admin_access" {
  for_each = {
    for name, repo in local.repos : name => repo
    if try(repo.team, null) != null
  }

  team_id    = github_team.teams["${each.value.team}-admins"].id
  repository = github_repository.repos[each.key].name
  permission = "admin"

  depends_on = [github_team.teams]
}

# Repository team access - main team
resource "github_team_repository" "repo_team_access" {
  for_each = {
    for name, repo in local.repos : name => repo
    if try(repo.team, null) != null
  }

  team_id    = github_team.teams[each.value.team].id
  repository = github_repository.repos[each.key].name
  permission = try(each.value.permission, "push")

  depends_on = [github_team.teams]
}

# CODEOWNERS file creation
resource "github_repository_file" "codeowners" {
  for_each = {
    for name, repo in local.repos : name => repo
    if try(repo.team, null) != null
  }

  repository          = github_repository.repos[each.key].name
  branch              = try(each.value.default_branch, "main")
  file                = ".github/CODEOWNERS"
  content             = templatefile("${path.module}/modules/codeowners/templates/CODEOWNERS.tpl", {
    parent_team      = "@${var.github_org}/${each.value.team}"
    pr_approvers_team = "@${var.github_org}/${each.value.team}-pr-approvers"
  })
  commit_message      = "Add CODEOWNERS file [terraform-managed]"
  commit_author       = "Terraform"
  commit_email        = var.commit_email
  overwrite_on_create = true

  depends_on = [
    github_repository.repos,
    github_team.teams
  ]
}

# Branch protection ruleset
resource "github_repository_ruleset" "main_protection" {
  for_each = local.repos

  name        = "main-branch-protection"
  repository  = github_repository.repos[each.key].name
  target      = "branch"
  enforcement = "active"

  conditions {
    ref_name {
      include = ["~DEFAULT_BRANCH"]
      exclude = []
    }
  }

  bypass_actors {
    actor_id    = try(data.github_team.bypass_actors[each.value.team].id, 0)
    actor_type  = "Team"
    bypass_mode = "pull_request"
  }

  rules {
    # Require pull request
    pull_request {
      required_approving_review_count   = 2
      dismiss_stale_reviews_on_push     = true
      require_code_owner_review         = true
      require_last_push_approval        = false
      required_review_thread_resolution = false
    }

    # No force pushes
    non_fast_forward = true

    # Require linear history (squash enforced)
    required_linear_history = true

    # Require signed commits (optional)
    required_signatures = try(each.value.require_signed_commits, false)
  }

  depends_on = [
    github_repository.repos,
    github_repository_file.codeowners
  ]
}

# Data source for bypass actors
data "github_team" "bypass_actors" {
  for_each = {
    for name, repo in local.repos : repo.team => repo.team
    if try(repo.team, null) != null
  }
  
  slug = "${each.key}-admins"

  depends_on = [github_team.teams]
}
