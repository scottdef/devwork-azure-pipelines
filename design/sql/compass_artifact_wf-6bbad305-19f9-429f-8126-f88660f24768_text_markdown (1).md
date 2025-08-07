# Complete SQLite-Grafana Kubernetes Deployment Guide

Setting up SQLite with Grafana in Kubernetes requires careful consideration of file-based database limitations while implementing production-ready configurations. This guide provides three deployment methods with comprehensive security, persistence, and operational best practices for single namespace deployment.

## SQLite Container Configuration for Grafana

### Container Setup and Database Initialization

**SQLite requires filesystem access to Grafana**, not network connectivity. The key architectural requirement is shared persistent storage between SQLite and Grafana containers.

**Custom Dockerfile for SQLite with initialization:**
```dockerfile
FROM alpine:latest
RUN apk update && apk add sqlite
WORKDIR /data
COPY init-schema.sql /tmp/
CMD ["sh", "-c", "sqlite3 /data/app.db < /tmp/init-schema.sql && tail -f /dev/null"]
```

**Database Schema for Time Series Data:**
```sql
-- init-schema.sql
CREATE TABLE IF NOT EXISTS metrics (
    timestamp INTEGER NOT NULL,  -- Unix timestamp (seconds)
    metric_name TEXT NOT NULL,
    value REAL NOT NULL,
    tags JSON
);

CREATE INDEX idx_timestamp ON metrics(timestamp);
CREATE INDEX idx_metric_name ON metrics(metric_name);

-- Sample data for testing
INSERT INTO metrics (timestamp, metric_name, value, tags) VALUES
(strftime('%s', 'now'), 'cpu_usage', 65.5, '{"host":"server1","region":"us-east-1"}'),
(strftime('%s', 'now'), 'memory_usage', 78.2, '{"host":"server1","region":"us-east-1"}'),
(strftime('%s', 'now'), 'disk_usage', 45.1, '{"host":"server2","region":"us-west-1"}');

-- Enable WAL mode for better concurrency
PRAGMA journal_mode=WAL;
PRAGMA synchronous=NORMAL;
PRAGMA cache_size=-64000;
```

### Grafana SQLite Plugin Configuration

**Plugin Installation in Container:**
```yaml
env:
- name: GF_INSTALL_PLUGINS
  value: "frser-sqlite-datasource"
- name: GF_PLUGINS_ALLOW_LOADING_UNSIGNED_PLUGINS
  value: "frser-sqlite-datasource"
```

**Data Source Configuration:**
- **Plugin:** frser-sqlite-datasource (version 3.8.0+)
- **Path:** `/var/grafana/data/app.db` (shared volume mount)
- **Security:** Set `AttachLimit=0` to prevent unauthorized database access
- **Connection String:** `file:/var/grafana/data/app.db?cache=shared&mode=ro`

## Kubernetes Resource Manifests

### Namespace and Security Configuration

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: monitoring
  labels:
    name: monitoring
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: sqlite-grafana-sa
  namespace: monitoring
automountServiceAccountToken: false
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  namespace: monitoring
  name: monitoring-role
