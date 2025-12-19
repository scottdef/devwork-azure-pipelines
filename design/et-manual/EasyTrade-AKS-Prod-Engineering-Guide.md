# Deploying EasyTrade on Private AKS: A Production Engineering Guide

Dynatrace EasyTrade is a microservices-based stock trading demo application designed for observability showcases. This guide provides battle-tested procedures for deploying EasyTrade to a private Azure Kubernetes Service cluster with enterprise-grade CI/CD automation, Istio service mesh integration, and comprehensive troubleshooting capabilities. **EasyTrade ships with raw Kubernetes manifests rather than Helm charts**, so this guide covers both native manifest deployment and creating a custom Helm wrapper for production use.

The environment targets cluster `prod-cus-aks-sre-lab-003` in resource group `prod-cus-platform-base-rg-001` using ACR `prodcentralimagerepo.azurecr.io` for image storage.

---

## Prerequisites and environment setup

### Required tooling installation on Ubuntu 24.04

Install the complete toolchain for AKS operations:

```bash
#!/bin/bash
# install-prerequisites.sh

# Azure CLI
curl -sL https://aka.ms/InstallAzureCLIDeb | sudo bash

# kubectl and kubelogin via Azure CLI
sudo az aks install-cli

# Helm 3
curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash

# Docker
sudo apt-get update
sudo apt-get install -y ca-certificates curl gnupg
sudo install -m 0755 -d /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /etc/apt/keyrings/docker.gpg
echo "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu $(. /etc/os-release && echo "$VERSION_CODENAME") stable" | sudo tee /etc/apt/sources.list.d/docker.list > /dev/null
sudo apt-get update
sudo apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin

# Git and utilities
sudo apt-get install -y git curl jq make

# Verify installations
az --version | head -1
kubectl version --client --short 2>/dev/null || kubectl version --client
kubelogin --version
helm version --short
docker --version
```

### Private AKS cluster authentication

Private clusters require **kubelogin** since the API server endpoint is only accessible via private IP. Configure authentication using the Azure CLI login mode:

```bash
# Set environment variables for your deployment
export SUBSCRIPTION="tango-CICD-platform-github-gitflow"
export RESOURCE_GROUP="prod-cus-platform-base-rg-001"
export CLUSTER_NAME="prod-cus-aks-sre-lab-003"
export ACR_NAME="prod-central-image-repo"
export ACR_LOGIN_SERVER="prodcentralimagerepo.azurecr.io"

# Authenticate to Azure
az login
az account set --subscription "$SUBSCRIPTION"

# Get cluster credentials (automatically uses exec format for kubelogin)
az aks get-credentials \
    --resource-group "$RESOURCE_GROUP" \
    --name "$CLUSTER_NAME" \
    --overwrite-existing

# Convert kubeconfig to use Azure CLI authentication
kubelogin convert-kubeconfig -l azurecli

# Verify connectivity
kubectl get nodes
```

For CI/CD pipelines using service principals, use the `spn` login mode:

```bash
export AAD_SERVICE_PRINCIPAL_CLIENT_ID="<client-id>"
export AAD_SERVICE_PRINCIPAL_CLIENT_SECRET="<client-secret>"

az login --service-principal \
    -u "$AAD_SERVICE_PRINCIPAL_CLIENT_ID" \
    -p "$AAD_SERVICE_PRINCIPAL_CLIENT_SECRET" \
    --tenant "<tenant-id>"

az aks get-credentials -g "$RESOURCE_GROUP" -n "$CLUSTER_NAME"
kubelogin convert-kubeconfig -l spn
```

### ACR integration and image pull permissions

Attach ACR to AKS for seamless image pulls without explicit credentials:

```bash
# Attach ACR to AKS (assigns AcrPull role to kubelet identity)
az aks update \
    --name "$CLUSTER_NAME" \
    --resource-group "$RESOURCE_GROUP" \
    --attach-acr "$ACR_NAME"

# Verify ACR connectivity
az aks check-acr \
    --name "$CLUSTER_NAME" \
    --resource-group "$RESOURCE_GROUP" \
    --acr "${ACR_LOGIN_SERVER}"

# Manual role assignment if needed
KUBELET_ID=$(az aks show -g "$RESOURCE_GROUP" -n "$CLUSTER_NAME" \
    --query identityProfile.kubeletidentity.clientId -o tsv)
ACR_ID=$(az acr show --name "$ACR_NAME" --query id -o tsv)
az role assignment create --assignee "$KUBELET_ID" --role "acrpull" --scope "$ACR_ID"
```

---

## Installation using Kubernetes manifests

### Cloning the repository and namespace setup

```bash
git clone https://github.com/Dynatrace/easytrade.git
cd easytrade

# Create dedicated namespace with resource quotas
kubectl create namespace easytrade

# Apply resource quota for production workloads
cat <<EOF | kubectl apply -f -
apiVersion: v1
kind: ResourceQuota
metadata:
  name: easytrade-quota
  namespace: easytrade
spec:
  hard:
    requests.cpu: "8"
    requests.memory: "16Gi"
    limits.cpu: "16"
    limits.memory: "32Gi"
    persistentvolumeclaims: "5"
EOF
```

