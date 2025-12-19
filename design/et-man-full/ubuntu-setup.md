# Ubuntu 24.04 Setup Guide for EasyTrade on AKS

This guide provides step-by-step instructions for deploying EasyTrade to private AKS clusters from an Ubuntu 24.04 VM.

## Prerequisites

- Ubuntu 24.04 LTS VM with internet access
- SSH access to the VM
- Azure account with AKS cluster access
- Cluster: `prod-cus-aks-sre-lab-003`
- Resource Group: `prod-cus-platform-base-rg-001`
- ACR: `prod-central-image-repo`

## Quick Start (5 Minutes)

```bash
# Clone repository
git clone https://github.com/your-org/easytrade-deployment.git
cd easytrade-deployment

# Install all prerequisites
make ubuntu-install-tools

# Configure AKS access
make configure-aks

# Deploy EasyTrade (choose one)
make deploy-manifests    # Option 1: Raw Kubernetes manifests
make deploy-helm         # Option 2: Helm chart

# Setup access
make setup-portforward   # Access via http://localhost:8080
```

## Detailed Step-by-Step Guide

### Step 1: System Preparation

```bash
# Update system packages
sudo apt update
sudo apt upgrade -y

# Create working directory
mkdir -p ~/easytrade-work
cd ~/easytrade-work
```

### Step 2: Install Required Tools

#### Option A: Automated Installation (Recommended)

```bash
# Clone repository
git clone https://github.com/your-org/easytrade-deployment.git
cd easytrade-deployment

# Install all tools
make ubuntu-install-tools

# Verify installations
make verify-tools
```

#### Option B: Manual Installation

```bash
# Install Azure CLI
curl -sL https://aka.ms/InstallAzureCLIDeb | sudo bash

# Install kubectl and kubelogin
sudo az aks install-cli

# Install Helm
curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash

# Install Docker
sudo apt-get install -y ca-certificates curl gnupg
sudo install -m 0755 -d /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /etc/apt/keyrings/docker.gpg
echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu $(. /etc/os-release && echo "$VERSION_CODENAME") stable" | sudo tee /etc/apt/sources.list.d/docker.list
sudo apt-get update
sudo apt-get install -y docker-ce docker-ce-cli containerd.io
sudo usermod -aG docker $USER

# Install utilities
sudo apt-get install -y git jq make curl

# Logout and login for docker group to take effect
```

### Step 3: Azure and AKS Configuration

```bash
# Login to Azure
az login

# Set subscription
az account set --subscription "tango-CICD-platform-github-gitflow"

# Verify subscription
az account show

# Get AKS credentials
az aks get-credentials \
    --resource-group "prod-cus-platform-base-rg-001" \
    --name "prod-cus-aks-sre-lab-003" \
    --overwrite-existing

# Configure kubelogin for private cluster
kubelogin convert-kubeconfig -l azurecli

# Test connectivity
kubectl get nodes

# If direct connectivity fails (normal for private clusters):
# Use az aks command invoke
az aks command invoke \
    -g prod-cus-platform-base-rg-001 \
    -n prod-cus-aks-sre-lab-003 \
    --command "kubectl get nodes"
```

### Step 4: Image Management (Optional)

If using private ACR instead of upstream images:

```bash
# Login to ACR
az acr login --name prod-central-image-repo

# Mirror images to ACR
make mirror-images

# Verify images
az acr repository list --name prod-central-image-repo -o table | grep easytrade
```

### Step 5: Deploy EasyTrade

#### Option A: Using Kubernetes Manifests

```bash
# Create namespace
kubectl create namespace easytrade

# Deploy using upstream images
make deploy-manifests

# OR deploy using ACR images
make deploy-manifests USE_ACR_IMAGES=true

# Monitor deployment
watch -n 2 'kubectl get pods -n easytrade'

# Wait for all pods to be Running
# This may take 5-10 minutes for first deployment
```

#### Option B: Using Helm Chart

```bash
# Deploy with default values
make deploy-helm

# OR deploy with production values
make deploy-helm-production

# OR custom Helm installation
helm install easytrade ./helm/easytrade \
    --namespace easytrade \
    --create-namespace \
    --set global.imageRegistry=prodcentralimagerepo.azurecr.io \
    --wait --timeout 10m
```

### Step 6: Verify Deployment

```bash
# Check pod status
kubectl get pods -n easytrade

# Check services
kubectl get svc -n easytrade

# View recent events
kubectl get events -n easytrade --sort-by='.lastTimestamp' | tail -20

# Check specific service logs
kubectl logs -n easytrade -l app=broker-service --tail=50
```

### Step 7: Setup Access

#### Method 1: kubectl port-forward (Development)

```bash
# Start port-forward (foreground)
make setup-portforward

# OR start in background
make setup-portforward-background

# Access EasyTrade at http://localhost:8080
curl http://localhost:8080

# Stop background port-forward
make stop-portforward
```

#### Method 2: Internal LoadBalancer (Testing)

```bash
# Deploy LoadBalancer
make setup-loadbalancer

# Get LoadBalancer IP
kubectl get svc easytrade-lb -n easytrade

# Access via LoadBalancer IP
LB_IP=$(kubectl get svc easytrade-lb -n easytrade -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
curl http://${LB_IP}
```

#### Method 3: Istio Gateway (Production)

```bash
# Deploy Istio configuration
make setup-istio

# Get Istio ingress IP
kubectl get svc -n aks-istio-ingress aks-istio-ingressgateway-internal
```

### Step 8: Test Application

