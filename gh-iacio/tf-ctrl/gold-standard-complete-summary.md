# Gold Standard GitHub Repository Module - Complete Package

**Version**: 1.0.0  
**GitHub Provider**: v6.6.0  
**Terraform**: >= 1.5.0

## 📦 What You've Got

A production-ready, enterprise-grade Terraform module for creating GitHub repositories with:

### ✅ Core Features
- **Every v6.6 parameter exposed** - 150+ configurable variables with sensible defaults
- **Pre-flight validation** - Fails fast if repository already exists
- **Gold standard defaults** - Automatic tagging, security hardening, Actions disabled
- **Team hierarchy support** - Parent/child team relationships with inheritance
- **Custom roles** - Organization-level custom roles with `{repo-name}-superdev` pattern
- **Multi-environment** - `nonproductive` and `prod` environments out of the box
- **Repository rulesets** - Modern branch protection with comprehensive controls
- **REST API integration** - Custom metadata injection via GitHub API

### 📋 Required Resources (Per Module Specs)

✅ `github_repository` - With topics, Actions disabled, workflow-accessible  
✅ `github_repository_environment` - Two environments (nonproductive, prod)  
✅ `github_repository_ruleset` - Comprehensive branch protection  
✅ `github_rest_api` - Custom metadata injection  
✅ `github_team` - With configurable parent team  
✅ `github_team_repository` - Default admin permission  
✅ `github_team_members` - Team membership management  
✅ `github_organization_custom_role` - `{repo-name}-superdev` pattern

### 📊 Required Data Sources

✅ `github_user` - Creator user details  
✅ `github_rest_api` - Repository metadata  
✅ `github_team` - Parent team lookup  
✅ `github_organization_teams` - Organization teams  
✅ `github_repositories` - Pre-flight existence check (fails if repo exists)

## 🏗️ Complete File Structure

```
modules/gold-standard-github-repository/
├── README.md                      # Full documentation
├── QUICKSTART.md                  # 5-minute getting started
├── CHANGELOG.md                   # Version history
├── LICENSE                        # MIT license
├── .gitignore                     # Security-focused ignores
│
├── main.tf                        # All resources (1 file, ~800 lines)
├── variables.tf                   # 150+ input variables
├── outputs.tf                     # Comprehensive outputs
├── versions.tf                    # Provider requirements
│
├── examples/
│   ├── simple/                    # Minimal example
│   │   ├── main.tf
│   │   └── terraform.tfvars.example
│   │
│   ├── complete/                  # Full-featured example
│   │   ├── main.tf               # 12 different patterns
│   │   ├── outputs.tf
│   │   └── terraform.tfvars.example
│   │
│   └── advanced-patterns/         # Module composition examples
│       └── main.tf               # YAML-driven, factories, multi-team
│
├── tests/
│   ├── repository_test.go        # Terratest suite (10 tests)
│   ├── go.mod
│   └── go.sum
│
├── .github/
│   ├── workflows/
│   │   ├── terraform-validation.yml    # Format, lint, validate
│   │   ├── terratest.yml              # Integration tests
│   │   ├── security-scan.yml          # tfsec, Checkov, Trivy
│   │   ├── documentation.yml          # Auto-generate docs
│   │   ├── release.yml                # Automated releases
│   │   ├── example-apply.yml          # Manual testing
│   │   ├── pr-labeler.yml             # Auto-label PRs
│   │   └── stale.yml                  # Stale issue management
│   │
│   ├── dependabot.yml             # Automated dependency updates
│   ├── labeler.yml                # PR labeling rules
│   └── CODEOWNERS                 # Code ownership
│
└── docs/
    ├── advanced-patterns.md       # Advanced usage patterns
    ├── security.md                # Security best practices
    └── migration.md               # Migration guide
```

## 🚀 Quick Start (3 Commands)

```bash
# 1. Set credentials
export TF_VAR_github_app_id="123456"
export TF_VAR_github_app_installation_id="12345678"
export TF_VAR_github_app_pem_file="$(cat github-app.pem)"

# 2. Create main.tf
cat > main.tf <<'EOF'
module "my_repo" {
  source = "./modules/gold-standard-github-repository"
  
  repository_name  = "my-awesome-service"
  parent_team_slug = "platform-team"
  
  team_members = [
    { username = "alice", role = "maintainer" }
  ]
}
EOF

# 3. Deploy
terraform init && terraform apply
```