### Deploying with native manifests

Deploy the core application stack:

```bash
# Deploy all release manifests
kubectl -n easytrade apply -f ./kubernetes-manifests/release

# Monitor deployment rollout
kubectl -n easytrade rollout status deployment --timeout=300s

# Optional: Deploy problem pattern CronJobs (enables patterns once daily)
kubectl -n easytrade apply -f ./kubernetes-manifests/problem-patterns

# Watch pod status until stable
watch -n 2 'kubectl -n easytrade get pods'
```

### Retagging images to private ACR

For private cluster deployments, mirror the upstream images to your ACR:

```bash
#!/bin/bash
# mirror-images.sh

SOURCE_REGISTRY="europe-docker.pkg.dev/dynatrace-demoability/docker/easytrade"
TARGET_REGISTRY="prodcentralimagerepo.azurecr.io/easytrade"

# List of EasyTrade services
SERVICES=(
    "accountservice"
    "broker-service"
    "contentcreator"
    "credit-card-order-service"
    "engine"
    "factory"
    "feature-flag-service"
    "frontend"
    "frontendreverseproxy"
    "headless-loadgen"
    "loginservice"
    "manager"
    "offerservice"
    "pricingservice"
    "thirdpartyservice"
)

# Login to ACR
az acr login --name prod-central-image-repo

for SERVICE in "${SERVICES[@]}"; do
    echo "Importing ${SERVICE}..."
    az acr import \
        --name prod-central-image-repo \
        --source "${SOURCE_REGISTRY}/${SERVICE}:latest" \
        --image "easytrade/${SERVICE}:latest" \
        --force
done

echo "Image import complete."
```

Update manifests to reference your ACR:

```bash
# Patch all deployments to use private ACR
for DEPLOYMENT in $(kubectl -n easytrade get deployments -o name); do
    kubectl -n easytrade patch "$DEPLOYMENT" --type='json' -p='[
        {"op": "replace", "path": "/spec/template/spec/containers/0/image", 
         "value": "prodcentralimagerepo.azurecr.io/easytrade/'$(basename $DEPLOYMENT)':latest"}
    ]' 2>/dev/null || true
done
```

---

## Creating a custom Helm chart for EasyTrade

Since EasyTrade lacks an official Helm chart, create a wrapper chart for production deployment flexibility.

### Chart structure

```
easytrade-helm/
├── Chart.yaml
├── values.yaml
├── values-production.yaml
├── templates/
│   ├── _helpers.tpl
│   ├── namespace.yaml
│   ├── configmap.yaml
│   ├── database.yaml
│   ├── services.yaml
│   ├── deployments.yaml
│   ├── hpa.yaml
│   └── networkpolicy.yaml
```

### Minimal values.yaml

```yaml
# values.yaml - EasyTrade Helm Chart Configuration

global:
  namespace: easytrade
  imageRegistry: prodcentralimagerepo.azurecr.io
  imagePullPolicy: IfNotPresent
  
labels:
  app.kubernetes.io/name: easytrade
  app.kubernetes.io/managed-by: Helm

# Database configuration
database:
  enabled: true
  image: mcr.microsoft.com/mssql/server:2022-latest
  resources:
    requests:
      cpu: "500m"
      memory: "2Gi"
    limits:
      cpu: "2"
      memory: "4Gi"
  persistence:
    enabled: true
    size: 10Gi
    storageClass: managed-csi

# Frontend reverse proxy (NGINX)
frontendreverseproxy:
  enabled: true
  replicaCount: 2
  image:
    repository: easytrade/frontendreverseproxy
    tag: latest
  service:
    type: ClusterIP
    port: 80
  resources:
    requests:
      cpu: "100m"
      memory: "128Mi"
    limits:
      cpu: "500m"
      memory: "256Mi"

# Core services configuration
services:
  accountservice:
    enabled: true
    replicaCount: 2
    port: 8089
    resources:
      requests:
        cpu: "200m"
        memory: "256Mi"
      limits:
        cpu: "1"
        memory: "512Mi"
  
  brokerservice:
    enabled: true
    replicaCount: 2
    port: 8080
    env:
      FEATURE_FLAG_CACHE_DURATION_S: "30"
      HIGH_CPU_USAGE_REQUEST_DELAY_MS: "1000"
      HIGH_CPU_USAGE_CONCURRENCY: "4"
    resources:
      requests:
        cpu: "200m"
        memory: "256Mi"
      limits:
        cpu: "1"
        memory: "512Mi"
  
  featureflagservice:
    enabled: true
    replicaCount: 1
    port: 8080
    resources:
      requests:
        cpu: "100m"
        memory: "128Mi"
      limits:
        cpu: "500m"
        memory: "256Mi"
  
  offerservice:
    enabled: true
    replicaCount: 2
    port: 8087
    resources:
      requests:
        cpu: "200m"
        memory: "256Mi"
      limits:
        cpu: "1"
        memory: "512Mi"
  
  pricingservice:
    enabled: true
    replicaCount: 2
    port: 8080
    resources:
      requests:
        cpu: "200m"
        memory: "256Mi"
      limits:
        cpu: "1"
        memory: "512Mi"
  
  loginservice:
    enabled: true
    replicaCount: 2
    port: 8080
    resources:
      requests:
        cpu: "200m"
        memory: "256Mi"
      limits:
        cpu: "1"
        memory: "512Mi"

# Load generator (optional)
loadgen:
  enabled: false
  replicaCount: 1

# Problem patterns CronJobs
problemPatterns:
  enabled: false
  schedule: "0 8 * * *"  # Daily at 8 AM UTC
```

