# SQLite Container Deployment for AKS with Grafana Integration

This deployment provides a basic SQLite container configured as a Grafana datasource with support for both persistent (Azure Blob) and ephemeral storage options, optimized for ADO REST API data ingestion.

## Quick Start

```bash
# Clone and setup
git clone <your-repo>
cd sqlite-aks-deployment

# Configure ADO credentials
export ADO_ORG="your-organization"
export ADO_PROJECT="your-project" 
export ADO_PAT="your-personal-access-token"

# Build and deploy ephemeral version
./scripts/build-deploy.sh ephemeral

# Or deploy persistent version
./scripts/build-deploy.sh persistent
```

## Directory Structure

```
├── Dockerfile
├── README.md
├── scripts/
│   ├── build-deploy.sh
│   ├── fetch-ado-data.sh
│   └── json-to-sqlite.sh
├── sql/
│   └── init-schema.sql
├── manifests/
│   ├── sqlite-ephemeral.yaml
│   ├── sqlite-persistent.yaml
│   └── storage-class.yaml
└── config/
    └── grafana-datasource.yaml
```

## Container Files

### Dockerfile
```dockerfile
FROM alpine:3.18

# Install SQLite and dependencies
RUN apk update && \
    apk add --no-cache \
    sqlite \
    curl \
    jq \
    bash \
    tzdata && \
    rm -rf /var/cache/apk/*

# Create app directory and user
RUN addgroup -g 1000 sqlite && \
    adduser -u 1000 -G sqlite -s /bin/bash -D sqlite

WORKDIR /app

# Copy initialization scripts
COPY sql/init-schema.sql /app/
COPY scripts/json-to-sqlite.sh /app/
COPY data/ /app/data/

# Set permissions
RUN chown -R sqlite:sqlite /app && \
    chmod +x /app/json-to-sqlite.sh

# Create database directory
RUN mkdir -p /data && chown sqlite:sqlite /data

USER sqlite

# Initialize database on startup
CMD ["sh", "-c", "sqlite3 /data/grafana.db < /app/init-schema.sql && /app/json-to-sqlite.sh && tail -f /dev/null"]
```

### SQL Schema (sql/init-schema.sql)
```sql
-- Initialize Grafana-compatible schema for ADO data
CREATE TABLE IF NOT EXISTS ado_work_items (
    id INTEGER PRIMARY KEY,
    work_item_id INTEGER NOT NULL,
    title TEXT,
    work_item_type TEXT,
    state TEXT,
    assigned_to TEXT,
    created_date TEXT,
    changed_date TEXT,
    resolved_date TEXT,
    area_path TEXT,
    iteration_path TEXT,
    priority INTEGER,
    severity TEXT,
    story_points REAL,
    effort REAL,
    original_estimate REAL,
    remaining_work REAL,
    completed_work REAL,
    tags TEXT,
    project TEXT,
    created_timestamp INTEGER, -- Unix timestamp for Grafana
    changed_timestamp INTEGER,
    resolved_timestamp INTEGER
);

CREATE INDEX idx_work_item_id ON ado_work_items(work_item_id);
CREATE INDEX idx_state ON ado_work_items(state);
CREATE INDEX idx_work_item_type ON ado_work_items(work_item_type);
CREATE INDEX idx_created_timestamp ON ado_work_items(created_timestamp);
CREATE INDEX idx_changed_timestamp ON ado_work_items(changed_timestamp);

-- Table for build data
CREATE TABLE IF NOT EXISTS ado_builds (
    id INTEGER PRIMARY KEY,
    build_id INTEGER NOT NULL,
    build_number TEXT,
    status TEXT,
    result TEXT,
    queue_time TEXT,
    start_time TEXT,
    finish_time TEXT,
    source_branch TEXT,
    source_version TEXT,
    definition_name TEXT,
    definition_id INTEGER,
    project TEXT,
    requested_for TEXT,
    queue_timestamp INTEGER,
    start_timestamp INTEGER,
    finish_timestamp INTEGER
);

CREATE INDEX idx_build_id ON ado_builds(build_id);
CREATE INDEX idx_status ON ado_builds(status);
CREATE INDEX idx_result ON ado_builds(result);
CREATE INDEX idx_start_timestamp ON ado_builds(start_timestamp);

-- Table for release data
CREATE TABLE IF NOT EXISTS ado_releases (
    id INTEGER PRIMARY KEY,
    release_id INTEGER NOT NULL,
    release_name TEXT,
    status TEXT,
    created_date TEXT,
    modified_date TEXT,
    created_by TEXT,
    modified_by TEXT,
    definition_name TEXT,
    definition_id INTEGER,
    project TEXT,
    created_timestamp INTEGER,
    modified_timestamp INTEGER
);

CREATE INDEX idx_release_id ON ado_releases(release_id);
CREATE INDEX idx_release_status ON ado_releases(status);
CREATE INDEX idx_created_timestamp_rel ON ado_releases(created_timestamp);

-- Enable WAL mode for better concurrent access
PRAGMA journal_mode=WAL;
PRAGMA synchronous=NORMAL;
PRAGMA cache_size=-64000;
```