## 📝 What Gets Created

### Repository Configuration
```yaml
Name: my-awesome-service
Visibility: private
Topics: [gold-standard, terraform-managed, creator:terraform]
Security:
  - Secret scanning: enabled
  - Push protection: enabled  
  - Vulnerability alerts: enabled
Features:
  - Issues: enabled
  - Projects: enabled
  - Wiki: disabled
  - Actions: disabled (but workflow-accessible)
```

### Team Structure
```yaml
Team: my-awesome-service-team
Parent: platform-team
Permission: admin
Members:
  - alice (maintainer)
```

### Environments
```yaml
nonproductive:
  wait_timer: 0 minutes
  can_admins_bypass: true
  prevent_self_review: false
  
prod:
  wait_timer: 5 minutes
  can_admins_bypass: false
  prevent_self_review: true
```

### Custom Role
```yaml
Name: my-awesome-service-superdev
Base: write
Permissions:
  - read_code
  - write_code
  - read_actions
  - write_actions
  - read_deployments
  - write_deployments
  - manage_pull_requests
  - manage_issues
```

### Ruleset
```yaml
Name: Default Branch Protection
Target: Main branch
Enforcement: active
Rules:
  - Deletion: blocked
  - Non-fast-forward: blocked
  - Required reviews: 2
  - Code owner review: required
  - Stale reviews: dismissed on new push
  - Review thread resolution: required
```

## 🔧 Configuration Examples

### Minimal (3 variables)
```hcl
module "simple" {
  source = "./modules/gold-standard-github-repository"
  
  repository_name  = "my-repo"
  parent_team_slug = "my-team"
  team_members     = [{ username = "alice", role = "maintainer" }]
}
```

### Production-Ready (Stricter controls)
```hcl
module "production" {
  source = "./modules/gold-standard-github-repository"
  
  repository_name = "critical-api"
  parent_team_slug = "platform-team"
  
  # Security hardening
  ruleset_required_signatures = true
  ruleset_required_linear_history = true
  ruleset_pull_request_required_approving_review_count = 3
  ruleset_pull_request_require_last_push_approval = true
  
  # Extended production environment
  environments = {
    nonproductive = {
      wait_timer = 0
      can_admins_bypass = true
    }
    prod = {
      wait_timer = 600  # 10 minutes
      can_admins_bypass = false
      prevent_self_review = true
      reviewers_users = ["alice", "bob", "charlie"]
    }
  }
  
  team_members = [
    { username = "platform-lead", role = "maintainer" }
  ]
}
```

### From Template
```hcl
module "from_template" {
  source = "./modules/gold-standard-github-repository"
  
  repository_name = "new-microservice"
  
  template_owner = "my-org"
  template_repository = "microservice-template"
  
  parent_team_slug = "backend-team"
}
```

### YAML-Driven (Scale to 100+ repos)
```hcl
locals {
  repo_configs = yamldecode(file("repos.yaml")).repositories
  repositories = { for repo in local.repo_configs : repo.name => repo }
}

module "yaml_repos" {
  source   = "./modules/gold-standard-github-repository"
  for_each = local.repositories
  
  repository_name  = each.value.name
  parent_team_slug = each.value.team
  team_members     = each.value.members
}
```

## 🧪 Testing

### Run All Tests
```bash
cd tests
go test -v -timeout 30m
```

### Run Specific Test
```bash
go test -v -timeout 30m -run TestBasicRepositoryCreation
```

### Available Tests
1. `TestBasicRepositoryCreation` - Core functionality
2. `TestRepositoryWithSecurityFeatures` - Security settings
3. `TestTeamCreationAndAccess` - Team management
4. `TestEnvironmentCreation` - Environment configuration
5. `TestRulesetConfiguration` - Ruleset validation
6. `TestPreflightValidationFailsForExistingRepo` - Validation logic
7. `TestCustomRoleCreation` - Custom role creation (Enterprise Cloud only)
8. `TestRepositoryFromTemplate` - Template usage
9. `TestIdempotency` - Repeated applies
10. `TestActionsDisabledByDefault` - Actions configuration