### Production values override

```yaml
# values-production.yaml

global:
  namespace: easytrade
  imageRegistry: prodcentralimagerepo.azurecr.io
  imagePullPolicy: Always

# Production replica counts
frontendreverseproxy:
  replicaCount: 3
  resources:
    requests:
      cpu: "250m"
      memory: "256Mi"
    limits:
      cpu: "1"
      memory: "512Mi"

services:
  accountservice:
    replicaCount: 3
    resources:
      requests:
        cpu: "500m"
        memory: "512Mi"
      limits:
        cpu: "2"
        memory: "1Gi"
  
  brokerservice:
    replicaCount: 3
    resources:
      requests:
        cpu: "500m"
        memory: "512Mi"
      limits:
        cpu: "2"
        memory: "1Gi"

# Database with production storage
database:
  persistence:
    size: 50Gi
    storageClass: managed-csi-premium
  resources:
    requests:
      cpu: "1"
      memory: "4Gi"
    limits:
      cpu: "4"
      memory: "8Gi"

# Horizontal Pod Autoscaling
autoscaling:
  enabled: true
  minReplicas: 2
  maxReplicas: 10
  targetCPUUtilization: 70
  targetMemoryUtilization: 80

# Network policies
networkPolicy:
  enabled: true
  
# Pod disruption budgets
podDisruptionBudget:
  enabled: true
  minAvailable: 1
```

### Helm deployment commands

```bash
# Install with default values
helm install easytrade ./easytrade-helm \
    --namespace easytrade \
    --create-namespace

# Install with production values
helm install easytrade ./easytrade-helm \
    --namespace easytrade \
    --create-namespace \
    -f ./easytrade-helm/values-production.yaml

# Upgrade existing deployment
helm upgrade easytrade ./easytrade-helm \
    --namespace easytrade \
    -f ./easytrade-helm/values-production.yaml \
    --set services.brokerservice.replicaCount=5

# Rollback on failure
helm rollback easytrade 1 --namespace easytrade
```

---

## Post-deployment verification and health checks

### Service health validation

```bash
#!/bin/bash
# verify-deployment.sh

NAMESPACE="easytrade"

echo "=== Pod Status ==="
kubectl -n $NAMESPACE get pods -o wide

echo -e "\n=== Service Endpoints ==="
kubectl -n $NAMESPACE get svc

echo -e "\n=== Deployment Readiness ==="
kubectl -n $NAMESPACE get deployments -o custom-columns=\
'NAME:.metadata.name,READY:.status.readyReplicas,DESIRED:.spec.replicas,AVAILABLE:.status.availableReplicas'

echo -e "\n=== Recent Events ==="
kubectl -n $NAMESPACE get events --sort-by='.lastTimestamp' | tail -20

echo -e "\n=== Health Check Endpoints ==="
# Port-forward to check internal health
kubectl -n $NAMESPACE port-forward svc/frontendreverseproxy 8080:80 &
PF_PID=$!
sleep 3
curl -s http://localhost:8080/ | head -20
kill $PF_PID 2>/dev/null
```

### Readiness probe verification

```bash
# Check readiness probe configurations
kubectl -n easytrade get pods -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.spec.containers[*].readinessProbe}{"\n"}{end}'

# Test individual service health endpoints
kubectl -n easytrade exec -it deploy/broker-service -- curl -s localhost:8080/actuator/health
kubectl -n easytrade exec -it deploy/accountservice -- curl -s localhost:8089/api/health
```

### Database connectivity check

```bash
# Verify database is accepting connections
kubectl -n easytrade exec -it deploy/broker-service -- \
    sh -c 'nc -zv db 1433 && echo "Database connection successful"'

# Check ContentCreator has populated data
kubectl -n easytrade logs -l app=contentcreator --tail=50
```

---

## Access methods for EasyTrade UI

### Method 1: kubectl port-forward

The simplest approach for development and debugging:

```bash
# Forward frontend proxy to local port
kubectl -n easytrade port-forward svc/frontendreverseproxy 8080:80

# Access at http://localhost:8080
# Login: demouser/demopass or specialuser/specialpass

# Forward multiple services for API testing
kubectl -n easytrade port-forward svc/frontendreverseproxy 8080:80 &
kubectl -n easytrade port-forward svc/feature-flag-service 8081:8080 &

# Cleanup
pkill -f "port-forward"
```

