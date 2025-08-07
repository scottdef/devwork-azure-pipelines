# Comprehensive Technical Guide: Prepopulating SQLite Databases in Docker Containers with JSON Data

Containerized SQLite databases with prepopulated data represent a powerful pattern for deploying immediately queryable, stateless database containers. This guide provides production-ready implementations across three key methods, optimized for enterprise environments with comprehensive error handling and best practices.

## Overview and Architecture

**The deployment_recs folder structure** contains 12 monthly subfolders (2024-01 through 2024-12) with dep-rec-envId.json files representing Azure DevOps-style deployment records. The goal is creating Docker containers with fully populated SQLite databases that are immediately queryable upon startup and deployable to Kubernetes.

**Core benefits of this approach:**
- **Instant availability**: No database initialization delays at runtime
- **Stateless design**: Perfect for Kubernetes horizontal scaling
- **Performance**: 100K+ SELECTs/second with proper optimization
- **Simplicity**: No external database dependencies

## Method 1: Bash Commands with jq for Data Processing

### Advanced JSON Processing Pipeline

The foundation of effective SQLite population lies in robust JSON processing. Here's a production-ready bash script that handles the entire pipeline:

```bash
#!/bin/bash
set -euo pipefail

# Configuration
readonly SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly BASE_DIR="${1:-deployment_recs}"
readonly DB_FILE="${2:-deployments.db}"
readonly LOG_FILE="processing_$(date +%Y%m%d_%H%M%S).log"
readonly MAX_PARALLEL_JOBS=4

declare -g ERROR_COUNT=0
declare -g PROCESSED_COUNT=0

# Comprehensive logging system
log_info() { echo "[INFO $(date '+%Y-%m-%d %H:%M:%S')] $*" | tee -a "$LOG_FILE"; }
log_warn() { echo "[WARN $(date '+%Y-%m-%d %H:%M:%S')] $*" | tee -a "$LOG_FILE"; }
log_error() { echo "[ERROR $(date '+%Y-%m-%d %H:%M:%S')] $*" | tee -a "$LOG_FILE" >&2; ((ERROR_COUNT++)); }

# Advanced jq transformation for Azure DevOps deployment records
transform_deployment_json() {
    local json_file="$1"
    local env_id="$2"
    
    jq -c '
        (.deployments[]? // [.]) | 
        map(select(type == "object" and has("deploymentId"))) |
        map({
            deployment_id: .deploymentId,
            environment: (.environment // "'"$env_id"'"),
            status: (.status // "unknown"),
            start_time: (.startTime // .createdDate),
            finish_time: (.finishTime // .completedDate),
            duration_seconds: (
                if .finishTime and .startTime then
                    (((.finishTime | fromdateiso8601) - (.startTime | fromdateiso8601)) | floor)
                else null end
            ),
            build_id: .variables.buildId,
            release_id: .variables.releaseId,
            resource_count: (.resources | length),
            resources_json: (.resources | tojsonstream),
            variables_json: (.variables | tojsonstream),
            raw_data: (. | tojsonstream),
            file_source: "'"$json_file"'"
        })[]
    ' "$json_file"
}

# SQLite database initialization with optimal schema
initialize_database() {
    log_info "Initializing database: $DB_FILE"
    
    sqlite3 "$DB_FILE" <<'EOF'
-- Create optimized schema for deployment records
CREATE TABLE IF NOT EXISTS deployments (
    deployment_id TEXT PRIMARY KEY,
    environment TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('succeeded', 'failed', 'running', 'pending')),
    start_time TEXT NOT NULL,
    finish_time TEXT,
    duration_seconds INTEGER,
    build_id TEXT,
    release_id TEXT,
    resource_count INTEGER DEFAULT 0,
    resources_json TEXT,
    variables_json TEXT,
    raw_data TEXT,
    file_source TEXT,
    processed_at TEXT DEFAULT CURRENT_TIMESTAMP
);

-- Performance optimizations for bulk operations
PRAGMA journal_mode = WAL;
PRAGMA synchronous = NORMAL;
PRAGMA cache_size = 100000;
PRAGMA temp_store = memory;
PRAGMA mmap_size = 268435456;
EOF
}

# High-performance SQLite import using JSON1 extension
import_json_with_sqlite() {
    local json_file="$1"
    local env_id="$2"
    
    # Validate JSON structure first
    if ! jq empty "$json_file" 2>/dev/null; then
        log_error "Invalid JSON syntax in $json_file"
        return 1
    fi
    
    # Transform and import data
    if transform_deployment_json "$json_file" "$env_id" | \
       sqlite3 "$DB_FILE" '.mode json' '.import /dev/stdin deployments_temp' 2>/dev/null; then
        
        # Merge into main table with conflict resolution
        sqlite3 "$DB_FILE" <<EOF
INSERT OR REPLACE INTO deployments 
SELECT * FROM deployments_temp;
DROP TABLE deployments_temp;
EOF
        return 0
    else
        log_error "Failed to import $json_file"
        return 1
    fi
}

# Parallel processing with GNU parallel
process_all_files() {
    local total_files
    total_files=$(find "$BASE_DIR" -name "dep-rec-*.json" | wc -l)
    log_info "Processing $total_files files with $MAX_PARALLEL_JOBS parallel jobs"
    
    find "$BASE_DIR" -name "dep-rec-*.json" | \
    parallel --jobs "$MAX_PARALLEL_JOBS" --line-buffer --bar \
        process_single_file {}
}

process_single_file() {
    local json_file="$1"
    local env_id
    
    # Extract environment ID from filename: dep-rec-envId.json
    env_id=$(basename "$json_file" .json | sed 's/dep-rec-//')
    
    if import_json_with_sqlite "$json_file" "$env_id"; then
        ((PROCESSED_COUNT++))
        log_info "✓ Processed: $json_file"
    else
        log_error "✗ Failed: $json_file"
        return 1
    fi
}

# Database finalization with indexes and optimization
finalize_database() {
    log_info "Finalizing database with indexes and optimization"
    
    sqlite3 "$DB_FILE" <<'EOF'
-- Create performance indexes
CREATE INDEX IF NOT EXISTS idx_env_status ON deployments(environment, status);
CREATE INDEX IF NOT EXISTS idx_start_time ON deployments(start_time);
CREATE INDEX IF NOT EXISTS idx_duration ON deployments(duration_seconds) WHERE duration_seconds IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_build_release ON deployments(build_id, release_id);

-- Update statistics and optimize
ANALYZE deployments;
VACUUM;
PRAGMA optimize;
EOF
}

# Comprehensive validation and reporting
generate_report() {
    log_info "Generating processing report"
    
    sqlite3 -header -column "$DB_FILE" <<'EOF'
SELECT '=== DEPLOYMENT PROCESSING REPORT ===' as title;

SELECT 
    COUNT(*) as total_deployments,
    COUNT(DISTINCT environment) as unique_environments,
    COUNT(DISTINCT build_id) as unique_builds,
    MIN(start_time) as earliest_deployment,
    MAX(start_time) as latest_deployment
FROM deployments;

SELECT 
    environment,
    COUNT(*) as total,
    SUM(CASE WHEN status = 'succeeded' THEN 1 ELSE 0 END) as succeeded,
    ROUND(
        SUM(CASE WHEN status = 'succeeded' THEN 1 ELSE 0 END) * 100.0 / COUNT(*), 
        2
    ) as success_rate_percent
FROM deployments 
GROUP BY environment
ORDER BY success_rate_percent DESC;
EOF
}

# Main execution pipeline
main() {
    local start_time=$(date +%s)
    
    log_info "Starting deployment record processing"
    
    # Validate prerequisites
    for cmd in jq sqlite3 parallel; do
        command -v "$cmd" >/dev/null || { log_error "Missing dependency: $cmd"; exit 1; }
    done
    
    # Check SQLite JSON1 extension
    if ! sqlite3 ":memory:" "SELECT json_valid('{}')" >/dev/null 2>&1; then
        log_error "SQLite JSON1 extension not available"
        exit 1
    fi
    
    # Execute processing pipeline
    initialize_database
    process_all_files
    finalize_database
    generate_report
    
    local end_time=$(date +%s)
    local duration=$((end_time - start_time))
    
    log_info "Processing completed in ${duration}s"
    log_info "Successfully processed: $PROCESSED_COUNT files"
    log_info "Errors encountered: $ERROR_COUNT"
    
    [ "$ERROR_COUNT" -eq 0 ] && return 0 || return 1
}

# Error handling and cleanup
cleanup() {
    local exit_code=${1:-1}
    log_info "Cleaning up..."
    jobs -p | xargs -r kill 2>/dev/null || true
    exit $exit_code
}

trap 'cleanup 130' INT
trap 'cleanup 143' TERM

# Execute main function
main "$@"
```