## Build Scripts

### ADO Data Fetcher (scripts/fetch-ado-data.sh)
```bash
#!/bin/bash
set -euo pipefail

# ADO REST API Data Fetcher for SQLite Preload
# Fetches work items, builds, and releases from Azure DevOps

# Configuration
ADO_ORG="${ADO_ORG:-}"
ADO_PROJECT="${ADO_PROJECT:-}"
ADO_PAT="${ADO_PAT:-}"
OUTPUT_DIR="./data"

# Validate required environment variables
if [[ -z "$ADO_ORG" || -z "$ADO_PROJECT" || -z "$ADO_PAT" ]]; then
    echo "❌ Missing required environment variables:"
    echo "   ADO_ORG, ADO_PROJECT, ADO_PAT"
    exit 1
fi

# Create output directory
mkdir -p "$OUTPUT_DIR"

echo "🔄 Fetching ADO data for organization: $ADO_ORG, project: $ADO_PROJECT"

# Base64 encode PAT for Basic Auth
AUTH_HEADER="Authorization: Basic $(echo -n ":$ADO_PAT" | base64 -w 0)"
BASE_URL="https://dev.azure.com/$ADO_ORG/$ADO_PROJECT/_apis"

# Function to make API calls with error handling
make_api_call() {
    local url="$1"
    local output_file="$2"
    local description="$3"
    
    echo "📡 Fetching $description..."
    
    if curl -s -H "$AUTH_HEADER" \
            -H "Content-Type: application/json" \
            "$url" \
            -o "$output_file"; then
        echo "✅ $description saved to $output_file"
    else
        echo "❌ Failed to fetch $description"
        return 1
    fi
}

# Fetch Work Items (last 1000, recent first)
make_api_call \
    "$BASE_URL/wit/workitems?\$top=1000&\$orderby=System.ChangedDate%20desc&api-version=7.0" \
    "$OUTPUT_DIR/work_items.json" \
    "Work Items"

# Fetch Work Item Details (get additional fields)
WORK_ITEM_IDS=$(jq -r '.value[].id' "$OUTPUT_DIR/work_items.json" | head -500 | tr '\n' ',' | sed 's/,$//')

if [[ -n "$WORK_ITEM_IDS" ]]; then
    make_api_call \
        "$BASE_URL/wit/workitems?ids=$WORK_ITEM_IDS&\$expand=All&api-version=7.0" \
        "$OUTPUT_DIR/work_items_detailed.json" \
        "Detailed Work Items"
fi

# Fetch Builds (last 500, recent first)
make_api_call \
    "$BASE_URL/build/builds?\$top=500&\$orderby=queueTime%20desc&api-version=7.0" \
    "$OUTPUT_DIR/builds.json" \
    "Builds"

# Fetch Build Definitions
make_api_call \
    "$BASE_URL/build/definitions?api-version=7.0" \
    "$OUTPUT_DIR/build_definitions.json" \
    "Build Definitions"

# Fetch Releases (last 200, recent first)
make_api_call \
    "$BASE_URL/release/releases?\$top=200&\$orderby=createdOn%20desc&api-version=7.0" \
    "$OUTPUT_DIR/releases.json" \
    "Releases"

# Fetch Release Definitions
make_api_call \
    "$BASE_URL/release/definitions?api-version=7.0" \
    "$OUTPUT_DIR/release_definitions.json" \
    "Release Definitions"

# Fetch Test Runs (last 100)
make_api_call \
    "$BASE_URL/test/runs?\$top=100&api-version=7.0" \
    "$OUTPUT_DIR/test_runs.json" \
    "Test Runs"

echo "🎉 ADO data fetch completed successfully!"
echo "📁 Data files saved to: $OUTPUT_DIR"
ls -la "$OUTPUT_DIR"
```