### Method 2: LoadBalancer service with private IP

For internal network access within your Azure VNet:

```yaml
# private-lb-service.yaml
apiVersion: v1
kind: Service
metadata:
  name: easytrade-private-lb
  namespace: easytrade
  annotations:
    service.beta.kubernetes.io/azure-load-balancer-internal: "true"
    service.beta.kubernetes.io/azure-load-balancer-internal-subnet: "aks-internal-subnet"
spec:
  type: LoadBalancer
  selector:
    app: frontendreverseproxy
  ports:
    - name: http
      port: 80
      targetPort: 80
    - name: https
      port: 443
      targetPort: 443
```

```bash
kubectl apply -f private-lb-service.yaml

# Get assigned internal IP
kubectl -n easytrade get svc easytrade-private-lb \
    -o jsonpath='{.status.loadBalancer.ingress[0].ip}'
```

### Method 3: Direct pod IP access

For testing from within the cluster network:

```bash
# Get frontend pod IP
FRONTEND_IP=$(kubectl -n easytrade get pod -l app=frontendreverseproxy \
    -o jsonpath='{.items[0].status.podIP}')

# Test from another pod in the cluster
kubectl run -it --rm curl-test --image=curlimages/curl --restart=Never -- \
    curl -s "http://${FRONTEND_IP}/"

# Or from a jump box within the VNet
curl "http://${FRONTEND_IP}/"
```

---

## Istio service mesh integration

### Enabling Istio sidecar injection

```bash
# Label namespace for automatic sidecar injection
kubectl label namespace easytrade istio-injection=enabled

# Restart deployments to inject sidecars
kubectl -n easytrade rollout restart deployment

# Verify sidecars are running
kubectl -n easytrade get pods -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{range .spec.containers[*]}{.name}{" "}{end}{"\n"}{end}'
```

### Gateway and VirtualService configuration

```yaml
# istio-gateway.yaml
apiVersion: networking.istio.io/v1beta1
kind: Gateway
metadata:
  name: easytrade-gateway
  namespace: easytrade
spec:
  selector:
    istio: aks-istio-ingressgateway-internal
  servers:
    - port:
        number: 80
        name: http
        protocol: HTTP
      hosts:
        - "easytrade.internal.company.com"
        - "easytrade.easytrade.svc.cluster.local"
    - port:
        number: 443
        name: https
        protocol: HTTPS
      hosts:
        - "easytrade.internal.company.com"
      tls:
        mode: SIMPLE
        credentialName: easytrade-tls-credential
---
apiVersion: networking.istio.io/v1beta1
kind: VirtualService
metadata:
  name: easytrade-vs
  namespace: easytrade
spec:
  hosts:
    - "easytrade.internal.company.com"
    - "easytrade.easytrade.svc.cluster.local"
  gateways:
    - easytrade-gateway
  http:
    # Feature flag service API
    - match:
        - uri:
            prefix: "/feature-flag-service"
      route:
        - destination:
            host: feature-flag-service.easytrade.svc.cluster.local
            port:
              number: 8080
      timeout: 30s
      retries:
        attempts: 3
        perTryTimeout: 10s
    
    # Broker service API
    - match:
        - uri:
            prefix: "/broker-service"
      route:
        - destination:
            host: broker-service.easytrade.svc.cluster.local
            port:
              number: 8080
      timeout: 60s
    
    # Default route to frontend
    - route:
        - destination:
            host: frontendreverseproxy.easytrade.svc.cluster.local
            port:
              number: 80
```

### mTLS configuration for service-to-service encryption

```yaml
# peer-authentication.yaml
apiVersion: security.istio.io/v1
kind: PeerAuthentication
metadata:
  name: easytrade-mtls
  namespace: easytrade
spec:
  mtls:
    mode: STRICT
  selector:
    matchLabels: {}  # Apply to all workloads
---
apiVersion: networking.istio.io/v1beta1
kind: DestinationRule
metadata:
  name: easytrade-mtls-destination
  namespace: easytrade
spec:
  host: "*.easytrade.svc.cluster.local"
  trafficPolicy:
    tls:
      mode: ISTIO_MUTUAL
    connectionPool:
      tcp:
        maxConnections: 100
      http:
        h2UpgradePolicy: UPGRADE
        http1MaxPendingRequests: 100
    outlierDetection:
      consecutive5xxErrors: 5
      interval: 30s
      baseEjectionTime: 60s
```

```bash
# Apply Istio configurations
kubectl apply -f istio-gateway.yaml
kubectl apply -f peer-authentication.yaml

# Get Istio ingress gateway IP
kubectl -n aks-istio-ingress get svc aks-istio-ingressgateway-internal \
    -o jsonpath='{.status.loadBalancer.ingress[0].ip}'
```

---

## Problem patterns for chaos engineering