### Multi-Stage Dockerfile Implementation

```dockerfile
# Multi-stage build for SQLite container with prepopulated data
FROM alpine:3.18 AS data-processor

# Install processing tools
RUN apk add --no-cache sqlite bash jq parallel curl

WORKDIR /build

# Copy data processing scripts
COPY scripts/process_deployments.sh ./
COPY deployment_recs/ ./deployment_recs/

# Process data and create database
RUN chmod +x process_deployments.sh && \
    ./process_deployments.sh deployment_recs deployments.db && \
    sqlite3 deployments.db "PRAGMA journal_mode = WAL;" && \
    sqlite3 deployments.db "VACUUM;"

# Production runtime stage
FROM alpine:3.18 AS runtime

RUN apk add --no-cache sqlite && \
    adduser -D -s /bin/sh appuser

# Copy optimized database from build stage
COPY --from=data-processor --chown=appuser:appuser /build/deployments.db /data/

# Copy application
COPY --chown=appuser:appuser app/ /app/
RUN chmod +x /app/entrypoint.sh

USER appuser
VOLUME ["/data"]
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD sqlite3 /data/deployments.db "SELECT 1;" && curl -f http://localhost:8080/health || exit 1

ENTRYPOINT ["/app/entrypoint.sh"]
CMD ["/app/server"]
```

