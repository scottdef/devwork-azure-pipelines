# Wiki Repository Terraform Configuration

Infrastructure as code for org-wide wiki repository managed by team-au.

## Architecture

```
team-au (parent team, owns repo)
  └── team-au-mgrs (subteam, write access)

Custom Role: wiki-mods (admin-equivalent access)

Repository: org-wiki (internal visibility)
  - All org members: read access (implicit for internal repos)
  - team-au-mgrs: maintain access
  - wiki-mods role: admin-equivalent access

Ruleset: strong-org-rules (org-wide)
  - Targets: all repos, default + main branches
  - Bypass: team-au-mgrs, wiki-mods
  - Requires: 2 PR approvals
  - Restricts: creation, transfers, deletions
  - No required status checks

Actions: restricted to workflow pattern {repo}-wiki-custodian-job-*
```

## Prerequisites

- Terraform >= 1.5
- GitHub PAT with scopes:
  - `repo` (full)
  - `admin:org` (full)
  - `workflow`
- GitHub Enterprise Cloud (for custom roles)

## Setup

```bash
# Set GitHub token
export GITHUB_TOKEN="ghp_your_token_here"

# Configure terraform.tfvars
cat > terraform.tfvars <<EOF
github_org     = "your-org-name"
wiki_repo_name = "org-wiki"
EOF

# Initialize
terraform init

# Plan
terraform plan -out=tfplan

# Apply
terraform apply tfplan
```

## Workflow Integration

GitHub Actions workflow example for custodian jobs:

```yaml
# .github/workflows/org-wiki-wiki-custodian-job-sync.yml
name: org-wiki-wiki-custodian-job-sync

on:
  schedule:
    - cron: '0 2 * * *'
  workflow_dispatch:

jobs:
  sync:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Sync wiki content
        run: |
          # Your sync logic here
          echo "Syncing wiki content..."
```

## Repository Structure

```
.
├── provider.tf         # GitHub provider config
├── wiki-repo.tf        # Main resources
├── terraform.tfvars    # Variable values
└── README.md          # This file
```

## Permission Model

| Entity | Access Level | Permissions |
|--------|-------------|-------------|
| All org members | Read | View code, clone, open issues |
| team-au | Push | Same as all members (parent team) |
| team-au-mgrs | Maintain | Merge PRs, manage settings, bypass ruleset |
| wiki-mods role | Admin-equivalent | Full repo control, bypass ruleset |

## Ruleset Bypass

team-au-mgrs and wiki-mods can:
- Push directly to protected branches
- Bypass PR review requirements
- Create/delete branches
- Transfer repositories

Everyone else must:
- Create PRs for changes
- Get 2 approvals
- Pass all CI checks (when configured)

## Actions Restrictions

Only workflows matching pattern: `{wiki_repo_name}-wiki-custodian-job-*`

Examples:
- ✅ `org-wiki-wiki-custodian-job-sync.yml`
- ✅ `org-wiki-wiki-custodian-job-backup.yml`
- ❌ `random-workflow.yml`
- ❌ `wiki-sync.yml`

## Maintenance

```bash
# Check drift
terraform plan

# Update permissions
# Edit wiki-repo.tf, then:
terraform plan -out=tfplan
terraform apply tfplan

# Import existing resource
terraform import github_team.team_au 12345

# Destroy (careful!)
terraform destroy
```

## Custom Role Permissions

wiki-mods role includes:
- `manage_settings_wiki` - Wiki settings
- `edit_repo_metadata` - Description, topics
- `set_social_preview` - Social media card
- `manage_settings_pages` - GitHub Pages config
- `push_protected_branch` - Direct push capability
- `bypass_required_pull_request_reviews` - Skip PR reviews
- `delete_branch` - Branch deletion

## Notes

- Internal repos grant read access to all org members by default
- Custom roles require GitHub Enterprise Cloud
- Ruleset applies org-wide but can be bypassed by specified actors
- Actions restrictions are enforced at repository level
- Wiki feature is enabled on the repository
- Squash merges only, no merge commits or rebase

## Troubleshooting

**Issue**: Custom role creation fails
**Fix**: Ensure GitHub Enterprise Cloud subscription

**Issue**: Ruleset not applying
**Fix**: Check `enforcement = "active"` and target conditions

**Issue**: Actions workflow blocked
**Fix**: Verify workflow name matches pattern exactly

**Issue**: Team members can't push
**Fix**: Check team membership and repository permissions

## Links

- [Terraform GitHub Provider Docs](https://registry.terraform.io/providers/integrations/github/latest/docs)
- [GitHub Rulesets](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-rulesets)
- [Custom Roles](https://docs.github.com/en/enterprise-cloud@latest/organizations/managing-peoples-access-to-your-organization-with-roles/managing-custom-repository-roles-for-an-organization)
