# 🎉 EasyTrade AKS Deployment Bundle - Complete

## What You Have

A production-ready deployment solution for Dynatrace EasyTrade on private AKS clusters with:

✅ **Complete Kubernetes Manifests** - All 15+ microservices pre-configured  
✅ **Helm Chart Structure** - For advanced deployments  
✅ **Ubuntu 24.04 Automation** - Full Makefile with 50+ targets  
✅ **Windows 11 AVD Support** - Non-admin PowerShell scripts  
✅ **Feature Flags Management** - Chaos engineering & problem patterns  
✅ **Multiple Access Methods** - Port-forward, LoadBalancer, Istio Gateway  
✅ **Image Mirroring** - ACR integration for private deployments  
✅ **Health Checking** - Automated deployment verification  
✅ **Comprehensive Documentation** - Platform engineer guides  

## 📂 What's Included

```
easytrade-deployment/
├── INDEX.md                    ← FILE REFERENCE (START HERE)
├── README.md                   ← Main documentation
├── QUICKSTART.md              ← One-page quick reference
├── Makefile                   ← Ubuntu automation (50+ targets)
├── .env.example               ← Configuration template
│
├── docs/
│   ├── ubuntu-setup.md        ← Ubuntu 24.04 complete guide
│   ├── windows-setup.md       ← Windows 11 AVD guide (non-admin)
│   └── feature-flags-guide.md ← Platform engineer's chaos testing guide
│
├── kubernetes/
│   ├── namespace.yaml         ← Namespace + resource quotas
│   ├── configmap.yaml         ← Application configuration
│   ├── database.yaml          ← SQL Server deployment
│   ├── services/              ← All 15 microservice deployments
│   │   ├── broker-service.yaml
│   │   ├── frontend-services.yaml
│   │   └── backend-services.yaml
│   └── ingress/
│       ├── loadbalancer.yaml  ← Internal Azure LB
│       └── istio-gateway.yaml ← Istio service mesh config
│
├── scripts/
│   ├── ubuntu/
│   │   ├── 01-install-tools.sh      ← Install az, kubectl, helm, docker
│   │   └── 03-deploy-manifests.sh   ← Deploy EasyTrade
│   ├── windows/
│   │   ├── 01-Install-Tools.ps1     ← Non-admin tool installation
│   │   ├── 02-Configure-AKS.ps1     ← AKS access configuration
│   │   ├── 03-Deploy-EasyTrade.ps1  ← Deployment script
│   │   └── 04-Setup-Access.ps1      ← Access setup
│   ├── feature-flags.sh       ← Feature flags CLI (bash)
│   ├── health-check.sh        ← Deployment health check
│   └── mirror-images.sh       ← ACR image mirroring
│
└── helm/easytrade/
    ├── Chart.yaml            ← Helm chart metadata
    ├── values.yaml           ← Default values (to be expanded)
    └── values-production.yaml ← Production config (to be created)
```

## 🚀 Quick Start (Choose Your Platform)

### Ubuntu 24.04 VM (5 Minutes)

```bash
# 1. Clone and setup
git clone <your-repo-url>
cd easytrade-deployment
make ubuntu-install-tools

# 2. Configure Azure
make configure-aks

# 3. Deploy
make deploy-manifests

# 4. Access
make setup-portforward
# Visit http://localhost:8080
# Login: demouser/demopass
```

### Windows 11 AVD (10 Minutes)

```powershell
# 1. Clone and setup (Windows Terminal)
git clone <your-repo-url>
cd easytrade-deployment
.\scripts\windows\01-Install-Tools.ps1
# Restart terminal

# 2. Configure Azure
.\scripts\windows\02-Configure-AKS.ps1

# 3. Deploy
.\scripts\windows\03-Deploy-EasyTrade.ps1 -DeploymentType Manifests

# 4. Access
.\scripts\windows\04-Setup-Access.ps1 -AccessMethod PortForward
# Visit http://localhost:8080
```

## 📋 Essential Commands

### Ubuntu Makefile Targets

