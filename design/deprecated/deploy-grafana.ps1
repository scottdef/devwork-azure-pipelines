# Grafana OSS v12 Deployment Script for AKS (PowerShell)
# AKS Cluster: supercool-aks-cluster
# kubectl version: 1.30

param(
    [string]$Command = "deploy"
)

# Set error action preference
$ErrorActionPreference = "Stop"

# Function to print colored output
function Write-Status {
    param([string]$Message)
    Write-Host "[INFO] $Message" -ForegroundColor Green
}

function Write-Warning {
    param([string]$Message)
    Write-Host "[WARNING] $Message" -ForegroundColor Yellow
}

function Write-Error {
    param([string]$Message)
    Write-Host "[ERROR] $Message" -ForegroundColor Red
}

function Write-Step {
    param([string]$Message)
    Write-Host "[STEP] $Message" -ForegroundColor Blue
}

# Check prerequisites
function Test-Prerequisites {
    Write-Step "Checking prerequisites..."

    # Check kubectl
    try {
        $null = Get-Command kubectl -ErrorAction Stop
    }
    catch {
        Write-Error "kubectl is not installed or not in PATH"
        exit 1
    }

    # Check kubectl version
    try {
        $kubectlVersionOutput = kubectl version --client -o yaml 2>$null
        $kubectlVersion = ($kubectlVersionOutput | Select-String "gitVersion").ToString().Split('"')[1]
        Write-Status "kubectl version: $kubectlVersion"
    }
    catch {
        Write-Warning "Could not get kubectl version"
    }

    # Check cluster connectivity
    try {
        $null = kubectl cluster-info 2>$null
    }
    catch {
        Write-Error "Cannot connect to Kubernetes cluster. Check your kubectl configuration."
        exit 1
    }

    # Check if connected to the right cluster
    try {
        $currentContext = kubectl config current-context 2>$null
        Write-Status "Current context: $currentContext"

        if ($currentContext -notlike "*supercool-aks-cluster*") {
            Write-Warning "You are not connected to 'supercool-aks-cluster'. Current context: $currentContext"
            $response = Read-Host "Do you want to continue anyway? (y/N)"
            if ($response -ne 'y' -and $response -ne 'Y') {
                exit 1
            }
        }
    }
    catch {
        Write-Warning "Could not get current context"
    }

    Write-Status "Prerequisites check completed"
}

# Connect to AKS cluster
function Connect-ToAks {
    Write-Step "Connecting to AKS cluster..."

    $resourceGroup = Read-Host "Enter your Azure resource group name"

    if ([string]::IsNullOrWhiteSpace($resourceGroup)) {
        Write-Error "Resource group name cannot be empty"
        exit 1
    }

    Write-Status "Logging into Azure..."
    az login

    Write-Status "Getting AKS credentials..."
    az aks get-credentials --resource-group $resourceGroup --name supercool-aks-cluster --overwrite-existing

    Write-Status "Verifying cluster connection..."
    kubectl cluster-info
    kubectl get nodes
}

# Deploy Grafana
function Deploy-Grafana {
    Write-Step "Deploying Grafana OSS v12..."

    # Check if grafana-manifests.yaml exists
    if (-not (Test-Path "grafana-manifests.yaml")) {
        Write-Error "grafana-manifests.yaml not found in current directory"
        Write-Status "Please ensure the Kubernetes manifests file is present"
        exit 1
    }

    Write-Status "Applying Grafana manifests..."
    kubectl apply -f grafana-manifests.yaml

    Write-Status "Waiting for namespace to be ready..."
    kubectl wait --for=condition=Ready namespace/grafana --timeout=60s

    Write-Status "Waiting for PVC to be bound..."
    kubectl wait --for=condition=Bound pvc/grafana-pvc -n grafana --timeout=300s

    Write-Status "Waiting for deployment to be ready..."
    kubectl wait --for=condition=Available deployment/grafana -n grafana --timeout=600s

    Write-Status "Waiting for pods to be ready..."
    kubectl wait --for=condition=Ready pod -l app=grafana -n grafana --timeout=300s

    Write-Status "Grafana deployment completed successfully!"
}

# Verify deployment
function Test-Deployment {
    Write-Step "Verifying Grafana deployment..."

    Write-Status "Checking namespace:"
    kubectl get namespace grafana

    Write-Status "Checking ConfigMaps and Secrets:"
    kubectl get configmap,secret -n grafana

    Write-Status "Checking PersistentVolumeClaim:"
    kubectl get pvc -n grafana

    Write-Status "Checking Deployment:"
    kubectl get deployment -n grafana -o wide

    Write-Status "Checking Pods:"
    kubectl get pods -n grafana -o wide

    Write-Status "Checking Services:"
    kubectl get svc -n grafana

    Write-Status "Checking Pod logs (last 10 lines):"
    kubectl logs deployment/grafana -n grafana --tail=10

    # Check if pod is running
    try {
        $podStatus = kubectl get pods -n grafana -l app=grafana -o jsonpath='{.items[0].status.phase}' 2>$null
        if ($podStatus -eq "Running") {
            Write-Status "✅ Grafana pod is running successfully"
        }
        else {
            Write-Warning "⚠️  Grafana pod status: $podStatus"
        }
    }
    catch {
        Write-Warning "Could not get pod status"
    }

    Write-Status "Deployment verification completed"
}

# Test connectivity
function Test-Connectivity {
    Write-Step "Testing Grafana connectivity..."

    Write-Status "Testing internal cluster connectivity..."
    kubectl run test-grafana-connectivity --image=curlimages/curl --rm -i --tty --restart=Never --namespace=grafana -- curl -I http://grafana.grafana.svc.cluster.local/api/health

    Write-Status "Internal connectivity test completed"
}