### JSON to SQLite Converter (scripts/json-to-sqlite.sh)
```bash
#!/bin/bash
set -euo pipefail

# Convert ADO JSON responses to SQLite database
# Creates one column per JSON key for optimal Grafana integration

DB_PATH="/data/grafana.db"
DATA_DIR="/app/data"

echo "🔄 Converting JSON data to SQLite database: $DB_PATH"

# Function to convert ISO date to Unix timestamp
iso_to_timestamp() {
    local iso_date="$1"
    if [[ -n "$iso_date" && "$iso_date" != "null" ]]; then
        date -d "$iso_date" +%s 2>/dev/null || echo "0"
    else
        echo "0"
    fi
}

# Process Work Items
if [[ -f "$DATA_DIR/work_items_detailed.json" ]]; then
    echo "📊 Processing Work Items..."
    
    jq -r '.value[] | 
        [
            .id,
            .fields["System.Title"] // "",
            .fields["System.WorkItemType"] // "",
            .fields["System.State"] // "",
            .fields["System.AssignedTo"].displayName // "",
            .fields["System.CreatedDate"] // "",
            .fields["System.ChangedDate"] // "",
            .fields["Microsoft.VSTS.Common.ResolvedDate"] // "",
            .fields["System.AreaPath"] // "",
            .fields["System.IterationPath"] // "",
            .fields["Microsoft.VSTS.Common.Priority"] // "0",
            .fields["Microsoft.VSTS.Common.Severity"] // "",
            .fields["Microsoft.VSTS.Scheduling.StoryPoints"] // "0",
            .fields["Microsoft.VSTS.Scheduling.Effort"] // "0",
            .fields["Microsoft.VSTS.Scheduling.OriginalEstimate"] // "0",
            .fields["Microsoft.VSTS.Scheduling.RemainingWork"] // "0",
            .fields["Microsoft.VSTS.Scheduling.CompletedWork"] // "0",
            .fields["System.Tags"] // "",
            (.url | split("/")[4]) // ""
        ] | @csv' "$DATA_DIR/work_items_detailed.json" | \
    while IFS= read -r line; do
        # Parse CSV line and convert dates to timestamps
        eval "values=($line)"
        created_ts=$(iso_to_timestamp "${values[5]//\"/}")
        changed_ts=$(iso_to_timestamp "${values[6]//\"/}")
        resolved_ts=$(iso_to_timestamp "${values[7]//\"/}")
        
        sqlite3 "$DB_PATH" "INSERT OR REPLACE INTO ado_work_items (
            work_item_id, title, work_item_type, state, assigned_to,
            created_date, changed_date, resolved_date, area_path, iteration_path,
            priority, severity, story_points, effort, original_estimate,
            remaining_work, completed_work, tags, project,  
            created_timestamp, changed_timestamp, resolved_timestamp
        ) VALUES (
            ${values[0]//\"/}, ${values[1]}, ${values[2]}, ${values[3]}, ${values[4]},
            ${values[5]}, ${values[6]}, ${values[7]}, ${values[8]}, ${values[9]},
            ${values[10]//\"/}, ${values[11]}, ${values[12]//\"/}, ${values[13]//\"/}, ${values[14]//\"/},
            ${values[15]//\"/}, ${values[16]//\"/}, ${values[17]}, ${values[18]},
            $created_ts, $changed_ts, $resolved_ts
        );"
    done
    
    echo "✅ Work Items processed"
fi

# Process Builds
if [[ -f "$DATA_DIR/builds.json" ]]; then
    echo "📊 Processing Builds..."
    
    jq -r '.value[] | 
        [
            .id,
            .buildNumber // "",
            .status // "",
            .result // "",
            .queueTime // "",
            .startTime // "",
            .finishTime // "",
            .sourceBranch // "",
            .sourceVersion // "",
            .definition.name // "",
            .definition.id // "0",
            .project.name // "",
            .requestedFor.displayName // ""
        ] | @csv' "$DATA_DIR/builds.json" | \
    while IFS= read -r line; do
        eval "values=($line)"
        queue_ts=$(iso_to_timestamp "${values[4]//\"/}")
        start_ts=$(iso_to_timestamp "${values[5]//\"/}")
        finish_ts=$(iso_to_timestamp "${values[6]//\"/}")
        
        sqlite3 "$DB_PATH" "INSERT OR REPLACE INTO ado_builds (
            build_id, build_number, status, result, queue_time,
            start_time, finish_time, source_branch, source_version,
            definition_name, definition_id, project, requested_for,
            queue_timestamp, start_timestamp, finish_timestamp
        ) VALUES (
            ${values[0]//\"/}, ${values[1]}, ${values[2]}, ${values[3]}, ${values[4]},
            ${values[5]}, ${values[6]}, ${values[7]}, ${values[8]},
            ${values[9]}, ${values[10]//\"/}, ${values[11]}, ${values[12]},
            $queue_ts, $start_ts, $finish_ts
        );"
    done
    
    echo "✅ Builds processed"
fi

# Process Releases
if [[ -f "$DATA_DIR/releases.json" ]]; then
    echo "📊 Processing Releases..."
    
    jq -r '.value[] | 
        [
            .id,
            .name // "",
            .status // "",
            .createdOn // "",
            .modifiedOn // "",
            .createdBy.displayName // "",
            .modifiedBy.displayName // "",
            .releaseDefinition.name // "",
            .releaseDefinition.id // "0",
            (.url | split("/")[4]) // ""
        ] | @csv' "$DATA_DIR/releases.json" | \
    while IFS= read -r line; do
        eval "values=($line)"
        created_ts=$(iso_to_timestamp "${values[3]//\"/}")
        modified_ts=$(iso_to_timestamp "${values[4]//\"/}")
        
        sqlite3 "$DB_PATH" "INSERT OR REPLACE INTO ado_releases (
            release_id, release_name, status, created_date, modified_date,
            created_by, modified_by, definition_name, definition_id, project,
            created_timestamp, modified_timestamp
        ) VALUES (
            ${values[0]//\"/}, ${values[1]}, ${values[2]}, ${values[3]}, ${values[4]},
            ${values[5]}, ${values[6]}, ${values[7]}, ${values[8]//\"/}, ${values[9]},
            $created_ts, $modified_ts
        );"
    done
    
    echo "✅ Releases processed"
fi

# Verify data import
echo "📋 Database Summary:"
echo "Work Items: $(sqlite3 "$DB_PATH" "SELECT COUNT(*) FROM ado_work_items;")"
echo "Builds: $(sqlite3 "$DB_PATH" "SELECT COUNT(*) FROM ado_builds;")"
echo "Releases: $(sqlite3 "$DB_PATH" "SELECT COUNT(*) FROM ado_releases;")"

echo "🎉 JSON to SQLite conversion completed!"
```

