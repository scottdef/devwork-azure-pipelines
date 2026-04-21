###############################################################################
# Azure Synapse CI/CD — GitHub Repository Infrastructure
# Creates: 3 repos, branches, rulesets, and team references
###############################################################################

terraform {
  required_version = ">= 1.6.0"

  required_providers {
    github = {
      source  = "integrations/github"
      version = "~> 6.0"
    }
  }

  backend "azurerm" {
    resource_group_name  = "rg-synapse-cicd-tfstate"
    storage_account_name = "synapsecicdtfstate"
    container_name       = "tfstate"
    key                  = "github-repos.tfstate"
  }
}

provider "github" {
  owner = var.github_org
  # Token sourced from GITHUB_TOKEN env var (use GitHub App installation token)
}

###############################################################################
# DATA — existing teams
###############################################################################

data "github_team" "db_ops" {
  slug = "db-ops"
}

data "github_team" "devops" {
  slug = "devops"
}

###############################################################################
# REPOSITORIES
###############################################################################

resource "github_repository" "dev" {
  name                   = "synapse-repo-dev"
  description            = "Azure Synapse workspace — development artifacts (Git-integrated)"
  visibility             = "private"
  auto_init              = true
  delete_branch_on_merge = false
  has_issues             = true
  has_wiki               = false

  lifecycle {
    prevent_destroy = true
  }
}

resource "github_repository" "test" {
  name                   = "synapse-repo-test"
  description            = "Azure Synapse workspace — test artifacts and CI/CD"
  visibility             = "private"
  auto_init              = true
  delete_branch_on_merge = false
  has_issues             = true
  has_wiki               = false

  lifecycle {
    prevent_destroy = true
  }
}

resource "github_repository" "prod" {
  name                   = "synapse-repo-prod"
  description            = "Azure Synapse workspace — production artifacts and CI/CD"
  visibility             = "private"
  auto_init              = true
  delete_branch_on_merge = false
  has_issues             = true
  has_wiki               = false

  lifecycle {
    prevent_destroy = true
  }
}

###############################################################################
# BRANCHES — synapse-repo-dev  (dev, test, prod, publish_branch, main)
###############################################################################

resource "github_branch" "dev_branch_dev" {
  repository = github_repository.dev.name
  branch     = "dev"
  source_branch = "main"
}

resource "github_branch" "dev_branch_test" {
  repository    = github_repository.dev.name
  branch        = "test"
  source_branch = "dev"
  depends_on    = [github_branch.dev_branch_dev]
}

resource "github_branch" "dev_branch_prod" {
  repository    = github_repository.dev.name
  branch        = "prod"
  source_branch = "main"
}

resource "github_branch" "dev_branch_publish" {
  repository    = github_repository.dev.name
  branch        = "publish_branch"
  source_branch = "main"
}

# Set default branch to dev for Synapse integration
resource "github_branch_default" "dev_default" {
  repository = github_repository.dev.name
  branch     = "dev"
  depends_on = [github_branch.dev_branch_dev]
}

###############################################################################
# BRANCHES — synapse-repo-test  (test, publish_branch, main)
###############################################################################

resource "github_branch" "test_branch_test" {
  repository    = github_repository.test.name
  branch        = "test"
  source_branch = "main"
}

resource "github_branch" "test_branch_publish" {
  repository    = github_repository.test.name
  branch        = "publish_branch"
  source_branch = "main"
}

resource "github_branch_default" "test_default" {
  repository = github_repository.test.name
  branch     = "test"
  depends_on = [github_branch.test_branch_test]
}

###############################################################################
# BRANCHES — synapse-repo-prod  (prod, publish_branch, main)
###############################################################################

resource "github_branch" "prod_branch_prod" {
  repository    = github_repository.prod.name
  branch        = "prod"
  source_branch = "main"
}

resource "github_branch" "prod_branch_publish" {
  repository    = github_repository.prod.name
  branch        = "publish_branch"
  source_branch = "main"
}

resource "github_branch_default" "prod_default" {
  repository = github_repository.prod.name
  branch     = "prod"
  depends_on = [github_branch.prod_branch_prod]
}

###############################################################################
# RULESETS — synapse-repo-dev
###############################################################################

# ── dev/test branch: protect test branch ───────────────────────────────────
resource "github_repository_ruleset" "dev_test_branch" {
  repository  = github_repository.dev.name
  name        = "test-branch-protection"
  target      = "branch"
  enforcement = "active"

  conditions {
    ref_name {
      include = ["refs/heads/test"]
      exclude = []
    }
  }

  bypass_actors {
    actor_id    = data.github_team.db_ops.id
    actor_type  = "Team"
    bypass_mode = "always"
  }

  rules {
    pull_request {
      required_approving_review_count   = 2
      require_code_owner_review         = false
      dismiss_stale_reviews_on_push     = true
      require_last_push_approval        = true
      required_review_thread_resolution = true
    }

    required_status_checks {
      strict_required_status_checks_policy = true
      required_check {
        # Must match: "<workflow-name> / <job-name>"
        context    = "synapse-dev-cicd-workflow / validate"
        integration_id = 15368  # GitHub Actions app ID
      }
    }

    # Restrict who can push directly to test
    restrict_pushes {
      blocks_creations = false
      push_allowances  = []
    }

    non_fast_forward = true
    deletion         = true
  }
}

