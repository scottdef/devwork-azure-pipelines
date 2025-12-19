# Kubernetes Dashboard Deployment Package - Complete Summary

## 📦 Package Contents

This complete deployment package contains everything needed to deploy Kubernetes Dashboard v12 (Grafana OSS compatible) on Azure Kubernetes Service (AKS).

**Target Cluster:** prod-cus-aks-sre-lab-003  
**Resource Group:** prod-cus-platform-base-rg-001  
**Subscription:** tango-CICD-platform-github-gitflow  
**Software Versions:** kubectl 1.30, Grafana OSS 12, Go 1.21

---

## 📁 File Structure

```
kubernetes-dashboard-aks/
├── README.md                          # Main documentation
├── QUICK_REFERENCE.md                 # Quick command reference card
├── PLATFORM_ENGINEERING_GUIDE.md     # Comprehensive platform engineering guide
├── k8s-dashboard-deployment-guide.md  # Detailed deployment instructions
├── Makefile                           # Automated commands
│
├── manifests/                         # Pure Kubernetes manifests
│   ├── namespace.yaml
│   ├── serviceaccount.yaml
│   ├── rbac.yaml
│   ├── deployment.yaml
│   ├── service.yaml
│   ├── network-policy.yaml
│   └── istio-ingress.yaml
│
├── helm/                              # Helm values files
│   ├── values-dev.yaml               # Development configuration
│   ├── values-prod.yaml              # Production configuration
│   └── values-acr.yaml               # ACR images configuration
│
├── scripts/
│   ├── ubuntu/                       # Ubuntu 24.04 bash scripts
│   │   ├── 01-prerequisites.sh
│   │   ├── 02-aks-connect.sh
│   │   ├── 03-deploy-manifests.sh
│   │   ├── 04-deploy-helm.sh
│   │   ├── 05-setup-access.sh
│   │   └── verify-deployment.sh
│   │
│   └── windows/                      # Windows 11 PowerShell scripts
│       ├── 01-prerequisites.ps1
│       ├── 02-aks-connect.ps1
│       ├── 03-deploy-manifests.ps1
│       ├── 04-deploy-helm.ps1
│       ├── 05-setup-access.ps1
│       └── verify-deployment.ps1
│
├── terraform/                        # Infrastructure as Code
│   ├── main.tf
│   ├── variables.tf
│   ├── outputs.tf
│   └── terraform.tfvars.example
│
├── docker/                           # Custom image building
│   ├── Dockerfile
│   └── .dockerignore
│
└── .github/
    └── workflows/                    # CI/CD pipelines
        ├── deploy-dashboard.yml
        └── build-custom-image.yml
```

---

## 🚀 Quick Start Guide

### Option 1: Ubuntu 24.04 (Azure VM)

```bash
# 1. Install prerequisites
bash scripts/ubuntu/01-prerequisites.sh

# 2. Connect to AKS
bash scripts/ubuntu/02-aks-connect.sh

# 3. Deploy (choose one)
bash scripts/ubuntu/04-deploy-helm.sh           # Recommended
# OR
bash scripts/ubuntu/03-deploy-manifests.sh      # Alternative

# 4. Setup access
bash scripts/ubuntu/05-setup-access.sh port-forward

# 5. Get admin token
kubectl create token dashboard-admin -n kubernetes-dashboard --duration=24h

# 6. Access at https://localhost:8443
```

### Option 2: Windows 11 (Azure Virtual Desktop)

```powershell
# 1. Install prerequisites (run in PowerShell)
.\scripts\windows\01-prerequisites.ps1

# 2. Connect to AKS
.\scripts\windows\02-aks-connect.ps1

# 3. Deploy (choose one)
.\scripts\windows\04-deploy-helm.ps1           # Recommended
# OR
.\scripts\windows\03-deploy-manifests.ps1      # Alternative

# 4. Setup access
.\scripts\windows\05-setup-access.ps1 -Method port-forward

# 5. Access at https://localhost:8443 with the generated token
```

### Option 3: Using Makefile (Ubuntu)

```bash
# Complete automated deployment
make install-ubuntu
make connect
make deploy-helm
make port-forward  # In terminal 1
make token         # In terminal 2
```

### Option 4: Terraform

```bash
# Configure variables
cp terraform/terraform.tfvars.example terraform/terraform.tfvars
# Edit terraform.tfvars with your values

# Deploy infrastructure
cd terraform
terraform init
terraform plan
terraform apply

# Get outputs
terraform output -json
```

---

## 📋 Prerequisites Checklist

### Ubuntu 24.04 Requirements