### Build and Deploy Script (scripts/build-deploy.sh)
```bash
#!/bin/bash
set -euo pipefail

# Build and Deploy Script for Ubuntu 22.04
# Usage: ./build-deploy.sh [ephemeral|persistent]

DEPLOYMENT_TYPE="${1:-ephemeral}"
REGISTRY="${REGISTRY:-your-registry.azurecr.io}"
IMAGE_NAME="sqlite-grafana"
TAG="${TAG:-latest}"
NAMESPACE="monitoring"

echo "🚀 Building and deploying SQLite container for AKS"
echo "📦 Deployment type: $DEPLOYMENT_TYPE"

# Validate deployment type
if [[ "$DEPLOYMENT_TYPE" != "ephemeral" && "$DEPLOYMENT_TYPE" != "persistent" ]]; then
    echo "❌ Invalid deployment type. Use 'ephemeral' or 'persistent'"
    exit 1
fi

# Check prerequisites
check_prerequisites() {
    echo "🔍 Checking prerequisites..."
    
    # Check if running on Ubuntu 22.04
    if ! grep -q "Ubuntu 22.04" /etc/os-release 2>/dev/null; then
        echo "⚠️  Warning: This script is designed for Ubuntu 22.04"
    fi
    
    # Check required tools
    local required_tools=("docker" "kubectl" "az" "jq")
    for tool in "${required_tools[@]}"; do
        if ! command -v "$tool" &> /dev/null; then
            echo "❌ Required tool not found: $tool"
            echo "📋 Install missing tools:"
            echo "   sudo apt update"
            echo "   sudo apt install -y docker.io jq"
            echo "   curl -LO https://dl.k8s.io/release/v1.30.0/bin/linux/amd64/kubectl"
            echo "   sudo install kubectl /usr/local/bin/"
            echo "   curl -sL https://aka.ms/InstallAzureCLI | sudo bash"
            exit 1
        fi
    done
    
    echo "✅ Prerequisites check passed"
}

# Fetch ADO data if credentials are provided
fetch_data() {
    if [[ -n "${ADO_ORG:-}" && -n "${ADO_PROJECT:-}" && -n "${ADO_PAT:-}" ]]; then
        echo "📡 Fetching ADO data..."
        ./scripts/fetch-ado-data.sh
    else
        echo "⚠️  ADO credentials not provided, using sample data"
        mkdir -p ./data
        echo '{"value":[{"id":1,"fields":{"System.Title":"Sample Work Item","System.WorkItemType":"Bug","System.State":"New","System.CreatedDate":"2024-01-01T00:00:00Z"}}]}' > ./data/work_items_detailed.json
        echo '{"value":[{"id":1,"buildNumber":"1.0.0","status":"completed","result":"succeeded","queueTime":"2024-01-01T00:00:00Z","definition":{"name":"Sample Build","id":1},"project":{"name":"Sample Project"}}]}' > ./data/builds.json
        echo '{"value":[{"id":1,"name":"Sample Release","status":"active","createdOn":"2024-01-01T00:00:00Z","releaseDefinition":{"name":"Sample Release Definition","id":1}}]}' > ./data/releases.json
    fi
}

# Build Docker image
build_image() {
    echo "🔨 Building Docker image..."
    
    # Login to Azure Container Registry
    if [[ "$REGISTRY" == *".azurecr.io" ]]; then
        echo "🔐 Logging into Azure Container Registry..."
        az acr login --name "${REGISTRY%%.*}"
    fi
    
    # Build image
    docker build -t "$REGISTRY/$IMAGE_NAME:$TAG" .
    
    # Push image
    echo "📤 Pushing image to registry..."
    docker push "$REGISTRY/$IMAGE_NAME:$TAG"
    
    echo "✅ Image built and pushed: $REGISTRY/$IMAGE_NAME:$TAG"
}

# Deploy to Kubernetes
deploy_to_k8s() {
    echo "🚀 Deploying to Kubernetes..."
    
    # Create namespace
    kubectl create namespace "$NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -
    
    # Update image in manifest
    sed -i.bak "s|image: .*|image: $REGISTRY/$IMAGE_NAME:$TAG|g" "manifests/sqlite-$DEPLOYMENT_TYPE.yaml"
    
    # Apply storage class if persistent
    if [[ "$DEPLOYMENT_TYPE" == "persistent" ]]; then
        kubectl apply -f manifests/storage-class.yaml
    fi
    
    # Apply deployment
    kubectl apply -f "manifests/sqlite-$DEPLOYMENT_TYPE.yaml"
    
    # Wait for deployment
    echo "⏳ Waiting for deployment to be ready..."
    if [[ "$DEPLOYMENT_TYPE" == "persistent" ]]; then
        kubectl rollout status statefulset/sqlite-grafana -n "$NAMESPACE" --timeout=300s
    else
        kubectl rollout status deployment/sqlite-grafana -n "$NAMESPACE" --timeout=300s
    fi
    
    echo "✅ Deployment completed successfully!"
}

# Test deployment
test_deployment() {
    echo "🧪 Testing deployment..."
    
    # Get pod name
    POD_NAME=$(kubectl get pods -n "$NAMESPACE" -l app=sqlite-grafana -o jsonpath='{.items[0].metadata.name}')
    
    # Test database connectivity
    echo "📊 Testing database..."
    WORK_ITEMS_COUNT=$(kubectl exec -n "$NAMESPACE" "$POD_NAME" -- sqlite3 /data/grafana.db "SELECT COUNT(*) FROM ado_work_items;" 2>/dev/null || echo "0")
    BUILDS_COUNT=$(kubectl exec -n "$NAMESPACE" "$POD_NAME" -- sqlite3 /data/grafana.db "SELECT COUNT(*) FROM ado_builds;" 2>/dev/null || echo "0")
    
    echo "📈 Database contents:"
    echo "   Work Items: $WORK_ITEMS_COUNT"
    echo "   Builds: $BUILDS_COUNT"
    
    if [[ "$WORK_ITEMS_COUNT" -gt 0 || "$BUILDS_COUNT" -gt 0 ]]; then
        echo "✅ Database test passed"
    else
        echo "⚠️  Database appears empty, check logs"
    fi
    
    # Show connection info
    echo "🔗 Connection Information:"
    echo "   Namespace: $NAMESPACE"
    echo "   Service: sqlite-grafana-service"
    echo "   Database Path: /data/grafana.db"
    echo ""
    echo "📋 To connect from Grafana:"
    echo "   Data Source Type: SQLite"
    echo "   Path: /var/lib/grafana/sqlite/grafana.db"
    echo "   (Ensure shared volume mount between SQLite and Grafana)"
}

# Main execution
main() {
    check_prerequisites
    fetch_data
    build_image
    deploy_to_k8s
    test_deployment
    
    echo ""
    echo "🎉 SQLite deployment completed successfully!"
    echo "🔧 Next steps:"
    echo "   1. Deploy Grafana with shared volume mount"
    echo "   2. Install SQLite datasource plugin in Grafana"
    echo "   3. Configure datasource with path: /var/lib/grafana/sqlite/grafana.db"
    echo ""
    echo "📖 View logs: kubectl logs -n $NAMESPACE -l app=sqlite-grafana"
    echo "🔍 Debug: kubectl exec -it -n $NAMESPACE $POD_NAME -- /bin/bash"
}

# Run main function
main "$@"
```