EasyTrade includes **four built-in problem patterns** controlled via the Feature Flag Service API. These simulate real-world failure scenarios for observability testing.

### Problem pattern reference

| Pattern ID | Effect | Duration | Use Case |
|------------|--------|----------|----------|
| `db_not_responding` | Database throws errors on Trade table | ~20 min for Dynatrace alert | Database failure simulation |
| `ergo_aggregator_slowdown` | 2 aggregators receive slow responses | 15-30 min | Service degradation testing |
| `factory_crisis` | Factory stops producing credit cards | Persistent until disabled | Supply chain failure |
| `high_cpu_usage` | Broker service CPU spike via Collatz calculations | Persistent | Resource exhaustion testing |

### Feature Flag Service API

```bash
# Get the service endpoint
EASYTRADE_URL="http://$(kubectl -n easytrade get svc frontendreverseproxy -o jsonpath='{.status.loadBalancer.ingress[0].ip}')"

# Or via port-forward
kubectl -n easytrade port-forward svc/frontendreverseproxy 8080:80 &
EASYTRADE_URL="http://localhost:8080"

# List all feature flags
curl -s "${EASYTRADE_URL}/feature-flag-service/v1/flags" | jq

# Enable a problem pattern
curl -X PUT "${EASYTRADE_URL}/feature-flag-service/v1/flags/db_not_responding/" \
    -H "Content-Type: application/json" \
    -d '{"enabled": true}'

# Disable a problem pattern
curl -X PUT "${EASYTRADE_URL}/feature-flag-service/v1/flags/high_cpu_usage/" \
    -H "Content-Type: application/json" \
    -d '{"enabled": false}'

# Get specific flag status
curl -s "${EASYTRADE_URL}/feature-flag-service/v1/flags/ergo_aggregator_slowdown/" | jq
```

### Automated problem pattern script

```bash
#!/bin/bash
# problem-pattern.sh - Toggle EasyTrade problem patterns

EASYTRADE_URL="${EASYTRADE_URL:-http://localhost:8080}"

usage() {
    echo "Usage: $0 <pattern> <enable|disable>"
    echo "Patterns: db_not_responding, ergo_aggregator_slowdown, factory_crisis, high_cpu_usage"
    exit 1
}

[[ $# -ne 2 ]] && usage

PATTERN="$1"
ACTION="$2"

case "$ACTION" in
    enable)  VALUE="true" ;;
    disable) VALUE="false" ;;
    *)       usage ;;
esac

curl -X PUT "${EASYTRADE_URL}/feature-flag-service/v1/flags/${PATTERN}/" \
    -H "Content-Type: application/json" \
    -d "{\"enabled\": ${VALUE}}" && echo -e "\n${PATTERN} ${ACTION}d successfully"
```

---

## Manual CLI command reference

### Azure and cluster operations

```bash
# Switch subscriptions
az account list --output table
az account set --subscription "tango-CICD-platform-github-gitflow"

# Cluster information
az aks show -g prod-cus-platform-base-rg-001 -n prod-cus-aks-sre-lab-003 --query "kubernetesVersion"
az aks nodepool list -g prod-cus-platform-base-rg-001 --cluster-name prod-cus-aks-sre-lab-003 -o table

# Refresh credentials (token expiry)
az aks get-credentials -g prod-cus-platform-base-rg-001 -n prod-cus-aks-sre-lab-003 --overwrite-existing
kubelogin convert-kubeconfig -l azurecli

# Run command on private cluster without direct access
az aks command invoke \
    -g prod-cus-platform-base-rg-001 \
    -n prod-cus-aks-sre-lab-003 \
    --command "kubectl get pods -n easytrade"
```

### kubectl operations

```bash
# Context management
kubectl config get-contexts
kubectl config use-context prod-cus-aks-sre-lab-003
kubectl config current-context

# Namespace operations
kubectl get namespaces
kubectl config set-context --current --namespace=easytrade

# Resource inspection
kubectl -n easytrade get all
kubectl -n easytrade describe pod -l app=broker-service
kubectl -n easytrade logs -l app=broker-service --tail=100 -f
kubectl -n easytrade top pods

# Debugging
kubectl -n easytrade exec -it deploy/broker-service -- /bin/sh
kubectl -n easytrade run debug --rm -it --image=nicolaka/netshoot -- /bin/bash

# Rolling updates
kubectl -n easytrade set image deployment/broker-service broker-service=prodcentralimagerepo.azurecr.io/easytrade/broker-service:v2
kubectl -n easytrade rollout status deployment/broker-service
kubectl -n easytrade rollout undo deployment/broker-service
```

### ACR image management

