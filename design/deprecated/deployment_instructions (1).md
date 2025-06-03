# Grafana OSS v12 PowerShell Deployment Guide for AKS

Complete PowerShell commands to deploy Grafana OSS v12 on Azure Kubernetes Service (AKS) using Helm.

## Environment Setup

- **AKS Cluster**: `supercool-aks-cluster`
- **Grafana Version**: OSS v12.0.0
- **kubectl Version**: 1.30
- **Helm Binary**: `C:\Users\user01\kube-bin\helm.exe`
- **Values File**: `C:\Users\user01\kube-bin\my-values\custom-values.yaml`
- **Platform**: Windows 11 with PowerShell

## Prerequisites Commands

### Connect to AKS Cluster

```powershell
# Connect to AKS cluster (adjust resource group name)
az aks get-credentials --resource-group <your-resource-group> --name supercool-aks-cluster --overwrite-existing

# Verify connection
kubectl cluster-info
kubectl get nodes

# Check current context
kubectl config current-context
```

## Helm Installation Commands

### 1. Add Grafana Helm Repository

```powershell
# Add the official Grafana Helm repository
& "C:\Users\user01\kube-bin\helm.exe" repo add grafana https://grafana.github.io/helm-charts

# Update repository information
& "C:\Users\user01\kube-bin\helm.exe" repo update

# Verify repository was added
& "C:\Users\user01\kube-bin\helm.exe" repo list

# Search for available Grafana chart versions
& "C:\Users\user01\kube-bin\helm.exe" search repo grafana/grafana --versions
```

### 2. Create Namespace

```powershell
# Create dedicated namespace for Grafana
kubectl create namespace grafana

# Alternatively, create namespace declaratively
kubectl create namespace grafana --dry-run=client -o yaml | kubectl apply -f -
```

### 3. Install Grafana OSS v12

```powershell
# Install Grafana using your custom values file
& "C:\Users\user01\kube-bin\helm.exe" install grafana grafana/grafana `
  --namespace grafana `
  --values "C:\Users\user01\kube-bin\my-values\custom-values.yaml" `
  --version 8.0.0 `
  --create-namespace

# Alternative: Install with specific timeout
& "C:\Users\user01\kube-bin\helm.exe" install grafana grafana/grafana `
  --namespace grafana `
  --values "C:\Users\user01\kube-bin\my-values\custom-values.yaml" `
  --version 8.0.0 `
  --timeout 10m `
  --wait
```

### 4. Verify Installation

```powershell
# Check Helm release status
& "C:\Users\user01\kube-bin\helm.exe" status grafana -n grafana

# List Helm releases
& "C:\Users\user01\kube-bin\helm.exe" list -n grafana

# Check all Kubernetes resources
kubectl get all -n grafana

# Check specific resources
kubectl get pods -n grafana -o wide
kubectl get svc -n grafana
kubectl get pvc -n grafana

# Check pod logs
kubectl logs deployment/grafana -n grafana --tail=20

# Wait for deployment to be ready
kubectl wait --for=condition=Available deployment/grafana -n grafana --timeout=600s

# Wait for pods to be ready
kubectl wait --for=condition=Ready pod -l app.kubernetes.io/name=grafana -n grafana --timeout=300s
```

## Access Grafana

### Set up Port Forwarding

```powershell
# Start port-forward for local access
kubectl port-forward -n grafana svc/grafana 3000:80

# For remote access (binds to all interfaces)
kubectl port-forward -n grafana --address 0.0.0.0 svc/grafana 3000:80
```

### Start Port Forward as Background Job

```powershell
# Start port-forward as background job
$job = Start-Job -ScriptBlock {
    kubectl port-forward -n grafana --address 0.0.0.0 svc/grafana 3000:80
}

Write-Host "Port-forward started as background job (ID: $($job.Id))" -ForegroundColor Green
Write-Host "Grafana will be accessible at: http://localhost:3000" -ForegroundColor Yellow

# Check job status
Get-Job -Id $job.Id

# To stop the port-forward later:
# Stop-Job -Id $job.Id; Remove-Job -Id $job.Id
```

### Get Admin Password

```powershell
# Retrieve and decode admin password from secret
$adminPasswordBase64 = kubectl get secret grafana-admin-secret -n grafana -o jsonpath='{.data.admin-password}'
$adminPassword = [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String($adminPasswordBase64))
Write-Host "Admin Username: admin" -ForegroundColor Green
Write-Host "Admin Password: $adminPassword" -ForegroundColor Green

