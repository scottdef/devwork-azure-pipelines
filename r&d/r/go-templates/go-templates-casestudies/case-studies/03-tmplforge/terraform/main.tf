# tmplforge repository, governed as code: the repo itself, a ruleset on
# main, the GO_VERSION Actions variable the workflows read, and a "beta"
# deployment environment for the loadtest workflow.

resource "github_repository" "tmplforge" {
  name        = var.repository_name
  description = var.description
  visibility  = "internal"

  has_issues      = true
  has_discussions = false
  has_projects    = false
  has_wiki        = false

  allow_merge_commit     = false
  allow_rebase_merge     = false
  allow_squash_merge     = true
  allow_auto_merge       = true
  delete_branch_on_merge = true

  squash_merge_commit_title   = "PR_TITLE"
  squash_merge_commit_message = "PR_BODY"

  vulnerability_alerts = true
  archive_on_destroy   = true

  topics = ["go", "templates", "plan9", "htmx", "tui", "issueops"]
}

data "github_team" "beta_reviewers" {
  count = var.beta_reviewers_team == "" ? 0 : 1
  slug  = var.beta_reviewers_team
}

data "github_team" "admin_bypass" {
  count = var.admin_bypass_team == "" ? 0 : 1
  slug  = var.admin_bypass_team
}

resource "github_repository_ruleset" "main" {
  name        = "main"
  repository  = github_repository.tmplforge.name
  target      = "branch"
  enforcement = "active"

  conditions {
    ref_name {
      include = ["~DEFAULT_BRANCH"]
      exclude = []
    }
  }

  dynamic "bypass_actors" {
    for_each = data.github_team.admin_bypass
    content {
      actor_id    = bypass_actors.value.id
      actor_type  = "Team"
      bypass_mode = "pull_request"
    }
  }

  rules {
    creation                = false
    deletion                = true
    non_fast_forward        = true
    required_linear_history = true
    required_signatures     = false

    pull_request {
      required_approving_review_count   = var.required_approvals
      dismiss_stale_reviews_on_push     = true
      require_code_owner_review         = false
      require_last_push_approval        = true
      required_review_thread_resolution = true
    }

    required_status_checks {
      strict_required_status_checks_policy = true

      dynamic "required_check" {
        for_each = var.required_checks
        content {
          context = required_check.value
        }
      }
    }
  }
}

resource "github_actions_variable" "go_version" {
  repository    = github_repository.tmplforge.name
  variable_name = "GO_VERSION"
  value         = var.go_version
}

resource "github_repository_environment" "beta" {
  repository          = github_repository.tmplforge.name
  environment         = "beta"
  can_admins_bypass   = false
  prevent_self_review = true

  dynamic "reviewers" {
    for_each = data.github_team.beta_reviewers
    content {
      teams = [reviewers.value.id]
    }
  }

  deployment_branch_policy {
    protected_branches     = true
    custom_branch_policies = false
  }
}

resource "github_actions_environment_variable" "beta_loadtest_rate" {
  repository    = github_repository.tmplforge.name
  environment   = github_repository_environment.beta.environment
  variable_name = "LOADTEST_RATE"
  value         = "20"
}
