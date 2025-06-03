# Grafana OSS v12 Deployment on Azure Kubernetes Service (AKS)

## Prerequisites

- Azure CLI installed and configured
- kubectl v1.30 installed
- Helm 3.x installed
- Access to AKS cluster: `supercool-aks-cluster`

## 1. Connect to AKS Cluster

```bash
# Login to Azure (if not already logged in)
az login

# Get AKS credentials and configure kubectl context
az aks get-credentials --resource-group <your-resource-group> --name supercool-aks-cluster

# Verify connection to cluster
kubectl cluster-info
kubectl get nodes
```

## 2. Setup Helm Repository

```bash
# Add Grafana Helm repository
helm repo add grafana https://grafana.github.io/helm-charts

# Update Helm repositories
helm repo update

# Verify Grafana chart is available
helm search repo grafana/grafana --versions | head -10
```

## 3. Create Namespace (Optional but Recommended)

```bash
# Create dedicated namespace for Grafana
kubectl create namespace grafana

# Set default namespace context (optional)
kubectl config set-context --current --namespace=grafana
```

## 4. Installation with Default Values

### Install Grafana with Default Configuration

```bash
# Install Grafana OSS v12 using default values
helm install grafana grafana/grafana \
  --namespace grafana \
  --version 8.0.0 \
  --set image.tag="12.0.0" \
  --create-namespace

# Check installation status
helm status grafana -n grafana

# Watch pods until they're running
kubectl get pods -n grafana -w
```

### Get Admin Password

```bash
# Retrieve the auto-generated admin password
kubectl get secret --namespace grafana grafana -o jsonpath="{.data.admin-password}" | base64 --decode ; echo
```

## 5. Installation with Custom Values (Optional)

### Create Custom Values File

```bash
# Create custom-values.yaml file
cat > custom-values.yaml << 'EOF'
# Grafana Custom Configuration
image:
  tag: "12.0.0"

# Admin credentials
adminUser: admin
adminPassword: "YourSecurePassword123!"

# Persistence
persistence:
  enabled: true
  type: pvc
  size: 10Gi
  storageClassName: managed-csi

# Service configuration
service:
  type: LoadBalancer
  port: 80
  targetPort: 3000

# Resource limits
resources:
  limits:
    cpu: 1000m
    memory: 1Gi
  requests:
    cpu: 500m
    memory: 512Mi

# Environment variables for plugins
env:
  GF_INSTALL_PLUGINS: "yesoreyeram-infinity-datasource,grafana-resourcesexporter-app"

# Additional plugins configuration
plugins:
  - yesoreyeram-infinity-datasource
  - grafana-resourcesexporter-app

# Security context
securityContext:
  runAsNonRoot: true
  runAsUser: 472
  fsGroup: 472

# Grafana configuration
grafana.ini:
  server:
    domain: grafana.yourdomain.com  # Update with your domain
    root_url: "http://grafana.yourdomain.com"
  security:
    admin_user: admin
    admin_password: YourSecurePassword123!
  plugins:
    enable_alpha: true
    allow_loading_unsigned_plugins: yesoreyeram-infinity-datasource,grafana-resourcesexporter-app

# Ingress configuration (optional)
ingress:
  enabled: false  # Set to true if using ingress
  annotations: {}
  #   kubernetes.io/ingress.class: nginx
  #   cert-manager.io/cluster-issuer: letsencrypt-prod
  hosts:
    - host: grafana.yourdomain.com
      paths:
        - path: /
          pathType: Prefix
  tls: []
  #  - secretName: grafana-tls
  #    hosts:
  #      - grafana.yourdomain.com

# ServiceMonitor for Prometheus (if using Prometheus Operator)
serviceMonitor:
  enabled: false
  path: /metrics
  interval: 30s

# Node selector (optional)
nodeSelector: {}

# Tolerations (optional)
tolerations: []

# Affinity (optional)
affinity: {}
EOF
```

### Install with Custom Values

```bash
# Install Grafana with custom configuration
helm install grafana grafana/grafana \
  --namespace grafana \
  --version 8.0.0 \
  --values custom-values.yaml \
  --create-namespace

# Check installation status
helm status grafana -n grafana
```

## 6. Verify Installation

```bash
# Check all resources in grafana namespace
kubectl get all -n grafana

# Check pod logs
kubectl logs -n grafana deployment/grafana

# Check persistent volume claims (if using persistence)
kubectl get pvc -n grafana

# Check services
kubectl get svc -n grafana
```

## 7. Access Grafana Web Interface

### Method 1: Port Forward (Development/Testing)

```bash
# Forward local port 3000 to Grafana service
kubectl port-forward -n grafana svc/grafana 3000:80

# Access Grafana at: http://localhost:3000
# Default credentials: admin / <password-from-secret>
```

