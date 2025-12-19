# Deploying Kubernetes Dashboard on Private AKS Clusters

Kubernetes Dashboard **v7.14.0** (Helm-only since v7.0.0) provides visual cluster management for your private AKS cluster "prod-cus-aks-sre-lab-003." This guide covers deployment via Helm, Azure AD authentication with kubelogin, Istio integration, and production security hardening—all tailored for CKAs using Ubuntu 24.04 runners with kubectl 1.30.

The shift to Helm-only deployment reflects Dashboard's new multi-container architecture with Kong API gateway, requiring careful configuration for private cluster environments where network isolation and authentication complexity demand precise implementation.

---

## Prerequisites and environment setup

Before deploying, ensure your Ubuntu 24.04 workstation or GitHub runner has the required tooling. Dashboard v7.14.0 works with kubectl 1.30, though **Kubernetes 1.32** offers full compatibility (1.29-1.31 show partial support with potential API version issues).

### Installing required tools on Ubuntu 24.04

```bash
#!/bin/bash
# prereq-install.sh - Install all required tools

# Azure CLI
curl -sL https://aka.ms/InstallAzureCLIDeb | sudo bash

# kubectl and kubelogin via az cli (recommended)
sudo az aks install-cli

# Helm 3.x
curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash

# Verify installations
az version
kubectl version --client
kubelogin --version
helm version

# Docker (for image building)
sudo apt-get update
sudo apt-get install -y docker.io
sudo usermod -aG docker $USER
```

### PowerShell equivalent for hybrid environments

```powershell
# prereq-install.ps1
# Install Azure CLI
Invoke-WebRequest -Uri https://aka.ms/installazurecliwindows -OutFile .\AzureCLI.msi
Start-Process msiexec.exe -Wait -ArgumentList '/I AzureCLI.msi /quiet'

# Install kubectl and kubelogin
az aks install-cli

# Install Helm
choco install kubernetes-helm

# Verify
az version
kubectl version --client
kubelogin --version
```

---

## Connecting to the private AKS cluster

Private AKS clusters use Azure Private Link, making the API server accessible only within the VNet or connected networks. Your cluster "prod-cus-aks-sre-lab-003" requires proper authentication configuration.

### Azure AD authentication with kubelogin

```bash
# Set subscription context
az account set --subscription "tango-CICD-platform-github-gitflow"

# Get credentials for private cluster
az aks get-credentials \
  --resource-group prod-cus-platform-base-rg-001 \
  --name prod-cus-aks-sre-lab-003 \
  --overwrite-existing

# Convert kubeconfig for Azure AD authentication
kubelogin convert-kubeconfig -l azurecli

# Verify cluster access
kubectl get nodes
```

### Service principal authentication for CI/CD pipelines

For automated deployments from GitHub Actions, service principal authentication provides non-interactive access:

```bash
# Create service principal (run once)
az ad sp create-for-rbac --name "sp-aks-dashboard-deploy" --skip-assignment

# Store output values securely:
# - appId → AZURE_CLIENT_ID
# - password → AZURE_CLIENT_SECRET  
# - tenant → AZURE_TENANT_ID

# Get SP object ID for RBAC binding
SP_OBJECT_ID=$(az ad sp show --id <appId> --query "id" -o tsv)

# Assign AKS Cluster User role
az role assignment create \
  --assignee <appId> \
  --role "Azure Kubernetes Service Cluster User Role" \
  --scope "/subscriptions/<sub-id>/resourceGroups/prod-cus-platform-base-rg-001/providers/Microsoft.ContainerService/managedClusters/prod-cus-aks-sre-lab-003"
```

For kubelogin with service principal in scripts:

```bash
# Export credentials
export AAD_SERVICE_PRINCIPAL_CLIENT_ID=<appId>
export AAD_SERVICE_PRINCIPAL_CLIENT_SECRET=<password>

# Convert kubeconfig for SPN auth
kubelogin convert-kubeconfig -l spn

# Now kubectl commands work non-interactively
kubectl get namespaces
```