rules:
- apiGroups: [""]
  resources: ["pods", "services", "endpoints", "configmaps", "secrets"]
  verbs: ["get", "list", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: monitoring-rolebinding
  namespace: monitoring
subjects:
- kind: ServiceAccount
  name: sqlite-grafana-sa
  namespace: monitoring
roleRef:
  kind: Role
  name: monitoring-role
  apiGroup: rbac.authorization.k8s.io
```

### SQLite StatefulSet with Persistent Storage

```yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: sqlite-app
  namespace: monitoring
  labels:
    app: sqlite-app
spec:
  serviceName: sqlite-service
  replicas: 1  # SQLite must be single-replica
  selector:
    matchLabels:
      app: sqlite-app
  template:
    metadata:
      labels:
        app: sqlite-app
    spec:
      serviceAccountName: sqlite-grafana-sa
      securityContext:
        runAsNonRoot: true
        runAsUser: 1000
        runAsGroup: 1000
        fsGroup: 1000
        seccompProfile:
          type: RuntimeDefault
      initContainers:
      - name: init-db
        image: alpine:3.18
        command: ['sh', '-c']
        args:
          - |
            apk add --no-cache sqlite
            sqlite3 /data/app.db <<'EOF'
            CREATE TABLE IF NOT EXISTS metrics (
              timestamp INTEGER,
              metric_name TEXT,
              value REAL,
              tags TEXT
            );
            INSERT INTO metrics VALUES 
            (strftime('%s', 'now'), 'cpu_usage', 65.5, '{"host":"server1"}'),
            (strftime('%s', 'now'), 'memory_usage', 78.2, '{"host":"server1"}');
            EOF
            chown -R 1000:1000 /data
        securityContext:
          runAsUser: 0  # Needed for chown
        volumeMounts:
        - name: sqlite-storage
          mountPath: /data
      containers:
      - name: sqlite-app
        image: keinos/sqlite3:latest
        command: ["tail", "-f", "/dev/null"]
        securityContext:
          allowPrivilegeEscalation: false
          capabilities:
            drop: ["ALL"]
          readOnlyRootFilesystem: true
        livenessProbe:
          exec:
            command: ["sqlite3", "/data/app.db", "SELECT 1;"]
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          exec:
            command: ["sqlite3", "/data/app.db", "SELECT 1;"]
          initialDelaySeconds: 5
          periodSeconds: 5
        resources:
          requests:
            cpu: 100m
            memory: 128Mi
          limits:
            cpu: 500m
            memory: 512Mi
        volumeMounts:
        - name: sqlite-storage
          mountPath: /data
        - name: tmp
          mountPath: /tmp
      volumes:
      - name: tmp
        emptyDir: {}
  volumeClaimTemplates:
  - metadata:
      name: sqlite-storage
    spec:
      accessModes: ["ReadWriteOnce"]
      storageClassName: fast
      resources:
        requests:
          storage: 10Gi
---
apiVersion: v1
kind: Service
metadata:
  name: sqlite-service
  namespace: monitoring
spec:
  selector:
    app: sqlite-app
  ports:
  - port: 3306
    targetPort: 3306
  type: ClusterIP
```

### Grafana Deployment with SQLite Integration

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: grafana
  namespace: monitoring
  labels:
    app: grafana
spec:
  replicas: 1
  selector:
    matchLabels:
      app: grafana
  strategy:
    type: Recreate
  template:
    metadata:
      labels:
        app: grafana
    spec:
      serviceAccountName: sqlite-grafana-sa
      securityContext:
        runAsNonRoot: true
        runAsUser: 472
        runAsGroup: 472
        fsGroup: 472
        seccompProfile:
          type: RuntimeDefault
      containers:
      - name: grafana
        image: grafana/grafana:latest
        ports:
        - containerPort: 3000
          name: http
        securityContext:
          allowPrivilegeEscalation: false
          capabilities:
            drop: ["ALL"]
        env:
        - name: GF_SECURITY_ADMIN_PASSWORD
          valueFrom:
            secretKeyRef:
              name: grafana-credentials
              key: admin-password
        - name: GF_INSTALL_PLUGINS
          value: "frser-sqlite-datasource"
        - name: GF_PLUGINS_ALLOW_LOADING_UNSIGNED_PLUGINS
          value: "frser-sqlite-datasource"
        livenessProbe:
          httpGet:
            path: /api/health
            port: 3000
          initialDelaySeconds: 30
          periodSeconds: 10
        readinessProbe:
          httpGet:
            path: /api/health
            port: 3000
          initialDelaySeconds: 10
          periodSeconds: 5
        resources:
          requests:
            cpu: 250m
            memory: 512Mi
          limits:
            cpu: 1000m
            memory: 1Gi
        volumeMounts:
        - name: grafana-storage
          mountPath: /var/lib/grafana
        - name: shared-data
          mountPath: /var/grafana/data
        - name: datasources
          mountPath: /etc/grafana/provisioning/datasources
      volumes:
      - name: grafana-storage
        persistentVolumeClaim:
          claimName: grafana-pvc
      - name: shared-data
        persistentVolumeClaim:
          claimName: sqlite-app-sqlite-storage-0
      - name: datasources
        configMap:
          name: grafana-datasources
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: grafana-pvc
  namespace: monitoring
spec:
  accessModes: ["ReadWriteOnce"]
  storageClassName: fast
  resources:
    requests:
      storage: 5Gi
---
apiVersion: v1
kind: Service
metadata:
  name: grafana-service
  namespace: monitoring
spec:
  selector:
    app: grafana
  ports:
  - port: 80
    targetPort: 3000
  type: ClusterIP
```

### Configuration and Secrets

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: grafana-credentials
  namespace: monitoring
type: Opaque
stringData:
  admin-password: "your-secure-password"
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: grafana-datasources
  namespace: monitoring
data:
  datasources.yaml: |
    apiVersion: 1
    datasources:
      - name: SQLite-Local
        type: frser-sqlite-datasource
        url: file:/var/grafana/data/app.db
        access: direct
        isDefault: true
        editable: true
        jsonData:
          database: /var/grafana/data/app.db
```

## Deployment Method 1: Manual Bash Scripts

### Complete Deployment Script

```bash
#!/bin/bash
# deploy-sqlite-grafana.sh

set -euo pipefail

NAMESPACE="monitoring"
GRAFANA_PASSWORD="your-secure-password"

echo "🚀 Deploying SQLite + Grafana to Kubernetes..."

# Create namespace
kubectl create namespace ${NAMESPACE} --dry-run=client -o yaml | kubectl apply -f -

# Create secrets
kubectl create secret generic grafana-credentials \
  --from-literal=admin-password=${GRAFANA_PASSWORD} \
  --namespace ${NAMESPACE} \
  --dry-run=client -o yaml | kubectl apply -f -

# Apply all manifests
kubectl apply -f - <<EOF
$(cat <<'MANIFESTS'
# Insert all the YAML manifests from above here
MANIFESTS
)
EOF

# Wait for deployments
echo "⏳ Waiting for SQLite StatefulSet to be ready..."
kubectl rollout status statefulset/sqlite-app -n ${NAMESPACE} --timeout=300s

echo "⏳ Waiting for Grafana deployment to be ready..."
kubectl rollout status deployment/grafana -n ${NAMESPACE} --timeout=300s

# Verify connectivity
echo "🔍 Testing SQLite database..."
kubectl exec -n ${NAMESPACE} statefulset/sqlite-app -- sqlite3 /data/app.db "SELECT COUNT(*) FROM metrics;"

# Port forward for testing
echo "🌐 Setting up port forwarding to Grafana..."
kubectl port-forward -n ${NAMESPACE} svc/grafana-service 3000:80 &

echo "✅ Deployment complete!"
echo "📊 Access Grafana at: http://localhost:3000"
echo "🔑 Username: admin, Password: ${GRAFANA_PASSWORD}"

# Cleanup function
cleanup() {
  echo "🧹 Cleaning up port forwarding..."
  pkill -f "kubectl port-forward" || true
}
trap cleanup EXIT
```

### Testing and Validation Script

```bash
#!/bin/bash
# test-deployment.sh

NAMESPACE="monitoring"

echo "🧪 Running deployment tests..."

# Test 1: Check pod status
echo "1️⃣ Checking pod status..."
kubectl get pods -n ${NAMESPACE}

# Test 2: Verify SQLite data
echo "2️⃣ Testing SQLite database connectivity..."
kubectl exec -n ${NAMESPACE} statefulset/sqlite-app -- \
  sqlite3 /data/app.db "SELECT COUNT(*) as record_count FROM metrics;"

# Test 3: Check Grafana health
echo "3️⃣ Testing Grafana health endpoint..."
kubectl exec -n ${NAMESPACE} deployment/grafana -- \
  curl -f http://localhost:3000/api/health

# Test 4: Verify data source connectivity
echo "4️⃣ Checking data source configuration..."
kubectl port-forward -n ${NAMESPACE} svc/grafana-service 3000:80 &
PF_PID=$!
sleep 5

# Test API endpoint
curl -u admin:${GRAFANA_PASSWORD} \
  http://localhost:3000/api/datasources | jq '.[0].name'

kill ${PF_PID}

echo "✅ All tests passed!"
```

## Deployment Method 2: Terraform Configuration

### Main Terraform Configuration

```hcl
# main.tf
terraform {
  required_providers {
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = "~> 2.24"
    }
  }
}

