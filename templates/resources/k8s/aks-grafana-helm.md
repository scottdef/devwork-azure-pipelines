# Grafana OSS v12 Deployment on AKS - Private Repository Installation

## Prerequisites

- Azure CLI installed and configured
- kubectl v1.30 installed
- Helm 3.x installed
- Access to AKS cluster: `supercool-aks-cluster`
- Access to private repository (Azure DevOps or GitHub)
- Git client installed

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

## 2. Setup Private Repository Access

### Option A: Azure DevOps Private Repository

#### Create Personal Access Token (PAT)

1. Go to Azure DevOps → User Settings → Personal Access Tokens
2. Create new token with **Code (read)** permissions
3. Copy the token securely

#### Configure Repository Access

```bash
# Set environment variables
export AZURE_DEVOPS_ORG="your-organization"
export AZURE_DEVOPS_PROJECT="your-project"
export AZURE_DEVOPS_REPO="grafana-helm-charts"
export AZURE_DEVOPS_PAT="your-personal-access-token"

# Configure Git credentials for Azure DevOps
git config --global credential.helper store
echo "https://${AZURE_DEVOPS_PAT}@dev.azure.com" > ~/.git-credentials

# Alternative: Configure using Azure CLI
az devops configure --defaults organization=https://dev.azure.com/${AZURE_DEVOPS_ORG} project=${AZURE_DEVOPS_PROJECT}
az devops login --organization https://dev.azure.com/${AZURE_DEVOPS_ORG}
```

#### Clone Repository and Setup Helm

```bash
# Clone the private repository
git clone https://${AZURE_DEVOPS_PAT}@dev.azure.com/${AZURE_DEVOPS_ORG}/${AZURE_DEVOPS_PROJECT}/_git/${AZURE_DEVOPS_REPO}

# Navigate to repository
cd ${AZURE_DEVOPS_REPO}

# Verify Grafana chart exists
ls -la charts/grafana/ || ls -la grafana/

# Alternative: Add as Helm repository (if repo serves as Helm repo)
helm repo add private-azure "https://${AZURE_DEVOPS_PAT}@dev.azure.com/${AZURE_DEVOPS_ORG}/${AZURE_DEVOPS_PROJECT}/_git/${AZURE_DEVOPS_REPO}"
helm repo update
```

### Option B: GitHub Private Repository

#### Create Personal Access Token (PAT)

1. Go to GitHub → Settings → Developer settings → Personal Access Tokens → Tokens (classic)
2. Create new token with **repo** permissions
3. Copy the token securely

#### Configure Repository Access

```bash
# Set environment variables
export GITHUB_ORG="your-github-org"
export GITHUB_REPO="grafana-helm-charts"
export GITHUB_PAT="your-github-personal-access-token"

# Configure Git credentials for GitHub
git config --global credential.helper store
echo "https://${GITHUB_PAT}@github.com" > ~/.git-credentials

# Alternative: Use SSH key (recommended for production)
# Generate SSH key if not exists
ssh-keygen -t ed25519 -C "your-email@example.com" -f ~/.ssh/github_key

# Add SSH key to ssh-agent
eval "$(ssh-agent -s)"
ssh-add ~/.ssh/github_key

# Add public key to GitHub account (manually in GitHub UI)
cat ~/.ssh/github_key.pub
```

#### Clone Repository and Setup Helm

```bash
# Clone using HTTPS with PAT
git clone https://${GITHUB_PAT}@github.com/${GITHUB_ORG}/${GITHUB_REPO}.git

# Or clone using SSH (if SSH key configured)
git clone git@github.com:${GITHUB_ORG}/${GITHUB_REPO}.git

# Navigate to repository
cd ${GITHUB_REPO}

# Verify Grafana chart exists
ls -la charts/grafana/ || ls -la grafana/

# Alternative: Add as Helm repository (if repo has GitHub Pages/releases)
helm repo add private-github "https://${GITHUB_PAT}@raw.githubusercontent.com/${GITHUB_ORG}/${GITHUB_REPO}/main/"
helm repo update
```

### Option C: Using Helm OCI Registry (Azure Container Registry)

```bash
# Login to Azure Container Registry
az acr login --name your-registry-name

# Set registry variables
export ACR_NAME="your-registry-name"
export CHART_VERSION="1.0.0"

# Pull chart from private ACR
helm pull oci://${ACR_NAME}.azurecr.io/helm/grafana --version ${CHART_VERSION} --untar

# Navigate to extracted chart
cd grafana/
```