### Method 2: LoadBalancer (if configured)

```bash
# Get external IP (may take a few minutes)
kubectl get svc -n grafana grafana -w

# Once EXTERNAL-IP is assigned, access via:
# http://<EXTERNAL-IP>
```

### Method 3: NodePort (Alternative)

```bash
# Patch service to NodePort if needed
kubectl patch svc grafana -n grafana -p '{"spec":{"type":"NodePort"}}'

# Get NodePort
kubectl get svc -n grafana grafana

# Access via: http://<NODE-IP>:<NODEPORT>
```

## 8. Testing Web Interface Functionality

### Basic Connectivity Test

```bash
# Test if Grafana is responding (replace URL as needed)
curl -I http://localhost:3000/login

# Expected response: HTTP/1.1 200 OK
```

### Login and Basic Configuration Test

1. **Access Grafana Web UI** using one of the methods above
2. **Login** with admin credentials
3. **Verify Dashboard**: You should see the Grafana home dashboard
4. **Check Plugins**: Go to Configuration > Plugins to verify installed plugins
   - yesoreyeram-infinity-datasource
   - grafana-resourcesexporter-app (if custom values were used)
5. **Test Data Source**: Try adding a test data source (Configuration > Data Sources > Add data source > TestData DB)

### Automated Health Check

```bash
# Create a simple health check script
cat > test-grafana.sh << 'EOF'
#!/bin/bash

GRAFANA_URL="http://localhost:3000"
ADMIN_USER="admin"
ADMIN_PASS=$(kubectl get secret --namespace grafana grafana -o jsonpath="{.data.admin-password}" | base64 --decode)

echo "Testing Grafana connectivity..."

# Test login endpoint
LOGIN_RESPONSE=$(curl -s -o /dev/null -w "%{http_code}" "$GRAFANA_URL/login")
if [ "$LOGIN_RESPONSE" = "200" ]; then
    echo "✅ Grafana login page accessible"
else
    echo "❌ Grafana login page not accessible (HTTP $LOGIN_RESPONSE)"
    exit 1
fi

# Test API health
API_RESPONSE=$(curl -s -o /dev/null -w "%{http_code}" "$GRAFANA_URL/api/health")
if [ "$API_RESPONSE" = "200" ]; then
    echo "✅ Grafana API health check passed"
else
    echo "❌ Grafana API health check failed (HTTP $API_RESPONSE)"
    exit 1
fi

echo "🎉 All tests passed! Grafana is working correctly."
EOF

chmod +x test-grafana.sh

# Run the test (ensure port-forward is active)
./test-grafana.sh
```

## 9. Uninstall Grafana

### Complete Uninstall

```bash
# Uninstall Grafana Helm release
helm uninstall grafana -n grafana

# Delete persistent volume claims (if you want to remove data)
kubectl delete pvc -n grafana --all

# Delete namespace (optional - removes everything)
kubectl delete namespace grafana

# Verify removal
kubectl get all -n grafana
```

### Partial Uninstall (Keep Data)

```bash
# Uninstall only the Helm release (keeps PVCs and namespace)
helm uninstall grafana -n grafana

# Verify PVCs are preserved
kubectl get pvc -n grafana
```

## 10. Troubleshooting

### Common Issues and Solutions

```bash
# Check pod status and events
kubectl describe pod -n grafana -l app.kubernetes.io/name=grafana

# Check logs for errors
kubectl logs -n grafana -l app.kubernetes.io/name=grafana --tail=100

# Check service endpoints
kubectl get endpoints -n grafana grafana

# Check ingress (if configured)
kubectl describe ingress -n grafana

# Verify storage class (if using persistence)
kubectl get storageclass
```

### Resource Verification Commands

```bash
# List all Grafana-related resources
kubectl get all,pvc,secrets,configmaps -n grafana -l app.kubernetes.io/name=grafana

# Check Helm release history
helm history grafana -n grafana

# Check available Helm chart versions
helm search repo grafana/grafana --versions
```

## 11. Upgrade Grafana

```bash
# Upgrade to latest version
helm upgrade grafana grafana/grafana -n grafana

# Upgrade with custom values
helm upgrade grafana grafana/grafana -n grafana --values custom-values.yaml

# Rollback if needed
helm rollback grafana 1 -n grafana
```

## 12. Backup and Restore

### Backup Grafana Configuration

```bash
# Backup Grafana data (if using persistent storage)
kubectl exec -n grafana deployment/grafana -- tar czf - /var/lib/grafana > grafana-backup-$(date +%Y%m%d).tar.gz

# Backup using PVC snapshot (Azure specific)
# This would require Azure Disk snapshot capabilities
```

This completes the comprehensive deployment guide for Grafana OSS v12 on your AKS cluster!