- [x] Azure CLI (latest)
- [x] kubectl 1.30.x
- [x] kubelogin (latest)
- [x] Helm 3.13+
- [x] Docker (optional, for custom images)
- [x] Git
- [x] Make
- [x] jq

**Installation:** Run `scripts/ubuntu/01-prerequisites.sh`

### Windows 11 Requirements

- [x] Azure CLI (latest)
- [x] kubectl 1.30.x
- [x] kubelogin (latest)
- [x] Helm 3.13+
- [x] PowerShell 5.1+
- [x] Git
- [x] VS Code (recommended)

**Installation:** Run `scripts\windows\01-prerequisites.ps1`

---

## 🎯 Deployment Methods Comparison

| Method | Complexity | Flexibility | Best For |
|--------|-----------|-------------|----------|
| **Helm** | Low | High | Production, standard deployments |
| **Manifests** | Medium | Very High | Custom configurations, learning |
| **Terraform** | Medium | High | Infrastructure as Code, GitOps |
| **Makefile** | Low | Medium | Quick deployments, automation |
| **GitHub Actions** | Low | High | CI/CD, automated deployments |

---

## 🔐 Access Methods

### 1. Port Forward (Recommended for Private Clusters)

**Ubuntu:**
```bash
kubectl port-forward -n kubernetes-dashboard svc/kubernetes-dashboard-kong-proxy 8443:443
# Access: https://localhost:8443
```

**Windows:**
```powershell
kubectl port-forward -n kubernetes-dashboard svc/kubernetes-dashboard-kong-proxy 8443:443
# Access: https://localhost:8443
```

**Makefile:**
```bash
make port-forward
```

### 2. NodePort (Internal Network)

```bash
make nodeport
# Access: https://<node-ip>:30443
```

### 3. Internal Load Balancer (VNet Access)

```bash
make loadbalancer
# Access: https://<private-ip>
```

### 4. Istio Gateway (Advanced)

```bash
kubectl apply -f manifests/istio-ingress.yaml
# Access via configured hostname
```

---

## 🔑 Authentication

### Generate Admin Token (24 hours)

**Ubuntu:**
```bash
kubectl create token dashboard-admin -n kubernetes-dashboard --duration=24h
```

**Windows:**
```powershell
kubectl create token dashboard-admin -n kubernetes-dashboard --duration=24h
```

**Makefile:**
```bash
make token
```

### Generate Read-Only Token

```bash
make token-readonly
```

### Token Usage

