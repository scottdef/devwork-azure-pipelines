# Windows 11 Azure Virtual Desktop Setup Script (Non-Admin)
# This script installs tools in user space without requiring admin permissions

param(
    [switch]$SkipAzCLI,
    [switch]$SkipKubectl,
    [switch]$SkipHelm,
    [switch]$SkipGit
)

$ErrorActionPreference = "Stop"

# Color output functions
function Write-InfoLog {
    param([string]$Message)
    Write-Host "[INFO] $Message" -ForegroundColor Blue
}

function Write-SuccessLog {
    param([string]$Message)
    Write-Host "[SUCCESS] $Message" -ForegroundColor Green
}

function Write-WarningLog {
    param([string]$Message)
    Write-Host "[WARNING] $Message" -ForegroundColor Yellow
}

function Write-ErrorLog {
    param([string]$Message)
    Write-Host "[ERROR] $Message" -ForegroundColor Red
}

# Setup directories
$UserBin = "$env:USERPROFILE\.local\bin"
$UserTools = "$env:USERPROFILE\.local\tools"

function Initialize-Directories {
    Write-InfoLog "Creating user-space directories..."
    
    New-Item -ItemType Directory -Force -Path $UserBin | Out-Null
    New-Item -ItemType Directory -Force -Path $UserTools | Out-Null
    New-Item -ItemType Directory -Force -Path "$env:USERPROFILE\easytrade-aks-deployment" | Out-Null
    
    Write-SuccessLog "Directories created"
}

# Add to PATH if not already present
function Add-ToUserPath {
    param([string]$PathToAdd)
    
    $currentPath = [Environment]::GetEnvironmentVariable("Path", "User")
    
    if ($currentPath -notlike "*$PathToAdd*") {
        Write-InfoLog "Adding $PathToAdd to user PATH..."
        [Environment]::SetEnvironmentVariable(
            "Path",
            "$currentPath;$PathToAdd",
            "User"
        )
        $env:Path = "$env:Path;$PathToAdd"
        Write-SuccessLog "Added to PATH"
    } else {
        Write-WarningLog "Already in PATH: $PathToAdd"
    }
}

# Install Azure CLI (MSI installer - requires one-time admin)
function Install-AzureCLI {
    if ($SkipAzCLI) {
        Write-WarningLog "Skipping Azure CLI installation"
        return
    }
    
    Write-InfoLog "Checking Azure CLI..."
    
    if (Get-Command az -ErrorAction SilentlyContinue) {
        $version = az --version | Select-Object -First 1
        Write-WarningLog "Azure CLI already installed: $version"
        return
    }
    
    Write-InfoLog "Azure CLI requires one-time admin installation"
    Write-InfoLog "Please download and install from: https://aka.ms/installazurecliwindows"
    Write-InfoLog "After installation, re-run this script"
    
    $choice = Read-Host "Open browser to download Azure CLI? (Y/N)"
    if ($choice -eq 'Y' -or $choice -eq 'y') {
        Start-Process "https://aka.ms/installazurecliwindows"
    }
}

# Install kubectl (no admin required)
function Install-Kubectl {
    if ($SkipKubectl) {
        Write-WarningLog "Skipping kubectl installation"
        return
    }
    
    Write-InfoLog "Installing kubectl..."
    
    if (Get-Command kubectl -ErrorAction SilentlyContinue) {
        $version = kubectl version --client --short 2>$null
        if (-not $version) {
            $version = kubectl version --client 2>$null
        }
        Write-WarningLog "kubectl already installed: $version"
        return
    }
    
    $kubectlPath = "$UserBin\kubectl.exe"
    $kubectlUrl = "https://dl.k8s.io/release/v1.31.0/bin/windows/amd64/kubectl.exe"
    
    try {
        Write-InfoLog "Downloading kubectl..."
        Invoke-WebRequest -Uri $kubectlUrl -OutFile $kubectlPath
        
        # Verify download
        if (Test-Path $kubectlPath) {
            Write-SuccessLog "kubectl installed to $kubectlPath"
            Add-ToUserPath $UserBin
        }
    } catch {
        Write-ErrorLog "Failed to download kubectl: $_"
    }
}

# Install kubelogin (no admin required)
function Install-Kubelogin {
    Write-InfoLog "Installing kubelogin..."
    
    if (Get-Command kubelogin -ErrorAction SilentlyContinue) {
        $version = kubelogin --version
        Write-WarningLog "kubelogin already installed: $version"
        return
    }
    
    $kubeloginPath = "$UserBin\kubelogin.exe"
    $kubeloginZip = "$UserTools\kubelogin.zip"
    $kubeloginUrl = "https://github.com/Azure/kubelogin/releases/download/v0.1.4/kubelogin-win-amd64.zip"
    
    try {
        Write-InfoLog "Downloading kubelogin..."
        Invoke-WebRequest -Uri $kubeloginUrl -OutFile $kubeloginZip
        
        Write-InfoLog "Extracting kubelogin..."
        Expand-Archive -Path $kubeloginZip -DestinationPath $UserTools -Force
        
        # Move to bin directory
        $extractedPath = Get-ChildItem -Path "$UserTools\bin\windows_amd64\kubelogin.exe" -Recurse -ErrorAction SilentlyContinue
        if ($extractedPath) {
            Copy-Item -Path $extractedPath.FullName -Destination $kubeloginPath -Force
            Write-SuccessLog "kubelogin installed to $kubeloginPath"
            Add-ToUserPath $UserBin
        } else {
            Write-ErrorLog "kubelogin.exe not found in extracted files"
        }
        
        # Cleanup
        Remove-Item $kubeloginZip -Force -ErrorAction SilentlyContinue
    } catch {
        Write-ErrorLog "Failed to install kubelogin: $_"
    }
}

