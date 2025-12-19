# Windows 11 AVD Setup Guide for EasyTrade on AKS (Non-Admin)

This guide provides step-by-step instructions for deploying EasyTrade to private AKS clusters from a Windows 11 Azure Virtual Desktop without administrator permissions.

## Prerequisites

- Windows 11 Azure Virtual Desktop
- Windows Terminal installed
- Internet access
- Azure account with AKS cluster access
- Cluster: `prod-cus-aks-sre-lab-003`
- Resource Group: `prod-cus-platform-base-rg-001`
- ACR: `prod-central-image-repo`

**Important**: This guide uses tools that can be installed without admin privileges.

## Quick Start (10 Minutes)

```powershell
# Open Windows Terminal (PowerShell)

# Clone repository
git clone https://github.com/your-org/easytrade-deployment.git
cd easytrade-deployment

# Install all prerequisites (no admin required)
.\scripts\windows\01-Install-Tools.ps1

# Restart terminal to refresh PATH
# Open new Windows Terminal

# Navigate back to repository
cd easytrade-deployment

# Configure AKS access
.\scripts\windows\02-Configure-AKS.ps1

# Deploy EasyTrade
.\scripts\windows\03-Deploy-EasyTrade.ps1 -DeploymentType Manifests

# Setup access
.\scripts\windows\04-Setup-Access.ps1 -AccessMethod PortForward
```

## Detailed Step-by-Step Guide

### Step 1: Prepare Windows Environment

#### Check Execution Policy

```powershell
# Check current execution policy
Get-ExecutionPolicy

# If RestrictedAllSigned or Restricted, set for current user
Set-ExecutionPolicy -ExecutionPolicy RemoteSigned -Scope CurrentUser
```

#### Create Working Directory

```powershell
# Create directory
New-Item -ItemType Directory -Path "$env:USERPROFILE\easytrade-work" -Force
Set-Location "$env:USERPROFILE\easytrade-work"
```

### Step 2: Install Required Tools (Non-Admin)

#### Option A: Automated Installation (Recommended)

```powershell
# Clone repository
git clone https://github.com/your-org/easytrade-deployment.git
cd easytrade-deployment

# Install all tools
.\scripts\windows\01-Install-Tools.ps1 -UseScoop -Verbose

# Close and reopen Windows Terminal to refresh PATH
```

#### Option B: Manual Installation

**Install Scoop (Package Manager)**

```powershell
# Install Scoop
Set-ExecutionPolicy RemoteSigned -Scope CurrentUser
Invoke-RestMethod -Uri https://get.scoop.sh | Invoke-Expression
```

**Install Tools via Scoop**

```powershell
# Install Azure CLI
scoop install azure-cli

# Install kubectl
scoop install kubectl

# Install Helm
scoop install helm

# Install Git
scoop install git

# Install jq
scoop install jq
```

**Install kubelogin manually**

```powershell
# Create bin directory
$LocalBin = "$env:LOCALAPPDATA\bin"
New-Item -ItemType Directory -Path $LocalBin -Force

# Download kubelogin
$KubeloginUrl = "https://github.com/Azure/kubelogin/releases/download/v0.1.4/kubelogin-win-amd64.zip"
$TempZip = "$env:TEMP\kubelogin.zip"
Invoke-WebRequest -Uri $KubeloginUrl -OutFile $TempZip

# Extract
Expand-Archive -Path $TempZip -DestinationPath $env:TEMP -Force
Move-Item -Path "$env:TEMP\bin\windows_amd64\kubelogin.exe" -Destination "$LocalBin\kubelogin.exe" -Force

# Add to PATH
$CurrentPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($CurrentPath -notlike "*$LocalBin*") {
    [Environment]::SetEnvironmentVariable("Path", "$CurrentPath;$LocalBin", "User")
}

# Refresh current session
$env:Path = "$env:Path;$LocalBin"
```

**Verify Installations**

```powershell
az --version
kubectl version --client
kubelogin --version
helm version
git --version
jq --version
```

### Step 3: Azure and AKS Configuration

