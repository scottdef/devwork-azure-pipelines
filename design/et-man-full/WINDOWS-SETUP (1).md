# Windows 11 Azure Virtual Desktop Setup Guide (Non-Admin)

Complete guide for deploying EasyTrade to AKS from a Windows 11 Azure Virtual Desktop without administrator permissions.

## Prerequisites

- Windows 11 Azure Virtual Desktop access
- Windows Terminal installed
- PowerShell 7+ installed
- Azure subscription access
- No administrator permissions required (user-space installation)

## Step 1: Initial Setup

Open Windows Terminal (PowerShell):

```powershell
# Check PowerShell version
$PSVersionTable.PSVersion
# Should be 7.0 or higher
```

## Step 2: Download Repository

Using PowerShell (if Git is not available):

```powershell
# Create working directory
New-Item -ItemType Directory -Path "$env:USERPROFILE\easytrade-aks-deployment" -Force
cd "$env:USERPROFILE\easytrade-aks-deployment"

# Download repository as ZIP
$repoUrl = "https://github.com/<your-org>/easytrade-aks-deployment/archive/refs/heads/main.zip"
Invoke-WebRequest -Uri $repoUrl -OutFile "repo.zip"

# Extract
Expand-Archive -Path "repo.zip" -DestinationPath "." -Force
Move-Item "easytrade-aks-deployment-main\*" "." -Force
Remove-Item "repo.zip", "easytrade-aks-deployment-main" -Force
```

Or using Git (if installed):

```powershell
git clone <your-repo-url> $env:USERPROFILE\easytrade-aks-deployment
cd $env:USERPROFILE\easytrade-aks-deployment
```

## Step 3: Install Required Tools (User Space)

Run the automated setup script:

```powershell
Set-ExecutionPolicy -Scope CurrentUser -ExecutionPolicy RemoteSigned -Force

.\setup\windows-setup.ps1
```

The script will:
- Create user-space directories: `~\.local\bin`
- Install kubectl, kubelogin, Helm (no admin required)
- Configure PATH for current user
- Create helper scripts

**Note**: Azure CLI requires one-time administrator installation. If not installed:
1. Contact your IT administrator, OR
2. Use Azure Cloud Shell as fallback (see Alternative Methods below)

## Step 4: Verify Installation

Close and reopen Windows Terminal, then check installations:

```powershell
az --version
kubectl version --client
kubelogin --version
helm version --short
```

All commands should work without errors.

## Step 5: Authenticate to Azure and AKS

Run the authentication helper script:

```powershell
cd $env:USERPROFILE\easytrade-aks-deployment\scripts
.\authenticate.ps1
```

This will:
1. Prompt for Azure login (browser-based authentication)
2. Set the correct subscription
3. Get AKS cluster credentials
4. Configure kubelogin
5. Verify cluster connectivity

## Step 6: Deploy EasyTrade

### Option A: Using PowerShell Scripts

Deploy using manifests:

```powershell
# Create namespace
kubectl create namespace easytrade

# Clone EasyTrade repo
git clone https://github.com/Dynatrace/easytrade.git $env:TEMP\easytrade

# Apply manifests
kubectl -n easytrade apply -f $env:TEMP\easytrade\kubernetes-manifests\release\

# Wait for pods to be ready
kubectl -n easytrade get pods -w
```

### Option B: Using Helm (Recommended)

```powershell
# Create namespace
kubectl create namespace easytrade

# Install with Helm (after creating custom chart)
helm install easytrade .\helm-chart\easytrade `
    --namespace easytrade `
    --create-namespace
```

## Step 7: Verify Deployment

Check deployment status:

```powershell
# Check pods
kubectl -n easytrade get pods

# Check services
kubectl -n easytrade get svc

# Check deployments
kubectl -n easytrade get deployments
```

All pods should show `Running` status.

## Step 8: Access the Application

### Method 1: kubectl port-forward

Start port-forward:

```powershell
.\scripts\port-forward.ps1
```

Or manually:

```powershell
kubectl -n easytrade port-forward svc/frontendreverseproxy 8080:80
```

Open browser to: http://localhost:8080

**Credentials:**
- `demouser` / `demopass`
- `specialuser` / `specialpass`

### Method 2: Private LoadBalancer

Deploy LoadBalancer service:

```powershell
kubectl apply -f .\manifests\access\loadbalancer-private.yaml

# Get IP address
kubectl -n easytrade get svc easytrade-private-lb `
    -o jsonpath='{.status.loadBalancer.ingress[0].ip}'
```

Access via: `http://<private-ip>`

## Step 9: Manage Problem Patterns

Problem patterns must be controlled via API since bash scripts won't run natively on Windows.

### PowerShell Helper Functions

Create a PowerShell module for feature flags:

```powershell
# Create module
$modulePath = "$env:USERPROFILE\Documents\PowerShell\Modules\EasyTradePatterns"
New-Item -ItemType Directory -Path $modulePath -Force

# Create module file
@'
function Enable-ProblemPattern {
    param([string]$Pattern)
    
    $url = "http://localhost:8080/feature-flag-service/v1/flags/${Pattern}/"
    $body = @{ enabled = $true } | ConvertTo-Json
    
    Invoke-RestMethod -Uri $url -Method Put `
        -ContentType "application/json" `
        -Body $body
}

function Disable-ProblemPattern {
    param([string]$Pattern)
    
    $url = "http://localhost:8080/feature-flag-service/v1/flags/${Pattern}/"
    $body = @{ enabled = $false } | ConvertTo-Json
    
    Invoke-RestMethod -Uri $url -Method Put `
        -ContentType "application/json" `
        -Body $body
}

function Get-ProblemPatterns {
    $url = "http://localhost:8080/feature-flag-service/v1/flags/"
    Invoke-RestMethod -Uri $url -Method Get
}

Export-ModuleMember -Function Enable-ProblemPattern, Disable-ProblemPattern, Get-ProblemPatterns
'@ | Out-File -FilePath "$modulePath\EasyTradePatterns.psm1" -Encoding UTF8

# Import module
Import-Module EasyTradePatterns
```

### Using the Module

Start port-forward first:

```powershell
Start-Job -ScriptBlock { kubectl -n easytrade port-forward svc/frontendreverseproxy 8080:80 }
```

Then use the functions:

```powershell
# List all patterns
Get-ProblemPatterns | ConvertTo-Json

# Enable high CPU usage
Enable-ProblemPattern -Pattern "high_cpu_usage"

# Check broker service CPU
kubectl -n easytrade top pods -l app=broker-service

# Disable pattern
Disable-ProblemPattern -Pattern "high_cpu_usage"
```

### Direct API Calls

```powershell
# Enable pattern
$body = @{ enabled = $true } | ConvertTo-Json
Invoke-RestMethod -Uri "http://localhost:8080/feature-flag-service/v1/flags/high_cpu_usage/" `
    -Method Put `
    -ContentType "application/json" `
    -Body $body

# Disable pattern
$body = @{ enabled = $false } | ConvertTo-Json
Invoke-RestMethod -Uri "http://localhost:8080/feature-flag-service/v1/flags/high_cpu_usage/" `
    -Method Put `
    -ContentType "application/json" `
    -Body $body
```

## Common Operations (PowerShell)

### View logs

```powershell
kubectl -n easytrade logs -l app=broker-service --tail=100
```

### Restart deployment

```powershell
kubectl -n easytrade rollout restart deployment/broker-service
```

### Scale deployment

```powershell
kubectl -n easytrade scale deployment/broker-service --replicas=3
```

### Get pod status

```powershell
kubectl -n easytrade get pods -o wide
```

### Open shell in pod

```powershell
kubectl -n easytrade exec -it deploy/broker-service -- /bin/sh
```

### Port-forward multiple services