# Setup port forwarding
function Start-PortForward {
    Write-Step "Setting up port forwarding for browser access..."

    Write-Status "Starting port-forward in background..."

    # Start kubectl port-forward as background job
    $job = Start-Job -ScriptBlock {
        kubectl port-forward -n grafana --address 0.0.0.0 svc/grafana 3000:80
    }

    Start-Sleep -Seconds 5

    if ($job.State -eq "Running") {
        Write-Status "✅ Port-forward started successfully (Job ID: $($job.Id))"
        Write-Status "🌐 Grafana is now accessible at: http://localhost:3000"
        Write-Status "👤 Username: admin"
        Write-Status "🔑 Password: GrafanaAdmin123!"
        Write-Status ""
        Write-Status "To stop port-forward later, run: Stop-Job -Id $($job.Id); Remove-Job -Id $($job.Id)"
        Write-Status "Or use: Get-Job | Where-Object {`$_.Command -like '*port-forward*'} | Stop-Job"

        # Save job ID to file for later reference
        $job.Id | Out-File -FilePath "$env:TEMP\grafana-portforward-job.txt"
    }
    else {
        Write-Error "❌ Failed to start port-forward"
        Remove-Job -Job $job -Force
        exit 1
    }
}

# Get admin password
function Get-AdminPassword {
    Write-Step "Retrieving admin password..."

    try {
        $adminPasswordBase64 = kubectl get secret grafana-admin-secret -n grafana -o jsonpath='{.data.admin-password}' 2>$null
        $adminPassword = [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String($adminPasswordBase64))
        Write-Status "Admin Username: admin"
        Write-Status "Admin Password: $adminPassword"
    }
    catch {
        Write-Warning "Could not retrieve admin password from secret"
    }
}

# Show access information
function Show-AccessInfo {
    Write-Step "Access Information"

    Write-Host ""
    Write-Host "======================================" -ForegroundColor Cyan
    Write-Host "   Grafana OSS v12 - Access Info" -ForegroundColor Cyan
    Write-Host "======================================" -ForegroundColor Cyan
    Write-Host ""
    Write-Host "🔗 Local Access (port-forward):" -ForegroundColor Yellow
    Write-Host "   URL: http://localhost:3000"
    Write-Host "   Username: admin"
    Write-Host "   Password: GrafanaAdmin123!"
    Write-Host ""
    Write-Host "🔗 Alternative port-forward command:" -ForegroundColor Yellow
    Write-Host "   kubectl port-forward -n grafana svc/grafana 3000:80"
    Write-Host ""
    Write-Host "🔗 Remote access (from your server):" -ForegroundColor Yellow
    Write-Host "   kubectl port-forward -n grafana --address 0.0.0.0 svc/grafana 3000:80"
    Write-Host "   Then access via: http://<server-ip>:3000"
    Write-Host ""
    Write-Host "📊 Monitoring:" -ForegroundColor Yellow
    Write-Host "   kubectl get all -n grafana"
    Write-Host "   kubectl logs deployment/grafana -n grafana -f"
    Write-Host ""
    Write-Host "🔧 Troubleshooting:" -ForegroundColor Yellow
    Write-Host "   kubectl describe pod -l app=grafana -n grafana"
    Write-Host "   kubectl get events -n grafana --sort-by='.lastTimestamp'"
    Write-Host ""
    Write-Host "🗑️  Uninstall:" -ForegroundColor Yellow
    Write-Host "   kubectl delete -f grafana-manifests.yaml"
    Write-Host ""
    Write-Host "======================================" -ForegroundColor Cyan
}

# Main function
function Main {
    param([string]$Command)

    Write-Host ""
    Write-Host "======================================" -ForegroundColor Cyan
    Write-Host "  Grafana OSS v12 Deployment Script" -ForegroundColor Cyan
    Write-Host "  AKS Cluster: supercool-aks-cluster" -ForegroundColor Cyan
    Write-Host "======================================" -ForegroundColor Cyan
    Write-Host ""

    switch ($Command.ToLower()) {
        "connect" {
            Connect-ToAks
        }
        "deploy" {
            Test-Prerequisites
            Deploy-Grafana
            Test-Deployment
            Test-Connectivity
            Get-AdminPassword
            Show-AccessInfo
        }
        "verify" {
            Test-Deployment
        }
        "test" {
            Test-Connectivity
        }
        "port-forward" {
            Start-PortForward
        }
        "password" {
            Get-AdminPassword
        }
        "info" {
            Show-AccessInfo
        }
        "help" {
            Write-Host "Usage: .\deploy-grafana.ps1 [command]" -ForegroundColor Yellow
            Write-Host ""
            Write-Host "Commands:" -ForegroundColor Green
            Write-Host "  connect      - Connect to AKS cluster"
            Write-Host "  deploy       - Deploy Grafana (default)"
            Write-Host "  verify       - Verify deployment"
            Write-Host "  test         - Test connectivity"
            Write-Host "  port-forward - Setup port forwarding"
            Write-Host "  password     - Get admin password"
            Write-Host "  info         - Show access information"
            Write-Host "  help         - Show this help message"
        }
        default {
            Write-Error "Unknown command: $Command"
            Write-Host "Run '.\deploy-grafana.ps1 help' for usage information" -ForegroundColor Yellow
            exit 1
        }
    }
}

# Run main function
Main -Command $Command
