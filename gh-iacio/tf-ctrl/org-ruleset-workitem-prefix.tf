# ==============================================================================
# Organization Ruleset: require-commit-and-branch-work-item-prefix
# ==============================================================================
#
# This ruleset enforces Azure Boards work item prefixes on:
# - Commit messages: Must start with #AB followed by 6-10 digits
# - Branch names: Must start with #AB followed by 6-10 digits
#
# Valid Examples:
#   Commit: "#AB123975 fix really bad bug"
#   Branch: "#AB123975-good-branch"
#
# Invalid Examples:
#   Commit: "#AB75 fix really bad bug" (only 2 digits)
#   Commit: "#fix really bad bug" (no work item)
#   Branch: "#AB75really-bad-branch" (only 2 digits)
#   Branch: "#bad-branch" (no work item)
#
# ==============================================================================

# ------------------------------------------------------------------------------
# Data Sources
# ------------------------------------------------------------------------------

# Get the team that should bypass the ruleset
data "github_team" "sunglow" {
  slug = "team-sunglow"
}

# ------------------------------------------------------------------------------
# Organization Ruleset Resource
# ------------------------------------------------------------------------------

resource "github_organization_ruleset" "require_commit_and_branch_work_item_prefix" {
  name        = "require-commit-and-branch-work-item-prefix"
  target      = "branch"
  enforcement = "active"

  # Target all repositories
  conditions {
    ref_name {
      # Target all branches (include all refs)
      include = ["~ALL"]
      exclude = []
    }

    repository_name {
      # Apply to all repositories in the organization
      include = ["~ALL"]
      exclude = []
      protected = false
    }
  }

  # Bypass actors - team-sunglow can bypass this ruleset
  bypass_actors {
    actor_id    = data.github_team.sunglow.id
    actor_type  = "Team"
    bypass_mode = "always"
  }

  # Rules
  rules {
    # Commit message pattern enforcement
    # Pattern: #AB followed by 6-10 digits
    # Regex explanation:
    # ^        - Start of string
    # #AB      - Literal "#AB"
    # \d{6,10} - Exactly 6 to 10 digits
    # .*       - Followed by anything (the commit message)
    # $        - End of string
    commit_message_pattern {
      name     = "Azure Boards Work Item Prefix"
      operator = "regex"
      pattern  = "^#AB\\d{6,10}.*$"
      negate   = false
    }

    # Branch name pattern enforcement
    # Pattern: #AB followed by 6-10 digits
    # Regex explanation:
    # ^        - Start of string
    # #AB      - Literal "#AB"
    # \d{6,10} - Exactly 6 to 10 digits
    # .*       - Followed by anything (rest of branch name)
    # $        - End of string
    branch_name_pattern {
      name     = "Azure Boards Work Item Branch Prefix"
      operator = "regex"
      pattern  = "^#AB\\d{6,10}.*$"
      negate   = false
    }
  }
}

# ------------------------------------------------------------------------------
# Outputs
# ------------------------------------------------------------------------------

output "ruleset_id" {
  description = "ID of the created organization ruleset"
  value       = github_organization_ruleset.require_commit_and_branch_work_item_prefix.id
}

output "ruleset_node_id" {
  description = "Node ID of the created organization ruleset"
  value       = github_organization_ruleset.require_commit_and_branch_work_item_prefix.node_id
}

output "ruleset_name" {
  description = "Name of the created organization ruleset"
  value       = github_organization_ruleset.require_commit_and_branch_work_item_prefix.name
}

output "ruleset_enforcement" {
  description = "Enforcement level of the ruleset"
  value       = github_organization_ruleset.require_commit_and_branch_work_item_prefix.enforcement
}

output "bypass_team_id" {
  description = "Team ID that can bypass this ruleset"
  value       = data.github_team.sunglow.id
}

output "bypass_team_slug" {
  description = "Team slug that can bypass this ruleset"
  value       = data.github_team.sunglow.slug
}

# ------------------------------------------------------------------------------
# Pattern Testing Examples
# ------------------------------------------------------------------------------