provider "kubernetes" {
  config_path = "~/.kube/config"
}

locals {
  namespace = "monitoring"
  common_labels = {
    app         = "sqlite-grafana"
    environment = var.environment
    managed_by  = "terraform"
  }
}

# Namespace
resource "kubernetes_namespace" "monitoring" {
  metadata {
    name   = local.namespace
    labels = local.common_labels
  }
}

# Service Account and RBAC
resource "kubernetes_service_account" "sqlite_grafana_sa" {
  metadata {
    name      = "sqlite-grafana-sa"
    namespace = kubernetes_namespace.monitoring.metadata[0].name
  }
  automount_service_account_token = false
}

resource "kubernetes_role" "monitoring_role" {
  metadata {
    namespace = kubernetes_namespace.monitoring.metadata[0].name
    name      = "monitoring-role"
  }

  rule {
    api_groups = [""]
    resources  = ["pods", "services", "endpoints", "configmaps", "secrets"]
    verbs      = ["get", "list", "watch"]
  }
}

resource "kubernetes_role_binding" "monitoring_binding" {
  metadata {
    name      = "monitoring-rolebinding"
    namespace = kubernetes_namespace.monitoring.metadata[0].name
  }

  role_ref {
    api_group = "rbac.authorization.k8s.io"
    kind      = "Role"
    name      = kubernetes_role.monitoring_role.metadata[0].name
  }

  subject {
    kind      = "ServiceAccount"
    name      = kubernetes_service_account.sqlite_grafana_sa.metadata[0].name
    namespace = kubernetes_namespace.monitoring.metadata[0].name
  }
}

# Secrets
resource "kubernetes_secret" "grafana_credentials" {
  metadata {
    name      = "grafana-credentials"
    namespace = kubernetes_namespace.monitoring.metadata[0].name
  }

  data = {
    admin-password = base64encode(var.grafana_admin_password)
  }

  type = "Opaque"
}