```powershell
# Option A: Use configuration script
.\scripts\windows\02-Configure-AKS.ps1 `
    -SubscriptionName "tango-CICD-platform-github-gitflow" `
    -ResourceGroup "prod-cus-platform-base-rg-001" `
    -ClusterName "prod-cus-aks-sre-lab-003" `
    -ACRName "prod-central-image-repo"

# Option B: Manual configuration
# Login to Azure (browser-based device code flow)
az login --use-device-code

# Set subscription
az account set --subscription "tango-CICD-platform-github-gitflow"

# Verify subscription
az account show

# Get AKS credentials
az aks get-credentials `
    --resource-group "prod-cus-platform-base-rg-001" `
    --name "prod-cus-aks-sre-lab-003" `
    --overwrite-existing

# Configure kubelogin
kubelogin convert-kubeconfig -l azurecli

# Test connectivity (may fail for private clusters from non-VNet machines)
kubectl get nodes

# For private clusters without direct access, use command invoke:
az aks command invoke `
    -g prod-cus-platform-base-rg-001 `
    -n prod-cus-aks-sre-lab-003 `
    --command "kubectl get nodes"
```

### Step 4: Deploy EasyTrade

#### Method 1: Using PowerShell Script

```powershell
# Deploy with Kubernetes manifests (upstream images)
.\scripts\windows\03-Deploy-EasyTrade.ps1 -DeploymentType Manifests

# Deploy with Kubernetes manifests (ACR images)
.\scripts\windows\03-Deploy-EasyTrade.ps1 `
    -DeploymentType Manifests `
    -UseACRImages $true `
    -ACRLoginServer "prodcentralimagerepo.azurecr.io"

# Deploy with Helm (default values)
.\scripts\windows\03-Deploy-EasyTrade.ps1 -DeploymentType Helm

# Deploy with Helm (production values)
.\scripts\windows\03-Deploy-EasyTrade.ps1 -DeploymentType HelmProduction
```

#### Method 2: Manual Deployment

**Using Kubernetes Manifests**

```powershell
# Create namespace
kubectl create namespace easytrade

# Deploy database
kubectl -n easytrade apply -f .\kubernetes\database.yaml

# Wait for database to be ready
kubectl -n easytrade wait --for=condition=ready pod -l app=db --timeout=300s

# Deploy services
Get-ChildItem -Path ".\kubernetes\services\*.yaml" | ForEach-Object {
    Write-Host "Deploying $($_.Name)..."
    kubectl -n easytrade apply -f $_.FullName
}

# Wait for all deployments
kubectl -n easytrade wait --for=condition=available --timeout=600s deployment --all
```

**Using Helm**

```powershell
# Install with default values
helm install easytrade .\helm\easytrade `
    --namespace easytrade `
    --create-namespace `
    --wait `
    --timeout 10m

# Install with production values
helm install easytrade .\helm\easytrade `
    --namespace easytrade `
    --create-namespace `
    --values .\helm\easytrade\values-production.yaml `
    --set global.imageRegistry=prodcentralimagerepo.azurecr.io `
    --wait `
    --timeout 10m
```

### Step 5: Verify Deployment

```powershell
# Check pod status
kubectl get pods -n easytrade

# Check services
kubectl get svc -n easytrade

# View events
kubectl get events -n easytrade --sort-by='.lastTimestamp' | Select-Object -Last 20

# Check specific service logs
kubectl logs -n easytrade -l app=broker-service --tail=50
```

### Step 6: Setup Access

#### Method 1: Port-Forward

```powershell
# Option A: Use script (foreground)
.\scripts\windows\04-Setup-Access.ps1 -AccessMethod PortForward

# Option B: Use script (background)
.\scripts\windows\04-Setup-Access.ps1 -AccessMethod PortForward -Background

# Option C: Manual port-forward
kubectl -n easytrade port-forward svc/frontendreverseproxy 8080:80

# Access EasyTrade at http://localhost:8080
Start-Process "http://localhost:8080"
```

#### Method 2: LoadBalancer

```powershell
# Deploy LoadBalancer
.\scripts\windows\04-Setup-Access.ps1 -AccessMethod LoadBalancer

# Get LoadBalancer IP
$LBip = kubectl -n easytrade get svc easytrade-lb -o jsonpath='{.status.loadBalancer.ingress[0].ip}'
Write-Host "Access EasyTrade at: http://$LBip"