---

## Deploying Dashboard with Helm

Dashboard v7.x uses a multi-container architecture with **Kong gateway** as the API proxy, making Helm the only supported installation method.

### Adding the Helm repository

```bash
# Add official repository
helm repo add kubernetes-dashboard https://kubernetes.github.io/dashboard/
helm repo update

# Search available versions
helm search repo kubernetes-dashboard --versions | head -10
```

### Minimal deployment for development

```bash
# Development deployment - quick setup
helm upgrade --install kubernetes-dashboard kubernetes-dashboard/kubernetes-dashboard \
  --create-namespace \
  --namespace kubernetes-dashboard \
  --version 7.14.0
```

### Development values file (values-dev.yaml)

```yaml
# helm/values-dev.yaml
app:
  mode: 'dashboard'
  scheduling:
    nodeSelector: {}

kong:
  enabled: true
  proxy:
    type: ClusterIP

api:
  containers:
    resources:
      requests:
        cpu: 100m
        memory: 128Mi
      limits:
        cpu: 250m
        memory: 256Mi

web:
  containers:
    resources:
      requests:
        cpu: 50m
        memory: 64Mi
      limits:
        cpu: 100m
        memory: 128Mi

# Metrics server (skip if already installed)
metrics-server:
  enabled: false
```

### Production values file (values-prod.yaml)

```yaml
# helm/values-prod.yaml
app:
  mode: 'dashboard'
  security:
    containerSecurityContext:
      allowPrivilegeEscalation: false
      readOnlyRootFilesystem: true
      runAsNonRoot: true
      runAsUser: 1001
      runAsGroup: 2001
      capabilities:
        drop:
          - ALL
    podSecurityContext:
      seccompProfile:
        type: RuntimeDefault

kong:
  enabled: true
  proxy:
    type: ClusterIP
  podAnnotations:
    cluster-autoscaler.kubernetes.io/safe-to-evict: "true"

api:
  scaling:
    replicas: 2
  containers:
    resources:
      requests:
        cpu: 200m
        memory: 256Mi
      limits:
        cpu: 500m
        memory: 512Mi

web:
  scaling:
    replicas: 2
  containers:
    resources:
      requests:
        cpu: 100m
        memory: 128Mi
      limits:
        cpu: 250m
        memory: 256Mi

metricsScraper:
  scaling:
    replicas: 1
  containers:
    resources:
      requests:
        cpu: 50m
        memory: 64Mi
      limits:
        cpu: 100m
        memory: 128Mi

# Pod disruption budget
app:
  podDisruptionBudget:
    enabled: true
    minAvailable: 1

metrics-server:
  enabled: false

# Custom annotations for Azure
extras:
  serviceAnnotations:
    service.beta.kubernetes.io/azure-load-balancer-internal: "true"
```

### Production deployment command

```bash
# Production deployment with custom values
helm upgrade --install kubernetes-dashboard kubernetes-dashboard/kubernetes-dashboard \
  --namespace kubernetes-dashboard \
  --create-namespace \
  --version 7.14.0 \
  --values helm/values-prod.yaml \
  --wait \
  --timeout 10m \
  --atomic
```

---

## Using custom images from Azure Container Registry

For air-gapped environments or custom builds, pull images from your ACR "prod-central-image-repo" instead of Docker Hub.

### Importing official images to ACR

```bash
# Login to ACR
az acr login --name prod-central-image-repo

# Import Dashboard images
DASHBOARD_VERSION="1.14.0"
az acr import \
  --name prod-central-image-repo \
  --source docker.io/kubernetesui/dashboard-api:$DASHBOARD_VERSION \
  --image kubernetes-dashboard/dashboard-api:$DASHBOARD_VERSION

az acr import \
  --name prod-central-image-repo \
  --source docker.io/kubernetesui/dashboard-web:1.7.0 \
  --image kubernetes-dashboard/dashboard-web:1.7.0

az acr import \
  --name prod-central-image-repo \
  --source docker.io/kubernetesui/dashboard-auth:1.4.0 \
  --image kubernetes-dashboard/dashboard-auth:1.4.0

az acr import \
  --name prod-central-image-repo \
  --source docker.io/kubernetesui/dashboard-metrics-scraper:1.2.2 \
  --image kubernetes-dashboard/dashboard-metrics-scraper:1.2.2
```