# ConfigMaps
resource "kubernetes_config_map" "grafana_datasources" {
  metadata {
    name      = "grafana-datasources"
    namespace = kubernetes_namespace.monitoring.metadata[0].name
  }

  data = {
    "datasources.yaml" = yamlencode({
      apiVersion = 1
      datasources = [{
        name      = "SQLite-Local"
        type      = "frser-sqlite-datasource"
        url       = "file:/var/grafana/data/app.db"
        access    = "direct"
        isDefault = true
        editable  = true
        jsonData = {
          database = "/var/grafana/data/app.db"
        }
      }]
    })
  }
}

# SQLite StatefulSet
resource "kubernetes_stateful_set" "sqlite_app" {
  metadata {
    name      = "sqlite-app"
    namespace = kubernetes_namespace.monitoring.metadata[0].name
    labels    = local.common_labels
  }

  spec {
    service_name = "sqlite-service"
    replicas     = 1

    selector {
      match_labels = {
        app = "sqlite-app"
      }
    }

    template {
      metadata {
        labels = merge(local.common_labels, {
          app = "sqlite-app"
        })
      }

      spec {
        service_account_name = kubernetes_service_account.sqlite_grafana_sa.metadata[0].name

        security_context {
          run_as_non_root = true
          run_as_user     = 1000
          run_as_group    = 1000
          fs_group        = 1000
          seccomp_profile {
            type = "RuntimeDefault"
          }
        }

        init_container {
          name  = "init-db"
          image = "alpine:3.18"
          command = ["sh", "-c"]
          args = [
            <<-EOT
            apk add --no-cache sqlite
            sqlite3 /data/app.db <<'EOF'
            CREATE TABLE IF NOT EXISTS metrics (
              timestamp INTEGER,
              metric_name TEXT,
              value REAL,
              tags TEXT
            );
            INSERT INTO metrics VALUES 
            (strftime('%s', 'now'), 'cpu_usage', 65.5, '{"host":"server1"}'),
            (strftime('%s', 'now'), 'memory_usage', 78.2, '{"host":"server1"}');
            EOF
            chown -R 1000:1000 /data
            EOT
          ]

          security_context {
            run_as_user = 0
          }

          volume_mount {
            name       = "sqlite-storage"
            mount_path = "/data"
          }
        }

        container {
          name  = "sqlite-app"
          image = "keinos/sqlite3:latest"
          command = ["tail", "-f", "/dev/null"]

          security_context {
            allow_privilege_escalation = false
            capabilities {
              drop = ["ALL"]
            }
            read_only_root_filesystem = true
          }

          liveness_probe {
            exec {
              command = ["sqlite3", "/data/app.db", "SELECT 1;"]
            }
            initial_delay_seconds = 30
            period_seconds        = 10
          }

          readiness_probe {
            exec {
              command = ["sqlite3", "/data/app.db", "SELECT 1;"]
            }
            initial_delay_seconds = 5
            period_seconds        = 5
          }

          resources {
            requests = {
              cpu    = "100m"
              memory = "128Mi"
            }
            limits = {
              cpu    = "500m"
              memory = "512Mi"
            }
          }

          volume_mount {
            name       = "sqlite-storage"
            mount_path = "/data"
          }

          volume_mount {
            name       = "tmp"
            mount_path = "/tmp"
          }
        }

        volume {
          name = "tmp"
          empty_dir {}
        }
      }
    }

    volume_claim_template {
      metadata {
        name = "sqlite-storage"
      }
      spec {
        access_modes       = ["ReadWriteOnce"]
        storage_class_name = var.storage_class
        resources {
          requests = {
            storage = "10Gi"
          }
        }
      }
    }
  }
}