# Open in browser
Start-Process "http://$LBip"
```

#### Method 3: Direct Pod IP

```powershell
# Get pod IP
.\scripts\windows\04-Setup-Access.ps1 -AccessMethod DirectPod

# Test from within cluster
kubectl run test-curl --rm -it --image=curlimages/curl --restart=Never -n easytrade -- curl http://<pod-ip>
```

### Step 7: Test Application

```powershell
# Test health endpoint (with port-forward running)
Invoke-WebRequest -Uri "http://localhost:8080/" -UseBasicParsing

# Login credentials
# demouser / demopass
# specialuser / specialpass
```

## Working with Feature Flags & Problem Patterns

### Create PowerShell Feature Flags Script

Save this as `scripts\feature-flags.ps1`:

```powershell
param(
    [Parameter(Mandatory=$true)]
    [ValidateSet("List", "Get", "Enable", "Disable")]
    [string]$Action,
    
    [string]$Pattern,
    [string]$Namespace = "easytrade",
    [int]$Port = 8080
)

$ErrorActionPreference = "Stop"

# Get EasyTrade URL
$LBip = kubectl -n $Namespace get svc easytrade-lb -o jsonpath='{.status.loadBalancer.ingress[0].ip}' 2>$null

if ($LBip) {
    $BaseUrl = "http://$LBip"
} else {
    $BaseUrl = "http://localhost:$Port"
    Write-Host "Using port-forward, make sure it's running..." -ForegroundColor Yellow
}

$ApiUrl = "$BaseUrl/feature-flag-service/v1/flags"

switch ($Action) {
    "List" {
        $flags = Invoke-RestMethod -Uri $ApiUrl -Method Get
        Write-Host "`n=== Feature Flags ===" -ForegroundColor Green
        $flags | ForEach-Object {
            $status = if ($_.enabled) { "ENABLED" } else { "DISABLED" }
            $color = if ($_.enabled) { "Red" } else { "Green" }
            Write-Host "  $($_.name): $status" -ForegroundColor $color
        }
    }
    
    "Get" {
        if (-not $Pattern) {
            Write-Host "Error: -Pattern required for Get action" -ForegroundColor Red
            exit 1
        }
        $flag = Invoke-RestMethod -Uri "$ApiUrl/$Pattern/" -Method Get
        $flag | ConvertTo-Json
    }
    
    "Enable" {
        if (-not $Pattern) {
            Write-Host "Error: -Pattern required for Enable action" -ForegroundColor Red
            exit 1
        }
        $body = @{ enabled = $true } | ConvertTo-Json
        $result = Invoke-RestMethod -Uri "$ApiUrl/$Pattern/" -Method Put -Body $body -ContentType "application/json"
        Write-Host "✓ Pattern '$Pattern' enabled" -ForegroundColor Green
        $result | ConvertTo-Json
    }
    
    "Disable" {
        if (-not $Pattern) {
            Write-Host "Error: -Pattern required for Disable action" -ForegroundColor Red
            exit 1
        }
        $body = @{ enabled = $false } | ConvertTo-Json
        $result = Invoke-RestMethod -Uri "$ApiUrl/$Pattern/" -Method Put -Body $body -ContentType "application/json"
        Write-Host "✓ Pattern '$Pattern' disabled" -ForegroundColor Green
        $result | ConvertTo-Json
    }
}
```

### Using Feature Flags

```powershell
# List all flags
.\scripts\feature-flags.ps1 -Action List

# Enable problem pattern
.\scripts\feature-flags.ps1 -Action Enable -Pattern db_not_responding

# Monitor CPU usage
kubectl top pods -n easytrade -l app=broker-service

# Disable pattern
.\scripts\feature-flags.ps1 -Action Disable -Pattern db_not_responding
```

## Common Operations

### View Logs

```powershell
# All pods
kubectl logs -n easytrade --all-containers=true --tail=50

# Specific service
kubectl logs -n easytrade -l app=broker-service --tail=100 -f

# Export logs
kubectl logs -n easytrade -l app=broker-service --tail=500 > broker-logs.txt
```

### Debug Pod Issues

```powershell
# Describe pod
kubectl describe pod -n easytrade <pod-name>

# Get into pod shell
kubectl exec -it -n easytrade <pod-name> -- /bin/sh