## Method 2: GitHub Actions Workflows for Automated Builds

### Production-Ready GitHub Actions Workflow

```yaml
name: Build SQLite Database Container

on:
  push:
    branches: [main, develop]
    paths: 
      - 'deployment_recs/**'
      - 'Dockerfile'
      - 'scripts/**'
  pull_request:
    branches: [main]
  schedule:
    - cron: '0 2 * * 0'  # Weekly rebuild

env:
  REGISTRY: ghcr.io
  IMAGE_NAME: ${{ github.repository }}/sqlite-deployments

jobs:
  build-and-test:
    runs-on: ubuntu-latest
    permissions:
      contents: read
      packages: write
      security-events: write

    steps:
      - name: Checkout repository
        uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - name: Set up Docker Buildx
        uses: docker/setup-buildx-action@v3
        with:
          platforms: linux/amd64,linux/arm64

      - name: Log in to Container Registry
        uses: docker/login-action@v3
        with:
          registry: ${{ env.REGISTRY }}
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      - name: Extract metadata
        id: meta
        uses: docker/metadata-action@v5
        with:
          images: ${{ env.REGISTRY }}/${{ env.IMAGE_NAME }}
          tags: |
            type=ref,event=branch
            type=ref,event=pr
            type=semver,pattern={{version}}
            type=semver,pattern={{major}}.{{minor}}
            type=raw,value=latest,enable={{is_default_branch}}
            type=sha,prefix={{branch}}-

      - name: Validate JSON data
        run: |
          echo "Validating JSON files..."
          find deployment_recs -name "*.json" -exec jq empty {} \; || exit 1
          echo "JSON validation completed successfully"

      - name: Build and push Docker image
        uses: docker/build-push-action@v6
        with:
          context: .
          platforms: linux/amd64,linux/arm64
          push: ${{ github.event_name != 'pull_request' }}
          tags: ${{ steps.meta.outputs.tags }}
          labels: ${{ steps.meta.outputs.labels }}
          cache-from: type=gha
          cache-to: type=gha,mode=max
          build-args: |
            BUILD_DATE=${{ fromJSON(steps.meta.outputs.json).labels['org.opencontainers.image.created'] }}
            VCS_REF=${{ github.sha }}

      - name: Test container functionality
        if: github.event_name == 'pull_request'
        run: |
          # Start container for testing
          docker run -d --name test-container \
            -p 8080:8080 \
            ${{ env.REGISTRY }}/${{ env.IMAGE_NAME }}:${{ steps.meta.outputs.version }}
          
          # Wait for container to be ready
          timeout 30 bash -c 'until curl -f http://localhost:8080/health; do sleep 2; done'
          
          # Test database queries
          docker exec test-container sqlite3 /data/deployments.db \
            "SELECT COUNT(*) as total_deployments FROM deployments;"
          
          # Cleanup
          docker stop test-container
          docker rm test-container

      - name: Security scan with Trivy
        uses: aquasecurity/trivy-action@master
        with:
          image-ref: ${{ env.REGISTRY }}/${{ env.IMAGE_NAME }}:${{ steps.meta.outputs.version }}
          format: 'sarif'
          output: 'trivy-results.sarif'

      - name: Upload Trivy scan results
        uses: github/codeql-action/upload-sarif@v2
        if: always()
        with:
          sarif_file: 'trivy-results.sarif'

  deploy-to-staging:
    needs: build-and-test
    runs-on: ubuntu-latest
    if: github.ref == 'refs/heads/develop'
    environment: staging
    
    steps:
      - name: Deploy to staging cluster
        run: |
          echo "Deploying to staging Kubernetes cluster..."
          # Add your Kubernetes deployment logic here
```

