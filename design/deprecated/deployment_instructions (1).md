# Grafana OSS v12 Deployment on AKS - Complete Instructions (PowerShell)

## Overview
This guide provides step-by-step instructions to deploy Grafana OSS v12 on Azure Kubernetes Service (AKS) using only kubectl (no Helm required) with PowerShell scripts.

**Target Environment:**
- AKS Cluster: `supercool-aks-cluster`
- Grafana Version: OSS v12.0.0
- kubectl Version: 1.30
- Deployment Method: kubectl with YAML manifests
- Platform: Windows with PowerShell

## Prerequisites

### Local Machine Requirements
- Windows 10/11 or Windows Server
- PowerShell 5.1 or PowerShell 7+
- Azure CLI installed and configured
- kubectl v1.30 installed
- Access to Azure subscription with AKS cluster
- Web browser for accessing Grafana

### AKS Cluster Requirements
- Running AKS cluster named `supercool-aks-cluster`
- Cluster admin permissions
- Default storage class configured (managed-csi)

## Step 1: Prepare Your Environment

### 1.1 Verify Prerequisites
```powershell
# Check Azure CLI
az --version

# Check kubectl version
kubectl version --client

# Verify you have the required kubectl version (1.30.x)
kubectl version --client -o yaml | Select-String "gitVersion"

# Check PowerShell version
$PSVersionTable.PSVersion
```

### 1.2 Connect to AKS Cluster
```powershell
# Login to Azure
az login

# Set your subscription (if you have multiple)
az account set --subscription "your-subscription-id"

# Get AKS credentials
az aks get-credentials --resource-group <your-resource-group> --name supercool-aks-cluster --overwrite-existing

# Verify connection
kubectl cluster-info
kubectl get nodes
```

### 1.3 Verify Cluster Permissions
```powershell
# Test cluster admin permissions
kubectl auth can-i create namespace
kubectl auth can-i create persistentvolumeclaim
kubectl auth can-i create deployment
kubectl auth can-i create service
```

## Step 2: Prepare Deployment Files

### 2.1 Create Deployment Directory
```powershell
# Create a directory for Grafana deployment
New-Item -ItemType Directory -Path "$env:USERPROFILE\grafana-aks-deployment" -Force
Set-Location "$env:USERPROFILE\grafana-aks-deployment"
```

### 2.2 Save the Kubernetes Manifests
Save the provided `grafana-manifests.yaml` file to your deployment directory. This file contains all necessary Kubernetes resources:
- Namespace
- ConfigMap for Grafana configuration
- Secret for admin credentials
- PersistentVolumeClaim for data storage
- ServiceAccount
- Deployment
- Services
- NetworkPolicy
- ServiceMonitor (for Prometheus)

### 2.3 Save the Deployment Script
Save the provided `deploy-grafana.ps1` script:
```powershell
# Make sure the script can be executed
Set-ExecutionPolicy -ExecutionPolicy RemoteSigned -Scope CurrentUser
```

### 2.4 Save the Network Testing Script
Save the provided `test-network.ps1` script.

## Step 3: Deploy Grafana

### 3.1 Run the Deployment Script
```powershell
# Deploy Grafana with all verification steps
.\deploy-grafana.ps1 deploy
```

This script will:
- Check prerequisites
- Deploy all Kubernetes resources
- Wait for resources to be ready
- Verify the deployment
- Test connectivity
- Display access information

### 3.2 Manual Deployment (Alternative)
If you prefer to deploy manually:
```powershell
# Apply all manifests
kubectl apply -f grafana-manifests.yaml

# Wait for namespace
kubectl wait --for=condition=Ready namespace/grafana --timeout=60s

# Wait for PVC to be bound
kubectl wait --for=condition=Bound pvc/grafana-pvc -n grafana --timeout=300s

# Wait for deployment to be ready
kubectl wait --for=condition=Available deployment/grafana -n grafana --timeout=600s

# Wait for pods to be ready
kubectl wait --for=condition=Ready pod -l app=grafana -n grafana --timeout=300s
```

## Step 4: Verify Deployment

### 4.1 Check All Resources
```powershell
# Check all resources in the grafana namespace
kubectl get all -n grafana

# Check persistent storage
kubectl get pvc -n grafana

# Check configuration
kubectl get configmap,secret -n grafana
```

### 4.2 Check Pod Status and Logs
```powershell
# Check pod status
kubectl get pods -n grafana -o wide

# Check pod logs
kubectl logs deployment/grafana -n grafana --tail=20

# Follow logs in real-time
kubectl logs deployment/grafana -n grafana -f
```