# Alternative: One-liner to get password
$pwd = kubectl get secret grafana-admin-secret -n grafana -o jsonpath='{.data.admin-password}'
[System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String($pwd))
```

## Comprehensive Deployment Script

```powershell
# Grafana OSS v12 Deployment Script
Write-Host "Starting Grafana OSS v12 deployment..." -ForegroundColor Cyan

# Set variables
$helmPath = "C:\Users\user01\kube-bin\helm.exe"
$valuesFile = "C:\Users\user01\kube-bin\my-values\custom-values.yaml"

try {
    # Step 1: Add Helm repository
    Write-Host "Adding Grafana Helm repository..." -ForegroundColor Yellow
    & $helmPath repo add grafana https://grafana.github.io/helm-charts
    & $helmPath repo update

    # Step 2: Create namespace
    Write-Host "Creating grafana namespace..." -ForegroundColor Yellow
    kubectl create namespace grafana --dry-run=client -o yaml | kubectl apply -f -

    # Step 3: Install Grafana
    Write-Host "Installing Grafana OSS v12..." -ForegroundColor Yellow
    & $helmPath install grafana grafana/grafana `
      --namespace grafana `
      --values $valuesFile `
      --version 8.0.0 `
      --create-namespace

    # Step 4: Wait for deployment
    Write-Host "Waiting for Grafana to be ready..." -ForegroundColor Yellow
    kubectl wait --for=condition=Available deployment/grafana -n grafana --timeout=600s

    # Step 5: Get admin password
    Write-Host "Retrieving admin credentials..." -ForegroundColor Yellow
    Start-Sleep -Seconds 10  # Wait for secret to be created
    $adminPasswordBase64 = kubectl get secret grafana-admin-secret -n grafana -o jsonpath='{.data.admin-password}' 2>$null
    
    if ($adminPasswordBase64) {
        $adminPassword = [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String($adminPasswordBase64))
        Write-Host "✅ Grafana deployed successfully!" -ForegroundColor Green
        Write-Host "Username: admin" -ForegroundColor White
        Write-Host "Password: $adminPassword" -ForegroundColor White
    } else {
        Write-Host "⚠️ Deployment completed but could not retrieve password" -ForegroundColor Yellow
    }

    # Step 6: Show access information
    Write-Host "`n📋 Access Information:" -ForegroundColor Cyan
    Write-Host "Local URL: http://localhost:3000" -ForegroundColor White
    Write-Host "Port-forward command: kubectl port-forward -n grafana svc/grafana 3000:80" -ForegroundColor Gray
    
    # Step 7: Start port-forward
    $startPortForward = Read-Host "`nStart port-forward now? (y/N)"
    if ($startPortForward -eq 'y' -or $startPortForward -eq 'Y') {
        Write-Host "Starting port-forward..." -ForegroundColor Yellow
        kubectl port-forward -n grafana svc/grafana 3000:80
    }
    
} catch {
    Write-Host "❌ Deployment failed: $($_.Exception.Message)" -ForegroundColor Red
    exit 1
}
```

## Upgrade Commands

```powershell
# Update repository
& "C:\Users\user01\kube-bin\helm.exe" repo update

# Upgrade Grafana
& "C:\Users\user01\kube-bin\helm.exe" upgrade grafana grafana/grafana `
  --namespace grafana `
  --values "C:\Users\user01\kube-bin\my-values\custom-values.yaml" `
  --version 8.0.0

# Check upgrade status
& "C:\Users\user01\kube-bin\helm.exe" history grafana -n grafana

# Monitor rollout
kubectl rollout status deployment/grafana -n grafana

# Rollback if needed
& "C:\Users\user01\kube-bin\helm.exe" rollback grafana 1 -n grafana
```

## Troubleshooting Commands

### Helm-Related Troubleshooting

```powershell
# Check Helm release details
& "C:\Users\user01\kube-bin\helm.exe" status grafana -n grafana
& "C:\Users\user01\kube-bin\helm.exe" get values grafana -n grafana
& "C:\Users\user01\kube-bin\helm.exe" get manifest grafana -n grafana

# Debug Helm installation
& "C:\Users\user01\kube-bin\helm.exe" install grafana grafana/grafana `
  --namespace grafana `
  --values "C:\Users\user01\kube-bin\my-values\custom-values.yaml" `
  --version 8.0.0 `
  --dry-run --debug
```

### Kubernetes Troubleshooting