### Values file for ACR images (values-acr.yaml)

```yaml
# helm/values-acr.yaml
app:
  mode: 'dashboard'

# Override image repositories
api:
  containers:
    image:
      repository: prod-central-image-repo.azurecr.io/kubernetes-dashboard/dashboard-api
      tag: "1.14.0"

web:
  containers:
    image:
      repository: prod-central-image-repo.azurecr.io/kubernetes-dashboard/dashboard-web
      tag: "1.7.0"

auth:
  containers:
    image:
      repository: prod-central-image-repo.azurecr.io/kubernetes-dashboard/dashboard-auth
      tag: "1.4.0"

metricsScraper:
  containers:
    image:
      repository: prod-central-image-repo.azurecr.io/kubernetes-dashboard/dashboard-metrics-scraper
      tag: "1.2.2"

# Image pull secret for private ACR
app:
  image:
    pullSecrets:
      - acr-secret
```

### Creating ACR pull secret

```bash
# Create image pull secret
kubectl create secret docker-registry acr-secret \
  --namespace kubernetes-dashboard \
  --docker-server=prod-central-image-repo.azurecr.io \
  --docker-username=$(az acr credential show -n prod-central-image-repo --query username -o tsv) \
  --docker-password=$(az acr credential show -n prod-central-image-repo --query passwords[0].value -o tsv)
```

---

## Three methods for accessing the Dashboard

### Method 1: Port-forward (recommended for private clusters)

The most secure access method, requiring only kubectl connectivity:

```bash
# Standard port-forward
kubectl -n kubernetes-dashboard port-forward svc/kubernetes-dashboard-kong-proxy 8443:443

# Access from local browser
# https://localhost:8443

# Allow external connections (for remote development)
kubectl -n kubernetes-dashboard port-forward svc/kubernetes-dashboard-kong-proxy 8443:443 --address 0.0.0.0
```

**Bash wrapper script:**

```bash
#!/bin/bash
# dashboard-access.sh

NAMESPACE="kubernetes-dashboard"
SERVICE="kubernetes-dashboard-kong-proxy"
LOCAL_PORT="${1:-8443}"

echo "Starting port-forward to Kubernetes Dashboard..."
echo "Access URL: https://localhost:$LOCAL_PORT"
echo "Press Ctrl+C to stop"

kubectl -n $NAMESPACE port-forward svc/$SERVICE $LOCAL_PORT:443
```

### Method 2: NodePort service for internal network access

```yaml
# nodeport-service.yaml
apiVersion: v1
kind: Service
metadata:
  name: dashboard-nodeport
  namespace: kubernetes-dashboard
spec:
  type: NodePort
  selector:
    app.kubernetes.io/name: kong
    app.kubernetes.io/instance: kubernetes-dashboard
  ports:
    - port: 443
      targetPort: 8443
      nodePort: 30443
      protocol: TCP
```

```bash
# Apply NodePort service
kubectl apply -f nodeport-service.yaml

# Get node IP
NODE_IP=$(kubectl get nodes -o jsonpath='{.items[0].status.addresses[?(@.type=="InternalIP")].address}')
echo "Access Dashboard at: https://$NODE_IP:30443"
```

### Method 3: Internal Load Balancer for private IP access