### 4.3 Run Network Tests
```powershell
# Test DNS resolution
.\test-network.ps1 dns

# Test internal connectivity
.\test-network.ps1 internal

# Test Grafana API endpoints
.\test-network.ps1 api

# Run comprehensive test
.\test-network.ps1 comprehensive
```

## Step 5: Access Grafana

### 5.1 Setup Port Forwarding
```powershell
# Start port-forward to access Grafana locally
kubectl port-forward -n grafana svc/grafana 3000:80

# Or for remote access (binds to all interfaces)
kubectl port-forward -n grafana --address 0.0.0.0 svc/grafana 3000:80

# Or use the PowerShell script
.\deploy-grafana.ps1 port-forward
```

### 5.2 Access via Web Browser
1. Open your web browser
2. Navigate to: `http://localhost:3000`
3. Login with:
   - **Username:** `admin`
   - **Password:** `GrafanaAdmin123!`

### 5.3 Verify Grafana Installation
1. Confirm the dashboard loads successfully
2. Check the version in the bottom left corner (should show v12.x.x)
3. Navigate to Configuration → Data Sources
4. Add a TestData DB data source to verify functionality
5. Create a simple test dashboard

## Step 6: Configure Grafana (Optional)

### 6.1 Install Additional Plugins
```powershell
# Connect to the Grafana pod
$podName = kubectl get pods -n grafana -l app=grafana -o jsonpath='{.items[0].metadata.name}'
kubectl exec -it -n grafana $podName -- /bin/bash

# Inside the pod, install plugins
grafana-cli plugins install grafana-clock-panel
grafana-cli plugins install grafana-simple-json-datasource

# Exit the pod and restart Grafana to load new plugins
exit
kubectl rollout restart deployment/grafana -n grafana
```

### 6.2 Update Configuration
To modify Grafana configuration:
```powershell
# Edit the ConfigMap
kubectl edit configmap grafana-config -n grafana

# After editing, restart the deployment
kubectl rollout restart deployment/grafana -n grafana
```