## 🔒 Security Features

### Automatic Hardening
- ✅ Private visibility by default
- ✅ Secret scanning enabled
- ✅ Push protection enabled
- ✅ Vulnerability alerts enabled
- ✅ GitHub Actions disabled (but workflow-accessible)
- ✅ Branch deletion on merge
- ✅ Minimum 2 required reviews
- ✅ Code owner review required
- ✅ Stale review dismissal

### Security Scanning (CI/CD)
- ✅ tfsec - Terraform security scanning
- ✅ Checkov - Infrastructure as code analysis
- ✅ Trivy - Comprehensive security scanner
- ✅ TFLint - Terraform linting
- ✅ Automated SARIF upload to GitHub Security

## 📚 Advanced Patterns

### Multi-Team Access
```hcl
module "shared_repo" {
  source = "./modules/gold-standard-github-repository"
  
  repository_name = "shared-platform-library"
  parent_team_slug = "platform-team"
  team_repository_permission = "admin"
}

# Grant additional teams different access
resource "github_team_repository" "backend_write" {
  team_id    = data.github_team.backend.id
  repository = module.shared_repo.repository.name
  permission = "push"
}

resource "github_team_repository" "frontend_read" {
  team_id    = data.github_team.frontend.id
  repository = module.shared_repo.repository.name
  permission = "pull"
}
```

### Repository Factory
```hcl
locals {
  repo_templates = {
    microservice = {
      topics = ["microservice", "api"]
      required_reviews = 2
      environments = { nonproductive = {}, prod = {} }
    }
    library = {
      topics = ["library", "shared"]
      required_reviews = 1
      environments = { nonproductive = {} }
    }
  }
  
  repos = {
    "payment-api" = { type = "microservice", team = "payment-team" }
    "auth-lib"    = { type = "library", team = "platform-team" }
  }
}

module "factory_repos" {
  source   = "./modules/gold-standard-github-repository"
  for_each = local.repos
  
  repository_name  = each.key
  repository_topics = local.repo_templates[each.value.type].topics
  parent_team_slug = each.value.team
  
  ruleset_pull_request_required_approving_review_count = 
    local.repo_templates[each.value.type].required_reviews
  
  environments = local.repo_templates[each.value.type].environments
}
```

### Compliance Configuration
```hcl
module "compliant_repo" {
  source = "./modules/gold-standard-github-repository"
  
  repository_name = "financial-reporting"
  parent_team_slug = "finance-team"
  
  # Maximum security
  security_and_analysis_advanced_security = "enabled"
  ruleset_required_signatures = true
  ruleset_required_linear_history = true
  ruleset_pull_request_required_approving_review_count = 3
  ruleset_pull_request_require_last_push_approval = true
  
  # Compliance metadata
  rest_api_body = jsonencode({
    custom_properties = {
      compliance_frameworks = ["SOX", "PCI-DSS", "GDPR"]
      data_classification = "confidential"
      audit_required = "true"
      retention_years = "7"
    }
  })
}
```

## 🔄 CI/CD Integration

### Module Validation (Automatic)
- ✅ Format checking on every PR
- ✅ Validation on every commit
- ✅ TFLint for best practices
- ✅ Security scanning with tfsec/Checkov/Trivy
- ✅ Auto-generated documentation
- ✅ Automated testing with Terratest

### Automated Releases
- ✅ Semantic versioning via tags
- ✅ Automated changelog generation
- ✅ GitHub releases with usage examples
- ✅ Terraform Registry publication ready

### Usage in Other Repos
```yaml
# .github/workflows/infrastructure.yml
name: Deploy Infrastructure
on: [push]
jobs:
  terraform:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: hashicorp/setup-terraform@v3
      - run: terraform init
      - run: terraform plan
      - run: terraform apply -auto-approve
        if: github.ref == 'refs/heads/main'
```

## 🐛 Troubleshooting

### Repository Already Exists
**Error**: `Repository 'my-repo' already exists`  
**Solution**: Choose different name or import existing repo
```bash
terraform import 'module.my_repo.github_repository.main' "my-repo"
```

