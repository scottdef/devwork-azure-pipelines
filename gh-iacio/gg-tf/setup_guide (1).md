# Setup Guide

Get up and running in 10 minutes. No ceremony, just code.

## Prerequisites

```bash
# Required
terraform >= 1.5.0
python3 >= 3.8
git
gh (GitHub CLI)

# Optional but recommended
yq (for bash validation)
make
```

### Install Tools (macOS)

```bash
brew install terraform python3 gh yq
```

### Install Tools (Linux)

```bash
# Terraform
wget -O- https://apt.releases.hashicorp.com/gpg | sudo gpg --dearmor -o /usr/share/keyrings/hashicorp-archive-keyring.gpg
echo "deb [signed-by=/usr/share/keyrings/hashicorp-archive-keyring.gpg] https://apt.releases.hashicorp.com $(lsb_release -cs) main" | sudo tee /etc/apt/sources.list.d/hashicorp.list
sudo apt update && sudo apt install terraform

# GitHub CLI
curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg | sudo dd of=/usr/share/keyrings/githubcli-archive-keyring.gpg
sudo chmod go+r /usr/share/keyrings/githubcli-archive-keyring.gpg
echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" | sudo tee /etc/apt/sources.list.d/github-cli.list > /dev/null
sudo apt update && sudo apt install gh

# yq
sudo wget -qO /usr/local/bin/yq https://github.com/mikefarah/yq/releases/latest/download/yq_linux_amd64
sudo chmod +x /usr/local/bin/yq
```

## Initial Setup

### 1. Clone and Configure

```bash
# Clone this repository (or create from template)
git clone <your-repo-url> github-governance
cd github-governance

# Make scripts executable
chmod +x scripts/*.sh scripts/*.py
```

### 2. Create GitHub Token

Create a GitHub Personal Access Token (classic) with these scopes:
- `repo` (Full control of private repositories)
- `admin:org` (Full control of orgs and teams)
- `workflow` (Update GitHub Action workflows)

```bash
# Using gh CLI
gh auth login

# Or create token at:
# https://github.com/settings/tokens/new
```

### 3. Configure Secrets

#### For GitHub Actions

Add these secrets to your repository:

```bash
gh secret set GH_ADMIN_TOKEN --body "ghp_your_token_here"
gh secret set GITHUB_ORG --body "your-org-name"
gh secret set TF_API_TOKEN --body "your-tf-cloud-token"  # If using TF Cloud
```

#### For Local Development

```bash
# Create .env file (gitignored)
cat > .env <<EOF
export GITHUB_TOKEN="ghp_your_token_here"
export GITHUB_ORG="your-org-name"
export TF_VAR_github_token="\$GITHUB_TOKEN"
export TF_VAR_github_org="\$GITHUB_ORG"
EOF

# Source it
source .env
```

### 4. Configure Terraform Backend

Edit `terraform/main.tf` and configure your backend:

#### Option A: Terraform Cloud

```hcl
backend "remote" {
  organization = "your-org"
  
  workspaces {
    name = "github-governance"
  }
}
```

#### Option B: AWS S3

```hcl
backend "s3" {
  bucket         = "your-terraform-state"
  key            = "github-governance/terraform.tfstate"
  region         = "us-east-1"
  encrypt        = true
  dynamodb_table = "terraform-state-lock"
}
```

#### Option C: Local (testing only)

```hcl
# Comment out the backend block for local state
# backend "s3" { ... }
```

### 5. Initialize Terraform

```bash
cd terraform
terraform init
cd ..

# Or use make
make init
```

## First Run

### Validate Configuration

```bash
# Quick validation
make validate

# Or manually
python3 scripts/validate-yaml.py
./scripts/parse-yamls.sh validate
```

### Run Initial Plan

```bash
# Using make
make plan

# Or manually
cd terraform
terraform plan \
  -var="github_org=$GITHUB_ORG" \
  -var="github_token=$GITHUB_TOKEN"
```

### Apply Changes

```bash
# Interactive (will ask for confirmation)
make apply

# Or manually
cd terraform
terraform apply \
  -var="github_org=$GITHUB_ORG" \
  -var="github_token=$GITHUB_TOKEN"
```

## Create Your First Resources

### Add a Team

```bash
# Create team YAML
cat > team-yamls/engineering.yml <<'EOF'
teams:
  - name: engineering
    description: "Engineering Team"
    privacy: closed
    ad_group: "engineering-all"
    members:
      - username: your-github-username
        role: maintainer
EOF

# Validate
make validate

# Commit
git add team-yamls/engineering.yml
git commit -m "Add engineering team"
git push
```