```powershell
# Check pod status and logs
kubectl get pods -n grafana -o wide
kubectl describe pod -l app.kubernetes.io/name=grafana -n grafana
kubectl logs deployment/grafana -n grafana --tail=50
kubectl logs deployment/grafana -n grafana --previous

# Check events
kubectl get events -n grafana --sort-by='.lastTimestamp'

# Check service endpoints
kubectl get endpoints grafana -n grafana
kubectl describe svc grafana -n grafana

# Check persistent volumes
kubectl get pv,pvc -n grafana
kubectl describe pvc -n grafana

# Test connectivity
kubectl run test-grafana --image=curlimages/curl --rm -it --restart=Never -- curl -I http://grafana.grafana.svc.cluster.local/api/health
```

### Network Testing

```powershell
# Test DNS resolution
kubectl run dns-test --image=busybox --rm -it --restart=Never -- nslookup grafana.grafana.svc.cluster.local

# Test internal connectivity
kubectl run curl-test --image=curlimages/curl --rm -it --restart=Never -- curl -v http://grafana.grafana.svc.cluster.local/api/health

# Comprehensive network test
kubectl run network-test --image=nicolaka/netshoot --rm -it --restart=Never -- /bin/bash
```

## Port Forward Management

```powershell
# List running port-forward jobs
Get-Job | Where-Object { $_.Command -like "*port-forward*" }

# Stop all port-forward jobs
Get-Job | Where-Object { $_.Command -like "*port-forward*" } | Stop-Job
Get-Job | Where-Object { $_.Command -like "*port-forward*" } | Remove-Job

# Start new port-forward with status monitoring
$portForwardJob = Start-Job -ScriptBlock {
    kubectl port-forward -n grafana svc/grafana 3000:80
}

# Monitor job status
if ($portForwardJob.State -eq "Running") {
    Write-Host "✅ Port-forward active - Access Grafana at http://localhost:3000" -ForegroundColor Green
} else {
    Write-Host "❌ Port-forward failed to start" -ForegroundColor Red
    Receive-Job -Job $portForwardJob
}

# Check if port is available locally
Test-NetConnection -ComputerName localhost -Port 3000
```

## Maintenance Commands

### Backup and Restore

```powershell
# Backup Helm values
& "C:\Users\user01\kube-bin\helm.exe" get values grafana -n grafana > grafana-values-backup.yaml

# Backup Kubernetes resources
kubectl get all -n grafana -o yaml > grafana-k8s-backup.yaml

# Backup persistent data
$podName = kubectl get pods -n grafana -l app.kubernetes.io/name=grafana -o jsonpath='{.items[0].metadata.name}'
kubectl exec -n grafana $podName -- tar czf - /var/lib/grafana | Set-Content -Path "grafana-data-backup.tar.gz" -AsByteStream
```

### Update Resources