# Valid Commit Messages:
# ✅ "#AB123456 fix really bad bug"
# ✅ "#AB123975 add new feature"
# ✅ "#AB1234567890 maximum 10 digits"
# ✅ "#AB123456"
#
# Invalid Commit Messages:
# ❌ "#AB75 fix really bad bug" (only 2 digits, needs 6-10)
# ❌ "#AB12345 fix bug" (only 5 digits, needs 6-10)
# ❌ "#fix really bad bug" (no work item prefix)
# ❌ "fix really bad bug" (no prefix at all)
# ❌ "#AB12345678901 too many digits" (11 digits, max is 10)
# ❌ "AB123456 missing hash" (no # symbol)
# ❌ "#ab123456 lowercase" (lowercase ab instead of AB)
#
# Valid Branch Names:
# ✅ "#AB123456-feature-branch"
# ✅ "#AB123975-good-branch"
# ✅ "#AB1234567890-max-digits"
# ✅ "#AB123456"
#
# Invalid Branch Names:
# ❌ "#AB75-really-bad-branch" (only 2 digits)
# ❌ "#AB12345-short" (only 5 digits)
# ❌ "#bad-branch" (no work item prefix)
# ❌ "feature-branch" (no prefix at all)
# ❌ "#AB12345678901-too-long" (11 digits)
# ❌ "AB123456-no-hash" (no # symbol)
# ❌ "#ab123456-lowercase" (lowercase ab)

# ==============================================================================
# Usage Example
# ==============================================================================

# 1. Ensure GitHub provider is configured:
#
# provider "github" {
#   owner = "your-organization"
#   token = var.github_token
#   # or use app_auth for GitHub App
# }

# 2. Ensure team-sunglow exists in your organization

# 3. Apply this configuration:
#
# terraform init
# terraform plan
# terraform apply

# 4. Test the ruleset:
#
# # This will be ALLOWED:
# git checkout -b "#AB123456-feature-branch"
# git commit -m "#AB123456 implement new feature"
# git push
#
# # This will be BLOCKED:
# git checkout -b "feature-branch"
# git commit -m "implement new feature"
# git push
# # Error: Branch name does not match required pattern

# ==============================================================================
# Troubleshooting
# ==============================================================================

# If the ruleset is not working as expected:
#
# 1. Verify team exists:
#    gh api orgs/YOUR-ORG/teams | jq '.[] | select(.slug=="team-sunglow")'
#
# 2. Check ruleset status:
#    gh api orgs/YOUR-ORG/rulesets
#
# 3. Test regex pattern:
#    echo "#AB123456 test" | grep -E "^#AB\d{6,10}.*$"
#
# 4. Verify enforcement is active (not evaluate or disabled)
#
# 5. Check if user is member of bypass team:
#    gh api teams/TEAM-ID/members

# ==============================================================================
# Advanced Configuration Options
# ==============================================================================

# If you need to modify the ruleset for different requirements:

# Example: Make enforcement optional for testing
# enforcement = "evaluate"  # Logs violations but doesn't block

# Example: Add organization admins as bypass actors
# bypass_actors {
#   actor_id    = 1
#   actor_type  = "OrganizationAdmin"
#   bypass_mode = "always"
# }

# Example: Exclude specific repositories
# conditions {
#   repository_name {
#     include = ["~ALL"]
#     exclude = ["legacy-repo", "archived-*"]
#     protected = false
#   }
# }

# Example: Only apply to specific branches
# conditions {
#   ref_name {
#     include = ["refs/heads/main", "refs/heads/develop"]
#     exclude = []
#   }
# }

# Example: Case-insensitive pattern (allow #AB or #ab)
# commit_message_pattern {
#   name     = "Work Item Prefix (Case Insensitive)"
#   operator = "regex"
#   pattern  = "^#[Aa][Bb]\\d{6,10}.*$"
#   negate   = false
# }

# ==============================================================================
# End of Configuration
# ==============================================================================