```powershell
Start-Job { kubectl -n easytrade port-forward svc/frontendreverseproxy 8080:80 }
Start-Job { kubectl -n easytrade port-forward svc/feature-flag-service 8081:8080 }
Start-Job { kubectl -n easytrade port-forward svc/broker-service 8082:8080 }

# List jobs
Get-Job

# Stop all jobs
Get-Job | Stop-Job
Get-Job | Remove-Job
```

## Alternative Methods (No Admin Access)

### Using Azure Cloud Shell

If tool installation is blocked:

1. Open Azure Portal: https://portal.azure.com
2. Click Cloud Shell icon (>_) at top right
3. Choose Bash or PowerShell
4. All tools (az, kubectl, helm) are pre-installed

```bash
# In Cloud Shell
git clone <your-repo-url>
cd easytrade-aks-deployment

# Authenticate to AKS
az aks get-credentials \
    -g prod-cus-platform-base-rg-001 \
    -n prod-cus-aks-sre-lab-003

# Deploy
kubectl create namespace easytrade
kubectl -n easytrade apply -f manifests/
```

### Using VSCode Remote - Containers

If Docker Desktop is available:

1. Install VSCode extension: "Remote - Containers"
2. Create `.devcontainer/devcontainer.json`:

```json
{
  "name": "EasyTrade DevContainer",
  "image": "mcr.microsoft.com/azure-cli:latest",
  "features": {
    "kubectl-helm-minikube": "latest"
  }
}
```

3. Open in container: `Ctrl+Shift+P` → "Reopen in Container"

## Troubleshooting

### kubectl: command not found

Add to PATH manually:

```powershell
$env:Path += ";$env:USERPROFILE\.local\bin"
[Environment]::SetEnvironmentVariable(
    "Path",
    [Environment]::GetEnvironmentVariable("Path", "User") + ";$env:USERPROFILE\.local\bin",
    "User"
)
```

### Authentication failures

Clear cached tokens:

```powershell
Remove-Item -Path "$env:USERPROFILE\.kube\cache\kubelogin\*" -Recurse -Force
az aks get-credentials -g prod-cus-platform-base-rg-001 -n prod-cus-aks-sre-lab-003 --overwrite-existing
kubelogin convert-kubeconfig -l azurecli
```

### Port-forward won't stay open

Use background job:

```powershell
$job = Start-Job -ScriptBlock {
    kubectl -n easytrade port-forward svc/frontendreverseproxy 8080:80
}

# Check status
Get-Job -Id $job.Id

# Stop
Stop-Job -Id $job.Id
Remove-Job -Id $job.Id
```

### Can't install tools without admin

Use one of these alternatives:
1. **Azure Cloud Shell** (recommended - everything pre-installed)
2. **Request IT to install tools** (Azure CLI, kubectl, helm)
3. **Use GitHub Codespaces** (cloud-based development environment)
4. **Use portable versions** (Git Portable, kubectl.exe directly to user folder)

## VSCode Integration

### Recommended Extensions

Install these VSCode extensions for better Kubernetes development:

- Kubernetes (ms-kubernetes-tools.vscode-kubernetes-tools)
- YAML (redhat.vscode-yaml)
- Docker (ms-azuretools.vscode-docker)
- Azure Account (ms-vscode.azure-account)
- PowerShell (ms-vscode.powershell)

### Kubernetes Extension Configuration

1. Install Kubernetes extension
2. Click Kubernetes icon in sidebar
3. Right-click cluster → "Set as current context"
4. Browse namespaces, pods, services directly in VSCode

## Cleanup

Remove all EasyTrade resources:

```powershell
# Delete all resources in namespace
kubectl delete namespace easytrade

# Or delete specific resources
kubectl -n easytrade delete deployments --all
kubectl -n easytrade delete services --all
kubectl -n easytrade delete statefulsets --all
```

## Next Steps

- Set up Dynatrace monitoring
- Configure CI/CD with GitHub Actions
- Document standard operating procedures
- Test failover scenarios with problem patterns

## Support

For assistance:
1. Check Azure Cloud Shell as fallback
2. Contact IT for tool installation help
3. Review Azure documentation
4. Use VSCode Kubernetes extension for visual management