```bash
make help                    # Show all available targets
make ubuntu-install-tools    # Install all prerequisites
make configure-aks          # Configure AKS access
make mirror-images          # Mirror images to ACR
make deploy-manifests       # Deploy with manifests
make deploy-helm            # Deploy with Helm
make setup-portforward      # Port-forward access
make setup-loadbalancer     # Internal LB access
make feature-flags          # List feature flags
make health-check           # Verify deployment
make logs                   # Tail all logs
make pods                   # List pods
make top                    # Resource usage
make clean-all              # Complete cleanup
```

### Feature Flags & Problem Patterns

```bash
# Ubuntu/Linux
./scripts/feature-flags.sh list
./scripts/feature-flags.sh enable db_not_responding
./scripts/feature-flags.sh disable high_cpu_usage

# Windows
.\scripts\feature-flags.ps1 -Action List
.\scripts\feature-flags.ps1 -Action Enable -Pattern db_not_responding
```

**Available Patterns:**
- `db_not_responding` - Database errors (~20 min alert)
- `ergo_aggregator_slowdown` - Slow responses (15-30 min)
- `factory_crisis` - No credit cards (persistent)
- `high_cpu_usage` - CPU spike (5-10 min alert)

## 🎯 Your Environment Configuration

Update `.env` file with your values:

```bash
# Copy template
cp .env.example .env

# Edit with your values
vim .env  # or use any editor

# Key variables to set:
AZURE_SUBSCRIPTION_NAME="tango-CICD-platform-github-gitflow"
AZURE_RESOURCE_GROUP="prod-cus-platform-base-rg-001"
CLUSTER_NAME="prod-cus-aks-sre-lab-003"
ACR_NAME="prod-central-image-repo"
USE_ACR_IMAGES="true"  # Use private ACR vs upstream
```

## 🔐 Default Credentials

- **Frontend User 1**: `demouser` / `demopass`
- **Frontend User 2**: `specialuser` / `specialpass`
- **Database SA**: `sa` / `yourStrong(!)Password`

## 📊 Architecture Overview

```
Internet/VNet → Load Balancer/Port-Forward
                       ↓
            Frontend Reverse Proxy (NGINX)
                       ↓
    ┌──────────────────┴──────────────────┐
    │                                      │
Frontend         Feature Flag Service      │
    │                   ↓                  │
    │        ┌──────────────────┐         │
    │        │  Microservices:  │         │
    │        │  • Broker        │         │
    │        │  • Account       │         │
    │        │  • Login         │         │
    │        │  • Pricing       │         │
    │        │  • Offer         │         │
    │        │  • etc.          │         │
    │        └──────────────────┘         │
    │                 ↓                   │
    └─────────────────┴───────────────────┘
                      ↓
              SQL Server Database
            (Persistent Volume)
```

## 🔧 Common Operations

### View Deployment Status
```bash
# Ubuntu
kubectl get pods -n easytrade
make pods

# Windows
kubectl get pods -n easytrade
```

### View Logs
```bash
# Ubuntu
make logs
kubectl logs -n easytrade -l app=broker-service --tail=50

# Windows
kubectl logs -n easytrade -l app=broker-service --tail=50
```

### Scale Services
```bash
kubectl scale deployment broker-service --replicas=5 -n easytrade
```

### Update Images
```bash
# Ubuntu
./scripts/mirror-images.sh
kubectl set image deployment/broker-service broker-service=prodcentralimagerepo.azurecr.io/easytrade/broker-service:v2 -n easytrade

# Windows
kubectl set image deployment/broker-service broker-service=prodcentralimagerepo.azurecr.io/easytrade/broker-service:v2 -n easytrade
```

## 🆘 Troubleshooting

### Can't Connect to Cluster
```bash
# Refresh credentials
az aks get-credentials -g prod-cus-platform-base-rg-001 -n prod-cus-aks-sre-lab-003 --overwrite-existing
kubelogin convert-kubeconfig -l azurecli
kubectl get nodes
```

### Pods Not Starting
```bash
# Check pod status
kubectl describe pod <pod-name> -n easytrade
kubectl logs <pod-name> -n easytrade

# Common issues:
# 1. Image pull errors → attach ACR
az aks update -n prod-cus-aks-sre-lab-003 -g prod-cus-platform-base-rg-001 --attach-acr prod-central-image-repo

# 2. Database not ready → wait for DB pod
kubectl wait --for=condition=ready pod -l app=db -n easytrade --timeout=300s

# 3. Resource limits → check quotas
kubectl describe resourcequota -n easytrade
```