## 3. Create Namespace

```bash
# Create dedicated namespace for Grafana
kubectl create namespace grafana

# Set default namespace context (optional)
kubectl config set-context --current --namespace=grafana
```

## 4. Create Custom Values File

```bash
# Create custom values for private repository installation
cat > values-private-repo.yaml << 'EOF'
# Grafana OSS v12 Configuration for Private Repository Installation
image:
  repository: grafana/grafana-oss
  tag: "12.0.0"
  pullPolicy: IfNotPresent

# Admin credentials
adminUser: admin
adminPassword: "YourSecurePassword123!"

# Persistence configuration
persistence:
  enabled: true
  type: pvc
  size: 10Gi
  storageClassName: managed-csi
  accessModes:
    - ReadWriteOnce

# Service configuration for remote access
service:
  type: ClusterIP  # Using ClusterIP with port-forward for security
  port: 80
  targetPort: 3000
  annotations: {}

# Resource limits and requests
resources:
  limits:
    cpu: 1000m
    memory: 1Gi
  requests:
    cpu: 500m
    memory: 512Mi

# Environment variables for plugin installation
env:
  GF_INSTALL_PLUGINS: "yesoreyeram-infinity-datasource,grafana-resourcesexporter-app"
  GF_SECURITY_ALLOW_EMBEDDING: "false"
  GF_SECURITY_COOKIE_SECURE: "false"  # Set to true in production with HTTPS

# Plugin configuration
plugins:
  - yesoreyeram-infinity-datasource
  - grafana-resourcesexporter-app

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
    root_url: "http://localhost:3000"
    serve_from_sub_path: false
    enable_gzip: true
  security:
    admin_user: admin
    admin_password: YourSecurePassword123!
    allow_embedding: false
    cookie_secure: false
  plugins:
    enable_alpha: true
    allow_loading_unsigned_plugins: yesoreyeram-infinity-datasource,grafana-resourcesexporter-app
  log:
    mode: console
    level: info
  database:
    type: sqlite3
  analytics:
    reporting_enabled: false
    check_for_updates: false

# Ingress configuration (disabled for port-forward access)
ingress:
  enabled: false

# ServiceMonitor for Prometheus monitoring
serviceMonitor:
  enabled: false

# Deployment strategy
deploymentStrategy:
  type: RollingUpdate
  rollingUpdate:
    maxSurge: 1
    maxUnavailable: 0

# Pod annotations
podAnnotations:
  cluster-autoscaler.kubernetes.io/safe-to-evict: "true"

# Node selector for specific nodes (optional)
nodeSelector: {}

# Tolerations
tolerations: []

# Affinity rules
affinity: {}

# Liveness probe
livenessProbe:
  httpGet:
    path: /api/health
    port: 3000
  initialDelaySeconds: 60
  timeoutSeconds: 30
  failureThreshold: 3
  periodSeconds: 10

# Readiness probe
readinessProbe:
  httpGet:
    path: /api/health
    port: 3000
  initialDelaySeconds: 30
  timeoutSeconds: 30
  failureThreshold: 3
  periodSeconds: 5

# Extra containers (for sidecars, etc.)
extraContainers: []

# Extra init containers
extraInitContainers: []

# Extra volumes
extraVolumes: []

# Extra volume mounts
extraVolumeMounts: []
EOF
```

## 5. Install from Private Repository

### Option A: Install from Local Clone (Azure DevOps)

```bash
# Navigate to cloned repository
cd ${AZURE_DEVOPS_REPO}

# Install from local chart directory
helm install grafana ./grafana \
  --namespace grafana \
  --values values-private-repo.yaml \
  --create-namespace

# Alternative: Install from specific chart subdirectory
helm install grafana ./charts/grafana \
  --namespace grafana \
  --values values-private-repo.yaml \
  --create-namespace
```

### Option B: Install from Local Clone (GitHub)

```bash
# Navigate to cloned repository
cd ${GITHUB_REPO}

# Install from local chart directory
helm install grafana ./grafana \
  --namespace grafana \
  --values values-private-repo.yaml \
  --create-namespace

# Check installation status
helm status grafana -n grafana
```

### Option C: Install from Private Helm Repository