### Parent Team Not Found
**Error**: `parent team not found`  
**Solutions**:
1. Verify team slug: `gh api orgs/my-org/teams | jq '.[] | .slug'`
2. Update `parent_team_slug` variable
3. Create parent team first

### Custom Role Creation Failed
**Error**: `Custom roles require GitHub Enterprise Cloud`  
**Solution**: Disable custom role creation
```hcl
create_custom_role = false
```

### Actions Configuration Issues
**Note**: Actions are disabled by default but accessible by workflows.  
This is intentional per gold-standard requirements.

## 📊 Module Outputs

```hcl
# Complete repository information
output "repository" {
  value = module.my_repo.repository
  # Returns: id, name, full_name, html_url, ssh_clone_url, etc.
}

# Team information
output "team" {
  value = module.my_repo.team
  # Returns: id, name, slug, description
}

# Custom role (if created)
output "custom_role" {
  value = module.my_repo.custom_role
  # Returns: id, name, base_role, permissions
}

# Environments
output "environments" {
  value = module.my_repo.environments
  # Returns: map of environment_name => { id, environment }
}

# Ruleset
output "ruleset" {
  value = module.my_repo.ruleset
  # Returns: id, node_id, name, enforcement
}
```

## 🎯 Key Design Decisions

### 1. Single-File Module
All resources in one `main.tf` for simplicity. For very large deployments, consider splitting by resource type.

### 2. Comprehensive Variables
150+ variables with sensible defaults. Override only what you need.

### 3. Pre-flight Validation
Module fails fast if repository exists, preventing confusing error messages.

### 4. Gold Standard Defaults
Security-first configuration out of the box:
- Private visibility
- Secret scanning enabled
- Actions disabled
- Branch protection via rulesets
- Minimum 2 required reviews

### 5. Team Hierarchy Support
Designed for parent/child team structures common in enterprises.

### 6. Environment-First Design
Two environments (`nonproductive`, `prod`) created by default, easily extensible.

### 7. REST API Integration
Custom metadata injection for compliance, tracking, and automation.

## 📈 Scaling Strategies

### Small Scale (1-10 repos)
Direct module calls with explicit configuration.

### Medium Scale (10-50 repos)
Use `for_each` with local variables or data sources.

### Large Scale (50+ repos)
YAML-driven configuration with validation scripts.

### Enterprise Scale (100+ repos)
Repository factory pattern with template-based configuration.

## 🔐 Authentication Best Practices

### Development
```bash
# Personal Access Token
export GITHUB_TOKEN="ghp_xxxxxxxxxxxx"
```

### Production
```bash
# GitHub App (Recommended)
export TF_VAR_github_app_id="123456"
export TF_VAR_github_app_installation_id="12345678"
export TF_VAR_github_app_pem_file="$(cat github-app.pem)"
```

### CI/CD
Store credentials in GitHub Secrets or secrets manager (Vault, AWS Secrets Manager).

## 📦 Publishing to Terraform Registry

1. Tag release:
```bash
git tag -a v1.0.0 -m "Initial release"
git push origin v1.0.0
```

2. Create GitHub release from tag

3. Registry auto-detects and publishes

4. Users reference:
```hcl
module "gold_standard_repo" {
  source  = "your-org/gold-standard-github-repository/github"
  version = "1.0.0"
  
  repository_name  = "my-repo"
  parent_team_slug = "my-team"
}
```

## 🎓 Learning Path

1. **Start**: QUICKSTART.md (5 minutes)
2. **Basics**: examples/simple/ (10 minutes)
3. **Production**: examples/complete/ (30 minutes)
4. **Advanced**: examples/advanced-patterns/ (1 hour)
5. **Testing**: tests/ (30 minutes)
6. **CI/CD**: .github/workflows/ (30 minutes)

## 🤝 Contributing

1. Fork the repository
2. Create feature branch
3. Add tests for new features
4. Ensure all CI checks pass
5. Submit pull request

## 📄 License

MIT License - See LICENSE file for details

## 🙏 Credits

Built for enterprise GitHub organization management with Terraform provider v6.6.

---

**You're ready to deploy gold-standard GitHub repositories!**

```bash
terraform init && terraform apply
```
