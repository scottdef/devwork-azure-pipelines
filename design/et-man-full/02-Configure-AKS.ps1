# scripts/windows/02-Configure-AKS.ps1
# Configure access to private AKS cluster from Windows 11 AVD

param(
    [string]$SubscriptionName = "tango-CICD-platform-github-gitflow",
    [string]$ResourceGroup = "prod-cus-platform-base-rg-001",
    [string]$ClusterName = "prod-cus-aks-sre-lab-003",
    [string]$ACRName = "prod-central-image-repo",
    [switch]$Verbose = $false
)

$ErrorActionPreference = "Stop"

if ($Verbose) { $VerbosePreference = "Continue" }

Write-Host "=== Configuring AKS Access ===" -ForegroundColor Green
Write-Host "Subscription: $SubscriptionName" -ForegroundColor Yellow
Write-Host "Cluster: $ClusterName" -ForegroundColor Yellow
Write-Host "Resource Group: $ResourceGroup" -ForegroundColor Yellow

# Login to Azure
Write-Host "`nLogging into Azure..." -ForegroundColor Yellow
try {
    # Check if already logged in
    $account = az account show 2>$null | ConvertFrom-Json
    if ($account) {
        Write-Host "✓ Already logged in as: $($account.user.name)" -ForegroundColor Green
    }
} catch {
    Write-Host "Not logged in, initiating login..." -ForegroundColor Yellow
    az login --use-device-code
}

# Set subscription
Write-Host "`nSetting subscription..." -ForegroundColor Yellow
az account set --subscription $SubscriptionName

$CurrentSub = az account show | ConvertFrom-Json
Write-Host "✓ Using subscription: $($CurrentSub.name)" -ForegroundColor Green

# Get AKS credentials
Write-Host "`nRetrieving AKS credentials..." -ForegroundColor Yellow
az aks get-credentials `
    --resource-group $ResourceGroup `
    --name $ClusterName `
    --overwrite-existing

Write-Host "✓ Credentials retrieved" -ForegroundColor Green

# Convert kubeconfig for kubelogin
Write-Host "`nConfiguring kubelogin authentication..." -ForegroundColor Yellow
kubelogin convert-kubeconfig -l azurecli

Write-Host "✓ kubelogin configured" -ForegroundColor Green

# Test cluster connectivity
Write-Host "`nTesting cluster connectivity..." -ForegroundColor Yellow
try {
    $nodes = kubectl get nodes --no-headers 2>&1
    if ($LASTEXITCODE -eq 0) {
        Write-Host "✓ Successfully connected to cluster" -ForegroundColor Green
        Write-Host "`nCluster Nodes:" -ForegroundColor Cyan
        kubectl get nodes
    } else {
        Write-Host "Warning: Could not connect to cluster directly" -ForegroundColor Yellow
        Write-Host "This is normal for private clusters from non-VNet connected machines" -ForegroundColor Yellow
        Write-Host "You may need to use 'az aks command invoke' for operations" -ForegroundColor Yellow
    }
} catch {
    Write-Host "Warning: Connectivity test failed" -ForegroundColor Yellow
    Write-Host $_.Exception.Message -ForegroundColor Red
}

# Verify ACR access
Write-Host "`nVerifying ACR access..." -ForegroundColor Yellow
try {
    az aks check-acr `
        --name $ClusterName `
        --resource-group $ResourceGroup `
        --acr "${ACRName}.azurecr.io"
    Write-Host "✓ ACR access verified" -ForegroundColor Green
} catch {
    Write-Host "Warning: ACR check failed" -ForegroundColor Yellow
    Write-Host "You may need to attach ACR: az aks update --attach-acr $ACRName" -ForegroundColor Yellow
}

# Save configuration to .env file for reuse
Write-Host "`nSaving configuration..." -ForegroundColor Yellow
$EnvContent = @"
# Azure Configuration
AZURE_SUBSCRIPTION_NAME=$SubscriptionName
AZURE_RESOURCE_GROUP=$ResourceGroup
CLUSTER_NAME=$ClusterName
ACR_NAME=$ACRName
ACR_LOGIN_SERVER=${ACRName}.azurecr.io

# EasyTrade Configuration
EASYTRADE_NAMESPACE=easytrade
EASYTRADE_VERSION=latest
USE_ACR_IMAGES=true
"@

$EnvContent | Out-File -FilePath ".\.env.ps1" -Encoding UTF8
Write-Host "✓ Configuration saved to .env.ps1" -ForegroundColor Green

Write-Host "`n=== Configuration Complete ===" -ForegroundColor Green
Write-Host "`nNext steps:" -ForegroundColor Yellow
Write-Host "  Deploy EasyTrade: .\scripts\windows\03-Deploy-EasyTrade.ps1" -ForegroundColor Cyan

# Display helper commands
Write-Host "`nUseful commands:" -ForegroundColor Yellow
Write-Host "  List pods:       kubectl get pods -n easytrade" -ForegroundColor Cyan
Write-Host "  Port forward:    kubectl port-forward -n easytrade svc/frontendreverseproxy 8080:80" -ForegroundColor Cyan
Write-Host "  View logs:       kubectl logs -n easytrade -l app=broker-service" -ForegroundColor Cyan