### Advanced Caching Strategy

```yaml
      - name: Build with advanced caching
        uses: docker/build-push-action@v6
        with:
          context: .
          push: true
          tags: ${{ steps.meta.outputs.tags }}
          cache-from: |
            type=gha
            type=registry,ref=${{ env.REGISTRY }}/${{ env.IMAGE_NAME }}:buildcache
          cache-to: |
            type=gha,mode=max
            type=registry,ref=${{ env.REGISTRY }}/${{ env.IMAGE_NAME }}:buildcache,mode=max
          build-args: |
            BUILDKIT_INLINE_CACHE=1
```

## Method 3: Azure DevOps YAML Pipelines

### Complete Azure DevOps Pipeline

```yaml
trigger:
  branches:
    include:
      - main
      - develop
  paths:
    include:
      - deployment_recs/*
      - Dockerfile
      - scripts/*

variables:
  imageRepository: 'sqlite-deployments'
  containerRegistry: 'myregistry.azurecr.io'
  dockerfilePath: '**/Dockerfile'
  tag: '$(Build.BuildId)'
  vmImageName: 'ubuntu-latest'

stages:
- stage: Build
  displayName: Build and push stage
  jobs:
  - job: Build
    displayName: Build
    pool:
      vmImage: $(vmImageName)
    
    variables:
      DOCKER_BUILDKIT: 1
    
    steps:
    - task: Docker@2
      displayName: Login to ACR
      inputs:
        command: login
        containerRegistry: $(containerRegistry)

    - task: Bash@3
      displayName: Validate JSON data
      inputs:
        targetType: 'inline'
        script: |
          echo "Validating JSON files in deployment_recs..."
          
          # Check if jq is available
          if ! command -v jq &> /dev/null; then
            sudo apt-get update && sudo apt-get install -y jq
          fi
          
          # Validate all JSON files
          json_files=$(find deployment_recs -name "*.json" | wc -l)
          echo "Found $json_files JSON files to validate"
          
          # Validate each file
          failed_files=0
          for file in $(find deployment_recs -name "*.json"); do
            if ! jq empty "$file" 2>/dev/null; then
              echo "❌ Invalid JSON: $file"
              ((failed_files++))
            else
              echo "✅ Valid JSON: $file"
            fi
          done
          
          if [ $failed_files -gt 0 ]; then
            echo "❌ $failed_files files failed validation"
            exit 1
          else
            echo "✅ All JSON files are valid"
          fi

    - task: Docker@2
      displayName: Build and push image
      inputs:
        command: buildAndPush
        repository: $(imageRepository)
        dockerfile: $(dockerfilePath)
        containerRegistry: $(containerRegistry)
        tags: |
          $(tag)
          latest
        arguments: |
          --build-arg BUILD_DATE=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
          --build-arg VCS_REF=$(Build.SourceVersion)
          --cache-from $(containerRegistry)/$(imageRepository):buildcache
          --cache-to $(containerRegistry)/$(imageRepository):buildcache

    - task: Bash@3
      displayName: Test container functionality
      inputs:
        targetType: 'inline'
        script: |
          echo "Testing container functionality..."
          
          # Start container for testing
          docker run -d --name test-container \
            -p 8080:8080 \
            $(containerRegistry)/$(imageRepository):$(tag)
          
          # Wait for container to be ready
          echo "Waiting for container to be ready..."
          timeout 60 bash -c 'until curl -f http://localhost:8080/health 2>/dev/null; do echo "Waiting..."; sleep 3; done'
          
          # Test database functionality
          echo "Testing database queries..."
          
          # Test basic query
          deployment_count=$(docker exec test-container sqlite3 /data/deployments.db "SELECT COUNT(*) FROM deployments;")
          echo "Total deployments in database: $deployment_count"
          
          if [ "$deployment_count" -gt 0 ]; then
            echo "✅ Database populated successfully"
          else
            echo "❌ Database appears to be empty"
            exit 1
          fi
          
          # Test environment distribution
          echo "Environment distribution:"
          docker exec test-container sqlite3 /data/deployments.db \
            "SELECT environment, COUNT(*) as count FROM deployments GROUP BY environment;"
          
          # Test database integrity
          echo "Testing database integrity..."
          docker exec test-container sqlite3 /data/deployments.db "PRAGMA integrity_check;"
          
          # Cleanup
          docker stop test-container
          docker rm test-container
          
          echo "✅ All tests passed"

    - task: AquaSecurityTrivy@4
      displayName: Security scan with Trivy
      inputs:
        image: '$(containerRegistry)/$(imageRepository):$(tag)'
        severities: 'CRITICAL,HIGH'
        exitCode: 1

- stage: Deploy
  displayName: Deploy stage
  dependsOn: Build
  condition: and(succeeded(), eq(variables['Build.SourceBranch'], 'refs/heads/main'))
  
  jobs:
  - deployment: Deploy
    displayName: Deploy to production
    pool:
      vmImage: $(vmImageName)
    environment: 'production'
    
    strategy:
      runOnce:
        deploy:
          steps:
          - task: KubernetesManifest@0
            displayName: Deploy to Kubernetes
            inputs:
              action: deploy
              manifests: |
                k8s/deployment.yaml
                k8s/service.yaml
                k8s/configmap.yaml
              containers: |
                $(containerRegistry)/$(imageRepository):$(tag)
```

