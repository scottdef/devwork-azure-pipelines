# Kubernetes Dashboard Deployment for AKS

Complete deployment guide and automation for Kubernetes Dashboard on Azure Kubernetes Service (AKS) private cluster.

**Target Environment:**
- **Cluster:** prod-cus-aks-sre-lab-003
- **Resource Group:** prod-cus-platform-base-rg-001
- **Subscription:** tango-CICD-platform-github-gitflow
- **ACR:** prod-central-image-repo

## 📋 Table of Contents

- [Quick Start](#quick-start)
- [Prerequisites](#prerequisites)
- [Deployment Methods](#deployment-methods)
- [Access Methods](#access-methods)
- [Usage Guide](#usage-guide)
- [Troubleshooting](#troubleshooting)
- [Contributing](#contributing)

## 🚀 Quick Start

### Ubuntu 24.04

```bash
# Clone repository
git clone https://github.com/your-org/kubernetes-dashboard-aks.git
cd kubernetes-dashboard-aks

# Install prerequisites
make install-ubuntu

# Connect to AKS cluster
make connect

# Deploy with Helm (recommended)
make deploy-helm

# Start port-forward and access dashboard
make port-forward

# In another terminal, generate admin token
make token
```

### Windows 11

```powershell
# Clone repository
git clone https://github.com/your-org/kubernetes-dashboard-aks.git
cd kubernetes-dashboard-aks

# Install prerequisites
.\scripts\windows\01-prerequisites.ps1

# Connect to AKS cluster
.\scripts\windows\02-aks-connect.ps1

# Deploy with Helm
.\scripts\windows\04-deploy-helm.ps1

# Start port-forward
.\scripts\windows\05-setup-access.ps1

# Generate admin token
kubectl create token dashboard-admin -n kubernetes-dashboard --duration=24h
```

## 📦 Prerequisites

### Ubuntu 24.04 Requirements

| Tool | Version | Installation |
|------|---------|-------------|
| Azure CLI | latest | `curl -sL https://aka.ms/InstallAzureCLIDeb \| sudo bash` |
| kubectl | 1.30.x | `az aks install-cli` |
| kubelogin | latest | `az aks install-cli` |
| Helm | 3.13+ | `curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 \| bash` |
| Docker | latest | `sudo apt install docker.io` |
| Make | latest | `sudo apt install build-essential` |

### Windows 11 Requirements

| Tool | Version | Installation |
|------|---------|-------------|
| Azure CLI | latest | Download from https://aka.ms/installazurecliwindows |
| kubectl | 1.30.x | `az aks install-cli` |
| kubelogin | latest | `az aks install-cli` |
| Helm | 3.13+ | Manual download or `choco install kubernetes-helm` |
| Git | latest | Pre-installed |
| PowerShell | 5.1+ | Pre-installed |

### Automated Installation

```bash
# Ubuntu
make install-ubuntu

# Windows (run in PowerShell)
.\scripts\windows\01-prerequisites.ps1
```

## 🎯 Deployment Methods

### Method 1: Helm Chart (Recommended)

**Development:**
```bash
make deploy-helm
```

**Production:**
```bash
make deploy-helm-prod
```

**Production with ACR Images:**
```bash
make deploy-helm-acr
```

### Method 2: Pure Kubernetes Manifests

```bash
make deploy-manifests
```

### Method 3: Terraform

```bash
# Configure variables
cp terraform/terraform.tfvars.example terraform/terraform.tfvars
# Edit terraform.tfvars with your values

# Deploy
make deploy-terraform
```

### Method 4: GitHub Actions

Push to main branch or manually trigger workflow:
```bash
# Via GitHub UI: Actions → Deploy Kubernetes Dashboard → Run workflow
# Or via gh CLI:
gh workflow run deploy-dashboard.yml -f environment=prod -f deployment_method=helm
```

## 🔐 Access Methods

### Method 1: Port Forward (Recommended for Private Clusters)

```bash
# Start port-forward
make port-forward

# Access at: https://localhost:8443
```

**Background mode:**
```bash
make port-forward-bg  # Start in background
make stop-port-forward  # Stop
```

### Method 2: NodePort (Internal Network Access)

```bash
make nodeport
# Access at: https://<node-ip>:30443
```

### Method 3: Internal Load Balancer (VNet Access)

```bash
make loadbalancer
# Access at: https://<private-ip>
```

### Method 4: Istio Ingress Gateway

```bash
# Apply Istio configuration
kubectl apply -f manifests/istio-ingress.yaml

# Access via Istio ingress gateway
# https://dashboard.internal.example.com
```

## 🔑 Authentication

### Generate Admin Token

```bash
# 24-hour token
make token

# Or manually
kubectl create token dashboard-admin -n kubernetes-dashboard --duration=24h
```

### Generate Read-Only Token

```bash
# 24-hour token
make token-readonly

# Or manually
kubectl create token dashboard-readonly -n kubernetes-dashboard --duration=24h
```

### Token Usage

1. Access dashboard URL
2. Select "Token" authentication method
3. Paste the generated token
4. Click "Sign In"

## 📖 Usage Guide

### Makefile Commands

```bash
make help  # Show all available commands
```

**Common Commands:**
- `make connect` - Connect to AKS cluster
- `make deploy-helm` - Deploy with Helm
- `make port-forward` - Start port-forward
- `make token` - Generate admin token
- `make status` - Show deployment status
- `make logs` - View dashboard logs
- `make verify` - Verify deployment
- `make clean` - Remove deployment

### Script-Based Deployment (Ubuntu)

```bash
# 1. Prerequisites
./scripts/ubuntu/01-prerequisites.sh

# 2. Connect to cluster
./scripts/ubuntu/02-aks-connect.sh

# 3. Deploy (choose one)
./scripts/ubuntu/03-deploy-manifests.sh  # Pure manifests
./scripts/ubuntu/04-deploy-helm.sh       # Helm

# 4. Setup access
./scripts/ubuntu/05-setup-access.sh port-forward

# 5. Verify
./scripts/ubuntu/verify-deployment.sh
```

### Script-Based Deployment (Windows)

```powershell
# 1. Prerequisites
.\scripts\windows\01-prerequisites.ps1

# 2. Connect to cluster
.\scripts\windows\02-aks-connect.ps1

# 3. Deploy (choose one)
.\scripts\windows\03-deploy-manifests.ps1  # Pure manifests
.\scripts\windows\04-deploy-helm.ps1       # Helm

# 4. Setup access
.\scripts\windows\05-setup-access.ps1 -AccessMethod port-forward

# 5. Verify
.\scripts\windows\verify-deployment.ps1
```

## 🎨 Customization

### Helm Values

**Development (`helm/values-dev.yaml`):**
- 1 replica
- Lower resource limits
- Docker Hub images

**Production (`helm/values-prod.yaml`):**
- 2+ replicas
- Higher resource limits
- Pod disruption budgets
- Anti-affinity rules

**ACR Images (`helm/values-acr.yaml`):**
- Use images from Azure Container Registry
- Requires ACR pull secret

### Combining Values Files

```bash
helm upgrade --install kubernetes-dashboard kubernetes-dashboard/kubernetes-dashboard \
  --values helm/values-prod.yaml \
  --values helm/values-acr.yaml
```

### Environment Variables

Set in scripts or export:
```bash
export NAMESPACE="kubernetes-dashboard"
export CLUSTER_NAME="prod-cus-aks-sre-lab-003"
export RESOURCE_GROUP="prod-cus-platform-base-rg-001"
```

## 🐳 Working with ACR

### Import Official Images

```bash
make acr-import
```

### Build Custom Image

```bash
make docker-build
make docker-push
```

### Create ACR Pull Secret

```bash
make acr-secret
```

## 🔍 Verification and Monitoring

### Check Deployment Status

```bash
make status
```

### View Logs

```bash
make logs           # Dashboard logs
make logs-all       # All component logs
```

### View Events

```bash
make events
```

### Run Full Verification

```bash
make verify
```

## 🛠️ Troubleshooting

### Connection Issues

**Problem:** Cannot connect to cluster
```bash
# Solution: Verify authentication
az account show
az aks get-credentials --resource-group prod-cus-platform-base-rg-001 --name prod-cus-aks-sre-lab-003
kubelogin convert-kubeconfig -l azurecli
kubectl get nodes
```

**Problem:** kubelogin authentication fails
```bash
# Solution: Use device code flow
kubelogin convert-kubeconfig -l devicecode
```

### Pod Issues

**Problem:** Pods not starting
```bash
# Check pod status
kubectl get pods -n kubernetes-dashboard
kubectl describe pod <pod-name> -n kubernetes-dashboard
kubectl logs <pod-name> -n kubernetes-dashboard
```

**Problem:** Image pull errors
```bash
# Verify ACR access
az acr login --name prod-central-image-repo

# Recreate ACR secret
make acr-secret
```

### Access Issues

**Problem:** Port-forward fails
```bash
# Kill existing port-forwards
make stop-port-forward

# Try specific service
kubectl get svc -n kubernetes-dashboard
kubectl port-forward -n kubernetes-dashboard svc/<service-name> 8443:443
```

**Problem:** Token expired
```bash
# Generate new token
make token
```

### Helm Issues

**Problem:** Helm release failed
```bash
# Check release status
helm status kubernetes-dashboard -n kubernetes-dashboard

# View history
helm history kubernetes-dashboard -n kubernetes-dashboard

# Rollback
helm rollback kubernetes-dashboard <revision> -n kubernetes-dashboard
```

## 📁 Repository Structure

```
.
├── manifests/              # Pure Kubernetes manifests
│   ├── namespace.yaml
│   ├── serviceaccount.yaml
│   ├── rbac.yaml
│   ├── deployment.yaml
│   ├── service.yaml
│   ├── network-policy.yaml
│   └── istio-ingress.yaml
├── helm/                   # Helm values files
│   ├── values-dev.yaml
│   ├── values-prod.yaml
│   └── values-acr.yaml
├── scripts/                # Deployment scripts
│   ├── ubuntu/            # Ubuntu 24.04 scripts
│   └── windows/           # Windows 11 PowerShell scripts
├── terraform/              # Terraform configuration
│   ├── main.tf
│   ├── variables.tf
│   ├── outputs.tf
│   └── terraform.tfvars.example
├── docker/                 # Custom Docker image
│   ├── Dockerfile
│   └── .dockerignore
├── .github/
│   └── workflows/         # GitHub Actions workflows
│       ├── deploy-dashboard.yml
│       └── build-custom-image.yml
├── Makefile               # Make commands
└── README.md              # This file
```

## 🔒 Security Considerations

### Production Checklist

- [ ] Use short-lived tokens (24 hours maximum)
- [ ] Implement namespace-scoped RBAC instead of cluster-admin
- [ ] Enable network policies
- [ ] Use internal load balancer or port-forward only
- [ ] Enable Istio AuthorizationPolicy for IP restrictions
- [ ] Rotate service accounts periodically
- [ ] Enable audit logging
- [ ] Use ACR with private endpoints
- [ ] Implement Pod Security Standards
- [ ] Review all ClusterRoleBindings

### RBAC Best Practices

```bash
# Use read-only access by default
make token-readonly

# Grant admin access only when necessary
make token  # Admin token
```

## 🚀 CI/CD Integration

### GitHub Actions

Workflows are automatically triggered on:
- Push to `main` branch
- Pull requests
- Manual workflow dispatch

### Manual Trigger

```bash
gh workflow run deploy-dashboard.yml \
  -f environment=prod \
  -f deployment_method=helm \
  -f use_acr=true
```

## 📝 License

Copyright © 2024 Platform Engineering Team

## 🤝 Contributing

1. Fork the repository
2. Create a feature branch
3. Commit your changes
4. Push to the branch
5. Create a Pull Request

## 📞 Support

For issues and questions:
- GitHub Issues: https://github.com/your-org/kubernetes-dashboard-aks/issues
- Platform Engineering Team: platform-engineering@example.com

## 🔗 Useful Links

- [Kubernetes Dashboard Documentation](https://kubernetes.io/docs/tasks/access-application-cluster/web-ui-dashboard/)
- [Helm Chart Repository](https://github.com/kubernetes/dashboard/tree/master/charts/kubernetes-dashboard)
- [Azure AKS Documentation](https://learn.microsoft.com/en-us/azure/aks/)
- [kubelogin Documentation](https://azure.github.io/kubelogin/)

---

**Maintained by:** Platform Engineering Team  
**Last Updated:** December 2024  
**Version:** 1.0.0
