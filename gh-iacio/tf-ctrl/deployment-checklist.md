# Work Item Prefix Ruleset - Deployment Checklist

Complete guide to validate, test, and deploy the Azure Boards work item prefix enforcement ruleset.

## 📋 Pre-Deployment Checklist

### 1. Prerequisites

- [ ] GitHub organization admin access
- [ ] Terraform >= 1.5.0 installed
- [ ] GitHub provider v6.6.0 configured
- [ ] Team `team-sunglow` exists in organization
- [ ] GitHub App or PAT with required permissions:
  - `admin:org` scope
  - `repo` scope

### 2. Verify Team Exists

```bash
# Check if team-sunglow exists
gh api orgs/YOUR-ORG/teams | jq '.[] | select(.slug=="team-sunglow")'

# If not, create it first:
# gh api orgs/YOUR-ORG/teams -X POST -f name="Team Sunglow" -f privacy="closed"
```

## 🧪 Step 1: Validate the Pattern

### Option A: Bash Script

```bash
# Download and run validation script
chmod +x validate_workitem_pattern.sh
./validate_workitem_pattern.sh

# Expected output: "ALL TESTS PASSED! ✓"
```

### Option B: Python Script

```bash
# Run Python validation
python3 validate_workitem_pattern.py

# Expected output: "ALL TESTS PASSED! ✓"
```

### Option C: Quick Manual Test

```bash
# Test the pattern with grep
echo "#AB123456 test" | grep -qE '^#AB[0-9]{6,10}.*$' && echo "✓ VALID" || echo "✗ INVALID"
echo "#AB75 test" | grep -qE '^#AB[0-9]{6,10}.*$' && echo "✓ VALID" || echo "✗ INVALID"

# Expected:
# ✓ VALID (first test)
# ✗ INVALID (second test)
```

## 🚀 Step 2: Deploy the Ruleset

### 1. Prepare Terraform Files

Create your project structure:

```
work-item-ruleset/
├── main.tf              # Organization ruleset configuration
├── provider.tf          # GitHub provider setup
├── variables.tf         # Variables
└── terraform.tfvars     # Your values (DO NOT COMMIT)
```

### 2. Configure Provider

**provider.tf**
```hcl
terraform {
  required_version = ">= 1.5.0"
  
  required_providers {
    github = {
      source  = "integrations/github"
      version = "~> 6.6"
    }
  }
}

provider "github" {
  owner = var.github_organization
  
  # Option 1: GitHub App (Recommended)
  app_auth {
    id              = var.github_app_id
    installation_id = var.github_app_installation_id
    pem_file        = var.github_app_pem_file
  }
  
  # Option 2: Personal Access Token
  # token = var.github_token
}
```

**variables.tf**
```hcl
variable "github_organization" {
  description = "GitHub organization name"
  type        = string
}

variable "github_app_id" {
  description = "GitHub App ID"
  type        = string
  default     = ""
}

variable "github_app_installation_id" {
  description = "GitHub App Installation ID"
  type        = string
  default     = ""
}

variable "github_app_pem_file" {
  description = "GitHub App PEM file"
  type        = string
  sensitive   = true
  default     = ""
}
```

**terraform.tfvars** (DO NOT COMMIT!)
```hcl
github_organization        = "your-org-name"
github_app_id              = "123456"
github_app_installation_id = "12345678"
github_app_pem_file        = <<-EOT
-----BEGIN RSA PRIVATE KEY-----
...your key...
-----END RSA PRIVATE KEY-----
EOT
```

### 3. Initialize and Plan

```bash
# Initialize Terraform
terraform init

# Review what will be created
terraform plan

# Expected output:
# Plan: 1 to add, 0 to change, 0 to destroy
```

### 4. Apply the Ruleset

```bash
# Apply with confirmation
terraform apply

# Or auto-approve (use with caution)
terraform apply -auto-approve
```

### 5. Verify Deployment

```bash
# Check outputs
terraform output

# Verify in GitHub UI
# Navigate to: https://github.com/organizations/YOUR-ORG/settings/rules

# Or via API
gh api orgs/YOUR-ORG/rulesets | jq '.[] | select(.name=="require-commit-and-branch-work-item-prefix")'
```

## ✅ Step 3: Test the Ruleset

### Test 1: Valid Commit (Should Work)

```bash
# Create test repository (if needed)
gh repo create test-ruleset-validation --public --clone

cd test-ruleset-validation

# Create valid branch
git checkout -b "#AB123456-test-feature"

# Create valid commit
echo "test" > test.txt
git add test.txt
git commit -m "#AB123456 test valid commit"

# Push (should succeed)
git push -u origin "#AB123456-test-feature"

# Expected: Push succeeds ✓
```

### Test 2: Invalid Commit (Should Fail)

```bash
# Create invalid branch (should fail at push time for branch)
git checkout -b "invalid-branch"

# Create invalid commit
echo "test2" > test2.txt
git add test2.txt
git commit -m "invalid commit message"

# Push (should be blocked)
git push -u origin invalid-branch

# Expected: Error message about branch/commit pattern ✗
```

### Test 3: Invalid Branch Name (Should Fail)