```bash
# If repository is configured as Helm repo
helm install grafana private-azure/grafana \
  --namespace grafana \
  --values values-private-repo.yaml \
  --version 8.0.0 \
  --create-namespace

# Or for GitHub
helm install grafana private-github/grafana \
  --namespace grafana \
  --values values-private-repo.yaml \
  --version 8.0.0 \
  --create-namespace
```

## 6. Verify Installation

```bash
# Check all resources in grafana namespace
kubectl get all -n grafana

# Check pod logs for successful startup
kubectl logs -n grafana deployment/grafana -f

# Verify plugins are installing
kubectl logs -n grafana deployment/grafana | grep -i "plugin\|infinity\|resourcesexporter"

# Check persistent volume claims
kubectl get pvc -n grafana

# Check services
kubectl get svc -n grafana

# Verify installation source
helm get values grafana -n grafana
```

## 7. Remote Access Setup - Port Forwarding

### Method 1: Basic Port Forward (Local Access Only)

```bash
# Forward to localhost only (accessible only from local machine)
kubectl port-forward -n grafana svc/grafana 3000:80

# Access: http://localhost:3000
```

### Method 2: Port Forward with External Binding (Remote Access)

```bash
# Forward and bind to all interfaces (accessible from remote hosts)
kubectl port-forward -n grafana --address 0.0.0.0 svc/grafana 3000:80

# Access from remote browser: http://<server-ip>:3000
# Replace <server-ip> with actual IP address of the machine running kubectl
```

### Method 3: Port Forward with Specific Interface Binding

```bash
# Get server IP address
SERVER_IP=$(hostname -I | awk '{print $1}')
echo "Server IP: $SERVER_IP"

# Forward and bind to specific interface
kubectl port-forward -n grafana --address ${SERVER_IP} svc/grafana 3000:80

# Access from remote browser: http://${SERVER_IP}:3000
```

### Method 4: SSH Tunnel for Secure Remote Access

#### Setup SSH Tunnel from Remote Machine

```bash
# From your local machine/laptop, create SSH tunnel to the server
ssh -L 3000:localhost:3000 username@<server-ip>

# In another terminal on the server, run port-forward
kubectl port-forward -n grafana svc/grafana 3000:80

# Access from local browser: http://localhost:3000
```

#### Setup Reverse SSH Tunnel (Server to Local)

```bash
# From the server (where kubectl is running)
ssh -R 3000:localhost:3000 username@<your-local-machine-ip>

# On server, run port-forward
kubectl port-forward -n grafana svc/grafana 3000:80

# Access from local browser: http://localhost:3000
```

### Method 5: Multiple Port Forwards for High Availability

```bash
# Create multiple port forwards with different ports
kubectl port-forward -n grafana --address 0.0.0.0 svc/grafana 3000:80 &
kubectl port-forward -n grafana --address 0.0.0.0 svc/grafana 3001:80 &
kubectl port-forward -n grafana --address 0.0.0.0 svc/grafana 3002:80 &

# Access via any port: http://<server-ip>:3000, :3001, or :3002

# List background port-forward processes
jobs

# Kill specific background job
kill %1  # kills first background job
```

### Method 6: Persistent Port Forward with Screen/Tmux

```bash
# Install screen or tmux if not available
sudo apt-get install screen -y  # Ubuntu/Debian
# or
sudo yum install screen -y      # RHEL/CentOS

# Create persistent screen session
screen -S grafana-portforward

# Inside screen session, run port-forward
kubectl port-forward -n grafana --address 0.0.0.0 svc/grafana 3000:80

# Detach from screen: Ctrl+A, then D

# Reattach to screen session
screen -r grafana-portforward

# List screen sessions
screen -ls
```

## 8. Firewall and Security Configuration

### Configure Firewall for Remote Access

```bash
# Ubuntu/Debian - Open port 3000
sudo ufw allow 3000/tcp
sudo ufw reload
sudo ufw status

# RHEL/CentOS - Open port 3000
sudo firewall-cmd --permanent --add-port=3000/tcp
sudo firewall-cmd --reload
sudo firewall-cmd --list-ports

# Azure NSG - Add inbound rule (via Azure CLI)
az network nsg rule create \
  --resource-group <your-resource-group> \
  --nsg-name <your-nsg-name> \
  --name AllowGrafanaPort \
  --protocol tcp \
  --priority 1000 \
  --destination-port-range 3000 \
  --access allow \
  --direction inbound
```