## Kubernetes Manifests

### Ephemeral Deployment (manifests/sqlite-ephemeral.yaml)
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: sqlite-grafana
  namespace: monitoring
  labels:
    app: sqlite-grafana
    storage-type: ephemeral
spec:
  replicas: 1
  selector:
    matchLabels:
      app: sqlite-grafana
  template:
    metadata:
      labels:
        app: sqlite-grafana
        storage-type: ephemeral
    spec:
      securityContext:
        runAsNonRoot: true
        runAsUser: 1000
        runAsGroup: 1000
        fsGroup: 1000
      containers:
      - name: sqlite
        image: your-registry.azurecr.io/sqlite-grafana:latest
        ports:
        - containerPort: 8080
          name: http
        securityContext:
          allowPrivilegeEscalation: false
          capabilities:
            drop: ["ALL"]
          readOnlyRootFilesystem: false
        env:
        - name: SQLITE_DB_PATH
          value: "/data/grafana.db"
        resources:
          requests:
            cpu: 100m
            memory: 128Mi
          limits:
            cpu: 500m
            memory: 512Mi
        livenessProbe:
          exec:
            command: ["sqlite3", "/data/grafana.db", "SELECT 1;"]
          initialDelaySeconds: 30
          periodSeconds: 30
        readinessProbe:
          exec:
            command: ["sqlite3", "/data/grafana.db", "SELECT COUNT(*) FROM ado_work_items;"]
          initialDelaySeconds: 10
          periodSeconds: 10
        volumeMounts:
        - name: sqlite-data
          mountPath: /data
        - name: grafana-shared
          mountPath: /var/lib/grafana/sqlite
      volumes:
      - name: sqlite-data
        emptyDir: {}
      - name: grafana-shared
        emptyDir: {}