```bash
# Login to ACR
az acr login --name prod-central-image-repo

# List repositories and tags
az acr repository list --name prod-central-image-repo -o table
az acr repository show-tags --name prod-central-image-repo --repository easytrade/broker-service -o table

# Build and push from local
docker build -t prodcentralimagerepo.azurecr.io/easytrade/broker-service:v2 ./src/broker-service
docker push prodcentralimagerepo.azurecr.io/easytrade/broker-service:v2

# Build directly on ACR (no local Docker needed)
az acr build \
    --registry prod-central-image-repo \
    --image easytrade/broker-service:$(git rev-parse --short HEAD) \
    --file ./src/broker-service/Dockerfile \
    ./src/broker-service

# Import from upstream registry
az acr import \
    --name prod-central-image-repo \
    --source europe-docker.pkg.dev/dynatrace-demoability/docker/easytrade/broker-service:latest \
    --image easytrade/broker-service:latest

# Delete old images
az acr repository delete --name prod-central-image-repo --image easytrade/broker-service:old-tag --yes
```

### Helm operations

```bash
# Repository management
helm repo add dynatrace https://raw.githubusercontent.com/Dynatrace/dynatrace-operator/main/config/helm/repos/stable
helm repo update

# Release management
helm list -n easytrade
helm history easytrade -n easytrade
helm get values easytrade -n easytrade
helm get manifest easytrade -n easytrade

# Template rendering (dry-run)
helm template easytrade ./easytrade-helm -f values-production.yaml --debug

# Uninstall
helm uninstall easytrade -n easytrade
```

---

## GitHub Actions CI/CD workflows

### Build and push to ACR workflow

```yaml
# .github/workflows/build-push.yml
name: Build and Push EasyTrade Images

on:
  push:
    branches: [main]
    paths:
      - 'src/**'
  pull_request:
    branches: [main]
  workflow_dispatch:
    inputs:
      service:
        description: 'Service to build (or "all")'
        required: true
        default: 'all'
        type: choice
        options:
          - all
          - broker-service
          - accountservice
          - frontend

permissions:
  id-token: write
  contents: read

env:
  ACR_NAME: prod-central-image-repo
  ACR_LOGIN_SERVER: prodcentralimagerepo.azurecr.io

jobs:
  detect-changes:
    runs-on: ubuntu-24.04
    outputs:
      services: ${{ steps.detect.outputs.services }}
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 2
      
      - name: Detect changed services
        id: detect
        run: |
          if [[ "${{ github.event.inputs.service }}" == "all" ]] || [[ -z "${{ github.event.inputs.service }}" ]]; then
            CHANGED=$(git diff --name-only HEAD~1 | grep '^src/' | cut -d'/' -f2 | sort -u | jq -R -s -c 'split("\n")[:-1]')
            echo "services=${CHANGED}" >> $GITHUB_OUTPUT
          else
            echo 'services=["${{ github.event.inputs.service }}"]' >> $GITHUB_OUTPUT
          fi

  build:
    needs: detect-changes
    runs-on: ubuntu-24.04
    if: needs.detect-changes.outputs.services != '[]'
    strategy:
      matrix:
        service: ${{ fromJson(needs.detect-changes.outputs.services) }}
      fail-fast: false
    
    steps:
      - uses: actions/checkout@v4

      - name: Azure Login (OIDC)
        uses: azure/login@v2
        with:
          client-id: ${{ secrets.AZURE_CLIENT_ID }}
          tenant-id: ${{ secrets.AZURE_TENANT_ID }}
          subscription-id: ${{ secrets.AZURE_SUBSCRIPTION_ID }}

      - name: Build and Push to ACR
        uses: azure/CLI@v2
        with:
          azcliversion: latest
          inlineScript: |
            az acr build \
              --registry ${{ env.ACR_NAME }} \
              --image easytrade/${{ matrix.service }}:${{ github.sha }} \
              --image easytrade/${{ matrix.service }}:latest \
              --file ./src/${{ matrix.service }}/Dockerfile \
              ./src/${{ matrix.service }}

      - name: Scan image for vulnerabilities
        uses: azure/CLI@v2
        with:
          inlineScript: |
            az acr task run \
              --registry ${{ env.ACR_NAME }} \
              --name scan-${{ matrix.service }} \
              --set IMAGE=easytrade/${{ matrix.service }}:${{ github.sha }} \
              || echo "Scan task not configured, skipping"
```

### Deploy to AKS workflow

