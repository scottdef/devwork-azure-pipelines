# EasyTrade Deployment Bundle - File Index

## 📁 Root Files

| File | Purpose |
|------|---------|
| `README.md` | Main documentation and quick start guide |
| `QUICKSTART.md` | One-page quick reference card for common operations |
| `Makefile` | Automation for Ubuntu/Linux - all deployment tasks |
| `.env.example` | Environment variable template (copy to `.env`) |
| `STRUCTURE.txt` | Complete directory tree structure |

## 📚 Documentation (`docs/`)

| File | Target Audience | Purpose |
|------|----------------|---------|
| `ubuntu-setup.md` | DevOps/Platform Engineers | Complete Ubuntu 24.04 setup guide |
| `windows-setup.md` | Platform Engineers | Windows 11 AVD setup (non-admin) |
| `feature-flags-guide.md` | Platform Engineers | Chaos engineering & problem patterns guide |

## ⚙️ Scripts

### Ubuntu/Linux (`scripts/ubuntu/`)

| File | Purpose | Usage |
|------|---------|-------|
| `01-install-tools.sh` | Install az, kubectl, kubelogin, helm, docker | `bash 01-install-tools.sh` |
| `03-deploy-manifests.sh` | Deploy EasyTrade with raw manifests | `bash 03-deploy-manifests.sh --use-acr` |

### Windows (`scripts/windows/`)

| File | Purpose | Usage |
|------|---------|-------|
| `01-Install-Tools.ps1` | Install tools (non-admin via Scoop) | `.\01-Install-Tools.ps1` |
| `02-Configure-AKS.ps1` | Configure AKS cluster access | `.\02-Configure-AKS.ps1` |
| `03-Deploy-EasyTrade.ps1` | Deploy EasyTrade (manifests or Helm) | `.\03-Deploy-EasyTrade.ps1 -DeploymentType Manifests` |
| `04-Setup-Access.ps1` | Setup access (port-forward, LB, pod IP) | `.\04-Setup-Access.ps1 -AccessMethod PortForward` |

### Utility Scripts (`scripts/`)

| File | Purpose | Usage |
|------|---------|-------|
| `feature-flags.sh` | Manage feature flags & problem patterns | `./feature-flags.sh list` |
| `health-check.sh` | Comprehensive deployment health check | `./health-check.sh --verbose` |
| `mirror-images.sh` | Mirror EasyTrade images to private ACR | `./mirror-images.sh` |

## 🎯 Kubernetes Manifests (`kubernetes/`)

### Core Resources

| File | Resources | Purpose |
|------|-----------|---------|
| `namespace.yaml` | Namespace, ResourceQuota, LimitRange | Namespace setup with resource limits |
| `configmap.yaml` | ConfigMap | Application configuration |
| `database.yaml` | Deployment, Service, PVC, Secret | SQL Server database |

### Services (`kubernetes/services/`)

| File | Services | Purpose |
|------|----------|---------|
| `broker-service.yaml` | Broker Service | Main trading service |
| `frontend-services.yaml` | Frontend, Reverse Proxy, Feature Flags | Frontend & UI services |
| `backend-services.yaml` | Account, Login, Pricing, Offer, ContentCreator | Backend microservices |

### Ingress (`kubernetes/ingress/`)

| File | Resources | Purpose |
|------|-----------|---------|
| `loadbalancer.yaml` | Internal LoadBalancer Service | Private Azure Load Balancer |
| `istio-gateway.yaml` | Gateway, VirtualService, DestinationRule | Istio service mesh configuration |

## 📦 Helm Chart (`helm/easytrade/`)

| File | Purpose |
|------|---------|
| `Chart.yaml` | Helm chart metadata |
| `values.yaml` | Default configuration values |
| `values-production.yaml` | Production-grade configuration |
| `templates/` | Kubernetes resource templates (to be created) |

## 🎬 Quick Start Workflows

### Ubuntu Quick Deploy
```bash
# 1. Install tools
make ubuntu-install-tools

# 2. Configure AKS
make configure-aks

# 3. Deploy
make deploy-manifests

# 4. Access
make setup-portforward
```

### Windows Quick Deploy
```powershell
# 1. Install tools
.\scripts\windows\01-Install-Tools.ps1

# 2. Configure AKS
.\scripts\windows\02-Configure-AKS.ps1

# 3. Deploy
.\scripts\windows\03-Deploy-EasyTrade.ps1 -DeploymentType Manifests

# 4. Access
.\scripts\windows\04-Setup-Access.ps1 -AccessMethod PortForward
```

## 🔧 Makefile Targets (Ubuntu/Linux)

### Installation & Setup
- `ubuntu-install-tools` - Install all required tools
- `verify-tools` - Verify tool installations
- `azure-login` - Login to Azure
- `configure-aks` - Configure AKS cluster access
- `attach-acr` - Attach ACR to AKS

### Image Management
- `mirror-images` - Mirror images to ACR
- `acr-login` - Login to ACR
- `list-acr-images` - List ACR images

### Deployment
- `create-namespace` - Create easytrade namespace
- `deploy-manifests` - Deploy with raw manifests
- `deploy-helm` - Deploy with Helm (default values)
- `deploy-helm-production` - Deploy with Helm (production values)
- `verify-deployment` - Verify deployment health

### Access Setup
- `setup-portforward` - Port-forward (foreground)
- `setup-portforward-background` - Port-forward (background)
- `stop-portforward` - Stop background port-forward
- `setup-loadbalancer` - Deploy internal LoadBalancer
- `setup-istio` - Deploy Istio Gateway
- `get-access-info` - Display all access methods

### Feature Flags
- `feature-flags` - List all feature flags
- `enable-problem-pattern` - Enable pattern (PATTERN=db_not_responding)
- `disable-problem-pattern` - Disable pattern

