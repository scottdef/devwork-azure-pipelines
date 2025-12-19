# EasyTrade Quick Start Reference Card

## 🚀 Ubuntu 24.04 - Five Minute Deploy

```bash
git clone https://github.com/your-org/easytrade-deployment.git
cd easytrade-deployment
make ubuntu-install-tools
make configure-aks
make deploy-manifests
make setup-portforward
# Access at http://localhost:8080
```

## 🖥️ Windows 11 AVD - Five Minute Deploy

```powershell
git clone https://github.com/your-org/easytrade-deployment.git
cd easytrade-deployment
.\scripts\windows\01-Install-Tools.ps1
# Restart terminal
.\scripts\windows\02-Configure-AKS.ps1
.\scripts\windows\03-Deploy-EasyTrade.ps1 -DeploymentType Manifests
.\scripts\windows\04-Setup-Access.ps1 -AccessMethod PortForward
# Access at http://localhost:8080
```

## 📋 Essential Commands

### Ubuntu/Linux

```bash
# View pods
kubectl get pods -n easytrade

# View logs
kubectl logs -n easytrade -l app=broker-service --tail=50

# Health check
./scripts/health-check.sh

# Feature flags
./scripts/feature-flags.sh list
./scripts/feature-flags.sh enable high_cpu_usage
./scripts/feature-flags.sh disable high_cpu_usage

# Cleanup
make clean-all
```

### Windows

```powershell
# View pods
kubectl get pods -n easytrade

# View logs
kubectl logs -n easytrade -l app=broker-service --tail=50

# Feature flags
.\scripts\feature-flags.ps1 -Action List
.\scripts\feature-flags.ps1 -Action Enable -Pattern high_cpu_usage
.\scripts\feature-flags.ps1 -Action Disable -Pattern high_cpu_usage

# Cleanup
kubectl delete namespace easytrade
```

## 🔑 Default Credentials

- **User 1**: `demouser` / `demopass`
- **User 2**: `specialuser` / `specialpass`

## 🎯 Problem Patterns

| Pattern | Effect | Duration |
|---------|--------|----------|
| `db_not_responding` | Database errors | ~20 min alert |
| `ergo_aggregator_slowdown` | Slow responses | 15-30 min |
| `factory_crisis` | No credit cards | Persistent |
| `high_cpu_usage` | CPU spike | 5-10 min alert |

## 📊 Monitoring

```bash
# Resource usage
kubectl top pods -n easytrade

# Watch pods
watch -n 2 'kubectl get pods -n easytrade'

# Events
kubectl get events -n easytrade --sort-by='.lastTimestamp'
```

## 🔧 Troubleshooting

### Can't connect to cluster

```bash
# Ubuntu
az aks get-credentials -g prod-cus-platform-base-rg-001 -n prod-cus-aks-sre-lab-003 --overwrite-existing
kubelogin convert-kubeconfig -l azurecli

# Windows
az aks get-credentials -g prod-cus-platform-base-rg-001 -n prod-cus-aks-sre-lab-003 --overwrite-existing
kubelogin convert-kubeconfig -l azurecli
```

### Pods pending/failing

```bash
kubectl describe pod <pod-name> -n easytrade
kubectl logs <pod-name> -n easytrade
```

### Image pull errors

```bash
az aks update -n prod-cus-aks-sre-lab-003 -g prod-cus-platform-base-rg-001 --attach-acr prod-central-image-repo
```

## 📚 Documentation

- [Ubuntu Setup Guide](docs/ubuntu-setup.md) - Complete Ubuntu installation guide
- [Windows Setup Guide](docs/windows-setup.md) - Complete Windows installation guide  
- [Feature Flags Guide](docs/feature-flags-guide.md) - Platform engineer's guide to chaos testing
- [Makefile Targets](Makefile) - All available automation commands

## 🏗️ Architecture

```
User → LoadBalancer/Port-Forward → Frontend Reverse Proxy → Services
                                            ↓
                                    Feature Flag Service
                                            ↓
                    ┌──────────────────────────────────┐
                    │  Broker  │ Account │ Login │ etc│
                    └──────────────────────────────────┘
                                    ↓
                            SQL Server Database
```

## ⚡ Quick Operations

```bash
# Scale services
kubectl scale deployment broker-service --replicas=5 -n easytrade

# Restart service
kubectl rollout restart deployment/broker-service -n easytrade

# Update image
kubectl set image deployment/broker-service broker-service=prodcentralimagerepo.azurecr.io/easytrade/broker-service:v2 -n easytrade

# Export configuration
kubectl get all -n easytrade -o yaml > backup.yaml

# Delete and redeploy
kubectl delete namespace easytrade
make deploy-manifests
```

## 🔐 Access Methods

### 1. Port-Forward (Development)
```bash
kubectl -n easytrade port-forward svc/frontendreverseproxy 8080:80
# Access: http://localhost:8080
```

### 2. LoadBalancer (Internal)
```bash
kubectl apply -f kubernetes/ingress/loadbalancer.yaml
kubectl get svc easytrade-lb -n easytrade
# Access: http://<INTERNAL-IP>
```

### 3. Istio Gateway (Production)
```bash
kubectl apply -f kubernetes/ingress/istio-gateway.yaml
kubectl get svc -n aks-istio-ingress
# Access: http://<ISTIO-GATEWAY-IP>
```

## 💡 Tips

- Always check health after deployment: `./scripts/health-check.sh`
- Use `make help` to see all available commands
- Monitor during problem pattern tests
- Document any configuration changes
- Keep credentials secure

## 🆘 Support

- GitHub Issues: https://github.com/your-org/easytrade-deployment/issues
- EasyTrade Upstream: https://github.com/Dynatrace/easytrade
- Team Slack: #platform-engineering

---

**Version**: 1.0.0  
**Last Updated**: 2024-12-19