```yaml
# .github/workflows/deploy-aks.yml
name: Deploy EasyTrade to AKS

on:
  workflow_dispatch:
    inputs:
      environment:
        description: 'Target environment'
        required: true
        type: choice
        options:
          - dev
          - staging
          - production
      image_tag:
        description: 'Image tag to deploy'
        required: true
        default: 'latest'

permissions:
  id-token: write
  contents: read
  actions: read

env:
  ACR_LOGIN_SERVER: prodcentralimagerepo.azurecr.io
  RESOURCE_GROUP: prod-cus-platform-base-rg-001
  CLUSTER_NAME: prod-cus-aks-sre-lab-003
  NAMESPACE: easytrade

jobs:
  deploy:
    runs-on: ubuntu-24.04
    environment: ${{ github.event.inputs.environment }}
    
    steps:
      - uses: actions/checkout@v4

      - name: Azure Login (OIDC)
        uses: azure/login@v2
        with:
          client-id: ${{ secrets.AZURE_CLIENT_ID }}
          tenant-id: ${{ secrets.AZURE_TENANT_ID }}
          subscription-id: ${{ secrets.AZURE_SUBSCRIPTION_ID }}

      - name: Setup kubectl
        uses: azure/setup-kubectl@v4

      - name: Setup kubelogin
        uses: azure/use-kubelogin@v1.2
        with:
          kubelogin-version: 'v0.1.4'

      - name: Set AKS Context
        uses: azure/aks-set-context@v4
        with:
          resource-group: ${{ env.RESOURCE_GROUP }}
          cluster-name: ${{ env.CLUSTER_NAME }}
          admin: 'false'
          use-kubelogin: 'true'

      - name: Update image tags in manifests
        run: |
          # Update all deployment manifests with new image tag
          find ./kubernetes-manifests/release -name "*.yaml" -exec \
            sed -i "s|europe-docker.pkg.dev/dynatrace-demoability/docker/easytrade|${{ env.ACR_LOGIN_SERVER }}/easytrade|g" {} \;
          
          find ./kubernetes-manifests/release -name "*.yaml" -exec \
            sed -i "s|:latest|:${{ github.event.inputs.image_tag }}|g" {} \;

      - name: Deploy to AKS
        run: |
          kubectl create namespace ${{ env.NAMESPACE }} --dry-run=client -o yaml | kubectl apply -f -
          kubectl -n ${{ env.NAMESPACE }} apply -f ./kubernetes-manifests/release
          
          # Wait for rollout
          for DEPLOY in $(kubectl -n ${{ env.NAMESPACE }} get deployments -o name); do
            kubectl -n ${{ env.NAMESPACE }} rollout status "$DEPLOY" --timeout=300s
          done

      - name: Verify deployment
        run: |
          kubectl -n ${{ env.NAMESPACE }} get pods
          kubectl -n ${{ env.NAMESPACE }} get svc

      - name: Run smoke tests
        run: |
          # Port-forward and test health endpoint
          kubectl -n ${{ env.NAMESPACE }} port-forward svc/frontendreverseproxy 8080:80 &
          sleep 5
          curl -sf http://localhost:8080/ > /dev/null && echo "Health check passed" || exit 1
          pkill -f port-forward
```

### Reusable workflow for Helm deployments

```yaml
# .github/workflows/reusable-helm-deploy.yml
name: Reusable Helm Deploy

on:
  workflow_call:
    inputs:
      environment:
        required: true
        type: string
      helm_release_name:
        required: true
        type: string
      helm_chart_path:
        required: true
        type: string
      values_file:
        required: false
        type: string
        default: ''
      image_tag:
        required: true
        type: string
    secrets:
      AZURE_CLIENT_ID:
        required: true
      AZURE_TENANT_ID:
        required: true
      AZURE_SUBSCRIPTION_ID:
        required: true

jobs:
  helm-deploy:
    runs-on: ubuntu-24.04
    environment: ${{ inputs.environment }}
    
    steps:
      - uses: actions/checkout@v4

      - name: Azure Login
        uses: azure/login@v2
        with:
          client-id: ${{ secrets.AZURE_CLIENT_ID }}
          tenant-id: ${{ secrets.AZURE_TENANT_ID }}
          subscription-id: ${{ secrets.AZURE_SUBSCRIPTION_ID }}

      - name: Setup tools
        run: |
          curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash
          sudo az aks install-cli

      - name: Setup kubelogin
        uses: azure/use-kubelogin@v1.2

      - name: Set AKS Context
        uses: azure/aks-set-context@v4
        with:
          resource-group: prod-cus-platform-base-rg-001
          cluster-name: prod-cus-aks-sre-lab-003
          use-kubelogin: 'true'

      - name: Helm Deploy
        run: |
          VALUES_ARGS=""
          if [[ -n "${{ inputs.values_file }}" ]]; then
            VALUES_ARGS="-f ${{ inputs.values_file }}"
          fi
          
          helm upgrade --install ${{ inputs.helm_release_name }} \
            ${{ inputs.helm_chart_path }} \
            --namespace easytrade \
            --create-namespace \
            $VALUES_ARGS \
            --set global.imageTag=${{ inputs.image_tag }} \
            --wait \
            --timeout 10m
```

---

## Container image management

### Building custom images locally

```bash
# Clone repository
git clone https://github.com/Dynatrace/easytrade.git
cd easytrade

# Build a specific service
docker build -t prodcentralimagerepo.azurecr.io/easytrade/broker-service:custom \
    -f ./src/broker-service/Dockerfile \
    ./src/broker-service

# Build with custom arguments
docker build \
    --build-arg BUILD_DATE=$(date -u +"%Y-%m-%dT%H:%M:%SZ") \
    --build-arg VCS_REF=$(git rev-parse --short HEAD) \
    -t prodcentralimagerepo.azurecr.io/easytrade/broker-service:$(git rev-parse --short HEAD) \
    ./src/broker-service

# Push to ACR
az acr login --name prod-central-image-repo
docker push prodcentralimagerepo.azurecr.io/easytrade/broker-service:custom
```