1. Access dashboard URL (https://localhost:8443)
2. Select "Token" authentication
3. Paste generated token
4. Click "Sign In"

---

## 🐳 Working with Azure Container Registry

### Import Official Images

**Ubuntu:**
```bash
make acr-import
```

**Manual:**
```bash
az acr import --name prod-central-image-repo \
  --source docker.io/kubernetesui/dashboard-api:1.14.0 \
  --image kubernetes-dashboard/dashboard-api:1.14.0
```

### Create ACR Pull Secret

```bash
make acr-secret
```

### Deploy with ACR Images

```bash
make deploy-helm-acr
```

---

## 📊 Verification Commands

### Check Status

```bash
make status
# OR
kubectl get all -n kubernetes-dashboard
```

### View Logs

```bash
make logs
# OR
kubectl logs -n kubernetes-dashboard -l k8s-app=kubernetes-dashboard --tail=50
```

### Run Full Verification

**Ubuntu:**
```bash
bash scripts/ubuntu/verify-deployment.sh
```

**Windows:**
```powershell
.\scripts\windows\verify-deployment.ps1
```

**Makefile:**
```bash
make verify
```

---

## 🛠️ Troubleshooting

### Common Issues

**Cannot connect to cluster:**
```bash
az login
az aks get-credentials --resource-group prod-cus-platform-base-rg-001 --name prod-cus-aks-sre-lab-003
kubelogin convert-kubeconfig -l azurecli
```

**Token expired:**
```bash
make token
```

**Pods not ready:**
```bash
kubectl get pods -n kubernetes-dashboard
kubectl describe pod <pod-name> -n kubernetes-dashboard
kubectl logs <pod-name> -n kubernetes-dashboard
```

**Port-forward fails:**
```bash
make stop-port-forward
make port-forward
```

**Helm release failed:**
```bash
helm rollback kubernetes-dashboard -n kubernetes-dashboard
```

---

## 📖 Documentation Guide

### For Quick Reference
- **QUICK_REFERENCE.md** - Essential commands and one-liners

### For Step-by-Step Instructions
- **k8s-dashboard-deployment-guide.md** - Complete deployment walkthrough

### For Platform Engineering Teams
- **PLATFORM_ENGINEERING_GUIDE.md** - Enterprise patterns, best practices, security

### For General Overview
- **README.md** - Project overview, features, contributing

---

## 🔄 Update & Maintenance

### Update to New Version

```bash
# Backup current configuration
./scripts/backup-dashboard-config.sh

# Update with Helm
helm upgrade kubernetes-dashboard kubernetes-dashboard/kubernetes-dashboard \
  --version 7.15.0 \
  --values helm/values-prod.yaml

# Verify
make verify
```

### Rollback if Needed

```bash
helm rollback kubernetes-dashboard -n kubernetes-dashboard
```

---

## 🔒 Security Best Practices

1. ✅ Use short-lived tokens (1-24 hours max)
2. ✅ Prefer port-forward over LoadBalancer
3. ✅ Grant read-only access by default
4. ✅ Enable network policies
5. ✅ Use private IPs only
6. ✅ Rotate tokens regularly
7. ✅ Review RBAC bindings monthly
8. ✅ Enable audit logging
9. ✅ Use ACR with private endpoints
10. ✅ Implement Pod Security Standards

---

## 🎓 Training & Learning Paths

### Beginner Path
1. Read README.md
2. Follow QUICK_REFERENCE.md
3. Deploy with Helm using scripts
4. Practice port-forward access

### Intermediate Path
1. Deploy with pure manifests
2. Customize Helm values
3. Setup network policies
4. Configure RBAC roles

### Advanced Path
1. Deploy with Terraform
2. Setup CI/CD with GitHub Actions
3. Integrate with Istio
4. Implement custom monitoring
5. Read PLATFORM_ENGINEERING_GUIDE.md

---

## 📞 Support & Resources

### Internal Resources
- **Platform Engineering Team:** platform-engineering@example.com
- **GitHub Issues:** https://github.com/your-org/kubernetes-dashboard-aks/issues

### External Documentation
- **Kubernetes Dashboard:** https://kubernetes.io/docs/tasks/access-application-cluster/web-ui-dashboard/
- **Helm Chart:** https://github.com/kubernetes/dashboard/tree/master/charts/kubernetes-dashboard
- **Azure AKS:** https://learn.microsoft.com/en-us/azure/aks/
- **kubelogin:** https://azure.github.io/kubelogin/

---

## ⚡ Key Makefile Commands

```bash
make help              # Show all available commands
make install-ubuntu    # Install prerequisites
make connect          # Connect to AKS
make deploy-helm      # Deploy with Helm
make deploy-manifests # Deploy with manifests
make port-forward     # Start port-forward
make token            # Generate admin token
make status           # Check deployment status
make logs             # View dashboard logs
make verify           # Run verification
make clean            # Remove deployment
```

---

## 🎯 Success Criteria

Your deployment is successful when:

✅ All pods are in Running state
✅ Services are accessible via port-forward
✅ Admin token authenticates successfully
✅ Dashboard loads at https://localhost:8443
✅ Can view cluster resources in dashboard
✅ No error events in namespace

**Verify with:**
```bash
make verify
```

---

## 📝 Customization Options

### Environment-Specific Values

- **Development:** `helm/values-dev.yaml`
- **Production:** `helm/values-prod.yaml`
- **With ACR:** `helm/values-acr.yaml`

### Combine Multiple Values Files

```bash
helm upgrade --install kubernetes-dashboard kubernetes-dashboard/kubernetes-dashboard \
  --values helm/values-prod.yaml \
  --values helm/values-acr.yaml
```

### Override Specific Values

```bash
helm upgrade --install kubernetes-dashboard kubernetes-dashboard/kubernetes-dashboard \
  --values helm/values-prod.yaml \
  --set api.scaling.replicas=3
```

---

## 🚦 Deployment Workflow

```
┌─────────────────┐
│ Prerequisites   │
│ Installation    │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ Connect to AKS  │
│ (kubelogin)     │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ Choose Method:  │
│ • Helm          │
│ • Manifests     │
│ • Terraform     │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ Deploy          │
│ Dashboard       │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ Setup Access    │
│ (port-forward)  │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ Generate Token  │
└────────┬────────┘
         │
         ▼
┌─────────────────┐
│ Access UI       │
│ @ localhost:8443│
└─────────────────┘
```

---

## 🎉 You're Ready!

All files are ready to use. Start with the quick start guide above or explore the detailed documentation for your specific use case.

**Happy deploying! 🚀**