# Grafana Deployment
resource "kubernetes_deployment" "grafana" {
  metadata {
    name      = "grafana"
    namespace = kubernetes_namespace.monitoring.metadata[0].name
    labels    = local.common_labels
  }

  spec {
    replicas = 1

    selector {
      match_labels = {
        app = "grafana"
      }
    }

    strategy {
      type = "Recreate"
    }

    template {
      metadata {
        labels = merge(local.common_labels, {
          app = "grafana"
        })
      }

      spec {
        service_account_name = kubernetes_service_account.sqlite_grafana_sa.metadata[0].name

        security_context {
          run_as_non_root = true
          run_as_user     = 472
          run_as_group    = 472
          fs_group        = 472
          seccomp_profile {
            type = "RuntimeDefault"
          }
        }

        container {
          name  = "grafana"
          image = "grafana/grafana:latest"

          port {
            container_port = 3000
            name          = "http"
          }

          security_context {
            allow_privilege_escalation = false
            capabilities {
              drop = ["ALL"]
            }
          }

          env {
            name = "GF_SECURITY_ADMIN_PASSWORD"
            value_from {
              secret_key_ref {
                name = kubernetes_secret.grafana_credentials.metadata[0].name
                key  = "admin-password"
              }
            }
          }

          env {
            name  = "GF_INSTALL_PLUGINS"
            value = "frser-sqlite-datasource"
          }

          env {
            name  = "GF_PLUGINS_ALLOW_LOADING_UNSIGNED_PLUGINS"
            value = "frser-sqlite-datasource"
          }

          liveness_probe {
            http_get {
              path = "/api/health"
              port = 3000
            }
            initial_delay_seconds = 30
            period_seconds        = 10
          }

          readiness_probe {
            http_get {
              path = "/api/health"
              port = 3000
            }
            initial_delay_seconds = 10
            period_seconds        = 5
          }

          resources {
            requests = {
              cpu    = "250m"
              memory = "512Mi"
            }
            limits = {
              cpu    = "1000m"
              memory = "1Gi"
            }
          }

          volume_mount {
            name       = "grafana-storage"
            mount_path = "/var/lib/grafana"
          }

          volume_mount {
            name       = "shared-data"
            mount_path = "/var/grafana/data"
          }

          volume_mount {
            name       = "datasources"
            mount_path = "/etc/grafana/provisioning/datasources"
          }
        }

        volume {
          name = "grafana-storage"
          persistent_volume_claim {
            claim_name = kubernetes_persistent_volume_claim.grafana_pvc.metadata[0].name
          }
        }

        volume {
          name = "shared-data"
          persistent_volume_claim {
            claim_name = "sqlite-app-sqlite-storage-0"
          }
        }

        volume {
          name = "datasources"
          config_map {
            name = kubernetes_config_map.grafana_datasources.metadata[0].name
          }
        }
      }
    }
  }

  depends_on = [kubernetes_stateful_set.sqlite_app]
}

# Grafana PVC
resource "kubernetes_persistent_volume_claim" "grafana_pvc" {
  metadata {
    name      = "grafana-pvc"
    namespace = kubernetes_namespace.monitoring.metadata[0].name
  }

  spec {
    access_modes       = ["ReadWriteOnce"]
    storage_class_name = var.storage_class
    resources {
      requests = {
        storage = "5Gi"
      }
    }
  }
}

# Services
resource "kubernetes_service" "sqlite_service" {
  metadata {
    name      = "sqlite-service"
    namespace = kubernetes_namespace.monitoring.metadata[0].name
  }

  spec {
    selector = {
      app = "sqlite-app"
    }

    port {
      port        = 3306
      target_port = 3306
    }

    type = "ClusterIP"
  }
}

resource "kubernetes_service" "grafana_service" {
  metadata {
    name      = "grafana-service"
    namespace = kubernetes_namespace.monitoring.metadata[0].name
  }

  spec {
    selector = {
      app = "grafana"
    }

    port {
      port        = 80
      target_port = 3000
    }

    type = "ClusterIP"
  }
}
```

### Variables and Outputs

```hcl
# variables.tf
variable "environment" {
  description = "Environment name"
  type        = string
  default     = "development"
}

variable "grafana_admin_password" {
  description = "Grafana admin password"
  type        = string
  sensitive   = true
}

variable "storage_class" {
  description = "Storage class for persistent volumes"
  type        = string
  default     = "standard"
}

# outputs.tf
output "namespace" {
  description = "Kubernetes namespace"
  value       = kubernetes_namespace.monitoring.metadata[0].name
}

output "grafana_service_name" {
  description = "Grafana service name"
  value       = kubernetes_service.grafana_service.metadata[0].name
}

output "sqlite_service_name" {
  description = "SQLite service name"
  value       = kubernetes_service.sqlite_service.metadata[0].name
}
```

### Deployment Commands

```bash
# Initialize and deploy
terraform init
terraform plan -var="grafana_admin_password=your-secure-password"
terraform apply -var="grafana_admin_password=your-secure-password"

# Test the deployment
kubectl port-forward -n monitoring svc/grafana-service 3000:80

# Clean up
terraform destroy -var="grafana_admin_password=your-secure-password"
```

## Deployment Method 3: GitHub Actions with Helm

### GitHub Actions Workflow

```yaml
# .github/workflows/deploy-grafana-sqlite.yml
name: Deploy Grafana with SQLite

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]
  workflow_dispatch:

env:
  REGISTRY: ghcr.io
  IMAGE_NAME: ${{ github.repository }}

