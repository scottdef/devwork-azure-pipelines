# GitHub Governance with Terraform

> *"Simplicity is the ultimate sophistication."* - Rob Pike (paraphrasing da Vinci)

A declarative GitHub organization management system. YAML files define the desired state. Terraform makes it so. GitHub Actions orchestrates the whole dance.

## Philosophy

This system follows the Unix philosophy:

1. **Do one thing well**: YAML defines state, Terraform applies it
2. **Composability**: Small, focused files that work together
3. **Text as interface**: Everything is code, everything is reviewable
4. **Automation without ceremony**: Workflows trigger on PR, no manual intervention needed

## Quick Start

### Option 1: IssueOps (Recommended for Users)

```bash
# 1. Go to Issues → New Issue → "Create Repository"
# 2. Fill out the form
# 3. Wait for 2 platform-team approvals (👍 or /approve)
# 4. Automation creates PR
# 5. Review and merge
# 6. Repository created!
```

See [ISSUEOPS-QUICKSTART.md](ISSUEOPS-QUICKSTART.md) for details.

### Option 2: Direct YAML Editing (For Advanced Users)

```bash
# Clone the repo
git clone <your-repo-url>
cd github-governance

# Add a new repository
cat >> repo-yamls/myteam-repos.yml <<EOF
repos:
  - name: awesome-project
    description: "Something awesome"
    team: platform-team
    visibility: private
EOF

# Validate
./scripts/validate-yaml.py

# Commit and push
git add repo-yamls/myteam-repos.yml
git commit -m "Add awesome-project"
git push

# Open PR - plan runs automatically
# Merge PR - apply runs automatically
```

## Directory Structure

```
.
├── repo-yamls/          # Repository definitions
├── team-yamls/          # Team definitions  
├── terraform/           # Terraform module
├── .github/workflows/   # CI/CD automation
└── scripts/             # Helper scripts
```

## YAML Configuration

### Repositories (`repo-yamls/*.yml`)

```yaml
repos:
  - name: my-service
    description: "Service description"
    team: platform-team              # Parent team
    visibility: private              # public, private, internal
    permission: push                 # pull, triage, push, maintain, admin
    has_issues: true
    has_wiki: false
    require_signed_commits: false
```

**Automatic behaviors:**
- CODEOWNERS file created with parent team and `{team}-pr-approvers`
- Branch protection on main: requires 2 reviews from CODEOWNERS
- Squash merge enforced
- `{team}-admins` gets admin access

### Teams (`team-yamls/*.yml`)

```yaml
teams:
  - name: platform-team
    description: "Platform Engineering"
    privacy: closed                  # closed or secret
    ad_group: "eng-platform"         # Triggers subteam creation
    members:
      - username: alice
        role: maintainer              # member or maintainer
      - username: bob
        role: member
    review_request_delegation:
      algorithm: ROUND_ROBIN          # ROUND_ROBIN or LOAD_BALANCE
      member_count: 2
      notify: true
```

**When `ad_group` is present, three subteams are auto-created:**
- `{team}-admins` - Repository administrators
- `{team}-workflow-admins` - CI/CD management
- `{team}-pr-approvers` - Code reviewers in CODEOWNERS

Maintainers are automatically added to the `-admins` subteam.

### Subteams

```yaml
teams:
  - name: platform-sre
    description: "SRE subteam"
    parent_name: platform-team       # Creates hierarchy
    privacy: closed
    members:
      - username: alice
        role: maintainer
```

## Workflows

### IssueOps Workflows

**IssueOps**: Create infrastructure via GitHub Issues - no YAML editing required!

#### Approval Tracking (`issueops-approval.yml`)

**Triggers:**
- Issue comment created
- Issue labeled

**Actions:**
1. Validates commenter is in platform-team
2. Counts approvals (👍 or `/approve`)
3. Updates issue with status
4. Triggers create-repo workflow at threshold (2 approvals)

#### Create Repository (`issueops-create-repo.yml`)

**Trigger:** Manual dispatch from approval workflow

**Actions:**
1. Parses issue form
2. Updates YAML files
3. Creates branch `request/issue-{number}`
4. Opens PR automatically
5. Comments back on issue

See [ISSUEOPS.md](ISSUEOPS.md) for complete documentation.

### Terraform Workflows

#### Terraform Plan

**Triggers:**
- Pull request opened/updated targeting `main`
- Manual dispatch (any branch except `main`)

**Actions:**
1. Validates Terraform
2. Runs `terraform plan`
3. Posts plan to PR as comment
4. Uploads plan artifact

### Terraform Apply

**Triggers:**
- Pull request merged to `main`
- Manual dispatch with confirmation (not on `main`)

**Actions:**
1. Runs `terraform apply`
2. Posts result to PR
3. Triggers downstream workflows/webhooks (if configured)

### Workflow Trigger

**Manual dispatch only**

Orchestrates cross-repository workflows:
- Trigger plan/apply in other repos
- Send webhook notifications
- Record workflow runs via git commits

Example: Trigger apply in another repo:
```bash
gh workflow run workflow-trigger.yml \
  -f target_workflow=apply \
  -f target_repo=org/other-repo \
  -f target_branch=main \
  -f webhook_url=https://example.com/webhook
```

## Required Secrets

Configure in GitHub repository settings:

| Secret | Description |
|--------|-------------|
| `GH_ADMIN_TOKEN` | GitHub PAT with `admin:org`, `repo` scopes |
| `GITHUB_ORG` | Your GitHub organization name |
| `TF_API_TOKEN` | Terraform Cloud/Enterprise token (if using) |

## Validation Scripts

### Bash Script (`scripts/parse-yamls.sh`)

Fast, simple validation:

```bash
# Validate all YAMLs
./scripts/parse-yamls.sh validate

# List all repositories
./scripts/parse-yamls.sh list-repos

# List all teams
./scripts/parse-yamls.sh list-teams
```

Requires: `yq` ([github.com/mikefarah/yq](https://github.com/mikefarah/yq))

### Python Script (`scripts/validate-yaml.py`)

Advanced validation with cross-reference checking:

```bash
# Validate with detailed error messages
./scripts/validate-yaml.py

# Validate specific directory
./scripts/validate-yaml.py /path/to/repo
```

Checks:
- YAML syntax
- Required fields
- Team references exist
- No duplicate resources
- Valid GitHub constraints

## Terraform Backend

Configure your backend in `terraform/main.tf`:

```hcl
backend "s3" {
  bucket = "your-terraform-state"
  key    = "github-governance/terraform.tfstate"
  region = "us-east-1"
}
```

Or use Terraform Cloud:

```hcl
backend "remote" {
  organization = "your-org"
  workspaces {
    name = "github-governance"
  }
}
```

## Common Operations

### Add a New Repository

```bash
# Edit or create YAML file
vim repo-yamls/team-repos.yml

# Add repo definition
repos:
  - name: new-repo
    team: your-team
    description: "New repository"

# Validate
./scripts/validate-yaml.py

# Commit and push
git add repo-yamls/team-repos.yml
git commit -m "Add new-repo"
git push

# Open PR and merge after plan looks good
```

### Add a New Team

```bash
# Edit or create YAML file
vim team-yamls/new-team.yml

teams:
  - name: new-team
    description: "New team"
    ad_group: "ad-group-name"  # Creates subteams
    members:
      - username: alice
        role: maintainer

# Validate, commit, push, PR
```

### Modify Existing Resources

Edit the YAML file, commit, push, PR. The plan will show what changes.

### Delete Resources

Remove from YAML. Terraform will show destruction in plan. Review carefully before merging.

## Branch Protection Details

All repositories get a ruleset on the default branch (`main`) with:

- **Require pull request**: Yes
- **Required reviewers**: 2
- **Require code owner review**: Yes (from CODEOWNERS)
- **Dismiss stale reviews**: Yes
- **Require linear history**: Yes (enforces squash)
- **No force pushes**: Yes
- **Bypass actors**: `{team}-admins` team

## CI/CD Integration

The system can trigger external workflows and send webhooks. Configure in `terraform-apply.yml`:

```yaml
- name: Trigger Downstream
  run: |
    curl -X POST https://your-webhook.com/notify \
      -H "Content-Type: application/json" \
      -d '{"event": "infrastructure_updated"}'
```

Or use the workflow trigger to orchestrate:

```bash
gh workflow run workflow-trigger.yml \
  -f target_workflow=plan \
  -f target_repo=org/infrastructure \
  -f webhook_url=https://notify.com/hook
```

## Testing

Before merging to production:

1. **Validate locally**: `./scripts/validate-yaml.py`
2. **Check Terraform plan**: Review the plan output in PR
3. **Test in sandbox**: Use workflow dispatch on feature branch
4. **Peer review**: Have someone review the YAML changes

## Troubleshooting

### Plan fails with "team not found"

- Check team names in `team-yamls/` match exactly what you reference in `repo-yamls/`
- Teams must be created before repositories that reference them

### Apply fails with "reference already exists"

- CODEOWNERS file might already exist and not managed by Terraform
- Set `overwrite_on_create = true` in `github_repository_file` resource

### Subteams not created

- Verify `ad_group` field is present in team definition
- Check Terraform logs for team creation errors

### Workflow doesn't trigger

- Ensure the path filters in workflows match your changes
- Check you're not on the `main` branch for manual dispatch

## Design Decisions

### Why YAML over HCL?

- More accessible to non-Terraform users
- Easier to template and generate
- Better for line-of-business team ownership

### Why separate files per team?

- Clear ownership boundaries
- Parallel development without conflicts
- Easier to manage permissions on YAML files themselves

### Why not GitOps with ArgoCD/Flux?

- Terraform state provides better dependency management
- GitHub provider handles API intricacies
- Simpler for GitHub-only use case

### Why both bash and Python validators?

- Bash for fast, simple checks in CI
- Python for comprehensive validation and IDE integration
- Use the right tool for the job

## Contributing

1. Fork the repo
2. Create feature branch from `main`
3. Add your changes
4. Validate: `./scripts/validate-yaml.py`
5. Commit with clear message
6. Open PR
7. Review the plan output
8. Merge after approval

## License

MIT

## Authors

Built with the spirit of Rob Pike and Ken Thompson:
- Keep it simple
- Make it fast  
- Do one thing well
- Compose small tools
- Text is the universal interface

---

*"Debugging is twice as hard as writing the code in the first place. Therefore, if you write the code as cleverly as possible, you are, by definition, not smart enough to debug it."* - Brian Kernighan

Keep it simple. Ship it.