### Security Considerations

```bash
# Create security script for remote access
cat > secure-grafana-access.sh << 'EOF'
#!/bin/bash

# Set security environment variables
export GRAFANA_BIND_IP="0.0.0.0"
export GRAFANA_PORT="3000"
export ALLOWED_IPS="192.168.1.0/24,10.0.0.0/8"  # Adjust as needed

echo "🔒 Setting up secure Grafana remote access..."

# Start port-forward with security logging
kubectl port-forward -n grafana \
  --address ${GRAFANA_BIND_IP} \
  svc/grafana ${GRAFANA_PORT}:80 \
  --v=2 2>&1 | tee /tmp/grafana-portforward.log &

PORTFORWARD_PID=$!
echo "📡 Port-forward started with PID: $PORTFORWARD_PID"

# Function to cleanup on exit
cleanup() {
    echo "🛑 Stopping port-forward..."
    kill $PORTFORWARD_PID 2>/dev/null
    exit 0
}

# Trap cleanup function on script exit
trap cleanup EXIT INT TERM

# Monitor access
echo "🔍 Monitoring access log (Ctrl+C to stop)..."
tail -f /tmp/grafana-portforward.log
EOF

chmod +x secure-grafana-access.sh

# Run secure access script
./secure-grafana-access.sh
```

## 9. Testing Remote Web Interface Access

### Automated Connectivity Test

```bash
# Create comprehensive remote access test
cat > test-remote-grafana.sh << 'EOF'
#!/bin/bash

# Configuration
SERVER_IP=$(hostname -I | awk '{print $1}')
GRAFANA_PORT="3000"
GRAFANA_URL="http://${SERVER_IP}:${GRAFANA_PORT}"
ADMIN_USER="admin"
ADMIN_PASS="YourSecurePassword123!"

echo "🧪 Testing Grafana Remote Access..."
echo "🌐 Server IP: $SERVER_IP"
echo "🔗 Grafana URL: $GRAFANA_URL"
echo ""

# Test if port-forward is running
if ! lsof -i :${GRAFANA_PORT} >/dev/null 2>&1; then
    echo "❌ Port $GRAFANA_PORT is not open. Start port-forward first:"
    echo "   kubectl port-forward -n grafana --address 0.0.0.0 svc/grafana 3000:80"
    exit 1
fi

echo "✅ Port $GRAFANA_PORT is open"

# Test local connectivity
echo "Testing local connectivity..."
LOCAL_RESPONSE=$(curl -s -o /dev/null -w "%{http_code}" "http://localhost:${GRAFANA_PORT}/login")
if [ "$LOCAL_RESPONSE" = "200" ]; then
    echo "✅ Local access working (HTTP $LOCAL_RESPONSE)"
else
    echo "❌ Local access failed (HTTP $LOCAL_RESPONSE)"
fi

# Test remote connectivity
echo "Testing remote connectivity..."
REMOTE_RESPONSE=$(curl -s -o /dev/null -w "%{http_code}" "$GRAFANA_URL/login")
if [ "$REMOTE_RESPONSE" = "200" ]; then
    echo "✅ Remote access working (HTTP $REMOTE_RESPONSE)"
else
    echo "❌ Remote access failed (HTTP $REMOTE_RESPONSE)"
    echo "💡 Check firewall settings and NSG rules"
fi

# Test API authentication
echo "Testing API authentication..."
API_RESPONSE=$(curl -s -o /dev/null -w "%{http_code}" -u "$ADMIN_USER:$ADMIN_PASS" "$GRAFANA_URL/api/user")
if [ "$API_RESPONSE" = "200" ]; then
    echo "✅ API authentication successful (HTTP $API_RESPONSE)"
else
    echo "❌ API authentication failed (HTTP $API_RESPONSE)"
fi

# Test plugin availability
echo "Testing plugin availability..."
PLUGINS_RESPONSE=$(curl -s -u "$ADMIN_USER:$ADMIN_PASS" "$GRAFANA_URL/api/plugins")
if echo "$PLUGINS_RESPONSE" | grep -q "yesoreyeram-infinity-datasource\|grafana-resourcesexporter-app"; then
    echo "✅ Custom plugins detected"
else
    echo "⚠️  Custom plugins not found (may still be installing)"
fi

# Network connectivity info
echo ""
echo "📊 Network Information:"
echo "   Internal Grafana Service: $(kubectl get svc -n grafana grafana -o jsonpath='{.spec.clusterIP}'):80"
echo "   External Access URL: $GRAFANA_URL"
echo "   Port-forward Process: $(ps aux | grep 'port-forward' | grep -v grep || echo 'Not running')"

echo ""
echo "🎉 Remote access test completed!"
echo "🌐 Open your browser to: $GRAFANA_URL"
echo "👤 Username: $ADMIN_USER"
echo "🔑 Password: $ADMIN_PASS"
EOF

chmod +x test-remote-grafana.sh

# Run the remote access test
./test-remote-grafana.sh
```