### Building on ACR without local Docker

```bash
# Build single service
az acr build \
    --registry prod-central-image-repo \
    --image easytrade/broker-service:$(git rev-parse --short HEAD) \
    --file ./src/broker-service/Dockerfile \
    ./src/broker-service

# Build all services using ACR Tasks
for SERVICE_DIR in ./src/*/; do
    SERVICE=$(basename "$SERVICE_DIR")
    if [[ -f "${SERVICE_DIR}/Dockerfile" ]]; then
        echo "Building ${SERVICE}..."
        az acr build \
            --registry prod-central-image-repo \
            --image "easytrade/${SERVICE}:latest" \
            --file "${SERVICE_DIR}/Dockerfile" \
            "$SERVICE_DIR" &
    fi
done
wait
```

### Multi-architecture builds

```bash
# Create buildx builder
docker buildx create --name multiarch --use

# Build multi-arch image
docker buildx build \
    --platform linux/amd64,linux/arm64 \
    -t prodcentralimagerepo.azurecr.io/easytrade/broker-service:multiarch \
    --push \
    ./src/broker-service

# Note: EasyTrade images are NOT officially built for ARM
# This may require code modifications for full ARM support
```

---

## Troubleshooting common issues

### Authentication failures

```bash
# Clear cached tokens
kubelogin remove-tokens
rm -rf ~/.kube/cache/kubelogin/

# Re-authenticate
az login
az aks get-credentials -g prod-cus-platform-base-rg-001 -n prod-cus-aks-sre-lab-003 --overwrite-existing
kubelogin convert-kubeconfig -l azurecli

# Test with az aks command invoke if direct access fails
az aks command invoke \
    -g prod-cus-platform-base-rg-001 \
    -n prod-cus-aks-sre-lab-003 \
    --command "kubectl get nodes"
```

### Image pull errors

```bash
# Verify ACR attachment
az aks check-acr -n prod-cus-aks-sre-lab-003 -g prod-cus-platform-base-rg-001 --acr prodcentralimagerepo.azurecr.io

# Check pod events
kubectl -n easytrade describe pod <pod-name> | grep -A10 Events

# Re-attach ACR
az aks update -n prod-cus-aks-sre-lab-003 -g prod-cus-platform-base-rg-001 --attach-acr prod-central-image-repo
```

### Database startup race condition

The ContentCreator service may fail if SQL Server isn't ready. Verify database readiness:

```bash
# Check database pod status
kubectl -n easytrade get pods -l app=db

# Check database logs
kubectl -n easytrade logs -l app=db --tail=50

# Restart dependent services after database is ready
kubectl -n easytrade rollout restart deployment contentcreator
kubectl -n easytrade rollout restart deployment broker-service
```

### DNS resolution in private cluster

```bash
# Test DNS from within a pod
kubectl -n easytrade run dns-test --rm -it --image=busybox --restart=Never -- nslookup broker-service.easytrade.svc.cluster.local

# Check CoreDNS status
kubectl -n kube-system get pods -l k8s-app=kube-dns
kubectl -n kube-system logs -l k8s-app=kube-dns --tail=50
```

---

## Security and production hardening

Apply network policies to restrict traffic flow:

```yaml
# network-policy.yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: easytrade-default-deny
  namespace: easytrade
spec:
  podSelector: {}
  policyTypes:
    - Ingress
    - Egress
---
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: allow-frontend-ingress
  namespace: easytrade
spec:
  podSelector:
    matchLabels:
      app: frontendreverseproxy
  policyTypes:
    - Ingress
  ingress:
    - from:
        - namespaceSelector:
            matchLabels:
              name: istio-system
      ports:
        - protocol: TCP
          port: 80
---
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: allow-internal-traffic
  namespace: easytrade
spec:
  podSelector: {}
  policyTypes:
    - Ingress
    - Egress
  ingress:
    - from:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: easytrade
  egress:
    - to:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: easytrade
    - to:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: kube-system
      ports:
        - protocol: UDP
          port: 53
```

Run containers as non-root with security contexts applied to all deployments. Enable pod security standards at the namespace level and implement resource quotas to prevent runaway workloads from affecting cluster stability.

---

## Conclusion

EasyTrade provides a comprehensive microservices testbed for observability and chaos engineering scenarios. The key architectural considerations for production deployment include the absence of an official Helm chart (requiring custom chart creation), the SQL Server database dependency that can cause startup race conditions, and the **four problem patterns** that enable controlled failure injection for testing monitoring systems.

For private AKS clusters, the combination of kubelogin with Azure CLI authentication mode provides the most reliable access pattern. Image mirroring to your private ACR eliminates external registry dependencies, while Istio integration enables sophisticated traffic management and mTLS encryption. The GitHub Actions workflows demonstrate both traditional kubectl deployments and Helm-based releases with proper OIDC authentication, avoiding long-lived credentials in CI/CD pipelines.