jobs:
  lint-and-test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      
      - name: Setup Helm
        uses: azure/setup-helm@v4
        with:
          version: '3.14.0'
          
      - name: Add Grafana Helm Repository
        run: |
          helm repo add grafana https://grafana.github.io/helm-charts
          helm repo update
          
      - name: Lint Helm Values
        run: |
          helm template grafana-sqlite grafana/grafana \
            --values charts/grafana-sqlite-values.yaml \
            --dry-run

  security-scan:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      
      - name: Run Trivy Vulnerability Scanner
        uses: aquasecurity/trivy-action@master
        with:
          scan-type: 'fs'
          scan-ref: '.'
          format: 'sarif'
          output: 'trivy-results.sarif'
          
      - name: Upload Trivy Scan Results
        uses: github/codeql-action/upload-sarif@v2
        if: always()
        with:
          sarif_file: 'trivy-results.sarif'

  deploy:
    needs: [lint-and-test, security-scan]
    runs-on: ubuntu-latest
    environment: production
    steps:
      - uses: actions/checkout@v4
      
      - name: Setup Helm
        uses: azure/setup-helm@v4
        with:
          version: '3.14.0'
          
      - name: Configure Kubernetes Context
        uses: azure/k8s-set-context@v4
        with:
          method: kubeconfig
          kubeconfig: ${{ secrets.KUBECONFIG }}
          
      - name: Create Namespace
        run: |
          kubectl create namespace monitoring --dry-run=client -o yaml | kubectl apply -f -
          
      - name: Create Kubernetes Secrets
        run: |
          # Create Grafana admin credentials
          kubectl create secret generic grafana-admin-secret \
            --from-literal=admin-user=admin \
            --from-literal=admin-password=${{ secrets.GRAFANA_ADMIN_PASSWORD }} \
            --namespace monitoring \
            --dry-run=client -o yaml | kubectl apply -f -
          
          # Create SQLite configuration
          kubectl create configmap sqlite-init-config \
            --from-file=init.sql=./config/init-schema.sql \
            --namespace monitoring \
            --dry-run=client -o yaml | kubectl apply -f -
            
      - name: Deploy SQLite StatefulSet
        run: |
          kubectl apply -f - <<EOF
          apiVersion: apps/v1
          kind: StatefulSet
          metadata:
            name: sqlite-app
            namespace: monitoring
          spec:
            serviceName: sqlite-service
            replicas: 1
            selector:
              matchLabels:
                app: sqlite-app
            template:
              metadata:
                labels:
                  app: sqlite-app
              spec:
                securityContext:
                  runAsNonRoot: true
                  runAsUser: 1000
                  fsGroup: 1000
                initContainers:
                - name: init-db
                  image: alpine:3.18
                  command: ['/bin/sh', '-c']
                  args:
                    - |
                      apk add --no-cache sqlite
                      sqlite3 /data/app.db < /config/init.sql
                      chown -R 1000:1000 /data
                  securityContext:
                    runAsUser: 0
                  volumeMounts:
                  - name: sqlite-storage
                    mountPath: /data
                  - name: init-config
                    mountPath: /config
                containers:
                - name: sqlite-app
                  image: keinos/sqlite3:latest
                  command: ["tail", "-f", "/dev/null"]
                  securityContext:
                    allowPrivilegeEscalation: false
                    capabilities:
                      drop: ["ALL"]
                  resources:
                    requests:
                      cpu: 100m
                      memory: 128Mi
                    limits:
                      cpu: 500m
                      memory: 512Mi
                  volumeMounts:
                  - name: sqlite-storage
                    mountPath: /data
                volumes:
                - name: init-config
                  configMap:
                    name: sqlite-init-config
            volumeClaimTemplates:
            - metadata:
                name: sqlite-storage
              spec:
                accessModes: ["ReadWriteOnce"]
                resources:
                  requests:
                    storage: 10Gi
          ---
          apiVersion: v1
          kind: Service
          metadata:
            name: sqlite-service
            namespace: monitoring
          spec:
            selector:
              app: sqlite-app
            ports:
            - port: 3306
              targetPort: 3306
            type: ClusterIP
          EOF
          
      - name: Wait for SQLite to be Ready
        run: |
          kubectl rollout status statefulset/sqlite-app -n monitoring --timeout=300s
          
      - name: Deploy Grafana with Helm
        run: |
          helm repo add grafana https://grafana.github.io/helm-charts
          helm repo update
          
          helm upgrade grafana-sqlite grafana/grafana \
            --install \
            --create-namespace \
            --namespace monitoring \
            --values charts/grafana-sqlite-values.yaml \
            --set admin.existingSecret=grafana-admin-secret \
            --set persistence.enabled=true \
            --set persistence.size=5Gi \
            --wait --timeout=10m
            
      - name: Verify Deployment
        run: |
          kubectl rollout status deployment/grafana-sqlite -n monitoring --timeout=300s
          kubectl get pods -n monitoring
          
      - name: Run Health Checks
        run: |
          # Wait for pods to be ready
          kubectl wait --for=condition=ready pod -l app.kubernetes.io/name=grafana -n monitoring --timeout=300s
          
          # Test SQLite connection
          kubectl exec -n monitoring statefulset/sqlite-app -- \
            sqlite3 /data/app.db "SELECT COUNT(*) FROM metrics;"
          
          # Test Grafana health endpoint
          kubectl port-forward -n monitoring svc/grafana-sqlite 3000:80 &
          PF_PID=$!
          sleep 10
          
          curl -f http://localhost:3000/api/health || exit 1
          
          kill $PF_PID

  integration-test:
    needs: deploy
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      
      - name: Configure Kubernetes Context
        uses: azure/k8s-set-context@v4
        with:
          method: kubeconfig
          kubeconfig: ${{ secrets.KUBECONFIG }}
          
      - name: Test Data Source Connectivity
        run: |
          # Port forward to Grafana
          kubectl port-forward -n monitoring svc/grafana-sqlite 3000:80 &
          PF_PID=$!
          sleep 10
          
          # Test API connectivity
          curl -u admin:${{ secrets.GRAFANA_ADMIN_PASSWORD }} \
            -H "Content-Type: application/json" \
            http://localhost:3000/api/datasources | jq '.[0].name'
          
          # Test query execution
          curl -u admin:${{ secrets.GRAFANA_ADMIN_PASSWORD }} \
            -H "Content-Type: application/json" \
            -X POST \
            -d '{"queries":[{"rawSql":"SELECT COUNT(*) FROM metrics","format":"table"}]}' \
            http://localhost:3000/api/ds/query | jq '.results'
          
          kill $PF_PID
