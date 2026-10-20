locals {
  kv_readme = <<-EOT
    # tmplwatch-kv

    Key/value data for [tmplwatch](https://github.com/${var.owner}/tmplwatch).

    Every file under `kv/` is one key: `kv/motd/text` is the key `motd/text`,
    and its contents (minus one trailing newline) are the value. Change values
    with pull requests; tmplwatch polls `main` and re-renders only the
    templates that read the keys you changed.

    A push to `main` runs `.github/workflows/notify.yml`, which sends a
    `kv-changed` repository_dispatch to `${var.owner}/tmplwatch` so its
    render check runs against the new data.
  EOT

  # Files seeded once into the kv repository. Later edits belong to the
  # repository's own pull requests, so Terraform ignores content drift.
  kv_seed = {
    "README.md"                    = local.kv_readme
    "kv/motd/text"                 = "Welcome to the beta fleet. Changes go through ${var.owner}/tmplwatch-kv pull requests.\n"
    "kv/site/env"                  = "beta\n"
    ".github/workflows/notify.yml" = file("${path.module}/kv-repo-seed/.github/workflows/notify.yml")
  }
}

resource "github_repository" "tmplwatch" {
  name        = "tmplwatch"
  description = "Watch a key/value source, render Go templates, write files atomically."
  visibility  = var.visibility

  auto_init              = true
  has_issues             = true
  has_projects           = false
  has_wiki               = false
  allow_merge_commit     = false
  allow_squash_merge     = true
  allow_rebase_merge     = false
  delete_branch_on_merge = true
  vulnerability_alerts   = true

  topics = ["go", "templates", "configuration-management", "gitops"]
}

resource "github_repository" "tmplwatch_kv" {
  name        = "tmplwatch-kv"
  description = "Key/value data rendered by tmplwatch. One file per key under kv/."
  visibility  = var.visibility

  auto_init              = true
  has_issues             = false
  has_projects           = false
  has_wiki               = false
  allow_merge_commit     = false
  allow_squash_merge     = true
  allow_rebase_merge     = false
  delete_branch_on_merge = true
  vulnerability_alerts   = true

  topics = ["tmplwatch", "gitops"]
}

# One ruleset shape for both repositories: no deletion or force-push of
# main, pull requests with one approval, linear history. It depends on the
# seed files so that the first apply can still commit them to main.
resource "github_repository_ruleset" "main" {
  for_each = {
    tmplwatch    = github_repository.tmplwatch.name
    tmplwatch_kv = github_repository.tmplwatch_kv.name
  }

  name        = "protect-main"
  repository  = each.value
  target      = "branch"
  enforcement = "active"

  conditions {
    ref_name {
      include = ["~DEFAULT_BRANCH"]
      exclude = []
    }
  }

  # Repository admins may bypass through a pull request only. To let the
  # issueops-autoadmin-app bypass as well, add a second block with
  # actor_type = "Integration" and its numeric App ID.
  bypass_actors {
    actor_id    = 5 # repository admin role
    actor_type  = "RepositoryRole"
    bypass_mode = "pull_request"
  }

  rules {
    deletion                = true
    non_fast_forward        = true
    required_linear_history = true

    pull_request {
      required_approving_review_count   = 1
      dismiss_stale_reviews_on_push     = true
      require_last_push_approval        = true
      required_review_thread_resolution = true
    }
  }

  depends_on = [github_repository_file.kv_seed]
}

# Required status checks only make sense where CI runs: the code repo.
resource "github_repository_ruleset" "checks" {
  name        = "required-checks"
  repository  = github_repository.tmplwatch.name
  target      = "branch"
  enforcement = "active"

  conditions {
    ref_name {
      include = ["~DEFAULT_BRANCH"]
      exclude = []
    }
  }

  rules {
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

resource "github_repository_file" "kv_seed" {
  for_each = local.kv_seed

  repository          = github_repository.tmplwatch_kv.name
  branch              = "main"
  file                = each.key
  content             = each.value
  commit_message      = "seed ${each.key} (terraform)"
  commit_author       = "github-actions[bot]"
  commit_email        = "41898282+github-actions[bot]@users.noreply.github.com"
  overwrite_on_create = true

  lifecycle {
    ignore_changes = [content, commit_message]
  }

}

resource "github_actions_variable" "app_client_id" {
  for_each = {
    tmplwatch    = github_repository.tmplwatch.name
    tmplwatch_kv = github_repository.tmplwatch_kv.name
  }

  repository    = each.value
  variable_name = "APP_CLIENT_ID"
  value         = var.app_client_id
}

resource "github_actions_variable" "go_version" {
  repository    = github_repository.tmplwatch.name
  variable_name = "GO_VERSION"
  value         = var.go_version
}