---
apiVersion: v1
kind: Service
metadata:
  name: sqlite-grafana-service
  namespace: monitoring
spec:
  selector:
    app: sqlite-grafana
  ports:
  - port: 8080
    targetPort: 8080
    name: http
  type: ClusterIP
```

### Persistent Deployment with Azure Blob (manifests/sqlite-persistent.yaml)
```yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: sqlite-grafana
  namespace: monitoring
  labels:
    app: sqlite-grafana
    storage-type: persistent
spec:
  serviceName: sqlite-grafana-service
  replicas: 1
  selector:
    matchLabels:
      app: sqlite-grafana
  template:
    metadata:
      labels:
        app: sqlite-grafana
        storage-type: persistent
    spec:
      securityContext:
        runAsNonRoot: true
        runAsUser: 1000
        runAsGroup: 1000
        fsGroup: 1000
      containers:
      - name: sqlite
        image: your-registry.azurecr.io/sqlite-grafana:latest
        ports:
        - containerPort: 8080
          name: http
        securityContext:
          allowPrivilegeEscalation: false
          capabilities:
            drop: ["ALL"]
          readOnlyRootFilesystem: false
        env:
        - name: SQLITE_DB_PATH
          value: "/data/grafana.db"
        resources:
          requests:
            cpu: 100m
            memory: 128Mi
          limits:
            cpu: 500m
            memory: 1Gi
        livenessProbe:
          exec:
            command: ["sqlite3", "/data/grafana.db", "SELECT 1;"]
          initialDelaySeconds: 30
          periodSeconds: 30
        readinessProbe:
          exec:
            command: ["sqlite3", "/data/grafana.db", "SELECT COUNT(*) FROM ado_work_items;"]
          initialDelaySeconds: 10
          periodSeconds: 10
        volumeMounts:
        - name: sqlite-persistent-storage
          mountPath: /data
        - name: grafana-shared-persistent
          mountPath: /var/lib/grafana/sqlite
      volumes:
      - name: grafana-shared-persistent
        persistentVolumeClaim:
          claimName: grafana-shared-pvc
  volumeClaimTemplates:
  - metadata:
      name: sqlite-persistent-storage
    spec:
      accessModes: ["ReadWriteOnce"]
      storageClassName: "azure-blob-premium"
      resources:
        requests:
          storage: 10Gi
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: grafana-shared-pvc
  namespace: monitoring