```yaml
# internal-lb-service.yaml
apiVersion: v1
kind: Service
metadata:
  name: dashboard-internal-lb
  namespace: kubernetes-dashboard
  annotations:
    service.beta.kubernetes.io/azure-load-balancer-internal: "true"
    service.beta.kubernetes.io/azure-load-balancer-internal-subnet: "aks-subnet"
spec:
  type: LoadBalancer
  selector:
    app.kubernetes.io/name: kong
    app.kubernetes.io/instance: kubernetes-dashboard
  ports:
    - port: 443
      targetPort: 8443
      protocol: TCP
```

```bash
# Apply internal LB
kubectl apply -f internal-lb-service.yaml

# Wait for IP assignment
kubectl get svc dashboard-internal-lb -n kubernetes-dashboard -w

# Get assigned private IP
PRIVATE_IP=$(kubectl get svc dashboard-internal-lb -n kubernetes-dashboard -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
echo "Dashboard accessible at: https://$PRIVATE_IP"
```

---

## Istio service mesh integration

For clusters running Istio, expose Dashboard through the ingress gateway with proper mTLS configuration.

### Gateway and VirtualService configuration

```yaml
# istio/dashboard-gateway.yaml
apiVersion: networking.istio.io/v1beta1
kind: Gateway
metadata:
  name: dashboard-gateway
  namespace: kubernetes-dashboard
spec:
  selector:
    istio: ingressgateway
  servers:
  - port:
      number: 443
      name: https
      protocol: HTTPS
    tls:
      mode: PASSTHROUGH
    hosts:
    - dashboard.internal.example.com
---
apiVersion: networking.istio.io/v1beta1
kind: VirtualService
metadata:
  name: dashboard-vs
  namespace: kubernetes-dashboard
spec:
  hosts:
  - dashboard.internal.example.com
  gateways:
  - dashboard-gateway
  tls:
  - match:
    - port: 443
      sniHosts:
      - dashboard.internal.example.com
    route:
    - destination:
        host: kubernetes-dashboard-kong-proxy.kubernetes-dashboard.svc.cluster.local
        port:
          number: 443
```

### DestinationRule for traffic policy

```yaml
# istio/dashboard-destination-rule.yaml
apiVersion: networking.istio.io/v1beta1
kind: DestinationRule
metadata:
  name: dashboard-dr
  namespace: kubernetes-dashboard
spec:
  host: kubernetes-dashboard-kong-proxy.kubernetes-dashboard.svc.cluster.local
  trafficPolicy:
    tls:
      mode: DISABLE  # Dashboard handles its own TLS
```

### AuthorizationPolicy for private IP restriction

```yaml
# istio/dashboard-authz-policy.yaml
apiVersion: security.istio.io/v1
kind: AuthorizationPolicy
metadata:
  name: dashboard-private-only
  namespace: istio-system
spec:
  selector:
    matchLabels:
      app: istio-ingressgateway
  action: ALLOW
  rules:
  - from:
    - source:
        ipBlocks:
        - "10.0.0.0/8"
        - "172.16.0.0/12"
        - "192.168.0.0/16"
    to:
    - operation:
        hosts:
        - "dashboard.internal.example.com"
```

### Kubernetes NetworkPolicy for defense in depth

```yaml
# network-policy.yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: dashboard-network-policy
  namespace: kubernetes-dashboard
spec:
  podSelector:
    matchLabels:
      app.kubernetes.io/instance: kubernetes-dashboard
  policyTypes:
  - Ingress
  ingress:
  - from:
    - namespaceSelector:
        matchLabels:
          kubernetes.io/metadata.name: istio-system
      podSelector:
        matchLabels:
          app: istio-ingressgateway
    - namespaceSelector:
        matchLabels:
          kubernetes.io/metadata.name: kubernetes-dashboard
    ports:
    - protocol: TCP
      port: 8443
```

Apply Istio configuration:

```bash
kubectl apply -f istio/dashboard-gateway.yaml
kubectl apply -f istio/dashboard-destination-rule.yaml
kubectl apply -f istio/dashboard-authz-policy.yaml
kubectl apply -f network-policy.yaml
```

---