# Install Helm (no admin required)
function Install-Helm {
    if ($SkipHelm) {
        Write-WarningLog "Skipping Helm installation"
        return
    }
    
    Write-InfoLog "Installing Helm..."
    
    if (Get-Command helm -ErrorAction SilentlyContinue) {
        $version = helm version --short
        Write-WarningLog "Helm already installed: $version"
        return
    }
    
    $helmPath = "$UserBin\helm.exe"
    $helmZip = "$UserTools\helm.zip"
    $helmUrl = "https://get.helm.sh/helm-v3.16.0-windows-amd64.zip"
    
    try {
        Write-InfoLog "Downloading Helm..."
        Invoke-WebRequest -Uri $helmUrl -OutFile $helmZip
        
        Write-InfoLog "Extracting Helm..."
        Expand-Archive -Path $helmZip -DestinationPath $UserTools -Force
        
        # Move to bin directory
        $extractedHelm = "$UserTools\windows-amd64\helm.exe"
        if (Test-Path $extractedHelm) {
            Copy-Item -Path $extractedHelm -Destination $helmPath -Force
            Write-SuccessLog "Helm installed to $helmPath"
            Add-ToUserPath $UserBin
        } else {
            Write-ErrorLog "helm.exe not found in extracted files"
        }
        
        # Cleanup
        Remove-Item $helmZip -Force -ErrorAction SilentlyContinue
        Remove-Item "$UserTools\windows-amd64" -Recurse -Force -ErrorAction SilentlyContinue
    } catch {
        Write-ErrorLog "Failed to install Helm: $_"
    }
}

# Install Git (portable version - no admin required)
function Install-Git {
    if ($SkipGit) {
        Write-WarningLog "Skipping Git installation"
        return
    }
    
    Write-InfoLog "Checking Git..."
    
    if (Get-Command git -ErrorAction SilentlyContinue) {
        $version = git --version
        Write-WarningLog "Git already installed: $version"
        return
    }
    
    Write-InfoLog "Git Portable requires manual download"
    Write-InfoLog "Please download from: https://git-scm.com/download/win"
    Write-InfoLog "Choose 'Portable' version and extract to: $UserTools\git"
    Write-InfoLog "Then add $UserTools\git\bin to your PATH"
    
    $choice = Read-Host "Open browser to download Git Portable? (Y/N)"
    if ($choice -eq 'Y' -or $choice -eq 'y') {
        Start-Process "https://git-scm.com/download/win"
    }
}

# Configure kubectl context helper
function Create-KubectlHelper {
    $helperScript = @"
# kubectl context helper for EasyTrade AKS
function Connect-EasyTradeAKS {
    `$subscription = "tango-CICD-platform-github-gitflow"
    `$resourceGroup = "prod-cus-platform-base-rg-001"
    `$clusterName = "prod-cus-aks-sre-lab-003"
    
    Write-Host "Authenticating to Azure..." -ForegroundColor Blue
    az login
    az account set --subscription `$subscription
    
    Write-Host "Getting AKS credentials..." -ForegroundColor Blue
    az aks get-credentials -g `$resourceGroup -n `$clusterName --overwrite-existing
    
    Write-Host "Converting kubeconfig for kubelogin..." -ForegroundColor Blue
    kubelogin convert-kubeconfig -l azurecli
    
    Write-Host "Verifying connection..." -ForegroundColor Blue
    kubectl get nodes
}

function Start-EasyTradePortForward {
    kubectl -n easytrade port-forward svc/frontendreverseproxy 8080:80
}

Export-ModuleMember -Function Connect-EasyTradeAKS, Start-EasyTradePortForward
"@

    $modulePath = "$env:USERPROFILE\Documents\WindowsPowerShell\Modules\EasyTradeHelpers"
    New-Item -ItemType Directory -Force -Path $modulePath | Out-Null
    
    $helperScript | Out-File -FilePath "$modulePath\EasyTradeHelpers.psm1" -Encoding UTF8
    
    Write-SuccessLog "PowerShell helper module created at: $modulePath"
    Write-InfoLog "Import with: Import-Module EasyTradeHelpers"
}