```bash
# Try to create branch with too few digits
git checkout -b "#AB12-short-branch"
echo "test3" > test3.txt
git add test3.txt
git commit -m "#AB123456 valid commit"
git push -u origin "#AB12-short-branch"

# Expected: Error about branch name pattern ✗
```

### Test 4: Team Sunglow Bypass (Should Work)

```bash
# As a member of team-sunglow, you should be able to bypass

# Create non-compliant branch
git checkout -b "bypass-test"

# Create non-compliant commit
echo "bypass" > bypass.txt
git add bypass.txt
git commit -m "bypass test"

# Push should succeed if you're in team-sunglow
git push -u origin bypass-test

# Expected: Push succeeds if you're in team-sunglow ✓
```

## 📊 Step 4: Monitor and Verify

### Check Ruleset Status

```bash
# View ruleset details
terraform show

# Check GitHub API
gh api orgs/YOUR-ORG/rulesets/RULESET-ID

# View bypass actors
terraform output bypass_team_id
```

### Review Failed Push Attempts

Failed pushes will show error messages like:

```
remote: error: GH013: Repository rule violations found for refs/heads/invalid-branch
remote: 
remote: Branch name does not match required pattern:
remote:   - Pattern: ^#AB\d{6,10}.*$
remote:   - Your branch: invalid-branch
```

## 🔧 Step 5: Adjust if Needed

### Enable Evaluation Mode (Testing)

If you want to test without blocking:

```hcl
# In main.tf, change enforcement
enforcement = "evaluate"  # Instead of "active"

# Re-apply
terraform apply
```

This will log violations but not block pushes, allowing you to monitor impact.

### Add Additional Bypass Actors

```hcl
# Add organization admins
bypass_actors {
  actor_id    = 1
  actor_type  = "OrganizationAdmin"
  bypass_mode = "always"
}

# Add another team
bypass_actors {
  actor_id    = data.github_team.platform_team.id
  actor_type  = "Team"
  bypass_mode = "always"
}
```

### Modify Pattern (If Needed)

```hcl
# Example: Allow 4-10 digits instead of 6-10
pattern = "^#AB\\d{4,10}.*$"

# Example: Case insensitive
pattern = "^#[Aa][Bb]\\d{6,10}.*$"

# Example: Optional work item (warning only pattern)
pattern = "^(#AB\\d{6,10}.*|.*)$"
```

## 🗑️ Rollback Plan

If something goes wrong:

```bash
# Disable ruleset temporarily
terraform apply -var='ruleset_enforcement=disabled'

# Or destroy completely
terraform destroy

# Verify removal
gh api orgs/YOUR-ORG/rulesets
```

## 📈 Success Criteria

- [ ] Validation script shows "ALL TESTS PASSED"
- [ ] Terraform apply succeeds without errors
- [ ] Ruleset visible in GitHub organization settings
- [ ] Valid commits/branches (#AB123456-*) push successfully
- [ ] Invalid commits/branches are blocked with clear error messages
- [ ] Team Sunglow members can bypass restrictions
- [ ] All existing repositories are covered by the ruleset

## 🎯 Pattern Reference

### Valid Examples
```
✓ #AB123456 fix bug
✓ #AB1234567890 release v2.0
✓ #AB123456-feature-branch
✓ #AB987654-bugfix/login
```

### Invalid Examples
```
✗ #AB12 too short
✗ #AB12345678901 too long
✗ fix bug (no prefix)
✗ #ab123456 (lowercase)
```

## 📚 Additional Resources

- [Terraform GitHub Provider Docs](https://registry.terraform.io/providers/integrations/github/latest/docs)
- [GitHub Rulesets Documentation](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-rulesets/about-rulesets)
- [Regex Testing Tool](https://regex101.com/) - Test pattern: `^#AB\d{6,10}.*$`

## 🆘 Troubleshooting

### Issue: Team Not Found

```bash
# Check team slug
gh api orgs/YOUR-ORG/teams | jq '.[] | select(.name | contains("sunglow"))'

# Update terraform if slug is different
data "github_team" "sunglow" {
  slug = "actual-team-slug-here"
}
```

### Issue: Pattern Not Matching

```bash
# Test pattern locally first
./validate_workitem_pattern.sh

# Check regex syntax
python3 -c "import re; print(re.match(r'^#AB\d{6,10}.*$', '#AB123456 test'))"
```

### Issue: Bypass Not Working

```bash
# Verify user is in bypass team
gh api teams/TEAM-ID/members | jq '.[] | select(.login=="username")'

# Check bypass_mode is "always"
terraform show | grep bypass_mode
```

## ✅ Final Checklist

Before marking complete:

- [ ] Validation script passes all tests
- [ ] Terraform state is backed up
- [ ] Team leads notified of new requirements
- [ ] Documentation updated with work item format
- [ ] Git hooks installed (optional) for pre-commit validation
- [ ] CI/CD pipelines updated if needed
- [ ] Support team briefed on common errors
- [ ] Rollback procedure tested in non-prod

---

**Deployment Complete!** 🎉

Your organization now enforces Azure Boards work item prefixes on all commits and branches.

Users must format commits/branches as: `#AB<6-10 digits><description>`

Example: `#AB123456-feature-authentication`