```bash
# Access credentials
# Default users:
#   demouser / demopass
#   specialuser / specialpass

# Test health endpoint
curl http://localhost:8080/api/health

# List feature flags
make feature-flags

# OR manual:
curl -s http://localhost:8080/feature-flag-service/v1/flags | jq
```

## Working with Feature Flags & Problem Patterns

### Using the Management Script

```bash
# List all feature flags
./scripts/feature-flags.sh list

# Enable problem pattern
./scripts/feature-flags.sh enable db_not_responding

# Check status
./scripts/feature-flags.sh get db_not_responding

# Disable problem pattern
./scripts/feature-flags.sh disable db_not_responding
```

### Manual API Calls

```bash
# Set environment variable
export EASYTRADE_URL="http://localhost:8080"

# Enable high CPU usage pattern
curl -X PUT "${EASYTRADE_URL}/feature-flag-service/v1/flags/high_cpu_usage/" \
    -H "Content-Type: application/json" \
    -d '{"enabled": true}'

# Monitor CPU usage
watch -n 2 'kubectl top pods -n easytrade -l app=broker-service'

# Disable pattern
curl -X PUT "${EASYTRADE_URL}/feature-flag-service/v1/flags/high_cpu_usage/" \
    -H "Content-Type: application/json" \
    -d '{"enabled": false}'
```

## Common Operations

### View Logs

```bash
# All pods
kubectl logs -n easytrade --all-containers=true --tail=50

# Specific service
kubectl logs -n easytrade -l app=broker-service --tail=100 -f

# Recent logs from all services
for pod in $(kubectl get pods -n easytrade -o name); do
    echo "=== $pod ==="
    kubectl logs -n easytrade $pod --tail=20
done
```

### Debug Pod Issues

```bash
# Describe pod
kubectl describe pod -n easytrade <pod-name>

# Get into pod shell
kubectl exec -it -n easytrade <pod-name> -- /bin/sh

# Run debug pod
kubectl run -it --rm debug --image=nicolaka/netshoot --restart=Never -n easytrade -- /bin/bash
```

### Update Deployment

```bash
# Update image
kubectl set image deployment/broker-service \
    broker-service=prodcentralimagerepo.azurecr.io/easytrade/broker-service:v2 \
    -n easytrade

# Rollback
kubectl rollout undo deployment/broker-service -n easytrade

# View rollout history
kubectl rollout history deployment/broker-service -n easytrade
```

### Scale Services

```bash
# Scale up
kubectl scale deployment broker-service --replicas=5 -n easytrade

# Scale down
kubectl scale deployment broker-service --replicas=2 -n easytrade

# View current replicas
kubectl get deployment -n easytrade
```

## Cleanup

```bash
# Delete deployment (preserves namespace)
kubectl delete all --all -n easytrade

# Delete entire namespace
kubectl delete namespace easytrade

# OR use Makefile
make clean-all

# Helm cleanup
helm uninstall easytrade -n easytrade
```

## Troubleshooting

### Can't Connect to Cluster

```bash
# Refresh credentials
az aks get-credentials -g prod-cus-platform-base-rg-001 -n prod-cus-aks-sre-lab-003 --overwrite-existing
kubelogin convert-kubeconfig -l azurecli

# Use command invoke for private clusters
az aks command invoke \
    -g prod-cus-platform-base-rg-001 \
    -n prod-cus-aks-sre-lab-003 \
    --command "kubectl get pods -n easytrade"
```

### Pods Stuck in Pending

```bash
# Check events
kubectl describe pod <pod-name> -n easytrade

# Check node resources
kubectl top nodes

# Check resource quotas
kubectl describe resourcequota -n easytrade
```

### Image Pull Errors

```bash
# Verify ACR attachment
az aks check-acr \
    -n prod-cus-aks-sre-lab-003 \
    -g prod-cus-platform-base-rg-001 \
    --acr prodcentralimagerepo.azurecr.io

# Re-attach ACR
az aks update \
    -n prod-cus-aks-sre-lab-003 \
    -g prod-cus-platform-base-rg-001 \
    --attach-acr prod-central-image-repo
```

### Database Connection Issues

```bash
# Check database pod
kubectl get pods -n easytrade -l app=db

# Check database logs
kubectl logs -n easytrade -l app=db --tail=50

# Restart dependent services
kubectl rollout restart deployment/contentcreator -n easytrade
kubectl rollout restart deployment/broker-service -n easytrade
```

## Advanced: Using Makefile Targets

```bash
# View all available targets
make help

# Common workflows
make verify-tools         # Verify tool installations
make configure-aks        # Configure AKS access
make attach-acr          # Attach ACR to cluster
make mirror-images       # Mirror images to ACR
make deploy-manifests    # Deploy with manifests
make deploy-helm         # Deploy with Helm
make get-access-info     # Show all access methods
make logs                # Tail all logs
make pods                # List all pods
make top                 # Show resource usage
make clean-all           # Complete cleanup
```

## Next Steps

1. **Configure Monitoring**: Setup Dynatrace, Prometheus, or Azure Monitor
2. **Test Problem Patterns**: Use feature flags for chaos engineering
3. **Setup CI/CD**: Integrate with GitHub Actions or Azure Pipelines
4. **Production Hardening**: Apply network policies, resource limits, PodDisruptionBudgets
5. **Backup Configuration**: Export and version control your configurations

## Resources

- [EasyTrade GitHub](https://github.com/Dynatrace/easytrade)
- [Feature Flags Guide](./feature-flags-guide.md)
- [Troubleshooting Guide](./troubleshooting.md)
- [AKS Documentation](https://learn.microsoft.com/en-us/azure/aks/)