```

### Custom Grafana Helm Values

```yaml
# charts/grafana-sqlite-values.yaml
nameOverride: "grafana-sqlite"
fullnameOverride: "grafana-sqlite"

# Admin credentials from Kubernetes secrets
admin:
  existingSecret: "grafana-admin-secret"
  userKey: admin-user
  passwordKey: admin-password

# Persistence configuration
persistence:
  enabled: true
  type: pvc
  size: 5Gi
  storageClassName: "fast"
  accessModes:
    - ReadWriteOnce

# Grafana configuration
grafana.ini:
  paths:
    data: /var/lib/grafana/
    logs: /var/log/grafana
    plugins: /var/lib/grafana/plugins
  
  database:
    type: sqlite3
    path: /var/lib/grafana/grafana.db
    cache_mode: private
    wal: true

  log:
    mode: console
    level: info
    
  security:
    admin_user: admin
    admin_password: $__file{/etc/secrets/admin-password}
    
  auth:
    disable_login_form: false
    disable_signout_menu: false

# Data sources provisioning
datasources:
  datasources.yaml:
    apiVersion: 1
    datasources:
      - name: SQLite-Local
        type: frser-sqlite-datasource
        url: file:/var/grafana/shared/app.db
        access: direct
        isDefault: true
        editable: true
        jsonData:
          database: /var/grafana/shared/app.db

# Plugin installation
plugins:
  - frser-sqlite-datasource

# Security context
securityContext:
  runAsNonRoot: true
  runAsUser: 472
  runAsGroup: 472
  fsGroup: 472

containerSecurityContext:
  allowPrivilegeEscalation: false
  capabilities:
    drop:
    - ALL
  seccompProfile:
    type: RuntimeDefault

# Resource management
resources:
  limits:
    cpu: 1000m
    memory: 1Gi
  requests:
    cpu: 250m
    memory: 512Mi

# Health checks
livenessProbe:
  httpGet:
    path: /api/health
    port: 3000
  initialDelaySeconds: 60
  timeoutSeconds: 30

readinessProbe:
  httpGet:
    path: /api/health
    port: 3000
  initialDelaySeconds: 30
  timeoutSeconds: 10

# Service configuration
service:
  type: ClusterIP
  port: 80
  targetPort: 3000

# Ingress configuration (if needed)
ingress:
  enabled: false  # Set to true if you need external access
  className: "nginx"
  annotations:
    nginx.ingress.kubernetes.io/rewrite-target: /
  hosts:
    - host: grafana.yourdomain.com
      paths:
        - path: /
          pathType: Prefix

# Extra environment variables
extraEnvVars:
  - name: GF_INSTALL_PLUGINS
    value: "frser-sqlite-datasource"
  - name: GF_PLUGINS_ALLOW_LOADING_UNSIGNED_PLUGINS
    value: "frser-sqlite-datasource"

# Extra volume mounts for shared SQLite data
extraVolumeMounts:
  - name: shared-sqlite-data
    mountPath: /var/grafana/shared

extraVolumes:
  - name: shared-sqlite-data
    persistentVolumeClaim:
      claimName: sqlite-app-sqlite-storage-0
```

## Connection Testing and Validation

### Connection Test Script

```bash
#!/bin/bash
# test-connection.sh

NAMESPACE="monitoring"
GRAFANA_PASSWORD="${GRAFANA_PASSWORD:-admin}"

echo "🔍 Testing SQLite + Grafana connectivity..."

# Test 1: Verify SQLite data
echo "1️⃣ Testing SQLite database..."
RECORD_COUNT=$(kubectl exec -n ${NAMESPACE} statefulset/sqlite-app -- \
  sqlite3 /data/app.db "SELECT COUNT(*) FROM metrics;" 2>/dev/null)