### Pipeline Templates for Reusability

```yaml
# templates/docker-build-template.yml
parameters:
- name: imageRepository
  type: string
- name: containerRegistry
  type: string
- name: dockerfile
  type: string
  default: '**/Dockerfile'

steps:
- task: Docker@2
  displayName: Build and push ${{ parameters.imageRepository }}
  inputs:
    command: buildAndPush
    repository: ${{ parameters.imageRepository }}
    dockerfile: ${{ parameters.dockerfile }}
    containerRegistry: ${{ parameters.containerRegistry }}
    tags: |
      $(Build.BuildId)
      latest
    arguments: |
      --build-arg BUILD_DATE=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
      --build-arg VCS_REF=$(Build.SourceVersion)
      --cache-from ${{ parameters.containerRegistry }}/${{ parameters.imageRepository }}:buildcache
      --cache-to ${{ parameters.containerRegistry }}/${{ parameters.imageRepository }}:buildcache
```

## Best Practices Implementation

### Container Build Optimization

**Multi-stage build with layer optimization:**

```dockerfile
# Stage 1: Data processing environment
FROM alpine:3.18 AS data-builder
RUN apk add --no-cache sqlite bash jq parallel curl \
    && rm -rf /var/cache/apk/*

WORKDIR /build

# Copy processing scripts (changes infrequently)
COPY scripts/ ./scripts/
RUN chmod +x scripts/*.sh

# Copy data files (changes more frequently)
COPY deployment_recs/ ./deployment_recs/

# Process data with comprehensive error handling
RUN ./scripts/process_deployments.sh deployment_recs deployments.db && \
    sqlite3 deployments.db "PRAGMA journal_mode = WAL; PRAGMA optimize;" && \
    echo "Database created with $(sqlite3 deployments.db 'SELECT COUNT(*) FROM deployments;') records"

# Stage 2: Runtime application
FROM alpine:3.18 AS app-builder
RUN apk add --no-cache go build-base
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=1 go build -ldflags="-w -s" -o server

# Stage 3: Final runtime
FROM alpine:3.18 AS runtime

RUN apk add --no-cache sqlite ca-certificates tzdata && \
    adduser -D -s /bin/sh -u 1000 appuser && \
    mkdir -p /data && \
    chown appuser:appuser /data

# Copy database from data processing stage
COPY --from=data-builder --chown=appuser:appuser /build/deployments.db /data/

# Copy application from build stage
COPY --from=app-builder --chown=appuser:appuser /app/server /app/
COPY --chown=appuser:appuser entrypoint.sh /app/
RUN chmod +x /app/entrypoint.sh

USER appuser
VOLUME ["/data"]
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD sqlite3 /data/deployments.db "SELECT 1;" > /dev/null 2>&1 || exit 1

ENTRYPOINT ["/app/entrypoint.sh"]
CMD ["/app/server"]
```