# ── dev/prod branch: protect prod branch ───────────────────────────────────
resource "github_repository_ruleset" "dev_prod_branch" {
  repository  = github_repository.dev.name
  name        = "prod-branch-protection"
  target      = "branch"
  enforcement = "active"

  conditions {
    ref_name {
      include = ["refs/heads/prod"]
      exclude = []
    }
  }

  bypass_actors {
    actor_id    = data.github_team.db_ops.id
    actor_type  = "Team"
    bypass_mode = "always"
  }

  rules {
    pull_request {
      required_approving_review_count   = 2
      require_code_owner_review         = false
      dismiss_stale_reviews_on_push     = true
      require_last_push_approval        = true
      required_review_thread_resolution = true
    }

    restrict_pushes {
      blocks_creations = false
      push_allowances  = []
    }

    non_fast_forward = true
    deletion         = true
  }
}

# ── dev/main branch: protect main branch ───────────────────────────────────
resource "github_repository_ruleset" "dev_main_branch" {
  repository  = github_repository.dev.name
  name        = "main-branch-protection"
  target      = "branch"
  enforcement = "active"

  conditions {
    ref_name {
      include = ["refs/heads/main"]
      exclude = []
    }
  }

  bypass_actors {
    actor_id    = data.github_team.devops.id
    actor_type  = "Team"
    bypass_mode = "always"
  }

  rules {
    pull_request {
      required_approving_review_count   = 1
      require_code_owner_review         = false
      dismiss_stale_reviews_on_push     = true
      require_last_push_approval        = false
      required_review_thread_resolution = true
    }

    non_fast_forward = true
    deletion         = true
  }
}

# ── dev/publish_branch: no direct pushes except CI bot ─────────────────────
resource "github_repository_ruleset" "dev_publish_branch" {
  repository  = github_repository.dev.name
  name        = "publish-branch-protection"
  target      = "branch"
  enforcement = "active"

  conditions {
    ref_name {
      include = ["refs/heads/publish_branch"]
      exclude = []
    }
  }

  rules {
    restrict_pushes {
      blocks_creations = false
      push_allowances  = []
    }
    deletion = true
  }
}

###############################################################################
# RULESETS — synapse-repo-test
###############################################################################

resource "github_repository_ruleset" "test_main_branch" {
  repository  = github_repository.test.name
  name        = "main-branch-protection"
  target      = "branch"
  enforcement = "active"

  conditions {
    ref_name {
      include = ["refs/heads/main"]
      exclude = []
    }
  }

  bypass_actors {
    actor_id    = data.github_team.db_ops.id
    actor_type  = "Team"
    bypass_mode = "always"
  }

  rules {
    pull_request {
      required_approving_review_count   = 1
      require_code_owner_review         = false
      dismiss_stale_reviews_on_push     = true
      require_last_push_approval        = false
      required_review_thread_resolution = true
    }

    required_status_checks {
      strict_required_status_checks_policy = true
      required_check {
        context        = "synapse-test-cicd-workflow / validate"
        integration_id = 15368
      }
    }

    non_fast_forward = true
    deletion         = true
  }
}

resource "github_repository_ruleset" "test_publish_branch" {
  repository  = github_repository.test.name
  name        = "publish-branch-protection"
  target      = "branch"
  enforcement = "active"

  conditions {
    ref_name {
      include = ["refs/heads/publish_branch"]
      exclude = []
    }
  }

  rules {
    restrict_pushes {
      blocks_creations = false
      push_allowances  = []
    }
    deletion = true
  }
}

###############################################################################
# RULESETS — synapse-repo-prod
###############################################################################

resource "github_repository_ruleset" "prod_main_branch" {
  repository  = github_repository.prod.name
  name        = "main-branch-protection"
  target      = "branch"
  enforcement = "active"

  conditions {
    ref_name {
      include = ["refs/heads/main"]
      exclude = []
    }
  }

  bypass_actors {
    actor_id    = data.github_team.db_ops.id
    actor_type  = "Team"
    bypass_mode = "always"
  }

  rules {
    pull_request {
      required_approving_review_count   = 2
      require_code_owner_review         = false
      dismiss_stale_reviews_on_push     = true
      require_last_push_approval        = true
      required_review_thread_resolution = true
    }

    required_status_checks {
      strict_required_status_checks_policy = true
      required_check {
        context        = "synapse-prod-cicd-workflow / validate"
        integration_id = 15368
      }
    }

    non_fast_forward = true
    deletion         = true
  }
}

resource "github_repository_ruleset" "prod_publish_branch" {
  repository  = github_repository.prod.name
  name        = "publish-branch-protection"
  target      = "branch"
  enforcement = "active"

  conditions {
    ref_name {
      include = ["refs/heads/publish_branch"]
      exclude = []
    }
  }

  rules {
    restrict_pushes {
      blocks_creations = false
      push_allowances  = []
    }
    deletion = true
  }
}

###############################################################################
# TEAM ACCESS
###############################################################################

resource "github_team_repository" "db_ops_dev" {
  team_id    = data.github_team.db_ops.id
  repository = github_repository.dev.name
  permission = "push"
}

resource "github_team_repository" "db_ops_test" {
  team_id    = data.github_team.db_ops.id
  repository = github_repository.test.name
  permission = "push"
}

resource "github_team_repository" "db_ops_prod" {
  team_id    = data.github_team.db_ops.id
  repository = github_repository.prod.name
  permission = "push"
}

resource "github_team_repository" "devops_dev" {
  team_id    = data.github_team.devops.id
  repository = github_repository.dev.name
  permission = "push"
}
