# GitHub Actions Workflows Documentation

Complete guide to automated Kubernetes Dashboard deployment workflows for AKS.

## 📋 Table of Contents

- [Overview](#overview)
- [Prerequisites](#prerequisites)
- [Workflow Catalog](#workflow-catalog)
- [Quick Start](#quick-start)
- [Deployment Workflows](#deployment-workflows)
- [Operations Workflows](#operations-workflows)
- [Security Workflows](#security-workflows)
- [Configuration](#configuration)
- [Best Practices](#best-practices)
- [Troubleshooting](#troubleshooting)

---

## Overview

This repository contains 8 GitHub Actions workflows for deploying and managing Kubernetes Dashboard on Azure Kubernetes Service (AKS):

| Workflow | Purpose | Complexity | Use Case |
|----------|---------|------------|----------|
| **deploy-simple.yml** | One-click deployment | Low | Quick manual deployments |
| **deploy-bash.yml** | Bash script-based | Low | Simple automated deployments |
| **deploy-multi-env.yml** | Progressive deployment | Medium | Dev → Staging → Prod |
| **deploy-terraform.yml** | Basic Terraform | Medium | IaC deployments |
| **deploy-terraform-optimized.yml** | Advanced Terraform | High | Production IaC with caching |
| **rollback.yml** | Rollback deployment | Low | Emergency recovery |
| **security-token-rotation.yml** | Security automation | Medium | Token rotation, RBAC audits |
| **operations.yml** | Maintenance tasks | Medium | Health checks, backups, scaling |

---

## Prerequisites

### Required GitHub Secrets

Configure these secrets in your repository (Settings → Secrets and variables → Actions):

```
AZURE_CLIENT_ID          # Azure AD Application (Client) ID
AZURE_TENANT_ID          # Azure AD Tenant ID
AZURE_SUBSCRIPTION_ID    # Azure Subscription ID
AZURE_CLIENT_SECRET      # Azure AD Client Secret (for some workflows)
```

### Azure Setup

1. **Create Azure AD Application**
   ```bash
   az ad sp create-for-rbac --name "github-actions-dashboard" \
     --role contributor \
     --scopes /subscriptions/{subscription-id}/resourceGroups/prod-cus-platform-base-rg-001
   ```

2. **Configure OIDC (Recommended)**
   ```bash
   az ad app federated-credential create \
     --id {app-id} \
     --parameters credential.json
   ```

3. **Grant AKS Permissions**
   ```bash
   az role assignment create \
     --assignee {client-id} \
     --role "Azure Kubernetes Service Cluster User Role" \
     --scope /subscriptions/{subscription-id}/resourceGroups/prod-cus-platform-base-rg-001/providers/Microsoft.ContainerService/managedClusters/prod-cus-aks-sre-lab-003
   ```

### GitHub Environment Setup

Create environments in GitHub (Settings → Environments):

- **dev** - No approval required
- **staging** - Optional approval
- **prod** - Required approval (recommended reviewers: 2+)

---

## Workflow Catalog

### 1. deploy-simple.yml - 🚀 Quick Deploy Dashboard

**Purpose:** Fastest way to deploy dashboard with minimal configuration.

**Trigger:** Manual (workflow_dispatch)

**Use When:**
- Quick development deployments
- Testing configuration changes
- Ad-hoc deployments

**Usage:**
```
Actions tab → Quick Deploy Dashboard → Run workflow
  Environment: dev/staging/prod
  Method: helm/manifests
```

**Features:**
- Single-click deployment
- Auto-generates admin token
- Displays access instructions
- No approval required

---

### 2. deploy-bash.yml - Deploy Dashboard (Bash Scripts)

**Purpose:** Production deployment using bash scripts with validation.

**Triggers:**
- Push to main (manifests/, helm/, scripts/)
- Pull requests
- Manual dispatch

**Use When:**
- Production deployments
- CI/CD pipelines
- Multiple environments

**Features:**
- Uses existing bash scripts
- Helm or manifest deployment
- Token generation and Key Vault storage
- Post-deployment validation
- API health checks

**Example:**
```yaml
# Manual trigger
Environment: prod
Method: helm
Create token: true
```

---

### 3. deploy-multi-env.yml - 🔄 Multi-Environment Deploy

**Purpose:** Progressive deployment through multiple environments with gates.

**Trigger:** 
- Push to main
- Manual dispatch

**Use When:**
- Production releases
- Progressive rollouts
- Safety-first deployments

**Pipeline:**
```
Dev → Staging → Production
 ↓       ↓          ↓
Auto   Auto    Approval Required
```

**Features:**
- Automated dev deployment
- Automated staging deployment  
- Manual approval for production
- Backup before production deploy
- Smoke tests at each stage
- Skip environments option

---

### 4. deploy-terraform.yml - Deploy Dashboard (Terraform)

**Purpose:** Basic infrastructure-as-code deployment.

**Triggers:**
- Push to main (terraform/)
- Pull requests (terraform/)
- Manual dispatch

**Use When:**
- Managing infrastructure as code
- Tracking state in Azure Storage
- Team-based deployments

**Features:**
- Terraform plan on PR
- Auto-apply on main branch
- State stored in Azure Blob
- Outputs saved as artifacts

---

### 5. deploy-terraform-optimized.yml - 🏗️ Terraform Deploy (Optimized)

**Purpose:** Production-grade Terraform with caching and optimizations.

**Triggers:**
- Push to main (terraform/, helm/)
- Pull requests (terraform/)
- Manual dispatch

**Use When:**
- Large-scale deployments
- Multiple simultaneous deployments
- Cost optimization needed

**Features:**
- Terraform caching (faster runs)
- Separate plan/apply jobs
- PR plan comments
- Environment-specific tfvars
- Destroy capability
- Output artifacts with 30-day retention

**Efficiency Gains:**
- 40-60% faster Terraform init (cached)
- Parallel plan/apply possible
- Reusable artifacts

**Workflow:**
```
1. Plan → Cache → Upload artifact
2. Approval (if prod)
3. Apply → Download cached plan
4. Verify → Generate outputs
```

---

### 6. rollback.yml - ⏮️ Rollback Dashboard

**Purpose:** Emergency rollback to previous working state.

**Trigger:** Manual (workflow_dispatch)

**Use When:**
- Deployment failure
- Service degradation
- Configuration issues
- Emergency recovery

**Rollback Methods:**

**A. Helm History (Recommended)**
```
Rolls back to previous Helm revision
Fast and safe
Preserves history
```

**B. Specific Revision**
```
Rollback to specific revision number
View history: helm history kubernetes-dashboard -n kubernetes-dashboard
```

**C. Backup Restore**
```
Restore from previous workflow backup
Manual artifact download
Full state restoration
```

**Safety Features:**
- Requires "ROLLBACK" confirmation
- Environment approval required
- Pre-rollback backup created
- Post-rollback health checks
- Artifact retention (30 days)

**Usage:**
```
Actions → Rollback Dashboard → Run workflow
  Environment: prod
  Rollback type: helm-history
  Confirm: ROLLBACK
```

---

### 7. security-token-rotation.yml - 🔐 Security & Token Rotation

**Purpose:** Automated security operations and compliance checks.

**Triggers:**
- Daily schedule (2 AM UTC)
- Manual dispatch

**Use When:**
- Token expiration (24h)
- Security audits
- Compliance checks
- RBAC reviews

**Operations:**

**Token Rotation:**
- Generates new 24h tokens
- Stores in Azure Key Vault
- Rotates: dashboard-admin, dashboard-readonly
- Matrix job (parallel rotation)

**RBAC Audit:**
- Lists service accounts
- Checks ClusterRoleBindings
- Identifies excessive permissions
- Generates audit report

**Security Scan:**
- Kubesec manifest scanning
- Pod Security Standards check
- Network policy validation
- Resource limits verification

**Compliance Check:**
- Label validation
- TLS configuration check
- PodDisruptionBudget check
- ResourceQuota validation

**Features:**
- Automated daily rotation
- Azure Key Vault integration
- Comprehensive reports
- Security event logging

---

### 8. operations.yml - 🔧 Operations & Maintenance

**Purpose:** Common operational tasks and maintenance.

**Trigger:** Manual (workflow_dispatch)

**Operations:**

**A. Health Check**
```bash
# Checks:
- Cluster connectivity
- Pod health status
- Service endpoints
- Resource usage
- Recent events
```

**B. Backup**
```bash
# Backs up:
- Helm values and manifests
- Kubernetes resources
- Secrets and ConfigMaps
- Service accounts
- RBAC bindings

# Retention: 90 days
```

**C. Restore**
```bash
# Restores from:
- Specified backup run number
- Downloads artifact
- Applies resources
- Verifies deployment
```

**D. Scale**
```bash
# Scales deployments to specified replicas
# Example: Scale to 3 replicas for high traffic
```

**E. Restart**
```bash
# Restarts all deployments
# Useful for: config changes, pod issues
```

**F. Cleanup**
```bash
# Removes:
- Failed pods
- Evicted pods
- Completed jobs
```

**G. Upgrade**
```bash
# Upgrades dashboard to latest version
# Uses existing values files
# Atomic rollback on failure
```

**Usage Examples:**
```
# Health check
Operation: health-check
Environment: prod

# Backup before changes
Operation: backup
Environment: prod

# Scale for high traffic
Operation: scale
Environment: prod
Replicas: 5

# Restore after issue
Operation: restore
Environment: prod
Backup run number: 123
```

---

## Quick Start

### First-Time Setup

1. **Configure Secrets**
   ```
   Repository → Settings → Secrets and variables → Actions → New repository secret
   
   Add:
   - AZURE_CLIENT_ID
   - AZURE_TENANT_ID
   - AZURE_SUBSCRIPTION_ID
   ```

2. **Create Environments**
   ```
   Repository → Settings → Environments → New environment
   
   Create:
   - dev (no approval)
   - staging (optional approval)
   - prod (required approval, 2 reviewers)
   ```

3. **Test Deployment**
   ```
   Actions → Quick Deploy Dashboard → Run workflow
   - Environment: dev
   - Method: helm
   ```

### Daily Development Workflow

```bash
# 1. Make changes to manifests/helm
git checkout -b feature/update-config
# Edit files
git commit -am "Update dashboard config"
git push origin feature/update-config

# 2. Create PR
# → deploy-bash.yml runs validation
# → Review plan in PR comments

# 3. Merge to main
# → deploy-multi-env.yml runs
# → Auto-deploys to dev
# → Auto-deploys to staging
# → Waits for approval for prod

# 4. Approve production deployment
# → Deploys to prod
# → Backup created automatically
```

### Production Release Workflow

```bash
# 1. Backup current state
Actions → Operations → Run workflow
  Operation: backup
  Environment: prod

# 2. Deploy using Terraform (recommended for prod)
Actions → Terraform Deploy (Optimized) → Run workflow
  Action: plan
  Environment: prod
# Review plan

# 3. Apply after approval
Actions → Terraform Deploy (Optimized) → Run workflow
  Action: apply
  Environment: prod

# 4. Verify deployment
Actions → Operations → Run workflow
  Operation: health-check
  Environment: prod

# 5. If issues occur, rollback
Actions → Rollback Dashboard → Run workflow
  Environment: prod
  Rollback type: helm-history
  Confirm: ROLLBACK
```

---

## Configuration

### Environment Variables

Each workflow uses these default values:

```yaml
env:
  AKS_CLUSTER: prod-cus-aks-sre-lab-003
  AKS_RG: prod-cus-platform-base-rg-001
  NAMESPACE: kubernetes-dashboard
  SUBSCRIPTION: tango-CICD-platform-github-gitflow
```

**To customize:** Edit workflow files or use repository variables.

### Terraform Backend

Terraform workflows use Azure Blob Storage for state:

```hcl
backend "azurerm" {
  resource_group_name  = "prod-cus-platform-terraform-rg-001"
  storage_account_name = "prodcusterraformst001"
  container_name       = "tfstate"
  key                  = "dashboard-{environment}.tfstate"
}
```

**Setup:**
```bash
# Create backend resources
az group create -n prod-cus-platform-terraform-rg-001 -l centralus
az storage account create -n prodcusterraformst001 -g prod-cus-platform-terraform-rg-001 --sku Standard_LRS
az storage container create -n tfstate --account-name prodcusterraformst001
```

### Helm Values

Workflows automatically select values files:

```
dev      → helm/values-dev.yaml
staging  → helm/values-dev.yaml (with overrides)
prod     → helm/values-prod.yaml
```

---

## Best Practices

### 1. Use Appropriate Workflow

| Scenario | Recommended Workflow |
|----------|---------------------|
| Quick test | deploy-simple.yml |
| Dev deployment | deploy-bash.yml |
| Prod deployment | deploy-terraform-optimized.yml |
| Emergency fix | deploy-simple.yml + rollback.yml |
| Regular maintenance | operations.yml |
| Security audit | security-token-rotation.yml |

### 2. Environment Strategy

```
Development:
- Use deploy-simple.yml or deploy-bash.yml
- No approval gates
- Rapid iteration

Staging:
- Use deploy-multi-env.yml
- Optional approval
- Integration testing

Production:
- Use deploy-terraform-optimized.yml
- Required approval (2+ reviewers)
- Backup before deployment
- Gradual rollout
```

### 3. Backup Strategy

```bash
# Before major changes
1. Run backup operation
2. Note backup run number
3. Proceed with changes
4. If issues, restore from backup

# Automated backups
- Production: Before every deployment (deploy-multi-env.yml)
- Retention: 90 days (operations.yml backup)
- Retention: 30 days (deployment backups)
```

### 4. Security Practices

```
✅ Rotate tokens daily (security-token-rotation.yml schedule)
✅ Use environment approvals for production
✅ Run security scans before production deploys
✅ Audit RBAC monthly
✅ Store tokens in Azure Key Vault
✅ Use short-lived tokens (24h max)
✅ Review workflow logs regularly
```

### 5. Monitoring

```bash
# Set up monitoring for:
- Workflow failures → Slack/Teams notification
- Deployment duration → Alert if >10min
- Rollback frequency → Alert if >2/week
- Security scan findings → Create issues

# GitHub Actions Status Badge
Add to README.md:
![Deploy](https://github.com/{owner}/{repo}/actions/workflows/deploy-terraform-optimized.yml/badge.svg)
```

---

## Troubleshooting

### Common Issues

**❌ Azure Login Failed**
```
Error: AADSTS700016: Application not found

Solution:
1. Verify AZURE_CLIENT_ID is correct
2. Check service principal exists: az ad sp show --id {client-id}
3. Verify federated credentials configured
```

**❌ kubectl Connection Timeout**
```
Error: Unable to connect to cluster

Solution:
1. Check AKS firewall rules
2. Verify service principal has AKS User role
3. Check cluster is running: az aks show -g {rg} -n {cluster}
```

**❌ Helm Release Failed**
```
Error: release failed: timed out waiting for the condition

Solution:
1. Check pod logs: kubectl logs -n kubernetes-dashboard -l app.kubernetes.io/instance=kubernetes-dashboard
2. Check events: kubectl get events -n kubernetes-dashboard --sort-by='.lastTimestamp'
3. Rollback: Use rollback.yml workflow
```

**❌ Token Generation Failed**
```
Error: service account not found

Solution:
1. Check service account exists: kubectl get sa -n kubernetes-dashboard
2. Verify RBAC bindings: kubectl get clusterrolebindings | grep dashboard
3. Recreate: kubectl apply -f manifests/serviceaccount.yaml
```

**❌ Terraform State Lock**
```
Error: state lock held by another process

Solution:
1. Check for running workflows
2. Force unlock (caution): terraform force-unlock {lock-id}
3. Wait 30 minutes for automatic timeout
```

### Debugging Workflows

**View workflow logs:**
```
Actions tab → Select workflow → Select run → View logs
```

**Download artifacts:**
```
Actions tab → Select run → Artifacts section → Download
```

**Re-run failed workflow:**
```
Actions tab → Select failed run → Re-run failed jobs
```

**Debug with tmate (SSH into runner):**
```yaml
# Add step to any workflow
- name: Debug
  uses: mxschmitt/action-tmate@v3
  if: failure()
```

---

## Workflow Decision Tree

```
Need to deploy?
├─ Quick test/dev? → deploy-simple.yml
├─ Production? 
│  ├─ First time? → deploy-bash.yml
│  ├─ Regular deployment? → deploy-terraform-optimized.yml
│  └─ Multi-environment? → deploy-multi-env.yml
└─ Issues occurred?
   ├─ Rollback needed? → rollback.yml
   ├─ Investigate? → operations.yml (health-check)
   └─ Maintenance? → operations.yml (backup/scale/restart)

Security tasks?
├─ Token expired? → security-token-rotation.yml
├─ Audit needed? → security-token-rotation.yml
└─ Compliance check? → security-token-rotation.yml

Regular maintenance?
└─ Daily/weekly tasks → operations.yml
```

---

## Additional Resources

### Related Documentation
- [Main README](../README.md) - Project overview
- [Platform Engineering Guide](../PLATFORM_ENGINEERING_GUIDE.md) - Enterprise patterns
- [Quick Reference](../QUICK_REFERENCE.md) - Command cheat sheet

### External Links
- [GitHub Actions Documentation](https://docs.github.com/en/actions)
- [Azure AD Workload Identity](https://learn.microsoft.com/en-us/azure/aks/workload-identity-overview)
- [Helm](https://helm.sh/docs/)
- [Terraform AzureRM Provider](https://registry.terraform.io/providers/hashicorp/azurerm/latest/docs)

---

## Summary

This workflow suite provides:

✅ **8 comprehensive workflows** covering all deployment scenarios  
✅ **Progressive deployment** with safety gates  
✅ **Efficient Terraform** with caching and optimization  
✅ **Emergency rollback** capabilities  
✅ **Automated security** with token rotation  
✅ **Complete operations** suite for maintenance  
✅ **Production-ready** with approvals and backups  

**Choose your workflow based on your needs:**
- **Speed**: deploy-simple.yml
- **Production**: deploy-terraform-optimized.yml  
- **Safety**: deploy-multi-env.yml
- **Recovery**: rollback.yml
- **Security**: security-token-rotation.yml
- **Maintenance**: operations.yml

All workflows follow Kubernetes and AKS best practices! 🚀
