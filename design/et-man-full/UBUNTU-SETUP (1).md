# Ubuntu 24.04 Setup Guide for EasyTrade AKS Deployment

Complete step-by-step guide for deploying EasyTrade to Azure Kubernetes Service from an Ubuntu 24.04 Azure VM.

## Prerequisites

- Ubuntu 24.04 Azure VM with internet access
- SSH access to the VM
- Azure subscription access
- Permissions to deploy to AKS cluster

## Step 1: Initial System Setup

Connect to your Ubuntu VM:

```bash
ssh azureuser@<vm-ip-address>
```

Update system packages:

```bash
sudo apt-get update
sudo apt-get upgrade -y
```

## Step 2: Install Required Tools

Run the automated setup script:

```bash
# Clone the repository
git clone <your-repo-url> ~/easytrade-aks-deployment
cd ~/easytrade-aks-deployment

# Make setup script executable
chmod +x setup/ubuntu-setup.sh

# Run the setup script
./setup/ubuntu-setup.sh
```

The script will install:
- Azure CLI
- kubectl
- kubelogin
- Helm 3
- Docker
- Terraform
- GitHub CLI
- Make and build tools

**Important**: Log out and back in after installation to activate Docker group membership:

```bash
exit
# SSH back in
ssh azureuser@<vm-ip-address>
cd ~/easytrade-aks-deployment
```

## Step 3: Verify Prerequisites

Check that all tools are installed correctly:

```bash
make check-prereqs
```

Expected output should show all tools with green checkmarks.

## Step 4: Configure Environment

Copy the environment template and configure your values:

```bash
cp .env.example .env
vi .env
```

Edit the following variables:

```bash
SUBSCRIPTION=tango-CICD-platform-github-gitflow
RESOURCE_GROUP=prod-cus-platform-base-rg-001
CLUSTER_NAME=prod-cus-aks-sre-lab-003
ACR_NAME=prod-central-image-repo
ACR_LOGIN_SERVER=prodcentralimagerepo.azurecr.io
NAMESPACE=easytrade
```

Load environment variables:

```bash
source .env
```

## Step 5: Authenticate to Azure and AKS

Authenticate to Azure:

```bash
make auth
```

This will:
1. Prompt for Azure login (opens browser for device code authentication)
2. Set the correct subscription
3. Get AKS cluster credentials
4. Configure kubelogin for private cluster access
5. Verify cluster connectivity

You should see a list of cluster nodes if successful.

## Step 6: Mirror Images to Private ACR

Mirror upstream EasyTrade images to your private ACR:

```bash
make mirror-images
```

This process takes 5-10 minutes and will pull all EasyTrade service images from the Dynatrace registry and push them to your ACR.

Verify images are in ACR:

```bash
az acr repository list --name prod-central-image-repo --output table
```

## Step 7: Deploy EasyTrade

### Option A: Deploy with Kubernetes Manifests

```bash
make deploy-manifests
```

### Option B: Deploy with Helm Chart

```bash
make deploy-helm
```

### Option C: Complete automated deployment (Recommended)

```bash
make deploy-existing
```

This runs: `auth` → `mirror-images` → `deploy-helm` → `verify`

## Step 8: Verify Deployment

Check deployment health:

```bash
make verify
```

View pod status:

```bash
kubectl -n easytrade get pods
```

Expected output: All pods should be in `Running` state with `READY 1/1`.

## Step 9: Access the Application

### Method 1: kubectl port-forward (Recommended for testing)

```bash
make port-forward
```

Open browser to: http://localhost:8080

**Login credentials:**
- Username: `demouser` Password: `demopass`
- Username: `specialuser` Password: `specialpass`

### Method 2: Private LoadBalancer (For internal network access)

Deploy private LoadBalancer:

```bash
make deploy-private-lb
```

Get the assigned IP:

```bash
make get-lb-ip
```

Access from within Azure VNet: `http://<private-ip>`

### Method 3: Istio Gateway (If service mesh is deployed)

```bash
make deploy-istio-gateway
make get-istio-ip
```

## Step 10: Test Problem Patterns

List available problem patterns:

```bash
make problem-list
```

Enable a problem pattern:

```bash
make problem-enable PATTERN=high_cpu_usage
```

Verify the pattern is active:

```bash
# Check CPU usage spike
kubectl -n easytrade top pods -l app=broker-service

# View logs
make logs SERVICE=broker-service
```

Disable the pattern:

```bash
make problem-disable PATTERN=high_cpu_usage
```

## Common Operations

### View logs for a specific service

```bash
make logs SERVICE=broker-service
```

### Open shell in a pod

```bash
make pod-shell SERVICE=broker-service
```

### Restart all deployments

```bash
make restart
```

### Scale a deployment

```bash
make scale SERVICE=broker-service REPLICAS=5
```

### View real-time pod status

```bash
make dev-watch
```

### Run diagnostics

```bash
make diagnose
```

## Building Custom Images

If you need to modify EasyTrade services:

```bash
# Build all services
make build-images

# Build specific service
make build-image SERVICE=broker-service

# Deploy with custom images
make build-and-deploy-helm
```

## Cleanup

To remove all EasyTrade resources:

```bash
make cleanup
```

This will prompt for confirmation and optionally:
- Delete all deployments, services, pods
- Delete the namespace
- Delete images from ACR

## Troubleshooting

### Authentication issues

```bash
# Clear cached tokens
kubelogin remove-tokens
rm -rf ~/.kube/cache/kubelogin/

# Re-authenticate
make auth
```

### Image pull errors

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

### Pods not starting

```bash
# Check pod events
kubectl -n easytrade describe pod <pod-name>

# Check logs
kubectl -n easytrade logs <pod-name>

# Restart deployment
kubectl -n easytrade rollout restart deployment/<deployment-name>
```

### Database connectivity issues

```bash
# Check database pod
kubectl -n easytrade get pods -l app=db

# Check database logs
kubectl -n easytrade logs -l app=db --tail=50

# Restart content creator (often waits for database)
kubectl -n easytrade rollout restart deployment/contentcreator
```

## Advanced Configuration

### Enable Horizontal Pod Autoscaling

```bash
kubectl -n easytrade autoscale deployment broker-service \
    --cpu-percent=70 \
    --min=2 \
    --max=10

# Watch HPA
watch kubectl -n easytrade get hpa
```

### Enable Istio Sidecar Injection

```bash
# Label namespace
kubectl label namespace easytrade istio-injection=enabled

# Restart pods to inject sidecars
kubectl -n easytrade rollout restart deployment
```

### Deploy with Production Values

```bash
make deploy-helm-prod
```

## Next Steps

- Configure Dynatrace monitoring
- Set up CI/CD pipelines
- Implement backup strategies
- Configure alerting
- Document runbooks

## Support

For issues:
1. Check prerequisites: `make check-prereqs`
2. Run diagnostics: `make diagnose`
3. Review troubleshooting section above
4. Check TROUBLESHOOTING.md in docs/