### Monitoring & Debug
- `logs` - Tail logs from all pods
- `pods` - List all pods
- `describe-pod` - Describe specific pod
- `shell` - Open shell in pod
- `debug-network` - Deploy debug pod
- `events` - Show recent events
- `top` - Show resource usage

### Cleanup
- `delete-deployment` - Delete EasyTrade
- `helm-uninstall` - Uninstall Helm release
- `clean-all` - Complete cleanup

## 📊 Feature Flags & Problem Patterns

### Available Patterns

| Pattern ID | Effect | Alert Time | Use Case |
|------------|--------|------------|----------|
| `db_not_responding` | Database errors | ~20 min | Database failure testing |
| `ergo_aggregator_slowdown` | Slow responses | 15-30 min | Latency testing |
| `factory_crisis` | No credit cards | Immediate | Supply chain failure |
| `high_cpu_usage` | CPU spike | 5-10 min | Resource exhaustion |

### Management Commands

```bash
# Ubuntu/Linux
./scripts/feature-flags.sh list
./scripts/feature-flags.sh enable db_not_responding
./scripts/feature-flags.sh disable high_cpu_usage

# Windows
.\scripts\feature-flags.ps1 -Action List
.\scripts\feature-flags.ps1 -Action Enable -Pattern db_not_responding
.\scripts\feature-flags.ps1 -Action Disable -Pattern high_cpu_usage
```

## 🔐 Default Configuration

### Credentials
- **User 1**: `demouser` / `demopass`
- **User 2**: `specialuser` / `specialpass`
- **Database SA**: `sa` / `yourStrong(!)Password`

### Azure Resources
- **Subscription**: `tango-CICD-platform-github-gitflow`
- **Resource Group**: `prod-cus-platform-base-rg-001`
- **Cluster**: `prod-cus-aks-sre-lab-003`
- **ACR**: `prod-central-image-repo`
- **Namespace**: `easytrade`

### Ports
- **Frontend**: 80 (ClusterIP)
- **Broker Service**: 8080
- **Account Service**: 8089
- **Feature Flag Service**: 8080
- **Database**: 1433
- **Local Port-Forward**: 8080

## 🎯 Access Methods

### 1. kubectl port-forward (Development)
```bash
# Ubuntu
make setup-portforward

# Windows
.\scripts\windows\04-Setup-Access.ps1 -AccessMethod PortForward

# Manual
kubectl -n easytrade port-forward svc/frontendreverseproxy 8080:80
```
Access: `http://localhost:8080`

### 2. Internal LoadBalancer (Testing)
```bash
# Ubuntu
make setup-loadbalancer

# Windows
.\scripts\windows\04-Setup-Access.ps1 -AccessMethod LoadBalancer

# Manual
kubectl apply -f kubernetes/ingress/loadbalancer.yaml
kubectl get svc easytrade-lb -n easytrade
```
Access: `http://<INTERNAL-IP>`

### 3. Istio Gateway (Production)
```bash
# Ubuntu
make setup-istio

# Manual
kubectl apply -f kubernetes/ingress/istio-gateway.yaml
```
Access: `http://<ISTIO-GATEWAY-IP>`

## 📝 File Checklist

Before deployment, ensure you have:

- [ ] Configured `.env` file (from `.env.example`)
- [ ] Azure CLI installed and logged in
- [ ] kubectl and kubelogin installed
- [ ] AKS cluster credentials configured
- [ ] ACR attached to AKS (if using private images)
- [ ] Namespace created
- [ ] All scripts executable (`chmod +x scripts/*.sh`)

## 🔄 Update Process

### Update Images
```bash
# Mirror new images
./scripts/mirror-images.sh

# Update deployment
kubectl set image deployment/broker-service broker-service=prodcentralimagerepo.azurecr.io/easytrade/broker-service:v2 -n easytrade

# Or redeploy
make deploy-manifests
```

### Update Configuration
```bash
# Edit ConfigMap
kubectl edit configmap easytrade-config -n easytrade

# Restart affected services
kubectl rollout restart deployment -n easytrade
```

## 🆘 Troubleshooting

### Common Issues

| Issue | Solution | Command |
|-------|----------|---------|
| Can't connect to cluster | Refresh credentials | `az aks get-credentials -g <rg> -n <cluster> --overwrite-existing` |
| Pods pending | Check node resources | `kubectl top nodes` |
| Image pull errors | Attach ACR | `az aks update --attach-acr <acr-name>` |
| Database not ready | Check logs | `kubectl logs -l app=db -n easytrade` |
| Feature flags not working | Restart frontend | `kubectl rollout restart deployment/frontendreverseproxy -n easytrade` |

## 📚 Additional Resources

- **EasyTrade GitHub**: https://github.com/Dynatrace/easytrade
- **Azure AKS Docs**: https://learn.microsoft.com/en-us/azure/aks/
- **Helm Docs**: https://helm.sh/docs/
- **Istio Docs**: https://istio.io/latest/docs/

## 🔐 Security Notes

- Never commit `.env` file to version control
- Rotate database passwords regularly
- Use Azure Key Vault for production secrets
- Apply network policies in production
- Enable Pod Security Standards
- Use RBAC for least-privilege access

## 📈 Production Checklist

- [ ] Resource limits configured
- [ ] Horizontal Pod Autoscaling enabled
- [ ] Pod Disruption Budgets created
- [ ] Network Policies applied
- [ ] Monitoring & alerting configured
- [ ] Backup strategy implemented
- [ ] Disaster recovery tested
- [ ] Security scan completed
- [ ] Performance tested

---

**Version**: 1.0.0  
**Last Updated**: 2024-12-19  
**Maintainer**: Platform Engineering Team