## Security configuration and RBAC

Dashboard security requires careful RBAC configuration. **Never grant cluster-admin in production** without explicit justification and audit controls.

### Creating admin ServiceAccount with token

```yaml
# rbac/admin-user.yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: dashboard-admin
  namespace: kubernetes-dashboard
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: dashboard-admin-binding
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: cluster-admin
subjects:
- kind: ServiceAccount
  name: dashboard-admin
  namespace: kubernetes-dashboard
```

### Read-only ServiceAccount (recommended for most users)

```yaml
# rbac/readonly-user.yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: dashboard-readonly
  namespace: kubernetes-dashboard
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: dashboard-readonly-binding
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: view
subjects:
- kind: ServiceAccount
  name: dashboard-readonly
  namespace: kubernetes-dashboard
```

### Namespace-scoped admin (production recommended)

```yaml
# rbac/namespace-admin.yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: team-alpha-admin
  namespace: kubernetes-dashboard
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: team-alpha-admin-binding
  namespace: team-alpha  # Only admin in this namespace
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: admin
subjects:
- kind: ServiceAccount
  name: team-alpha-admin
  namespace: kubernetes-dashboard
```

### Token generation commands

```bash
# Short-lived token (default 1 hour) - RECOMMENDED
kubectl -n kubernetes-dashboard create token dashboard-admin

# Token with custom duration (24 hours max recommended)
kubectl -n kubernetes-dashboard create token dashboard-admin --duration=24h

# Token with 10-minute expiration for temporary access
kubectl -n kubernetes-dashboard create token dashboard-readonly --duration=10m

# Verify token permissions
kubectl auth can-i list pods --as=system:serviceaccount:kubernetes-dashboard:dashboard-admin
kubectl auth can-i create deployments --as=system:serviceaccount:kubernetes-dashboard:dashboard-readonly
```

### Production security checklist

- [ ] Use short-lived tokens (1-24 hours max)
- [ ] Prefer read-only access; grant edit/admin only when necessary
- [ ] Use namespace-scoped RoleBindings over ClusterRoleBindings
- [ ] Never expose Dashboard via public LoadBalancer
- [ ] Enable Kubernetes audit logging for Dashboard access
- [ ] Rotate service accounts periodically
- [ ] Document all cluster-admin bindings with justification

---

## GitHub Actions workflows for automated deployment

### Main deployment workflow

```yaml
# .github/workflows/deploy-dashboard.yml
name: Deploy Kubernetes Dashboard

on:
  push:
    branches: [main]
    paths:
      - 'helm/**'
      - '.github/workflows/deploy-dashboard.yml'
  workflow_dispatch:
    inputs:
      environment:
        description: 'Target environment'
        required: true
        default: 'dev'
        type: choice
        options: [dev, staging, prod]

permissions:
  id-token: write
  contents: read

env:
  RESOURCE_GROUP: prod-cus-platform-base-rg-001
  CLUSTER_NAME: prod-cus-aks-sre-lab-003
  ACR_NAME: prod-central-image-repo

jobs:
  deploy:
    runs-on: ubuntu-24.04  # Self-hosted runner for private cluster access
    environment: ${{ github.event.inputs.environment || 'dev' }}
    
    steps:
      - name: Checkout repository
        uses: actions/checkout@v4

      - name: Azure Login (OIDC)
        uses: azure/login@v2
        with:
          client-id: ${{ secrets.AZURE_CLIENT_ID }}
          tenant-id: ${{ secrets.AZURE_TENANT_ID }}
          subscription-id: ${{ secrets.AZURE_SUBSCRIPTION_ID }}

      - name: Setup kubelogin
        uses: azure/use-kubelogin@v1.2
        with:
          kubelogin-version: 'v0.2.13'

      - name: Set AKS context
        uses: azure/aks-set-context@v4
        with:
          resource-group: ${{ env.RESOURCE_GROUP }}
          cluster-name: ${{ env.CLUSTER_NAME }}
          admin: 'false'
          use-kubelogin: 'true'

      - name: Setup Helm
        uses: azure/setup-helm@v4
        with:
          version: 'v3.13.0'

      - name: Add Helm repository
        run: |
          helm repo add kubernetes-dashboard https://kubernetes.github.io/dashboard/
          helm repo update

      - name: Deploy Dashboard
        run: |
          ENV=${{ github.event.inputs.environment || 'dev' }}
          helm upgrade --install kubernetes-dashboard kubernetes-dashboard/kubernetes-dashboard \
            --namespace kubernetes-dashboard \
            --create-namespace \
            --version 7.14.0 \
            --values helm/values-${ENV}.yaml \
            --wait \
            --timeout 10m \
            --atomic

      - name: Verify deployment
        run: |
          kubectl get pods -n kubernetes-dashboard
          kubectl rollout status deployment/kubernetes-dashboard-kong -n kubernetes-dashboard --timeout=300s
```

