# scripts/windows/01-Install-Tools.ps1
# Install required tools on Windows 11 AVD without admin permissions
# Uses user-scoped installers (winget, scoop) or portable versions

param(
    [switch]$UseScoop = $true,
    [switch]$Verbose = $false
)

$ErrorActionPreference = "Stop"

# Set verbose preference
if ($Verbose) { $VerbosePreference = "Continue" }

Write-Host "=== Installing Tools for EasyTrade Deployment (Non-Admin) ===" -ForegroundColor Green
Write-Host "Installation location: $env:LOCALAPPDATA" -ForegroundColor Yellow

# Create local bin directory
$LocalBin = "$env:LOCALAPPDATA\bin"
if (-not (Test-Path $LocalBin)) {
    New-Item -ItemType Directory -Path $LocalBin -Force | Out-Null
}

# Add to PATH if not already there
$CurrentPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($CurrentPath -notlike "*$LocalBin*") {
    [Environment]::SetEnvironmentVariable("Path", "$CurrentPath;$LocalBin", "User")
    $env:Path = "$env:Path;$LocalBin"
    Write-Host "✓ Added $LocalBin to PATH" -ForegroundColor Green
}

# Install Scoop (package manager for Windows, no admin required)
if ($UseScoop -and -not (Get-Command scoop -ErrorAction SilentlyContinue)) {
    Write-Host "Installing Scoop package manager..." -ForegroundColor Yellow
    Invoke-RestMethod -Uri https://get.scoop.sh | Invoke-Expression
    Write-Host "✓ Scoop installed" -ForegroundColor Green
}

# Install Azure CLI
if (-not (Get-Command az -ErrorAction SilentlyContinue)) {
    Write-Host "Installing Azure CLI..." -ForegroundColor Yellow
    
    if ($UseScoop) {
        scoop install azure-cli
    } else {
        # Download and install using MSI (may require admin)
        Write-Host "Warning: Azure CLI MSI installation requires admin privileges" -ForegroundColor Red
        Write-Host "Alternative: Use 'winget install Microsoft.AzureCLI --scope user'" -ForegroundColor Yellow
        winget install Microsoft.AzureCLI --scope user --accept-package-agreements --accept-source-agreements
    }
    
    Write-Host "✓ Azure CLI installed" -ForegroundColor Green
} else {
    Write-Host "✓ Azure CLI already installed" -ForegroundColor Green
}

# Install kubectl
if (-not (Get-Command kubectl -ErrorAction SilentlyContinue)) {
    Write-Host "Installing kubectl..." -ForegroundColor Yellow
    
    if ($UseScoop) {
        scoop install kubectl
    } else {
        # Download kubectl binary
        $KubectlUrl = "https://dl.k8s.io/release/v1.31.0/bin/windows/amd64/kubectl.exe"
        Invoke-WebRequest -Uri $KubectlUrl -OutFile "$LocalBin\kubectl.exe"
    }
    
    Write-Host "✓ kubectl installed" -ForegroundColor Green
} else {
    Write-Host "✓ kubectl already installed" -ForegroundColor Green
}

# Install kubelogin
if (-not (Get-Command kubelogin -ErrorAction SilentlyContinue)) {
    Write-Host "Installing kubelogin..." -ForegroundColor Yellow
    
    $KubeloginVersion = "v0.1.4"
    $KubeloginUrl = "https://github.com/Azure/kubelogin/releases/download/$KubeloginVersion/kubelogin-win-amd64.zip"
    $TempZip = "$env:TEMP\kubelogin.zip"
    
    Invoke-WebRequest -Uri $KubeloginUrl -OutFile $TempZip
    Expand-Archive -Path $TempZip -DestinationPath $env:TEMP -Force
    Move-Item -Path "$env:TEMP\bin\windows_amd64\kubelogin.exe" -Destination "$LocalBin\kubelogin.exe" -Force
    Remove-Item $TempZip
    
    Write-Host "✓ kubelogin installed" -ForegroundColor Green
} else {
    Write-Host "✓ kubelogin already installed" -ForegroundColor Green
}

# Install Helm
if (-not (Get-Command helm -ErrorAction SilentlyContinue)) {
    Write-Host "Installing Helm..." -ForegroundColor Yellow
    
    if ($UseScoop) {
        scoop install helm
    } else {
        # Download Helm binary
        $HelmVersion = "v3.16.1"
        $HelmUrl = "https://get.helm.sh/helm-$HelmVersion-windows-amd64.zip"
        $TempZip = "$env:TEMP\helm.zip"
        
        Invoke-WebRequest -Uri $HelmUrl -OutFile $TempZip
        Expand-Archive -Path $TempZip -DestinationPath $env:TEMP -Force
        Move-Item -Path "$env:TEMP\windows-amd64\helm.exe" -Destination "$LocalBin\helm.exe" -Force
        Remove-Item $TempZip
    }
    
    Write-Host "✓ Helm installed" -ForegroundColor Green
} else {
    Write-Host "✓ Helm already installed" -ForegroundColor Green
}

# Install Git (if not present)
if (-not (Get-Command git -ErrorAction SilentlyContinue)) {
    Write-Host "Installing Git..." -ForegroundColor Yellow
    
    if ($UseScoop) {
        scoop install git
    } else {
        winget install Git.Git --scope user --accept-package-agreements --accept-source-agreements
    }
    
    Write-Host "✓ Git installed" -ForegroundColor Green
} else {
    Write-Host "✓ Git already installed" -ForegroundColor Green
}

# Install jq for JSON processing
if (-not (Get-Command jq -ErrorAction SilentlyContinue)) {
    Write-Host "Installing jq..." -ForegroundColor Yellow
    
    if ($UseScoop) {
        scoop install jq
    } else {
        $JqUrl = "https://github.com/jqlang/jq/releases/download/jq-1.7.1/jq-win64.exe"
        Invoke-WebRequest -Uri $JqUrl -OutFile "$LocalBin\jq.exe"
    }
    
    Write-Host "✓ jq installed" -ForegroundColor Green
} else {
    Write-Host "✓ jq already installed" -ForegroundColor Green
}

# Verify installations
Write-Host "`n=== Installation Summary ===" -ForegroundColor Green
Write-Host "Azure CLI:   $((az version | ConvertFrom-Json).'azure-cli')"
Write-Host "kubectl:     $(kubectl version --client --short 2>$null)"
Write-Host "kubelogin:   $(kubelogin --version)"
Write-Host "Helm:        $(helm version --short)"
Write-Host "Git:         $(git --version)"
Write-Host "jq:          $(jq --version)"

Write-Host "`n✓ All tools installed successfully!" -ForegroundColor Green
Write-Host "`nNext steps:" -ForegroundColor Yellow
Write-Host "  1. Restart terminal to refresh PATH"
Write-Host "  2. Configure Azure: .\scripts\windows\02-Configure-AKS.ps1"
Write-Host "  3. Deploy EasyTrade: .\scripts\windows\03-Deploy-EasyTrade.ps1"
