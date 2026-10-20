# Two repositories, their Pages sites, labels, Actions variables and a
# ruleset that makes main PR-only for people while the App can push the
# library's data commits. App installation and its private key are done
# by hand (see README.md): Terraform should not hold a private key.

locals {
  repos = {
    intake = {
      name        = var.intake_repo
      description = "IssueOps intake: issue forms, the static request page and the intake workflows."
      visibility  = var.intake_visibility
      homepage    = "https://${lower(var.owner)}.github.io/${var.intake_repo}/"
      topics      = ["issueops", "github-pages", "go"]
    }
    library = {
      name        = var.library_repo
      description = "IssueOps library: rendered items, the loan ledger and the public catalog site."
      visibility  = "public"
      homepage    = "https://${lower(var.owner)}.github.io/${var.library_repo}/"
      topics      = ["issueops", "github-pages", "go", "templates"]
    }
  }

  # repo key x label name, for one github_issue_label per pair.
  repo_labels = {
    for pair in setproduct(keys(local.repos), keys(var.labels)) :
    "${pair[0]}:${pair[1]}" => { repo = pair[0], label = pair[1] }
  }
}

resource "github_repository" "this" {
  for_each = local.repos

  name         = each.value.name
  description  = each.value.description
  homepage_url = each.value.homepage
  visibility   = each.value.visibility
  topics       = each.value.topics

  has_issues      = true
  has_projects    = false
  has_wiki        = false
  has_discussions = false

  allow_merge_commit     = false
  allow_rebase_merge     = false
  allow_squash_merge     = true
  delete_branch_on_merge = true
  auto_init              = true

  vulnerability_alerts = true

  # Pages built by Actions (configure-pages / upload-pages-artifact /
  # deploy-pages), not from a branch. Confirm the block against the
  # provider docs for your version: build_type = "workflow" needs no source.
  pages {
    build_type = "workflow"
  }

  lifecycle {
    prevent_destroy = true
  }
}

resource "github_issue_label" "this" {
  for_each = local.repo_labels

  repository  = github_repository.this[each.value.repo].name
  name        = each.value.label
  color       = var.labels[each.value.label].color
  description = var.labels[each.value.label].description
}

resource "github_actions_variable" "app_client_id" {
  for_each = local.repos

  repository    = github_repository.this[each.key].name
  variable_name = "APP_CLIENT_ID"
  value         = var.app_client_id
}

resource "github_actions_variable" "go_version" {
  for_each = local.repos

  repository    = github_repository.this[each.key].name
  variable_name = "GO_VERSION"
  value         = var.go_version
}

resource "github_actions_variable" "library_base_url" {
  repository    = github_repository.this["library"].name
  variable_name = "LIBRARY_BASE_URL"
  value         = local.repos.library.homepage
}

# The github-pages environment is created by GitHub on first deploy; we
# declare it so deployment branch policy is code, not clicks.
resource "github_repository_environment" "pages" {
  for_each = local.repos

  repository  = github_repository.this[each.key].name
  environment = "github-pages"

  deployment_branch_policy {
    protected_branches     = false
    custom_branch_policies = true
  }
}

resource "github_repository_environment_deployment_policy" "pages_main" {
  for_each = local.repos

  repository     = github_repository.this[each.key].name
  environment    = github_repository_environment.pages[each.key].environment
  branch_pattern = "main"
}

# main: pull requests for people, no force pushes or deletion. The App is
# the bypass actor so publish/overdue/pages workflows can push data
# commits (items/, ledger/, site/) straight to main in the library repo.
# The intake repo gets the same rule; its workflows never push.
resource "github_repository_ruleset" "main" {
  for_each = local.repos

  name        = "main-pr-only"
  repository  = github_repository.this[each.key].name
  target      = "branch"
  enforcement = "active"

  conditions {
    ref_name {
      include = ["~DEFAULT_BRANCH"]
      exclude = []
    }
  }

  # actor_type "Integration" takes the App's numeric App ID (not the
  # installation ID, not the client ID). See README.md for the lookup.
  bypass_actors {
    actor_id    = var.app_id
    actor_type  = "Integration"
    bypass_mode = "always"
  }

  rules {
    deletion         = true
    non_fast_forward = true

    pull_request {
      required_approving_review_count   = 1
      dismiss_stale_reviews_on_push     = true
      require_last_push_approval        = true
      required_review_thread_resolution = true
    }
  }
}