### Reusable Helm deployment workflow

```yaml
# .github/workflows/reusable-helm-deploy.yml
name: Reusable Helm Deployment

on:
  workflow_call:
    inputs:
      environment:
        required: true
        type: string
      cluster-name:
        required: true
        type: string
      resource-group:
        required: true
        type: string
      namespace:
        required: true
        type: string
      release-name:
        required: true
        type: string
      chart-repo:
        required: true
        type: string
      chart-name:
        required: true
        type: string
      chart-version:
        required: true
        type: string
      values-file:
        required: false
        type: string
        default: ''
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

      - uses: azure/login@v2
        with:
          client-id: ${{ secrets.AZURE_CLIENT_ID }}
          tenant-id: ${{ secrets.AZURE_TENANT_ID }}
          subscription-id: ${{ secrets.AZURE_SUBSCRIPTION_ID }}

      - uses: azure/use-kubelogin@v1.2

      - uses: azure/aks-set-context@v4
        with:
          resource-group: ${{ inputs.resource-group }}
          cluster-name: ${{ inputs.cluster-name }}
          admin: 'false'
          use-kubelogin: 'true'

      - uses: azure/setup-helm@v4

      - name: Deploy
        run: |
          helm repo add chart-repo ${{ inputs.chart-repo }}
          helm repo update
          helm upgrade --install ${{ inputs.release-name }} \
            chart-repo/${{ inputs.chart-name }} \
            --namespace ${{ inputs.namespace }} \
            --create-namespace \
            --version ${{ inputs.chart-version }} \
            ${{ inputs.values-file != '' && format('--values {0}', inputs.values-file) || '' }} \
            --wait --atomic
```

### Image build and push workflow

```yaml
# .github/workflows/build-push-acr.yml
name: Build and Push to ACR

on:
  workflow_call:
    inputs:
      image-name:
        required: true
        type: string
      dockerfile:
        required: false
        type: string
        default: 'Dockerfile'
    outputs:
      image-tag:
        value: ${{ jobs.build.outputs.tag }}
    secrets:
      AZURE_CLIENT_ID:
        required: true
      AZURE_TENANT_ID:
        required: true
      AZURE_SUBSCRIPTION_ID:
        required: true

jobs:
  build:
    runs-on: ubuntu-24.04
    outputs:
      tag: ${{ steps.meta.outputs.version }}
    
    steps:
      - uses: actions/checkout@v4

      - uses: docker/setup-buildx-action@v3

      - uses: azure/login@v2
        with:
          client-id: ${{ secrets.AZURE_CLIENT_ID }}
          tenant-id: ${{ secrets.AZURE_TENANT_ID }}
          subscription-id: ${{ secrets.AZURE_SUBSCRIPTION_ID }}

      - name: Login to ACR
        run: az acr login --name prod-central-image-repo

      - name: Extract metadata
        id: meta
        uses: docker/metadata-action@v5
        with:
          images: prod-central-image-repo.azurecr.io/${{ inputs.image-name }}
          tags: |
            type=sha,prefix=
            type=ref,event=branch
            type=semver,pattern={{version}}

      - name: Build and push
        uses: docker/build-push-action@v5
        with:
          context: .
          file: ${{ inputs.dockerfile }}
          push: true
          tags: ${{ steps.meta.outputs.tags }}
          cache-from: type=gha
          cache-to: type=gha,mode=max
```