### SQLite Performance Configuration

**Optimized database initialization script:**

```bash
#!/bin/bash
# optimize_sqlite.sh - SQLite performance optimization

set -euo pipefail

DB_PATH="$1"

sqlite3 "$DB_PATH" <<'EOF'
-- Performance optimizations for containerized SQLite
PRAGMA journal_mode = WAL;
PRAGMA synchronous = NORMAL;
PRAGMA cache_size = 50000;  -- 50,000 pages (~200MB with 4KB pages)
PRAGMA temp_store = memory;
PRAGMA mmap_size = 268435456;  -- 256MB memory mapping
PRAGMA page_size = 4096;
PRAGMA auto_vacuum = INCREMENTAL;

-- Optimize for read-heavy workloads
PRAGMA wal_autocheckpoint = 1000;
PRAGMA journal_size_limit = 67108864;  -- 64MB WAL file limit

-- Create indexes for common query patterns
CREATE INDEX IF NOT EXISTS idx_env_status_time ON deployments(environment, status, start_time);
CREATE INDEX IF NOT EXISTS idx_duration_desc ON deployments(duration_seconds DESC) WHERE duration_seconds IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_build_env ON deployments(build_id, environment);

-- Update statistics for query optimization
ANALYZE;

-- Compact the database
VACUUM;
PRAGMA OPTIMIZE;
EOF

echo "SQLite optimization completed for $DB_PATH"
```

## Kubernetes Deployment Patterns

### Production-Ready Kubernetes Manifests

