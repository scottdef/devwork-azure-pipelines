# scripts/windows/04-Setup-Access.ps1
# Setup access to EasyTrade application

param(
    [Parameter(Mandatory=$true)]
    [ValidateSet("PortForward", "LoadBalancer", "DirectPod")]
    [string]$AccessMethod,
    
    [string]$Namespace = "easytrade",
    [int]$LocalPort = 8080,
    [switch]$Background = $false
)

$ErrorActionPreference = "Stop"

Write-Host "=== Setting up EasyTrade Access ===" -ForegroundColor Green
Write-Host "Access Method: $AccessMethod" -ForegroundColor Yellow
Write-Host "Namespace: $Namespace" -ForegroundColor Yellow

switch ($AccessMethod) {
    "PortForward" {
        Write-Host "`nSetting up kubectl port-forward..." -ForegroundColor Yellow
        Write-Host "Local Port: $LocalPort" -ForegroundColor Cyan
        Write-Host "Target: frontendreverseproxy:80" -ForegroundColor Cyan
        
        Write-Host "`nEasyTrade will be available at: http://localhost:$LocalPort" -ForegroundColor Green
        Write-Host "Press Ctrl+C to stop" -ForegroundColor Yellow
        Write-Host "`nDefault credentials:" -ForegroundColor Cyan
        Write-Host "  demouser / demopass" -ForegroundColor Gray
        Write-Host "  specialuser / specialpass" -ForegroundColor Gray
        
        if ($Background) {
            Start-Process -FilePath "kubectl" -ArgumentList "-n $Namespace port-forward svc/frontendreverseproxy ${LocalPort}:80" -NoNewWindow
            Write-Host "`n✓ Port-forward started in background" -ForegroundColor Green
        } else {
            kubectl -n $Namespace port-forward svc/frontendreverseproxy "${LocalPort}:80"
        }
    }
    
    "LoadBalancer" {
        Write-Host "`nDeploying internal LoadBalancer..." -ForegroundColor Yellow
        
        # Check if LoadBalancer already exists
        $LBExists = kubectl -n $Namespace get svc easytrade-lb 2>$null
        
        if (-not $LBExists) {
            kubectl apply -f .\kubernetes\ingress\loadbalancer.yaml
            Write-Host "✓ LoadBalancer service created" -ForegroundColor Green
        } else {
            Write-Host "✓ LoadBalancer service already exists" -ForegroundColor Green
        }
        
        Write-Host "`nWaiting for LoadBalancer IP assignment..." -ForegroundColor Yellow
        
        $MaxWait = 120
        $Elapsed = 0
        $LBip = $null
        
        while ($Elapsed -lt $MaxWait -and -not $LBip) {
            Start-Sleep -Seconds 2
            $Elapsed += 2
            
            $LBip = kubectl -n $Namespace get svc easytrade-lb -o jsonpath='{.status.loadBalancer.ingress[0].ip}' 2>$null
            
            if ($LBip) {
                break
            }
        }
        
        if ($LBip) {
            Write-Host "`n✓ LoadBalancer IP assigned: $LBip" -ForegroundColor Green
            Write-Host "`nAccess EasyTrade at: http://$LBip" -ForegroundColor Cyan
            Write-Host "`nNote: This is an internal IP accessible only from within the Azure VNet" -ForegroundColor Yellow
        } else {
            Write-Host "`nWarning: LoadBalancer IP not assigned within timeout" -ForegroundColor Yellow
            Write-Host "Check status with: kubectl -n $Namespace get svc easytrade-lb" -ForegroundColor Cyan
        }
    }
    
    "DirectPod" {
        Write-Host "`nRetrieving frontend pod IP..." -ForegroundColor Yellow
        
        $PodIP = kubectl -n $Namespace get pod -l app=frontendreverseproxy -o jsonpath='{.items[0].status.podIP}'
        
        if ($PodIP) {
            Write-Host "✓ Frontend Pod IP: $PodIP" -ForegroundColor Green
            Write-Host "`nDirect pod access URL: http://$PodIP" -ForegroundColor Cyan
            Write-Host "`nNote: Direct pod IPs are only accessible from within the cluster network" -ForegroundColor Yellow
            Write-Host "You can test from another pod with:" -ForegroundColor Cyan
            Write-Host "  kubectl run -n $Namespace test-curl --rm -it --image=curlimages/curl --restart=Never -- curl http://$PodIP" -ForegroundColor Gray
        } else {
            Write-Host "Error: Could not retrieve pod IP" -ForegroundColor Red
            exit 1
        }
    }
}

Write-Host "`n=== Access Information ===" -ForegroundColor Green
Write-Host "All available access methods:" -ForegroundColor Yellow
Write-Host "  1. Port-Forward:   .\scripts\windows\04-Setup-Access.ps1 -AccessMethod PortForward" -ForegroundColor Cyan
Write-Host "  2. LoadBalancer:   .\scripts\windows\04-Setup-Access.ps1 -AccessMethod LoadBalancer" -ForegroundColor Cyan
Write-Host "  3. Direct Pod IP:  .\scripts\windows\04-Setup-Access.ps1 -AccessMethod DirectPod" -ForegroundColor Cyan

Write-Host "`nFeature Flags & Problem Patterns:" -ForegroundColor Yellow
Write-Host "  List flags:        .\scripts\feature-flags.ps1 -Action List" -ForegroundColor Cyan
Write-Host "  Enable pattern:    .\scripts\feature-flags.ps1 -Action Enable -Pattern db_not_responding" -ForegroundColor Cyan
Write-Host "  Disable pattern:   .\scripts\feature-flags.ps1 -Action Disable -Pattern high_cpu_usage" -ForegroundColor Cyan