# Create deployment scripts
function Create-DeploymentScripts {
    Write-InfoLog "Creating deployment helper scripts..."
    
    $deployDir = "$env:USERPROFILE\easytrade-aks-deployment\scripts"
    New-Item -ItemType Directory -Force -Path $deployDir | Out-Null
    
    # Authenticate script
    $authScript = @"
# Authenticate to AKS cluster
`$ErrorActionPreference = "Stop"

`$subscription = "tango-CICD-platform-github-gitflow"
`$resourceGroup = "prod-cus-platform-base-rg-001"
`$clusterName = "prod-cus-aks-sre-lab-003"

Write-Host "Authenticating to Azure..." -ForegroundColor Blue
az login
az account set --subscription `$subscription

Write-Host "Getting AKS credentials..." -ForegroundColor Blue
az aks get-credentials -g `$resourceGroup -n `$clusterName --overwrite-existing

Write-Host "Converting kubeconfig for kubelogin..." -ForegroundColor Blue
kubelogin convert-kubeconfig -l azurecli

Write-Host "Verifying cluster access..." -ForegroundColor Green
kubectl cluster-info
kubectl get nodes

Write-Host "Authentication complete!" -ForegroundColor Green
"@
    
    $authScript | Out-File -FilePath "$deployDir\authenticate.ps1" -Encoding UTF8
    
    # Port-forward script
    $portForwardScript = @"
# Start port-forward to EasyTrade frontend
`$namespace = "easytrade"

Write-Host "Starting port-forward to localhost:8080..." -ForegroundColor Blue
Write-Host "Access EasyTrade at: http://localhost:8080" -ForegroundColor Green
kubectl -n `$namespace port-forward svc/frontendreverseproxy 8080:80
"@
    
    $portForwardScript | Out-File -FilePath "$deployDir\port-forward.ps1" -Encoding UTF8
    
    Write-SuccessLog "Deployment scripts created in: $deployDir"
}

# Verify installations
function Test-Installations {
    Write-InfoLog "Verifying installations..."
    
    $tools = @{
        "az" = "Azure CLI"
        "kubectl" = "kubectl"
        "kubelogin" = "kubelogin"
        "helm" = "Helm"
        "git" = "Git"
    }
    
    $allGood = $true
    
    foreach ($cmd in $tools.Keys) {
        if (Get-Command $cmd -ErrorAction SilentlyContinue) {
            Write-Host "✓ $($tools[$cmd])" -ForegroundColor Green
        } else {
            Write-Host "✗ $($tools[$cmd])" -ForegroundColor Red
            $allGood = $false
        }
    }
    
    if ($allGood) {
        Write-SuccessLog "All tools installed successfully!"
    } else {
        Write-WarningLog "Some tools are missing. Check output above."
        Write-InfoLog "You may need to close and reopen PowerShell for PATH changes to take effect"
    }
}

# Display next steps
function Show-NextSteps {
    Write-Host ""
    Write-SuccessLog "Windows setup complete!"
    Write-Host ""
    Write-Host "Next steps:" -ForegroundColor Yellow
    Write-Host "1. Close and reopen PowerShell for PATH changes to take effect"
    Write-Host ""
    Write-Host "2. Authenticate to AKS cluster:"
    Write-Host "   cd $env:USERPROFILE\easytrade-aks-deployment\scripts" -ForegroundColor Cyan
    Write-Host "   .\authenticate.ps1" -ForegroundColor Cyan
    Write-Host ""
    Write-Host "3. Deploy EasyTrade using manifests:"
    Write-Host "   kubectl create namespace easytrade" -ForegroundColor Cyan
    Write-Host "   kubectl -n easytrade apply -f ..\manifests\release\" -ForegroundColor Cyan
    Write-Host ""
    Write-Host "4. Or deploy using Helm:"
    Write-Host "   helm install easytrade ..\helm-chart\easytrade --namespace easytrade --create-namespace" -ForegroundColor Cyan
    Write-Host ""
    Write-Host "5. Access the application:"
    Write-Host "   .\port-forward.ps1" -ForegroundColor Cyan
    Write-Host "   # Then open browser to http://localhost:8080"
    Write-Host ""
    Write-Host "Alternative: Use PowerShell helper module" -ForegroundColor Yellow
    Write-Host "   Import-Module EasyTradeHelpers" -ForegroundColor Cyan
    Write-Host "   Connect-EasyTradeAKS" -ForegroundColor Cyan
    Write-Host "   Start-EasyTradePortForward" -ForegroundColor Cyan
    Write-Host ""
}

# Main execution
function Main {
    Write-InfoLog "Starting Windows 11 AVD setup for EasyTrade AKS deployment..."
    Write-WarningLog "This script installs tools in user space (no admin required)"
    
    Initialize-Directories
    Add-ToUserPath $UserBin
    
    Install-AzureCLI
    Install-Kubectl
    Install-Kubelogin
    Install-Helm
    Install-Git
    
    Create-KubectlHelper
    Create-DeploymentScripts
    
    Test-Installations
    Show-NextSteps
}

# Run main function
Main