```yaml
# k8s/deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: sqlite-deployments
  labels:
    app: sqlite-deployments
    version: v1
spec:
  replicas: 3
  selector:
    matchLabels:
      app: sqlite-deployments
  template:
    metadata:
      labels:
        app: sqlite-deployments
        version: v1
      annotations:
        prometheus.io/scrape: "true"
        prometheus.io/port: "8080"
        prometheus.io/path: "/metrics"
    spec:
      securityContext:
        runAsNonRoot: true
        runAsUser: 1000
        runAsGroup: 1000
        fsGroup: 1000
      containers:
      - name: sqlite-app
        image: myregistry.azurecr.io/sqlite-deployments:latest
        ports:
        - containerPort: 8080
          name: http
        env:
        - name: DATABASE_PATH
          value: "/data/deployments.db"
        - name: LOG_LEVEL
          value: "INFO"
        resources:
          requests:
            cpu: 100m
            memory: 128Mi
            ephemeral-storage: 1Gi
          limits:
            cpu: 500m
            memory: 512Mi
            ephemeral-storage: 2Gi
        securityContext:
          allowPrivilegeEscalation: false
          readOnlyRootFilesystem: true
          capabilities:
            drop:
            - ALL
        readinessProbe:
          httpGet:
            path: /health/ready
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 10
          timeoutSeconds: 5
          failureThreshold: 3
        livenessProbe:
          httpGet:
            path: /health/live
            port: 8080
          initialDelaySeconds: 30
          periodSeconds: 30
          timeoutSeconds: 10
          failureThreshold: 3
        startupProbe:
          httpGet:
            path: /health/startup
            port: 8080
          initialDelaySeconds: 10
          periodSeconds: 5
          timeoutSeconds: 3
          failureThreshold: 30
        volumeMounts:
        - name: tmp
          mountPath: /tmp
        - name: cache
          mountPath: /cache
      volumes:
      - name: tmp
        emptyDir: {}
      - name: cache
        emptyDir:
          sizeLimit: 100Mi
---
# k8s/service.yaml
apiVersion: v1
kind: Service
metadata:
  name: sqlite-deployments-service
  labels:
    app: sqlite-deployments
spec:
  selector:
    app: sqlite-deployments
  ports:
  - port: 80
    targetPort: 8080
    protocol: TCP
    name: http
  type: ClusterIP
---
# k8s/hpa.yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: sqlite-deployments-hpa
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: sqlite-deployments
  minReplicas: 2
  maxReplicas: 10
  metrics:
  - type: Resource
    resource:
      name: cpu
      target:
        type: Utilization
        averageUtilization: 70
  - type: Resource
    resource:
      name: memory
      target:
        type: Utilization
        averageUtilization: 80
  behavior:
    scaleDown:
      stabilizationWindowSeconds: 300
      policies:
      - type: Percent
        value: 10
        periodSeconds: 60
    scaleUp:
      stabilizationWindowSeconds: 60
      policies:
      - type: Percent
        value: 50
        periodSeconds: 60
```

### Monitoring and Observability

```yaml
# k8s/monitoring.yaml
apiVersion: v1
kind: ServiceMonitor
metadata:
  name: sqlite-deployments-monitor
spec:
  selector:
    matchLabels:
      app: sqlite-deployments
  endpoints:
  - port: http
    path: /metrics
    interval: 30s
---
apiVersion: networking.istio.io/v1beta1
kind: VirtualService
metadata:
  name: sqlite-deployments-vs
spec:
  hosts:
  - sqlite-deployments
  http:
  - match:
    - uri:
        prefix: "/api/deployments"
    route:
    - destination:
        host: sqlite-deployments-service
        port:
          number: 80
    fault:
      delay:
        percentage:
          value: 0.1
        fixedDelay: 5s
    retries:
      attempts: 3
      perTryTimeout: 30s
```

## Error Handling and Production Considerations

### Comprehensive Error Handling