# Copy files from pod
kubectl cp easytrade/<pod-name>:/path/to/file ./local-file
```

### Update Deployment

```powershell
# Update image
kubectl set image deployment/broker-service `
    broker-service=prodcentralimagerepo.azurecr.io/easytrade/broker-service:v2 `
    -n easytrade

# Rollback
kubectl rollout undo deployment/broker-service -n easytrade

# View rollout history
kubectl rollout history deployment/broker-service -n easytrade
```

### Scale Services

```powershell
# Scale up
kubectl scale deployment broker-service --replicas=5 -n easytrade

# Scale down
kubectl scale deployment broker-service --replicas=2 -n easytrade

# View current replicas
kubectl get deployment -n easytrade -o wide
```

## Cleanup

```powershell
# Delete all resources in namespace
kubectl delete all --all -n easytrade

# Delete namespace
kubectl delete namespace easytrade

# Helm cleanup
helm uninstall easytrade -n easytrade
```

## Troubleshooting

### PowerShell Execution Policy

```powershell
# If scripts won't run, set execution policy
Set-ExecutionPolicy -ExecutionPolicy RemoteSigned -Scope CurrentUser

# Check current policy
Get-ExecutionPolicy -List
```

### Can't Connect to Cluster

```powershell
# Refresh credentials
az aks get-credentials `
    -g prod-cus-platform-base-rg-001 `
    -n prod-cus-aks-sre-lab-003 `
    --overwrite-existing

kubelogin convert-kubeconfig -l azurecli

# Test with command invoke
az aks command invoke `
    -g prod-cus-platform-base-rg-001 `
    -n prod-cus-aks-sre-lab-003 `
    --command "kubectl get pods -n easytrade"
```

### Tool Not Found After Installation

```powershell
# Refresh PATH in current session
$env:Path = [System.Environment]::GetEnvironmentVariable("Path", "User") + ";" + [System.Environment]::GetEnvironmentVariable("Path", "Machine")

# OR restart Windows Terminal
```

### Port Forward Connection Issues

```powershell
# Check if port is already in use
Get-NetTCPConnection -LocalPort 8080 -ErrorAction SilentlyContinue

# Kill process using port
Get-Process -Id (Get-NetTCPConnection -LocalPort 8080).OwningProcess | Stop-Process -Force

# Use different port
kubectl -n easytrade port-forward svc/frontendreverseproxy 9090:80
```

### Image Pull Errors

```powershell
# Verify ACR attachment
az aks check-acr `
    -n prod-cus-aks-sre-lab-003 `
    -g prod-cus-platform-base-rg-001 `
    --acr prodcentralimagerepo.azurecr.io

# Re-attach ACR
az aks update `
    -n prod-cus-aks-sre-lab-003 `
    -g prod-cus-platform-base-rg-001 `
    --attach-acr prod-central-image-repo
```

## Working in VS Code

### Setup VS Code Kubernetes Extension

```powershell
# Install VS Code extensions (if allowed)
code --install-extension ms-kubernetes-tools.vscode-kubernetes-tools
code --install-extension ms-azuretools.vscode-azureterraform
code --install-extension ms-vscode.powershell
```

### Use Integrated Terminal

1. Open VS Code
2. File > Open Folder > Select `easytrade-deployment`
3. Terminal > New Terminal (PowerShell)
4. Run commands from integrated terminal

## Tips for Non-Admin Environment

1. **Use User-Scoped Tools**: Always install to `$env:LOCALAPPDATA` or `$env:USERPROFILE`
2. **Scoop Package Manager**: Best option for non-admin installations
3. **Portable Apps**: Download portable versions when available
4. **PATH Management**: Manually add to user PATH if needed
5. **Alternative: WSL2**: If available, use Ubuntu via WSL2

## Next Steps

1. **Configure Monitoring**: Setup Dynatrace agent
2. **Test Problem Patterns**: Use feature flags for chaos engineering
3. **Automation**: Create PowerShell scripts for common tasks
4. **Documentation**: Document your specific workflow

## Resources

- [EasyTrade GitHub](https://github.com/Dynatrace/easytrade)
- [Feature Flags Guide](./feature-flags-guide.md)
- [Scoop Documentation](https://scoop.sh)
- [Azure CLI Reference](https://learn.microsoft.com/en-us/cli/azure/)