### 6.3 Change Admin Password
```powershell
# Update the secret
$newPassword = "YourNewPassword"
$encodedPassword = [Convert]::ToBase64String([System.Text.Encoding]::UTF8.GetBytes($newPassword))
kubectl patch secret grafana-admin-secret -n grafana -p "{`"data`":{`"admin-password`":`"$encodedPassword`"}}"

# Restart the deployment
kubectl rollout restart deployment/grafana -n grafana
```

## Step 7: Production Considerations

### 7.1 Setup Ingress (For Production Access)
```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: grafana-ingress
  namespace: grafana
  annotations:
    kubernetes.io/ingress.class: azure/application-gateway
    cert-manager.io/cluster-issuer: letsencrypt-prod
spec:
  tls:
  - hosts:
    - grafana.yourdomain.com
    secretName: grafana-tls
  rules:
  - host: grafana.yourdomain.com
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: grafana
            port:
              number: 80
```

### 7.2 Setup Persistent Storage Backup
```powershell
# Create a backup job for Grafana data
kubectl create job grafana-backup --from=cronjob/grafana-backup -n grafana
```

### 7.3 Monitor Grafana with Prometheus
The provided manifests include a ServiceMonitor for Prometheus integration. Ensure you have Prometheus Operator installed:
```powershell
# Check if Prometheus Operator is available
kubectl get crd servicemonitors.monitoring.coreos.com
```

## Step 8: Troubleshooting

### 8.1 Common Issues and Solutions

**Pod Not Starting:**
```powershell
# Check pod events
kubectl describe pod -l app=grafana -n grafana

# Check resource constraints
kubectl top pod -n grafana
kubectl describe node
```

**Storage Issues:**
```powershell
# Check PVC status
kubectl describe pvc grafana-pvc -n grafana

# Check storage class
kubectl get storageclass
```

**Network Connectivity:**
```powershell
# Run network diagnostics
.\test-network.ps1 troubleshoot

# Check service endpoints
kubectl get endpoints grafana -n grafana
```

### 8.2 View Logs and Events
```powershell
# View recent events
kubectl get events -n grafana --sort-by='.lastTimestamp'

# View Grafana logs with timestamps
kubectl logs deployment/grafana -n grafana --timestamps=true

# View logs from previous container restart
kubectl logs deployment/grafana -n grafana --previous
```

### 8.3 Debug Network Issues
```powershell
# Test from within the cluster
kubectl run debug --image=nicolaka/netshoot --rm -it --restart=Never

# Inside the debug pod:
nslookup grafana.grafana.svc.cluster.local
curl -v http://grafana.grafana.svc.cluster.local/api/health
```

## Step 9: Maintenance

### 9.1 Update Grafana
```powershell
# Update the image version in the deployment
kubectl set image deployment/grafana grafana=grafana/grafana-oss:12.1.0 -n grafana

# Monitor the rollout
kubectl rollout status deployment/grafana -n grafana
```

### 9.2 Backup Configuration
```powershell
# Backup ConfigMaps and Secrets
kubectl get configmap grafana-config -n grafana -o yaml | Out-File -FilePath "grafana-config-backup.yaml"
kubectl get secret grafana-admin-secret -n grafana -o yaml | Out-File -FilePath "grafana-secret-backup.yaml"

# Backup persistent data
kubectl exec deployment/grafana -n grafana -- tar czf - /var/lib/grafana | Set-Content -Path "grafana-data-backup.tar.gz" -AsByteStream
```

### 9.3 Scale Grafana (If Needed)
```powershell
# Note: Grafana OSS doesn't support horizontal scaling
# You can only scale vertically by updating resource limits

# Update resource limits
$patchData = @{
    spec = @{
        template = @{
            spec = @{
                containers = @(
                    @{
                        name = "grafana"
                        resources = @{
                            limits = @{
                                cpu = "2000m"
                                memory = "2Gi"
                            }
                        }
                    }
                )
            }
        }
    }
} | ConvertTo-Json -Depth 10

kubectl patch deployment grafana -n grafana --type merge -p $patchData
```

## Step 10: Uninstall

### 10.1 Complete Removal
```powershell
# Stop port-forward if running
Get-Job | Where-Object { $_.Command -like "*port-forward*" } | Stop-Job
Get-Job | Where-Object { $_.Command -like "*port-forward*" } | Remove-Job

# Delete all Grafana resources
kubectl delete -f grafana-manifests.yaml

# Verify removal
kubectl get all -n grafana
```

### 10.2 Clean Up Storage (Optional)
```powershell
# If you want to remove persistent data
kubectl delete pvc grafana-pvc -n grafana
kubectl delete namespace grafana
```

## Security Notes

1. **Change Default Password:** Always change the default admin password in production
2. **Use HTTPS:** Configure TLS/SSL for production deployments
3. **Network Policies:** The provided NetworkPolicy restricts access appropriately
4. **RBAC:** The deployment uses minimal RBAC permissions
5. **Security Context:** Grafana runs as non-root user (UID 472)

## Support and Resources

- **Official Grafana Documentation:** https://grafana.com/docs/grafana/latest/
- **Grafana GitHub Repository:** https://github.com/grafana/grafana
- **AKS Documentation:** https://docs.microsoft.com/en-us/azure/aks/
- **kubectl Reference:** https://kubernetes.io/docs/reference/kubectl/

## Quick Reference Commands (PowerShell)

```powershell
# Check status
kubectl get all -n grafana

# View logs
kubectl logs deployment/grafana -n grafana -f

# Port forward
kubectl port-forward -n grafana svc/grafana 3000:80

# Get admin password
$adminPasswordBase64 = kubectl get secret grafana-admin-secret -n grafana -o jsonpath='{.data.admin-password}'
$adminPassword = [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String($adminPasswordBase64))
Write-Host "Admin password: $adminPassword"

# Restart Grafana
kubectl rollout restart deployment/grafana -n grafana

# Scale resources (example)
$resourcePatch = '{"spec":{"template":{"spec":{"containers":[{"name":"grafana","resources":{"limits":{"memory":"2Gi"}}}]}}}}'
kubectl patch deployment grafana -n grafana --type merge -p $resourcePatch

# Using PowerShell scripts
.\deploy-grafana.ps1 deploy      # Deploy Grafana
.\deploy-grafana.ps1 verify      # Verify deployment
.\deploy-grafana.ps1 port-forward # Start port forwarding
.\test-network.ps1 comprehensive # Test connectivity
```

## PowerShell-Specific Notes

1. **Execution Policy:** You may need to set the execution policy to run scripts:
   ```powershell
   Set-ExecutionPolicy -ExecutionPolicy RemoteSigned -Scope CurrentUser
   ```

2. **Background Jobs:** Port-forwarding uses PowerShell background jobs instead of Unix processes:
   ```powershell
   # List background jobs
   Get-Job
   
   # Stop port-forward jobs
   Get-Job | Where-Object { $_.Command -like "*port-forward*" } | Stop-Job
   ```

3. **Base64 Encoding/Decoding:** PowerShell uses .NET methods for base64 operations:
   ```powershell
   # Encode
   $encoded = [Convert]::ToBase64String([System.Text.Encoding]::UTF8.GetBytes("password"))
   
   # Decode
   $decoded = [System.Text.Encoding]::UTF8.GetString([System.Convert]::FromBase64String($encoded))
   ```

This completes your Grafana OSS v12 deployment on AKS using kubectl with PowerShell!