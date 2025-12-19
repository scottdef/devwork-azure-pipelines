# scripts/windows/03-Deploy-EasyTrade.ps1
# Deploy EasyTrade to AKS using either manifests or Helm

param(
    [Parameter(Mandatory=$true)]
    [ValidateSet("Manifests", "Helm", "HelmProduction")]
    [string]$DeploymentType,
    
    [string]$Namespace = "easytrade",
    [switch]$UseACRImages = $true,
    [string]$ACRLoginServer = "prodcentralimagerepo.azurecr.io",
    [switch]$Verbose = $false
)

$ErrorActionPreference = "Stop"

if ($Verbose) { $VerbosePreference = "Continue" }

# Load environment variables if .env.ps1 exists
if (Test-Path ".\.env.ps1") {
    . ".\.env.ps1"
}

Write-Host "=== Deploying EasyTrade to AKS ===" -ForegroundColor Green
Write-Host "Deployment Type: $DeploymentType" -ForegroundColor Yellow
Write-Host "Namespace: $Namespace" -ForegroundColor Yellow
Write-Host "Use ACR Images: $UseACRImages" -ForegroundColor Yellow

# Create namespace
Write-Host "`nCreating namespace..." -ForegroundColor Yellow
kubectl create namespace $Namespace --dry-run=client -o yaml | kubectl apply -f -

# Label for Istio injection
kubectl label namespace $Namespace istio-injection=enabled --overwrite 2>$null

Write-Host "✓ Namespace created" -ForegroundColor Green

# Deploy based on type
switch ($DeploymentType) {
    "Manifests" {
        Write-Host "`nDeploying with Kubernetes manifests..." -ForegroundColor Yellow
        
        # Deploy database first
        Write-Host "Deploying database..." -ForegroundColor Cyan
        kubectl -n $Namespace apply -f .\kubernetes\database.yaml
        
        # Wait for database to be ready
        Write-Host "Waiting for database to be ready..." -ForegroundColor Cyan
        kubectl -n $Namespace wait --for=condition=ready pod -l app=db --timeout=300s
        
        # Deploy services
        Write-Host "Deploying services..." -ForegroundColor Cyan
        $ServiceFiles = Get-ChildItem -Path ".\kubernetes\services\*.yaml"
        
        foreach ($File in $ServiceFiles) {
            Write-Host "  Deploying $($File.Name)..." -ForegroundColor Gray
            
            if ($UseACRImages) {
                # Replace image registry with ACR
                $Content = Get-Content $File.FullName -Raw
                $Content = $Content -replace "europe-docker.pkg.dev/dynatrace-demoability/docker/easytrade", "$ACRLoginServer/easytrade"
                $Content | kubectl -n $Namespace apply -f -
            } else {
                kubectl -n $Namespace apply -f $File.FullName
            }
        }
        
        Write-Host "✓ Manifests deployed" -ForegroundColor Green
    }
    
    "Helm" {
        Write-Host "`nDeploying with Helm (default values)..." -ForegroundColor Yellow
        
        $HelmArgs = @(
            "upgrade", "--install", "easytrade",
            ".\helm\easytrade",
            "--namespace", $Namespace,
            "--wait",
            "--timeout", "10m"
        )
        
        if ($UseACRImages) {
            $HelmArgs += "--set", "global.imageRegistry=$ACRLoginServer"
        }
        
        & helm $HelmArgs
        
        Write-Host "✓ Helm deployment complete" -ForegroundColor Green
    }
    
    "HelmProduction" {
        Write-Host "`nDeploying with Helm (PRODUCTION values)..." -ForegroundColor Yellow
        Write-Host "WARNING: This will deploy with production-grade resource requests" -ForegroundColor Red
        
        $Confirm = Read-Host "Continue? (yes/no)"
        if ($Confirm -ne "yes") {
            Write-Host "Deployment cancelled" -ForegroundColor Yellow
            exit 0
        }
        
        $HelmArgs = @(
            "upgrade", "--install", "easytrade",
            ".\helm\easytrade",
            "--namespace", $Namespace,
            "--values", ".\helm\easytrade\values-production.yaml",
            "--set", "global.imageRegistry=$ACRLoginServer",
            "--wait",
            "--timeout", "10m"
        )
        
        & helm $HelmArgs
        
        Write-Host "✓ Production Helm deployment complete" -ForegroundColor Green
    }
}

# Wait for rollout
Write-Host "`nWaiting for all deployments to be ready..." -ForegroundColor Yellow
kubectl -n $Namespace wait --for=condition=available --timeout=600s deployment --all

# Display deployment status
Write-Host "`n=== Deployment Status ===" -ForegroundColor Green
kubectl -n $Namespace get pods -o wide
Write-Host ""
kubectl -n $Namespace get svc

# Health check
Write-Host "`n=== Health Check ===" -ForegroundColor Green
$HealthyPods = (kubectl -n $Namespace get pods --field-selector=status.phase=Running --no-headers | Measure-Object).Count
$TotalPods = (kubectl -n $Namespace get pods --no-headers | Measure-Object).Count

Write-Host "Healthy Pods: $HealthyPods / $TotalPods" -ForegroundColor $(if ($HealthyPods -eq $TotalPods) { "Green" } else { "Yellow" })

Write-Host "`n✓ EasyTrade deployed successfully!" -ForegroundColor Green
Write-Host "`nNext steps:" -ForegroundColor Yellow
Write-Host "  Setup access: .\scripts\windows\04-Setup-Access.ps1 -AccessMethod PortForward" -ForegroundColor Cyan
Write-Host "  View pods:    kubectl get pods -n $Namespace" -ForegroundColor Cyan
Write-Host "  View logs:    kubectl logs -n $Namespace -l app=broker-service --tail=50" -ForegroundColor Cyan