```powershell
# Scale deployment (vertical scaling only for Grafana)
kubectl patch deployment grafana -n grafana -p '{"spec":{"template":{"spec":{"containers":[{"name":"grafana","resources":{"limits":{"memory":"2Gi","cpu":"2000m"}}}]}}}}'

# Update admin password
$newPassword = "NewSecurePassword123!"
$encodedPassword = [Convert]::ToBase64String([System.Text.Encoding]::UTF8.GetBytes($newPassword))
kubectl patch secret grafana-admin-secret -n grafana -p "{`"data`":{`"admin-password`":`"$encodedPassword`"}}"

# Restart deployment
kubectl rollout restart deployment/grafana -n grafana
```

## Uninstall Commands

```powershell
# Stop any running port-forwards
Get-Job | Where-Object { $_.Command -like "*port-forward*" } | Stop-Job
Get-Job | Where-Object { $_.Command -like "*port-forward*" } | Remove-Job

# Uninstall Grafana
& "C:\Users\user01\kube-bin\helm.exe" uninstall grafana -n grafana

# Delete persistent volume claims (optional - removes data)
kubectl delete pvc -n grafana --all

# Delete namespace
kubectl delete namespace grafana

# Remove Helm repository (optional)
& "C:\Users\user01\kube-bin\helm.exe" repo remove grafana

Write-Host "✅ Grafana uninstalled successfully" -ForegroundColor Green
```

## Sample Custom Values File

Create your `C:\Users\user01\kube-bin\my-values\custom-values.yaml` with:

```yaml
# Image configuration for Grafana OSS v12
image:
  repository: grafana/grafana-oss
  tag: "12.0.0"
  pullPolicy: IfNotPresent

# Admin credentials
adminUser: admin
adminPassword: "SecureGrafanaPassword123!"

# Persistence configuration
persistence:
  enabled: true
  size: 10Gi
  storageClassName: managed-csi
  accessModes:
    - ReadWriteOnce

# Service configuration
service:
  type: ClusterIP
  port: 80
  targetPort: 3000
  annotations: {}

# Resource configuration
resources:
  requests:
    cpu: 250m
    memory: 512Mi
  limits:
    cpu: 1000m
    memory: 1Gi

# Security context
securityContext:
  runAsNonRoot: true
  runAsUser: 472
  runAsGroup: 472
  fsGroup: 472

# Pod security context
podSecurityContext:
  runAsNonRoot: true
  runAsUser: 472
  runAsGroup: 472
  fsGroup: 472

# Grafana configuration
grafana.ini:
  server:
    domain: localhost
    root_url: "http://localhost:3000/"
    http_port: 3000
    enable_gzip: true
  security:
    admin_user: admin
    admin_password: SecureGrafanaPassword123!
    allow_embedding: false
    cookie_secure: false
  analytics:
    reporting_enabled: false
    check_for_updates: false
  log:
    mode: console
    level: info
  database:
    type: sqlite3
  plugins:
    enable_alpha: true

# Health checks
livenessProbe:
  httpGet:
    path: /api/health
    port: 3000
  initialDelaySeconds: 60
  timeoutSeconds: 30
  failureThreshold: 3

readinessProbe:
  httpGet:
    path: /api/health
    port: 3000
  initialDelaySeconds: 30
  timeoutSeconds: 30
  failureThreshold: 3

# Deployment strategy
deploymentStrategy:
  type: RollingUpdate
  rollingUpdate:
    maxSurge: 1
    maxUnavailable: 0

# Node selector and affinity (optional)
nodeSelector: {}
tolerations: []
affinity: {}
```

## Quick Access Commands

After deployment, use these one-liner commands:

```powershell
# Start port-forward
kubectl port-forward -n grafana svc/grafana 3000:80

# Get credentials (one-liner)
$pwd = kubectl get secret grafana-admin-secret -n grafana -o jsonpath='{.data.admin-password}'; $decoded = [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String($pwd)); Write-Host "Username: admin, Password: $decoded"

# Check status
kubectl get all -n grafana

# View logs
kubectl logs deployment/grafana -n grafana -f

# Test health
kubectl run health-check --image=curlimages/curl --rm --restart=Never -- curl -s http://grafana.grafana.svc.cluster.local/api/health
```

## Access Information

After successful deployment:

- **URL**: http://localhost:3000 (when port-forward is running)
- **Username**: admin
- **Password**: As specified in your custom-values.yaml or retrieved from secret
- **Health Check**: http://localhost:3000/api/health
- **Metrics**: http://localhost:3000/metrics

## PowerShell-Specific Notes

1. **Execution Policy**: You may need to set execution policy:
   ```powershell
   Set-ExecutionPolicy -ExecutionPolicy RemoteSigned -Scope CurrentUser
   ```

2. **Background Jobs**: Use PowerShell jobs for port-forwarding:
   ```powershell
   Get-Job                    # List jobs
   Stop-Job -Id <id>         # Stop specific job
   Remove-Job -Id <id>       # Remove job
   ```

3. **Base64 Handling**: PowerShell uses .NET methods:
   ```powershell
   # Encode
   [Convert]::ToBase64String([System.Text.Encoding]::UTF8.GetBytes("text"))
   # Decode
   [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String("base64"))
   ```

4. **Path Handling**: Use backslashes and quotes for Windows paths:
   ```powershell
   & "C:\Users\user01\kube-bin\helm.exe"
   ```

## Troubleshooting Common Issues

### Helm Binary Not Found
```powershell
# Verify helm binary exists
Test-Path "C:\Users\user01\kube-bin\helm.exe"
# If false, check the path or download Helm
```

### Values File Not Found
```powershell
# Verify values file exists
Test-Path "C:\Users\user01\kube-bin\my-values\custom-values.yaml"
# Create directory if needed
New-Item -ItemType Directory -Path "C:\Users\user01\kube-bin\my-values" -Force
```

### Port Already in Use
```powershell
# Check what's using port 3000
Get-NetTCPConnection -LocalPort 3000 -ErrorAction SilentlyContinue
# Kill process if needed or use different port
kubectl port-forward -n grafana svc/grafana 3001:80
```

This completes your comprehensive PowerShell deployment guide for Grafana OSS v12 on AKS!