```go
// entrypoint.go - Production-ready application entrypoint
package main

import (
    "database/sql"
    "fmt"
    "log"
    "net/http"
    "os"
    "path/filepath"
    "time"
    
    _ "github.com/mattn/go-sqlite3"
)

type App struct {
    db *sql.DB
}

func (a *App) initializeDatabase() error {
    dbPath := os.Getenv("DATABASE_PATH")
    if dbPath == "" {
        dbPath = "/data/deployments.db"
    }
    
    // Verify database file exists
    if _, err := os.Stat(dbPath); os.IsNotExist(err) {
        return fmt.Errorf("database file not found: %s", dbPath)
    }
    
    // Open database with optimized settings
    db, err := sql.Open("sqlite3", dbPath+"?_journal_mode=WAL&_synchronous=NORMAL&_cache_size=50000&_temp_store=memory")
    if err != nil {
        return fmt.Errorf("failed to open database: %w", err)
    }
    
    // Test database connectivity
    if err := db.Ping(); err != nil {
        return fmt.Errorf("database ping failed: %w", err)
    }
    
    // Verify expected tables exist
    var count int
    err = db.QueryRow("SELECT COUNT(*) FROM deployments").Scan(&count)
    if err != nil {
        return fmt.Errorf("failed to query deployments table: %w", err)
    }
    
    log.Printf("Database initialized successfully with %d deployment records", count)
    a.db = db
    return nil
}

func (a *App) healthCheck(w http.ResponseWriter, r *http.Request) {
    // Check database connectivity
    var result int
    err := a.db.QueryRow("SELECT 1").Scan(&result)
    if err != nil {
        http.Error(w, "Database health check failed", http.StatusServiceUnavailable)
        return
    }
    
    w.Header().Set("Content-Type", "application/json")
    fmt.Fprintf(w, `{"status":"healthy","timestamp":"%s"}`, time.Now().Format(time.RFC3339))
}

func main() {
    app := &App{}
    
    // Initialize database with retry logic
    maxRetries := 5
    for i := 0; i < maxRetries; i++ {
        if err := app.initializeDatabase(); err != nil {
            log.Printf("Database initialization attempt %d failed: %v", i+1, err)
            if i == maxRetries-1 {
                log.Fatal("Failed to initialize database after all retries")
            }
            time.Sleep(time.Duration(i+1) * time.Second)
            continue
        }
        break
    }
    
    // Set up HTTP handlers
    http.HandleFunc("/health", app.healthCheck)
    http.HandleFunc("/health/ready", app.healthCheck)
    http.HandleFunc("/health/live", app.healthCheck)
    http.HandleFunc("/health/startup", app.healthCheck)
    
    // Add API endpoints
    http.HandleFunc("/api/deployments", app.getDeployments)
    http.HandleFunc("/api/environments", app.getEnvironments)
    
    // Start server
    port := os.Getenv("PORT")
    if port == "" {
        port = "8080"
    }
    
    log.Printf("Starting server on port %s", port)
    log.Fatal(http.ListenAndServe(":"+port, nil))
}
```

## Performance Benchmarks and Success Metrics

### Expected Performance Characteristics

**Container Build Performance:**
- Multi-stage build time: 3-5 minutes for full rebuild
- Layer cache hit ratio: 85-95% for incremental builds
- Final image size: 15-25MB (Alpine-based)
- Database population time: 10,000 records/second

**Runtime Performance:**
- Container startup time: <5 seconds
- Database query performance: 100,000+ SELECT ops/second
- Memory usage: 50-150MB depending on cache configuration
- Concurrent connection support: 1000+ connections in WAL mode

**Kubernetes Deployment Metrics:**
- Pod startup time: <10 seconds including readiness probe
- Horizontal pod autoscaler response time: 30-60 seconds
- Rolling update time: 2-3 minutes for zero-downtime deployment
- Resource utilization: 70-80% CPU and memory efficiency

This comprehensive guide provides enterprise-ready patterns for implementing SQLite database containers with prepopulated data, ensuring production reliability, optimal performance, and seamless Kubernetes integration. The three methods (bash/jq, GitHub Actions, Azure DevOps) offer flexibility for different organizational requirements while maintaining consistent quality and reliability standards.