spec:
  accessModes:
    - ReadWriteMany
  storageClassName: azure-blob-premium
  resources:
    requests:
      storage: 5Gi
---
apiVersion: v1
kind: Service
metadata:
  name: sqlite-grafana-service
  namespace: monitoring
spec:
  selector:
    app: sqlite-grafana
  ports:
  - port: 8080
    targetPort: 8080
    name: http
  type: ClusterIP
  clusterIP: None
```

### Azure Blob Storage Class (manifests/storage-class.yaml)
```yaml
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: azure-blob-premium
provisioner: blob.csi.azure.com
parameters:
  skuName: Premium_LRS
  location: eastus
  resourceGroup: your-resource-group
  storageAccount: yourstorageaccount
  protocol: fuse
  tags: environment=production,component=grafana-sqlite
reclaimPolicy: Retain
allowVolumeExpansion: true
volumeBindingMode: Immediate
```

## Grafana Configuration

### Grafana Datasource Config (config/grafana-datasource.yaml)
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: grafana-sqlite-datasource
  namespace: monitoring
data:
  datasource.yaml: |
    apiVersion: 1
    datasources:
    - name: SQLite-ADO
      type: frser-sqlite-datasource
      access: direct
      url: file:/var/lib/grafana/sqlite/grafana.db
      isDefault: true
      editable: true
      jsonData:
        database: /var/lib/grafana/sqlite/grafana.db
      secureJsonData: {}
```