### Add a Repository

```bash
# Create repo YAML
cat > repo-yamls/engineering-repos.yml <<'EOF'
repos:
  - name: test-repo
    description: "Test repository"
    team: engineering
    visibility: private
EOF

# Validate
make validate

# Commit
git add repo-yamls/engineering-repos.yml
git commit -m "Add test-repo"
git push
```

### Create Pull Request

```bash
# Create PR using gh CLI
gh pr create \
  --title "Add initial resources" \
  --body "Adding engineering team and test repository"

# Or push to branch and create PR in web UI
git checkout -b add-resources
git push -u origin add-resources
```

The `terraform-plan` workflow will run automatically and comment on your PR with the plan output.

## Verify Setup

After merging your first PR:

1. Check the Actions tab - `terraform-apply` should have run
2. Verify teams exist: `gh api orgs/YOUR-ORG/teams`
3. Verify repos exist: `gh repo list YOUR-ORG`
4. Check CODEOWNERS: `gh api repos/YOUR-ORG/test-repo/contents/.github/CODEOWNERS`

## Common Issues

### Authentication Errors

```bash
# Verify token has correct scopes
gh auth status

# Refresh token
gh auth refresh -h github.com -s admin:org,repo,workflow
```

### Backend Configuration

```bash
# If using S3, create bucket first
aws s3 mb s3://your-terraform-state

# If using Terraform Cloud, create organization first
# https://app.terraform.io/
```

### Team Already Exists

If teams/repos already exist in your org:

```bash
# Import existing resources
cd terraform
terraform import 'github_team.teams["existing-team"]' "existing-team-id"
terraform import 'github_repository.repos["existing-repo"]' "existing-repo"
```

Find team ID:
```bash
gh api orgs/YOUR-ORG/teams/existing-team | jq .id
```

### CODEOWNERS Conflicts

If CODEOWNERS file already exists:

1. Backup existing CODEOWNERS
2. Let Terraform create new one
3. Merge any custom rules into the template at `terraform/modules/codeowners/templates/CODEOWNERS.tpl`

## Daily Workflow

```bash
# 1. Make changes to YAML files
vim repo-yamls/myteam-repos.yml

# 2. Validate locally
make validate

# 3. Commit and push
git add repo-yamls/myteam-repos.yml
git commit -m "Add new repository"
git push

# 4. Create PR
gh pr create --title "Add new repo" --body "Description"

# 5. Review plan in PR comments

# 6. Merge PR (or gh pr merge)

# 7. Watch apply run in Actions tab
```

## Next Steps

1. **Add more teams** - Create YAML files for each team
2. **Add repositories** - Define all your repositories
3. **Customize templates** - Modify CODEOWNERS template if needed
4. **Set up webhooks** - Configure workflow-trigger.yml for your needs
5. **Import existing** - Import existing GitHub resources into Terraform

## Getting Help

```bash
# List all make targets
make help

# Validate YAMLs
make validate

# List resources
make list-repos
make list-teams

# View Terraform state
make show

# View outputs
make output
```

## Security Best Practices

1. **Never commit tokens** - Use secrets management
2. **Limit token scope** - Only required permissions
3. **Rotate tokens regularly** - Every 90 days
4. **Use branch protection** - Protect main branch
5. **Review plans carefully** - Always check before merge
6. **Audit access** - Review team memberships quarterly

## Advanced Configuration

### Multi-Environment Setup

```bash
# Create separate state per environment
terraform workspace new production
terraform workspace new staging

# Or use separate directories
mkdir -p environments/{prod,staging}
```

### Custom Validation Rules

Edit `scripts/validate-yaml.py` to add custom validation logic:

```python
def _validate_custom_rules(self):
    """Add your custom validation"""
    for name, repo in self.repos.items():
        # Example: Enforce naming convention
        if not repo['name'].startswith('team-'):
            self.warnings.append(f"Repo {name} doesn't follow naming convention")
```

### Integrate with External Systems

Modify workflows to trigger external systems:

```yaml
- name: Notify Slack
  run: |
    curl -X POST ${{ secrets.SLACK_WEBHOOK }} \
      -d '{"text": "Infrastructure updated"}'
```

## Resources

- [Terraform GitHub Provider](https://registry.terraform.io/providers/integrations/github/latest/docs)
- [GitHub REST API](https://docs.github.com/en/rest)
- [GitHub Actions](https://docs.github.com/en/actions)
- [Terraform Documentation](https://www.terraform.io/docs)

---

**Remember**: Keep it simple. Make it work. Ship it.

If something's broken, `make clean && make init && make plan` usually fixes it.