### Health Check
```bash
# Ubuntu
./scripts/health-check.sh --verbose

# Manual checks
kubectl get pods -n easytrade
kubectl get svc -n easytrade
kubectl top pods -n easytrade
```

## 📚 Documentation Guide

**Start with:**
1. `INDEX.md` - Complete file reference
2. `QUICKSTART.md` - One-page quick reference

**Platform-specific setup:**
- Ubuntu: `docs/ubuntu-setup.md`
- Windows: `docs/windows-setup.md`

**Advanced topics:**
- Feature Flags: `docs/feature-flags-guide.md`
- Makefile: `Makefile` (50+ targets with help)

## 🎓 Learning Path for Platform Engineers

### Day 1: Basic Deployment
1. Read `QUICKSTART.md`
2. Complete Ubuntu or Windows setup guide
3. Deploy EasyTrade
4. Access via port-forward
5. Verify with health check

### Day 2: Feature Flags & Chaos Engineering
1. Read `docs/feature-flags-guide.md`
2. Test each problem pattern
3. Monitor alerts (if observability configured)
4. Document alert response times

### Day 3: Production Hardening
1. Deploy with LoadBalancer
2. Configure Istio Gateway
3. Apply network policies
4. Setup monitoring
5. Test autoscaling

### Day 4: CI/CD Integration
1. Setup GitHub repository
2. Configure GitHub Actions
3. Automate deployments
4. Implement GitOps

### Day 5: Operations
1. Backup/restore procedures
2. Disaster recovery testing
3. Performance tuning
4. Security hardening

## 🔒 Security Considerations

- [ ] Never commit `.env` file
- [ ] Rotate database passwords
- [ ] Use Azure Key Vault for secrets
- [ ] Apply network policies
- [ ] Enable Pod Security Standards
- [ ] Implement RBAC
- [ ] Scan images for vulnerabilities
- [ ] Enable mTLS with Istio

## 📈 Next Steps

### Immediate (Today)
1. Deploy to dev/test cluster
2. Test all access methods
3. Verify feature flags work
4. Run health checks

### Short-term (This Week)
1. Mirror images to ACR
2. Configure monitoring
3. Setup alerts
4. Document runbooks

### Long-term (This Month)
1. Production deployment
2. CI/CD automation
3. Disaster recovery testing
4. Performance optimization

## 💡 Tips & Best Practices

1. **Always use Makefile** (Ubuntu) - Consistent, repeatable operations
2. **Test in dev first** - Never deploy untested changes to production
3. **Monitor during chaos tests** - Feature flags should trigger alerts
4. **Document everything** - Future you will thank present you
5. **Use version control** - Track all configuration changes
6. **Automate everything** - Reduce human error

## 🤝 Contributing

To add improvements:

1. Test changes in dev environment
2. Update relevant documentation
3. Add Makefile targets if applicable
4. Update INDEX.md with new files
5. Submit pull request

## 📞 Support & Resources

- **EasyTrade Official**: https://github.com/Dynatrace/easytrade
- **Azure AKS Docs**: https://learn.microsoft.com/en-us/azure/aks/
- **Helm Documentation**: https://helm.sh/docs/
- **Istio Documentation**: https://istio.io/latest/docs/
- **Kubernetes Docs**: https://kubernetes.io/docs/

## ✅ Deployment Checklist

Before production deployment:

- [ ] All documentation reviewed
- [ ] Tools installed and verified
- [ ] AKS access configured
- [ ] ACR attached to cluster
- [ ] Images mirrored (if using private ACR)
- [ ] Environment variables configured
- [ ] Deployment tested in dev/test
- [ ] Health checks passing
- [ ] Access methods configured
- [ ] Monitoring/alerting setup
- [ ] Backup strategy documented
- [ ] Runbooks created
- [ ] Team trained on operations

---

**🎉 You're Ready to Deploy!**

Start with: `make ubuntu-install-tools` (Ubuntu) or `.\scripts\windows\01-Install-Tools.ps1` (Windows)

Questions? Check `INDEX.md` for complete file reference.

**Version**: 1.0.0  
**Last Updated**: 2024-12-19  
**Maintainer**: Platform Engineering Team