## Usage Guide

### Prerequisites (Ubuntu 22.04)
```bash
# Install required packages
sudo apt update
sudo apt install -y docker.io jq curl

# Install kubectl
curl -LO "https://dl.k8s.io/release/$(curl -L -s https://dl.k8s.io/release/stable.txt)/bin/linux/amd64/kubectl"
sudo install -o root -g root -m 0755 kubectl /usr/local/bin/kubectl

# Install Azure CLI
curl -sL https://aka.ms/InstallAzureCLI | sudo bash

# Add user to docker group
sudo usermod -aG docker $USER
newgrp docker
```

### Configuration
```bash
# Set required environment variables
export ADO_ORG="your-organization"
export ADO_PROJECT="your-project"  
export ADO_PAT="your-personal-access-token"
export REGISTRY="your-registry.azurecr.io"

# Configure Azure CLI and kubectl
az login
az aks get-credentials --resource-group your-rg --name your-aks-cluster
```

### Deployment Commands
```bash
# Deploy ephemeral version (data in container only)
./scripts/build-deploy.sh ephemeral

# Deploy persistent version (data on Azure Blob storage)
./scripts/build-deploy.sh persistent

# Check deployment status
kubectl get pods -n monitoring
kubectl logs -n monitoring -l app=sqlite-grafana

# Test database content
kubectl exec -n monitoring deployment/sqlite-grafana -- sqlite3 /data/grafana.db "SELECT COUNT(*) FROM ado_work_items;"
```

### Grafana Integration
```bash
# Deploy Grafana with SQLite datasource
kubectl apply -f config/grafana-datasource.yaml

# Configure Grafana deployment to mount shared volume
# Add this volume mount to your Grafana deployment:
volumeMounts:
- name: sqlite-shared
  mountPath: /var/lib/grafana/sqlite
  readOnly: true

volumes:
- name: sqlite-shared
  persistentVolumeClaim:
    claimName: grafana-shared-pvc  # For persistent
  # OR for ephemeral:
  # emptyDir: {}
```

## Sample Grafana Queries

### Work Items Over Time
```sql
SELECT 
  created_timestamp * 1000 as time_msec,
  work_item_type as metric,
  COUNT(*) as value
FROM ado_work_items 
WHERE created_timestamp >= $__from / 1000 
AND created_timestamp <= $__to / 1000
GROUP BY created_timestamp, work_item_type
ORDER BY created_timestamp
```

### Build Success Rate
```sql
SELECT 
  start_timestamp * 1000 as time_msec,
  'success_rate' as metric,
  (COUNT(CASE WHEN result = 'succeeded' THEN 1 END) * 100.0 / COUNT(*)) as value
FROM ado_builds 
WHERE start_timestamp >= $__from / 1000 
AND start_timestamp <= $__to / 1000
GROUP BY DATE(start_time)
ORDER BY start_timestamp
```

## Troubleshooting

### Common Issues
```bash
# Check pod logs
kubectl logs -n monitoring -l app=sqlite-grafana

# Verify database file exists
kubectl exec -n monitoring deployment/sqlite-grafana -- ls -la /data/

# Test database connectivity
kubectl exec -n monitoring deployment/sqlite-grafana -- sqlite3 /data/grafana.db ".tables"

# Check volume mounts
kubectl describe pod -n monitoring -l app=sqlite-grafana
```

### Database Maintenance
```bash
# Backup database (persistent storage)
kubectl exec -n monitoring statefulset/sqlite-grafana -- sqlite3 /data/grafana.db ".backup /data/backup.db"

# Vacuum database for optimization
kubectl exec -n monitoring deployment/sqlite-grafana -- sqlite3 /data/grafana.db "VACUUM;"

# Check database integrity
kubectl exec -n monitoring deployment/sqlite-grafana -- sqlite3 /data/grafana.db "PRAGMA integrity_check;"
```

This deployment provides a production-ready SQLite container optimized for Grafana datasource integration with comprehensive ADO REST API data ingestion capabilities.