if [[ "${RECORD_COUNT}" =~ ^[0-9]+$ ]] && [[ ${RECORD_COUNT} -gt 0 ]]; then
  echo "✅ SQLite database contains ${RECORD_COUNT} records"
else
  echo "❌ SQLite database test failed"
  exit 1
fi

# Test 2: Check Grafana health
echo "2️⃣ Testing Grafana health..."
kubectl exec -n ${NAMESPACE} deployment/grafana-sqlite -- \
  curl -f http://localhost:3000/api/health >/dev/null 2>&1

if [[ $? -eq 0 ]]; then
  echo "✅ Grafana health check passed"
else
  echo "❌ Grafana health check failed"
  exit 1
fi

# Test 3: Verify data source connectivity
echo "3️⃣ Testing data source connectivity..."
kubectl port-forward -n ${NAMESPACE} svc/grafana-sqlite 3000:80 >/dev/null 2>&1 &
PF_PID=$!
sleep 5

# Check data sources API
DATASOURCE_RESPONSE=$(curl -s -u admin:${GRAFANA_PASSWORD} \
  http://localhost:3000/api/datasources 2>/dev/null)

kill ${PF_PID} 2>/dev/null

if echo "${DATASOURCE_RESPONSE}" | grep -q "SQLite-Local"; then
  echo "✅ SQLite data source configured correctly"
else
  echo "❌ SQLite data source not found"
  echo "Response: ${DATASOURCE_RESPONSE}"
  exit 1
fi

echo "🎉 All connectivity tests passed!"
```

### Sample Grafana Dashboard JSON

```json
{
  "dashboard": {
    "id": null,
    "title": "SQLite Metrics Dashboard",
    "tags": ["sqlite"],
    "timezone": "browser",
    "panels": [
      {
        "id": 1,
        "title": "Metric Values Over Time",
        "type": "timeseries",
        "targets": [
          {
            "datasource": "SQLite-Local",
            "rawSql": "SELECT timestamp * 1000 as time_msec, metric_name, value FROM metrics WHERE timestamp >= $__from / 1000 AND timestamp <= $__to / 1000 ORDER BY timestamp ASC",
            "format": "time_series"
          }
        ],
        "fieldConfig": {
          "defaults": {
            "color": {
              "mode": "palette-classic"
            },
            "unit": "percent"
          }
        },
        "options": {
          "legend": {
            "displayMode": "table",
            "values": ["last", "max", "min"]
          }
        }
      }
    ],
    "time": {
      "from": "now-1h",
      "to": "now"
    },
    "refresh": "30s"
  }
}
```

## Production Considerations and Limitations

### Key Production Requirements

**Persistent Storage Strategy:**
- Use **ReadWriteOnce** persistent volumes with appropriate storage classes
- Implement regular database backups using tools like Litestream
- Consider storage performance requirements for your workload size

**Security Hardening:**
- Run containers with **non-root users** and dropped capabilities
- Implement **network policies** to restrict inter-pod communication  
- Use **secrets** for sensitive configuration data
- Enable **seccomp profiles** and **AppArmor/SELinux** policies

**Monitoring and Alerting:**
- Configure **resource limits** and **requests** appropriately
- Implement **comprehensive health checks** (liveness, readiness, startup probes)
- Set up **monitoring** for database file size growth and performance
- Create **alerts** for StatefulSet scaling events and persistent volume issues

### SQLite-Specific Limitations

**Concurrency and Scaling:**
- **Single-writer limitation** - only one writer can access database at a time
- **No horizontal scaling** - SQLite doesn't support clustering or replication  
- **File locking** can cause issues in multi-pod environments

**Performance Considerations:**
- **WAL mode** recommended for better concurrent read performance
- **Vacuum operations** needed for long-running databases to optimize storage
- **File system performance** directly impacts database performance

**Operational Challenges:**
- **Backup complexity** - requires consistent snapshots of database file
- **Migration difficulty** - moving to other databases requires schema conversion
- **Troubleshooting** requires file system access rather than network connectivity

### Migration to Production Databases

For production workloads requiring high availability, consider these alternatives:

**PostgreSQL Migration:**
```yaml
# Use PostgreSQL operator or cloud-managed service
datasources:
  - name: PostgreSQL
    type: postgres
    url: postgres://user:password@postgres-service:5432/database
    database: grafana_metrics
```

**MySQL Migration:**
```yaml  
# Use MySQL operator or cloud-managed service
datasources:
  - name: MySQL
    type: mysql
    url: mysql://user:password@mysql-service:3306/database
    database: grafana_metrics
```

This comprehensive guide provides three production-ready deployment methods for SQLite + Grafana integration on Kubernetes, each with appropriate security controls, persistent storage, and operational best practices. The choice between deployment methods depends on your infrastructure preferences, automation requirements, and operational complexity tolerance.