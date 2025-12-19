# Kubernetes Dashboard Quick Reference Card

## 🚀 Quick Start Commands

### Ubuntu 24.04

```bash
# Setup and Deploy
make install-ubuntu && make connect && make deploy-helm

# Access
make port-forward  # Terminal 1
make token         # Terminal 2, then login at https://localhost:8443
```

### Windows 11

```powershell
# Setup and Deploy
.\scripts\windows\01-prerequisites.ps1
.\scripts\windows\02-aks-connect.ps1
.\scripts\windows\04-deploy-helm.ps1

# Access
.\scripts\windows\05-setup-access.ps1
kubectl create token dashboard-admin -n kubernetes-dashboard --duration=24h
```

## 🔧 Essential Commands

| Task | Command |
|------|---------|
| **Connect to AKS** | `make connect` |
| **Deploy Dashboard** | `make deploy-helm` |
| **Port Forward** | `make port-forward` |
| **Generate Token** | `make token` |
| **Check Status** | `make status` |
| **View Logs** | `make logs` |
| **Verify Deployment** | `make verify` |
| **Clean Up** | `make clean` |

## 🔑 Authentication

```bash
# Admin token (24h)
kubectl create token dashboard-admin -n kubernetes-dashboard --duration=24h

# Read-only token (24h)
kubectl create token dashboard-readonly -n kubernetes-dashboard --duration=24h

# Short-lived token (1h)
kubectl create token dashboard-admin -n kubernetes-dashboard --duration=1h
```

## 🌐 Access Methods

### Port Forward (Recommended)
```bash
kubectl port-forward -n kubernetes-dashboard svc/kubernetes-dashboard-kong-proxy 8443:443
# Access: https://localhost:8443
```

### NodePort
```bash
kubectl apply -f manifests/nodeport-service.yaml
# Access: https://<node-ip>:30443
```

### Internal Load Balancer
```bash
kubectl apply -f manifests/loadbalancer-service.yaml
# Get IP: kubectl get svc dashboard-loadbalancer -n kubernetes-dashboard
```

## 🐛 Troubleshooting

| Issue | Solution |
|-------|----------|
| Cannot connect | `az login && az aks get-credentials --resource-group prod-cus-platform-base-rg-001 --name prod-cus-aks-sre-lab-003` |
| Token expired | `make token` |
| Pods not ready | `kubectl get pods -n kubernetes-dashboard && kubectl describe pod <pod> -n kubernetes-dashboard` |
| Port-forward fails | `make stop-port-forward && make port-forward` |
| Helm error | `helm rollback kubernetes-dashboard -n kubernetes-dashboard` |

## 📊 Monitoring

```bash
# Pod status
kubectl get pods -n kubernetes-dashboard -w

# Resource usage
kubectl top pods -n kubernetes-dashboard

# Events
kubectl get events -n kubernetes-dashboard --sort-by='.lastTimestamp'

# Logs (live)
kubectl logs -n kubernetes-dashboard -l k8s-app=kubernetes-dashboard -f
```

## 🔐 Security Best Practices

- ✅ Use short-lived tokens (1-24 hours max)
- ✅ Prefer port-forward over LoadBalancer
- ✅ Grant read-only access by default
- ✅ Enable network policies
- ✅ Use private IPs only
- ✅ Rotate tokens regularly
- ✅ Review RBAC bindings monthly

## 📦 ACR Operations

```bash
# Login
az acr login --name prod-central-image-repo

# Import images
make acr-import

# Create pull secret
make acr-secret

# Build custom image
make docker-build && make docker-push
```

## 🔄 Upgrade/Rollback

```bash
# Upgrade to new version
helm upgrade kubernetes-dashboard kubernetes-dashboard/kubernetes-dashboard \
  --version 7.15.0 \
  --values helm/values-prod.yaml

# Rollback
helm rollback kubernetes-dashboard -n kubernetes-dashboard
```

## 🎯 Production Deployment

```bash
# Full production setup
make connect
make acr-import
make acr-secret
make deploy-helm-prod
make loadbalancer
make token > admin-token.txt
```

## 📞 Support

- **GitHub Issues:** https://github.com/your-org/kubernetes-dashboard-aks/issues
- **Platform Team:** platform-engineering@example.com
- **Docs:** https://kubernetes.io/docs/tasks/access-application-cluster/web-ui-dashboard/

## 🔗 Important URLs

| Resource | URL |
|----------|-----|
| Dashboard (Port-Forward) | https://localhost:8443 |
| Helm Chart Repo | https://kubernetes.github.io/dashboard/ |
| ACR | https://prod-central-image-repo.azurecr.io |
| AKS Docs | https://learn.microsoft.com/en-us/azure/aks/ |

## ⚡ One-Liners

```bash
# Complete setup (Ubuntu)
curl -fsSL https://raw.githubusercontent.com/your-org/kubernetes-dashboard-aks/main/scripts/ubuntu/quick-setup.sh | bash

# Get dashboard URL after deployment
echo "https://localhost:8443"

# Get all dashboard resources
kubectl get all,secret,configmap -n kubernetes-dashboard

# Force delete stuck pod
kubectl delete pod <pod-name> -n kubernetes-dashboard --grace-period=0 --force

# Check if dashboard is accessible
curl -k https://localhost:8443 && echo "Dashboard is UP" || echo "Dashboard is DOWN"
```

---

**Version:** 1.0  
**Last Updated:** December 2024  
**Print this card for quick reference!**
