# Complete EasyTrade Deployment Examples

This guide provides actual command examples for every deployment scenario.

## 📋 Table of Contents

- [Ubuntu Deployment Examples](#ubuntu-deployment-examples)
- [Windows Deployment Examples](#windows-deployment-examples)
- [Manual Deployment Examples](#manual-deployment-examples)
- [Helm Deployment Examples](#helm-deployment-examples)
- [Terraform Deployment Examples](#terraform-deployment-examples)
- [Feature Flags Examples](#feature-flags-examples)
- [Troubleshooting Examples](#troubleshooting-examples)

---

## Ubuntu Deployment Examples

### Complete Automated Deployment

```bash
# Clone repository
git clone https://github.com/your-org/easytrade-deployment.git
cd easytrade-deployment

# Create .env file
cp .env.example .env
vim .env  # Update with your values

# Install all tools
make ubuntu-install-tools

# Verify installation
make verify-tools

# Configure Azure and AKS
make azure-login
make configure-aks
make attach-acr
make verify-acr

# Mirror images to ACR (optional but recommended)
make acr-login
make mirror-images
make list-acr-images

# Deploy EasyTrade with manifests
make create-namespace
make deploy-manifests USE_ACR_IMAGES=true

# Wait and verify
kubectl -n easytrade get pods -w
make verify-deployment

# Setup access
make setup-portforward-background

# Open in browser
firefox http://localhost:8080 &

# Test feature flags
make feature-flags
./scripts/feature-flags.sh enable high_cpu_usage

# Watch CPU spike
watch -n 2 'kubectl top pods -n easytrade -l app=broker-service'

# Disable pattern
./scripts/feature-flags.sh disable high_cpu_usage
```

### Manual Step-by-Step

```bash
# 1. Install Azure CLI
curl -sL https://aka.ms/InstallAzureCLIDeb | sudo bash

# 2. Install kubectl and kubelogin
sudo az aks install-cli

# 3. Install Helm
curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash

# 4. Install Docker
sudo apt-get update
sudo apt-get install -y docker.io
sudo usermod -aG docker $USER
newgrp docker

# 5. Azure login
az login
az account set --subscription "tango-CICD-platform-github-gitflow"

# 6. Get AKS credentials
az aks get-credentials \
    --resource-group "prod-cus-platform-base-rg-001" \
    --name "prod-cus-aks-sre-lab-003" \
    --overwrite-existing

# 7. Configure kubelogin
kubelogin convert-kubeconfig -l azurecli

# 8. Test connectivity
kubectl get nodes

# 9. Create namespace
kubectl create namespace easytrade
kubectl label namespace easytrade istio-injection=enabled

# 10. Deploy database
kubectl -n easytrade apply -f kubernetes/database.yaml
kubectl -n easytrade wait --for=condition=ready pod -l app=db --timeout=300s

# 11. Deploy services
kubectl -n easytrade apply -f kubernetes/services/

# 12. Wait for rollout
kubectl -n easytrade rollout status deployment --timeout=600s

# 13. Verify
kubectl -n easytrade get pods
kubectl -n easytrade get svc

# 14. Port-forward
kubectl -n easytrade port-forward svc/frontendreverseproxy 8080:80

# Access at http://localhost:8080
```

---

## Windows Deployment Examples

### Complete PowerShell Deployment

```powershell
# Clone repository
git clone https://github.com/your-org/easytrade-deployment.git
cd easytrade-deployment

# Create .env file
Copy-Item .env.example .env.ps1
notepad .env.ps1  # Update with your values

# Install all tools (non-admin)
.\scripts\windows\01-Install-Tools.ps1 -UseScoop -Verbose

# Close and reopen terminal for PATH refresh

# Configure Azure and AKS
.\scripts\windows\02-Configure-AKS.ps1 `
    -SubscriptionName "tango-CICD-platform-github-gitflow" `
    -ResourceGroup "prod-cus-platform-base-rg-001" `
    -ClusterName "prod-cus-aks-sre-lab-003" `
    -ACRName "prod-central-image-repo"

# Deploy with manifests
.\scripts\windows\03-Deploy-EasyTrade.ps1 `
    -DeploymentType Manifests `
    -UseACRImages $true `
    -Verbose

# Setup access
.\scripts\windows\04-Setup-Access.ps1 -AccessMethod PortForward

# Test feature flags
.\scripts\feature-flags.ps1 -Action List
.\scripts\feature-flags.ps1 -Action Enable -Pattern db_not_responding

# Check pods
kubectl get pods -n easytrade -w

# Disable pattern
.\scripts\feature-flags.ps1 -Action Disable -Pattern db_not_responding
```

### Manual PowerShell Steps

```powershell
# 1. Install Scoop
Set-ExecutionPolicy RemoteSigned -Scope CurrentUser
Invoke-RestMethod -Uri https://get.scoop.sh | Invoke-Expression

# 2. Install tools
scoop install azure-cli kubectl helm git jq

# 3. Install kubelogin
$LocalBin = "$env:LOCALAPPDATA\bin"
New-Item -ItemType Directory -Path $LocalBin -Force
$KubeloginUrl = "https://github.com/Azure/kubelogin/releases/download/v0.1.4/kubelogin-win-amd64.zip"
Invoke-WebRequest -Uri $KubeloginUrl -OutFile "$env:TEMP\kubelogin.zip"
Expand-Archive -Path "$env:TEMP\kubelogin.zip" -DestinationPath $env:TEMP -Force
Move-Item -Path "$env:TEMP\bin\windows_amd64\kubelogin.exe" -Destination "$LocalBin\kubelogin.exe" -Force

# 4. Add to PATH
$env:Path = "$env:Path;$LocalBin"
[Environment]::SetEnvironmentVariable("Path", "$env:Path;$LocalBin", "User")

# 5. Azure login
az login --use-device-code
az account set --subscription "tango-CICD-platform-github-gitflow"

# 6. Get AKS credentials
az aks get-credentials `
    --resource-group "prod-cus-platform-base-rg-001" `
    --name "prod-cus-aks-sre-lab-003" `
    --overwrite-existing

# 7. Configure kubelogin
kubelogin convert-kubeconfig -l azurecli

# 8. Test connectivity
kubectl get nodes

# 9. Deploy
kubectl create namespace easytrade
kubectl -n easytrade apply -f .\kubernetes\database.yaml
kubectl -n easytrade wait --for=condition=ready pod -l app=db --timeout=300s
kubectl -n easytrade apply -f .\kubernetes\services\

# 10. Port-forward
kubectl -n easytrade port-forward svc/frontendreverseproxy 8080:80

# Access at http://localhost:8080
```

---

## Manual Deployment Examples

### Using Pure Kubernetes Manifests

```bash
# Clone EasyTrade repository
git clone https://github.com/Dynatrace/easytrade.git
cd easytrade

# Create namespace
kubectl create namespace easytrade
kubectl label namespace easytrade istio-injection=enabled

# Update image registry (if using ACR)
find kubernetes-manifests/release -name "*.yaml" -exec \
    sed -i 's|europe-docker.pkg.dev/dynatrace-demoability/docker/easytrade|prodcentralimagerepo.azurecr.io/easytrade|g' {} \;

# Deploy
kubectl -n easytrade apply -f kubernetes-manifests/release/

# Wait
kubectl -n easytrade wait --for=condition=available --timeout=600s deployment --all

# Verify
kubectl -n easytrade get pods
kubectl -n easytrade get svc
```

### Building Custom Images

```bash
# Build locally
cd easytrade/src/broker-service
docker build -t prodcentralimagerepo.azurecr.io/easytrade/broker-service:custom .
docker push prodcentralimagerepo.azurecr.io/easytrade/broker-service:custom

# Update deployment
kubectl set image deployment/broker-service \
    broker-service=prodcentralimagerepo.azurecr.io/easytrade/broker-service:custom \
    -n easytrade

# OR build on ACR directly
az acr build \
    --registry prod-central-image-repo \
    --image easytrade/broker-service:$(git rev-parse --short HEAD) \
    --file ./Dockerfile \
    .
```

---

## Helm Deployment Examples

### Basic Helm Deployment

```bash
# Install with default values
helm install easytrade ./helm/easytrade \
    --namespace easytrade \
    --create-namespace \
    --wait --timeout 10m

# Verify
helm list -n easytrade
helm status easytrade -n easytrade

# Get values
helm get values easytrade -n easytrade
```

### Production Helm Deployment

```bash
# Install with production values
helm install easytrade ./helm/easytrade \
    --namespace easytrade \
    --create-namespace \
    --values ./helm/easytrade/values-production.yaml \
    --set global.imageRegistry=prodcentralimagerepo.azurecr.io \
    --wait --timeout 10m

# Or using Makefile
make deploy-helm-production
```

### Helm Upgrades

```bash
# Upgrade deployment
helm upgrade easytrade ./helm/easytrade \
    --namespace easytrade \
    --set brokerService.replicaCount=5 \
    --wait

# Rollback
helm rollback easytrade 1 --namespace easytrade

# View history
helm history easytrade -n easytrade
```

### Custom Values

```yaml
# custom-values.yaml
global:
  imageRegistry: prodcentralimagerepo.azurecr.io
  namespace: easytrade-prod

brokerService:
  replicaCount: 5
  resources:
    requests:
      cpu: "500m"
      memory: "512Mi"
    limits:
      cpu: "2"
      memory: "2Gi"

database:
  persistence:
    size: 50Gi
    storageClass: managed-csi-premium

ingress:
  loadBalancer:
    enabled: true
```

```bash
# Deploy with custom values
helm install easytrade ./helm/easytrade \
    --namespace easytrade-prod \
    --create-namespace \
    --values custom-values.yaml \
    --wait
```

---

## Terraform Deployment Examples

### Basic Terraform Deployment

```bash
# Initialize
cd terraform
terraform init

# Create tfvars
cp terraform.tfvars.example terraform.tfvars
vim terraform.tfvars  # Update values

# Plan
terraform plan -out=tfplan

# Apply
terraform apply tfplan

# OR using Makefile
make tf-init
make tf-plan
make tf-apply
```

### Terraform with Helm

```hcl
# terraform.tfvars
subscription_id      = "your-subscription-id"
resource_group_name = "prod-cus-platform-base-rg-001"
cluster_name        = "prod-cus-aks-sre-lab-003"
acr_name           = "prod-central-image-repo"
namespace           = "easytrade"
deploy_via_helm     = true
use_acr_images      = true

enable_istio            = true
enable_autoscaling      = true
enable_network_policies = true
```

```bash
# Deploy
terraform apply -auto-approve

# Get outputs
terraform output
terraform output kubectl_config_command
terraform output port_forward_command

# Destroy
terraform destroy -auto-approve
```

---

## Feature Flags Examples

### Bash Examples

```bash
# List all flags
./scripts/feature-flags.sh list

# Get specific flag
./scripts/feature-flags.sh get db_not_responding

# Enable problem pattern
./scripts/feature-flags.sh enable high_cpu_usage

# Monitor impact
watch -n 2 'kubectl top pods -n easytrade -l app=broker-service'

# Check logs
kubectl logs -n easytrade -l app=broker-service --tail=50 -f

# Disable pattern
./scripts/feature-flags.sh disable high_cpu_usage
```

### PowerShell Examples

```powershell
# List all flags
.\scripts\feature-flags.ps1 -Action List

# Enable pattern
.\scripts\feature-flags.ps1 -Action Enable -Pattern factory_crisis

# Check status
.\scripts\feature-flags.ps1 -Action Get -Pattern factory_crisis

# Monitor
kubectl top pods -n easytrade

# Disable
.\scripts\feature-flags.ps1 -Action Disable -Pattern factory_crisis
```

### Manual API Calls

```bash
# Set variables
EASYTRADE_URL="http://localhost:8080"

# List flags
curl -s "${EASYTRADE_URL}/feature-flag-service/v1/flags" | jq

# Enable pattern
curl -X PUT "${EASYTRADE_URL}/feature-flag-service/v1/flags/ergo_aggregator_slowdown/" \
    -H "Content-Type: application/json" \
    -d '{"enabled": true}'

# Check response times
for i in {1..10}; do
    time curl -s "${EASYTRADE_URL}/api/trade" > /dev/null
    sleep 1
done

# Disable pattern
curl -X PUT "${EASYTRADE_URL}/feature-flag-service/v1/flags/ergo_aggregator_slowdown/" \
    -H "Content-Type: application/json" \
    -d '{"enabled": false}'
```

---

## Troubleshooting Examples

### Pod Issues

```bash
# Check pod status
kubectl -n easytrade get pods
kubectl -n easytrade describe pod <pod-name>
kubectl -n easytrade logs <pod-name> --tail=100 -f

# Check previous logs (if crashed)
kubectl -n easytrade logs <pod-name> --previous

# Get into pod shell
kubectl -n easytrade exec -it <pod-name> -- /bin/sh

# Check container logs (multi-container pod)
kubectl -n easytrade logs <pod-name> -c <container-name>
```

### Network Issues

```bash
# Check services
kubectl -n easytrade get svc
kubectl -n easytrade get endpoints

# Test DNS
kubectl -n easytrade run test-dns --rm -it --image=busybox -- \
    nslookup broker-service.easytrade.svc.cluster.local

# Test connectivity
kubectl -n easytrade run test-curl --rm -it --image=curlimages/curl -- \
    curl -v http://broker-service:8080/actuator/health

# Check network policies
kubectl -n easytrade get networkpolicies
```

### Image Pull Issues

```bash
# Check imagePullSecrets
kubectl -n easytrade get secrets

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

# Manual image pull test
kubectl -n easytrade run test-pull --rm -it \
    --image=prodcentralimagerepo.azurecr.io/easytrade/broker-service:latest \
    -- /bin/sh
```

### Database Issues

```bash
# Check database pod
kubectl -n easytrade get pod -l app=db
kubectl -n easytrade logs -l app=db --tail=50

# Test database connection
kubectl -n easytrade exec -it deploy/broker-service -- \
    sh -c 'nc -zv db 1433 && echo "DB connection OK"'

# Check database initialization
kubectl -n easytrade logs -l app=contentcreator --tail=100

# Restart database-dependent services
kubectl -n easytrade rollout restart deployment/contentcreator
kubectl -n easytrade rollout restart deployment/broker-service
```

### Performance Issues

```bash
# Check resource usage
kubectl top nodes
kubectl top pods -n easytrade

# Check resource quotas
kubectl describe resourcequota -n easytrade
kubectl describe limitrange -n easytrade

# Check HPA (if enabled)
kubectl get hpa -n easytrade
kubectl describe hpa <hpa-name> -n easytrade

# Check events
kubectl get events -n easytrade --sort-by='.lastTimestamp' | tail -30
```

---

## Complete End-to-End Example

```bash
#!/bin/bash
# Complete deployment and testing workflow

set -e

echo "=== Step 1: Setup ==="
cd easytrade-deployment
cp .env.example .env
# Edit .env with your values

echo "=== Step 2: Install Tools ==="
make ubuntu-install-tools
make verify-tools

echo "=== Step 3: Configure Azure ==="
make azure-login
make configure-aks
make attach-acr

echo "=== Step 4: Mirror Images ==="
make mirror-images
make list-acr-images

echo "=== Step 5: Deploy ==="
make create-namespace
make deploy-manifests USE_ACR_IMAGES=true

echo "=== Step 6: Verify ==="
./scripts/health-check.sh --verbose

echo "=== Step 7: Setup Access ==="
make setup-loadbalancer
LB_IP=$(kubectl -n easytrade get svc easytrade-lb -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
echo "EasyTrade URL: http://${LB_IP}"

echo "=== Step 8: Test Feature Flags ==="
./scripts/feature-flags.sh list
./scripts/feature-flags.sh enable high_cpu_usage
sleep 60
kubectl top pods -n easytrade -l app=broker-service
./scripts/feature-flags.sh disable high_cpu_usage

echo "=== Step 9: Run Tests ==="
./scripts/test-deployment.sh --verbose

echo "=== Deployment Complete ==="
echo "Access EasyTrade at: http://${LB_IP}"
echo "Default credentials: demouser/demopass"
```

---

This examples guide provides actual, working commands for every scenario. Copy and customize as needed for your environment!
