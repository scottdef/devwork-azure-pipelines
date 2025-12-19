# scripts/feature-flags.ps1
# Manage EasyTrade feature flags and problem patterns

param(
    [Parameter(Mandatory=$true)]
    [ValidateSet("List", "Get", "Enable", "Disable")]
    [string]$Action,
    
    [string]$Pattern,
    [string]$Namespace = "easytrade",
    [int]$Port = 8080,
    [switch]$UseLoadBalancer = $false
)

$ErrorActionPreference = "Stop"

# Function to get EasyTrade URL
function Get-EasyTradeUrl {
    if ($UseLoadBalancer) {
        $LBip = kubectl -n $Namespace get svc easytrade-lb -o jsonpath='{.status.loadBalancer.ingress[0].ip}' 2>$null
        
        if ($LBip) {
            return "http://$LBip"
        }
    }
    
    return "http://localhost:$Port"
}

# Function to ensure port-forward is running
function Start-PortForward {
    $url = Get-EasyTradeUrl
    
    if ($url -like "*localhost*") {
        Write-Host "Starting port-forward for feature flag access..." -ForegroundColor Yellow
        
        # Check if already running
        $existing = Get-Process kubectl -ErrorAction SilentlyContinue | Where-Object {
            $_.CommandLine -like "*port-forward*frontendreverseproxy*"
        }
        
        if (-not $existing) {
            Start-Process -FilePath "kubectl" `
                -ArgumentList "-n $Namespace port-forward svc/frontendreverseproxy ${Port}:80" `
                -WindowStyle Hidden `
                -PassThru
            
            Start-Sleep -Seconds 3
            Write-Host "✓ Port-forward started" -ForegroundColor Green
        }
    }
}

# Function to list all flags
function Get-AllFlags {
    $url = Get-EasyTradeUrl
    $apiUrl = "$url/feature-flag-service/v1/flags"
    
    try {
        $flags = Invoke-RestMethod -Uri $apiUrl -Method Get -ErrorAction Stop
        
        Write-Host "`n=== Feature Flags ===" -ForegroundColor Green
        Write-Host "URL: $apiUrl`n" -ForegroundColor Cyan
        
        foreach ($flag in $flags) {
            $status = if ($flag.enabled) { "ENABLED" } else { "DISABLED" }
            $color = if ($flag.enabled) { "Red" } else { "Green" }
            Write-Host "  $($flag.name): $status" -ForegroundColor $color
        }
        
        return $true
    }
    catch {
        Write-Host "Error: Could not fetch feature flags" -ForegroundColor Red
        Write-Host $_.Exception.Message -ForegroundColor Red
        return $false
    }
}

# Function to get specific flag
function Get-SpecificFlag {
    param([string]$FlagName)
    
    $url = Get-EasyTradeUrl
    $apiUrl = "$url/feature-flag-service/v1/flags/$FlagName/"
    
    try {
        $flag = Invoke-RestMethod -Uri $apiUrl -Method Get -ErrorAction Stop
        
        Write-Host "`nFeature Flag: $FlagName" -ForegroundColor Cyan
        $flag | ConvertTo-Json -Depth 3
        
        return $true
    }
    catch {
        Write-Host "Error: Could not fetch flag '$FlagName'" -ForegroundColor Red
        Write-Host $_.Exception.Message -ForegroundColor Red
        return $false
    }
}

# Function to enable flag
function Enable-Flag {
    param([string]$FlagName)
    
    $url = Get-EasyTradeUrl
    $apiUrl = "$url/feature-flag-service/v1/flags/$FlagName/"
    
    Write-Host "Enabling feature flag: $FlagName" -ForegroundColor Yellow
    
    try {
        $body = @{ enabled = $true } | ConvertTo-Json
        $result = Invoke-RestMethod -Uri $apiUrl -Method Put -Body $body -ContentType "application/json" -ErrorAction Stop
        
        Write-Host "✓ Feature flag '$FlagName' enabled" -ForegroundColor Green
        $result | ConvertTo-Json -Depth 3
        
        return $true
    }
    catch {
        Write-Host "Error: Could not enable flag '$FlagName'" -ForegroundColor Red
        Write-Host $_.Exception.Message -ForegroundColor Red
        return $false
    }
}

# Function to disable flag
function Disable-Flag {
    param([string]$FlagName)
    
    $url = Get-EasyTradeUrl
    $apiUrl = "$url/feature-flag-service/v1/flags/$FlagName/"
    
    Write-Host "Disabling feature flag: $FlagName" -ForegroundColor Yellow
    
    try {
        $body = @{ enabled = $false } | ConvertTo-Json
        $result = Invoke-RestMethod -Uri $apiUrl -Method Put -Body $body -ContentType "application/json" -ErrorAction Stop
        
        Write-Host "✓ Feature flag '$FlagName' disabled" -ForegroundColor Green
        $result | ConvertTo-Json -Depth 3
        
        return $true
    }
    catch {
        Write-Host "Error: Could not disable flag '$FlagName'" -ForegroundColor Red
        Write-Host $_.Exception.Message -ForegroundColor Red
        return $false
    }
}

# Main execution
Write-Host "=== EasyTrade Feature Flags Manager ===" -ForegroundColor Green

# Start port-forward if needed
Start-PortForward

# Execute action
$success = $false

switch ($Action) {
    "List" {
        $success = Get-AllFlags
    }
    
    "Get" {
        if (-not $Pattern) {
            Write-Host "Error: -Pattern required for Get action" -ForegroundColor Red
            exit 1
        }
        $success = Get-SpecificFlag -FlagName $Pattern
    }
    
    "Enable" {
        if (-not $Pattern) {
            Write-Host "Error: -Pattern required for Enable action" -ForegroundColor Red
            exit 1
        }
        $success = Enable-Flag -FlagName $Pattern
    }
    
    "Disable" {
        if (-not $Pattern) {
            Write-Host "Error: -Pattern required for Disable action" -ForegroundColor Red
            exit 1
        }
        $success = Disable-Flag -FlagName $Pattern
    }
}

# Display help
if ($success) {
    Write-Host "`nProblem Patterns (Chaos Engineering):" -ForegroundColor Yellow
    Write-Host "  db_not_responding        - Database throws errors (~20 min alert)" -ForegroundColor Gray
    Write-Host "  ergo_aggregator_slowdown - Slow responses (15-30 min)" -ForegroundColor Gray
    Write-Host "  factory_crisis           - Factory stops producing (persistent)" -ForegroundColor Gray
    Write-Host "  high_cpu_usage           - CPU spike (5-10 min alert)" -ForegroundColor Gray
}

exit ($success ? 0 : 1)