---

## Dashboard admin user guide

### Logging into the Dashboard

1. Start port-forward: `kubectl -n kubernetes-dashboard port-forward svc/kubernetes-dashboard-kong-proxy 8443:443`
2. Generate token: `kubectl -n kubernetes-dashboard create token dashboard-admin`
3. Navigate to `https://localhost:8443`
4. Select "Token" authentication, paste the token

### Common administrative tasks

| Task | Dashboard Navigation |
|------|---------------------|
| View cluster overview | Home → Cluster section |
| Check pod logs | Workloads → Pods → Select pod → Logs icon |
| Scale deployment | Workloads → Deployments → Scale icon |
| View events | Cluster → Events |
| Inspect ConfigMaps | Config and Storage → Config Maps |
| View node resources | Cluster → Nodes |
| Check persistent volumes | Config and Storage → Persistent Volume Claims |

### Resource management via Dashboard

```bash
# Before using Dashboard for edits, ensure RBAC allows it
kubectl auth can-i update deployments --as=system:serviceaccount:kubernetes-dashboard:dashboard-admin -n default
```

For production environments, prefer GitOps workflows over Dashboard edits. Use Dashboard primarily for:
- **Monitoring** cluster health and resource utilization
- **Debugging** by viewing logs and events
- **Discovery** of resource relationships
- **Training** new team members on Kubernetes concepts

---

## Post-installation verification

### Automated verification script

```bash
#!/bin/bash
# verify-dashboard.sh

echo "=== Kubernetes Dashboard Verification ==="

echo -e "\n1. Checking namespace..."
kubectl get namespace kubernetes-dashboard || exit 1

echo -e "\n2. Checking pods..."
kubectl get pods -n kubernetes-dashboard -o wide
READY_PODS=$(kubectl get pods -n kubernetes-dashboard --field-selector=status.phase=Running --no-headers | wc -l)
TOTAL_PODS=$(kubectl get pods -n kubernetes-dashboard --no-headers | wc -l)
echo "Ready pods: $READY_PODS/$TOTAL_PODS"

echo -e "\n3. Checking services..."
kubectl get svc -n kubernetes-dashboard

echo -e "\n4. Checking deployments..."
kubectl get deployments -n kubernetes-dashboard

echo -e "\n5. Checking rollout status..."
for deploy in $(kubectl get deployments -n kubernetes-dashboard -o name); do
  kubectl rollout status $deploy -n kubernetes-dashboard --timeout=60s
done

echo -e "\n6. Checking service accounts..."
kubectl get serviceaccounts -n kubernetes-dashboard

echo -e "\n7. Checking RBAC bindings..."
kubectl get clusterrolebindings | grep dashboard

echo -e "\n8. Checking recent events..."
kubectl get events -n kubernetes-dashboard --sort-by='.lastTimestamp' | tail -10

echo -e "\n9. Testing API connectivity..."
kubectl -n kubernetes-dashboard get pods -l app.kubernetes.io/name=kong -o jsonpath='{.items[0].status.phase}'

echo -e "\n=== Verification Complete ==="
```

### Health check endpoints

```bash
# Check Kong gateway health
kubectl -n kubernetes-dashboard port-forward svc/kubernetes-dashboard-kong-proxy 8443:443 &
sleep 5
curl -k https://localhost:8443/api/v1/health
pkill -f "port-forward.*8443"
```

---

## Troubleshooting common issues

### Connection and authentication issues