### Manual Browser Testing Steps

1. **Start Port-Forward**: Run port-forward command in background
2. **Open Browser**: Navigate to `http://<server-ip>:3000`
3. **Login**: Use admin credentials from values file
4. **Verify Plugins**: Configuration → Plugins → Check for custom plugins
5. **Test Dashboard**: Try creating a simple dashboard
6. **Test Data Source**: Add TestData DB data source

## 10. Repository Updates and Continuous Deployment

### Update Chart from Private Repository

```bash
# Navigate to repository directory
cd ${AZURE_DEVOPS_REPO}  # or ${GITHUB_REPO}

# Pull latest changes
git pull origin main

# Upgrade Grafana installation
helm upgrade grafana ./grafana \
  --namespace grafana \
  --values values-private-repo.yaml

# Check upgrade status
helm history grafana -n grafana
```

### Automated Repository Sync

```bash
# Create automated sync script
cat > sync-private-repo.sh << 'EOF'
#!/bin/bash

REPO_DIR="${AZURE_DEVOPS_REPO:-$GITHUB_REPO}"
cd "$REPO_DIR"

echo "🔄 Syncing private repository..."

# Pull latest changes
git fetch origin
LOCAL_COMMIT=$(git rev-parse HEAD)
REMOTE_COMMIT=$(git rev-parse origin/main)

if [ "$LOCAL_COMMIT" != "$REMOTE_COMMIT" ]; then
    echo "📥 New changes detected, updating..."
    git pull origin main

    # Upgrade Grafana with new chart
    helm upgrade grafana ./grafana \
      --namespace grafana \
      --values values-private-repo.yaml

    echo "✅ Grafana updated to latest chart version"
else
    echo "✅ Repository is up to date"
fi
EOF

chmod +x sync-private-repo.sh

# Run sync manually or add to crontab
./sync-private-repo.sh
```

## 11. Uninstall Grafana

### Complete Uninstall

```bash
# Stop any running port-forwards
pkill -f "kubectl port-forward.*grafana"

# Uninstall Helm release
helm uninstall grafana -n grafana

# Delete persistent volume claims
kubectl delete pvc -n grafana --all

# Delete namespace
kubectl delete namespace grafana

# Clean up local repository (optional)
rm -rf ${AZURE_DEVOPS_REPO} ${GITHUB_REPO}

# Remove Git credentials (optional)
rm -f ~/.git-credentials
```

## 12. Troubleshooting Private Repository Access

### Repository Access Issues

```bash
# Test repository connectivity
git ls-remote https://${AZURE_DEVOPS_PAT}@dev.azure.com/${AZURE_DEVOPS_ORG}/${AZURE_DEVOPS_PROJECT}/_git/${AZURE_DEVOPS_REPO}

# Or for GitHub
git ls-remote https://${GITHUB_PAT}@github.com/${GITHUB_ORG}/${GITHUB_REPO}.git

# Check Git credentials
git config --list | grep credential

# Test Helm repository (if configured)
helm repo list
helm search repo private-azure/grafana || helm search repo private-github/grafana
```

### Port-Forward Troubleshooting

```bash
# Check if port is in use
lsof -i :3000
netstat -tlnp | grep :3000

# Kill existing port-forwards
pkill -f "kubectl port-forward.*grafana"

# Check pod connectivity
kubectl exec -n grafana deployment/grafana -- wget -qO- http://localhost:3000/api/health

# Test service connectivity
kubectl run test-pod --rm -i --tty --image=curlimages/curl -- sh
# Inside pod: curl http://grafana.grafana.svc.cluster.local/api/health

# Check network policies
kubectl get networkpolicies -n grafana
```

This completes the comprehensive guide for deploying Grafana from private repositories with full remote access capabilities!