| Symptom | Cause | Solution |
|---------|-------|----------|
| "Unable to connect to server" | API server unreachable | Verify VPN/network connectivity to private cluster |
| "Unauthorized (401)" | Invalid/expired token | Generate new token: `kubectl create token` |
| "Forbidden (403)" | Missing RBAC permissions | Create ClusterRoleBinding for ServiceAccount |
| Dashboard loads but shows "Nothing to display" | Read permissions missing | Bind ServiceAccount to `view` ClusterRole |
| Connection timeout | Private DNS not resolving | Verify DNS can resolve `*.privatelink.*.azmk8s.io` |

### Pod-level troubleshooting

```bash
# Check pod status
kubectl get pods -n kubernetes-dashboard

# View pod logs
kubectl logs -n kubernetes-dashboard -l app.kubernetes.io/instance=kubernetes-dashboard --all-containers

# Describe pod for events
kubectl describe pod -n kubernetes-dashboard -l app.kubernetes.io/name=kong

# Check for resource constraints
kubectl top pods -n kubernetes-dashboard

# View previous container logs (if crashed)
kubectl logs -n kubernetes-dashboard <pod-name> --previous
```

### Network troubleshooting

```bash
# Verify endpoints exist
kubectl get endpoints -n kubernetes-dashboard

# Test internal DNS resolution
kubectl run dns-test --image=busybox:1.36 --rm -it --restart=Never -- nslookup kubernetes-dashboard-kong-proxy.kubernetes-dashboard.svc.cluster.local

# Check network policies
kubectl get networkpolicies -n kubernetes-dashboard -o yaml
```

### Helm troubleshooting

```bash
# View Helm release status
helm status kubernetes-dashboard -n kubernetes-dashboard

# Get Helm release history
helm history kubernetes-dashboard -n kubernetes-dashboard

# Rollback to previous version
helm rollback kubernetes-dashboard 1 -n kubernetes-dashboard

# Debug Helm template rendering
helm template kubernetes-dashboard kubernetes-dashboard/kubernetes-dashboard --values values-prod.yaml > rendered.yaml
```

---

## Production use cases and alternatives

### Valid production use cases

- **Incident response triage**: Quick visual assessment during outages
- **Developer self-service**: Namespace-scoped access for application teams
- **On-call monitoring**: Real-time pod and event visualization
- **Compliance audits**: Visual verification of deployed resources
- **Training environments**: Onboarding new Kubernetes administrators

### When to use alternatives

| Scenario | Recommended Alternative |
|----------|------------------------|
| Multi-cluster management | Rancher, Lens, or Plural |
| Heavy metrics/alerting | Grafana + Prometheus stack |
| GitOps deployments | ArgoCD or Flux |
| Power users preferring CLI | K9s |
| Air-gapped with strict security | K9s (no in-cluster component) |

### Decision framework

Use Kubernetes Dashboard when you need **quick visual access** to a single cluster without complex monitoring requirements. For enterprise multi-cluster environments or strict security postures where exposing any in-cluster UI creates compliance concerns, consider desktop-based tools like Lens or terminal-based K9s that don't require cluster-side deployment.

---

## Conclusion

Deploying Kubernetes Dashboard on private AKS clusters requires attention to **three critical areas**: network connectivity through proper VNet access, authentication via kubelogin and Azure AD integration, and security through least-privilege RBAC configuration. The shift to Helm-only deployment in v7.x simplifies installation but demands careful values customization for production environments.

Key takeaways for CKAs managing private AKS:
- Always use **short-lived tokens** generated via `kubectl create token`
- Prefer **port-forward access** over exposed services for maximum security
- Implement **namespace-scoped RBAC** rather than cluster-wide permissions
- Deploy **self-hosted GitHub runners** within the VNet for CI/CD automation
- Use **Istio AuthorizationPolicy** to restrict access to private IP ranges when exposing via ingress

For the specific cluster "prod-cus-aks-sre-lab-003," combine the production Helm values with ACR image configuration and internal load balancer access to maintain the security posture expected in production platform environments.