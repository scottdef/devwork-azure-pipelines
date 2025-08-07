# Certified Kubernetes Administrator Terminal Guide

A comprehensive guide for CKA tasks using kubectl 1.30, Grafana OSS 12, and Go 1.21 on Azure Kubernetes Service (AKS), focusing on terminal bash commands and JSON manipulation with jq.

## Environment Setup and Prerequisites

**The Certified Kubernetes Administrator exam has evolved significantly in 2025, with troubleshooting now comprising 30% of the exam** - the largest domain. This guide focuses on terminal-based approaches that align with current CKA requirements while leveraging AKS-specific capabilities.

### Initial Environment Configuration

```bash
# Export core environment variables
export CLUSTER_NAME="aks-cka-cluster"
export RESOURCE_GROUP="cka-rg"
export REGION="eastus"
export KUBECTL_VERSION="1.30"
export GRAFANA_VERSION="12"

# Set up workspace directory structure
mkdir -p ~/cka-workspace/{manifests,outputs,scripts,configs,data}
cd ~/cka-workspace

# Configure kubectl context
az aks get-credentials --resource-group $RESOURCE_GROUP --name $CLUSTER_NAME
kubectl config current-context > configs/current-context.txt
```

### Software Version Verification

```bash
# Verify kubectl 1.30 installation
kubectl version --client -o json | jq '.clientVersion.gitVersion'

# Check AKS cluster version compatibility
kubectl version -o json | jq '.serverVersion.gitVersion'

# Verify Go 1.21 for custom tooling
go version | tee configs/go-version.txt
```

## JSON File to JSON Table Conversion with Heredoc

### Converting Kubernetes Resource Data to Structured Tables

```bash
# Collect pod information and store in environment variables
NAMESPACE="kube-system"
POD_DATA=$(kubectl get pods -n $NAMESPACE -o json)

# Extract key pod information into variables
POD_NAMES=$(echo "$POD_DATA" | jq -r '.items[].metadata.name')
POD_IMAGES=$(echo "$POD_DATA" | jq -r '.items[].spec.containers[0].image')
POD_STATUS=$(echo "$POD_DATA" | jq -r '.items[].status.phase')

# Create JSON table using heredoc with variable interpolation
cat > data/pod-table.json <<EOF
{
  "metadata": {
    "created": "$(date -Iseconds)",
    "namespace": "$NAMESPACE",
    "cluster": "$CLUSTER_NAME"
  },
  "table_structure": {
    "columns": ["pod_name", "image", "status", "node"],
    "data_type": "kubernetes_pods"
  },
  "rows": [
$(kubectl get pods -n $NAMESPACE -o json | jq -c '.items[] | {
  pod_name: .metadata.name,
  image: .spec.containers[0].image,
  status: .status.phase,
  node: .spec.nodeName
}' | sed 's/$/,/' | sed '$s/,$//')
  ]
}
EOF

# Create node resource table with variable interpolation
NODE_COUNT=$(kubectl get nodes --no-headers | wc -l)
TOTAL_CPU=$(kubectl get nodes -o json | jq '[.items[].status.capacity.cpu | tonumber] | add')

cat > data/node-resource-table.json <<EOF
{
  "cluster_summary": {
    "name": "$CLUSTER_NAME",
    "region": "$REGION",
    "total_nodes": $NODE_COUNT,
    "total_cpu_cores": $TOTAL_CPU,
    "collection_time": "$(date -Iseconds)"
  },
  "node_details": [
$(kubectl get nodes -o json | jq -c '.items[] | {
  name: .metadata.name,
  cpu_capacity: .status.capacity.cpu,
  memory_capacity: .status.capacity.memory,
  kubelet_version: .status.nodeInfo.kubeletVersion,
  os_image: .status.nodeInfo.osImage
}' | sed 's/$/,/' | sed '$s/,$//')
  ]
}
EOF
```

## JSON Object Selection and File Storage with jq

### Extracting and Storing JSON Elements in Files

```bash
# Create dedicated directories for different JSON data types
mkdir -p data/{pods,services,nodes,events,configs}

# Select and store pod-specific JSON objects
kubectl get pods --all-namespaces -o json | jq '.items[] | select(.status.phase != "Running")' > data/pods/non-running-pods.json

kubectl get pods --all-namespaces -o json | jq '.items[] | select(.spec.containers[].resources.limits != null)' > data/pods/pods-with-limits.json

kubectl get pods --all-namespaces -o json | jq '.items[] | {
  name: .metadata.name,
  namespace: .metadata.namespace,
  containers: [.spec.containers[] | {name: .name, image: .image}],
  restart_count: (.status.containerStatuses[]?.restartCount // 0)
}' > data/pods/pod-container-info.json

# Extract service configurations
kubectl get services --all-namespaces -o json | jq '.items[] | select(.spec.type == "LoadBalancer")' > data/services/loadbalancer-services.json

kubectl get services --all-namespaces -o json | jq '.items[] | {
  name: .metadata.name,
  namespace: .metadata.namespace,
  cluster_ip: .spec.clusterIP,
  ports: .spec.ports,
  selector: .spec.selector
}' > data/services/service-details.json

# Store endpoint information
kubectl get endpoints --all-namespaces -o json | jq '.items[] | select(.subsets | length > 0)' > data/services/active-endpoints.json

# Extract node information with specific attributes
kubectl get nodes -o json | jq '.items[] | {
  name: .metadata.name,
  labels: .metadata.labels,
  capacity: .status.capacity,
  conditions: [.status.conditions[] | select(.type == "Ready")]
}' > data/nodes/node-status.json

# Store configuration data
kubectl get configmaps --all-namespaces -o json | jq '.items[] | select(.metadata.name | startswith("kube-"))' > data/configs/system-configmaps.json
```

## Environment Variables with JSON Templates

### Storing jq Results in Variables for Template Population

```bash
# Extract cluster information into environment variables
export CLUSTER_VERSION=$(kubectl version -o json | jq -r '.serverVersion.gitVersion')
export NODE_COUNT=$(kubectl get nodes --no-headers | wc -l)
export NAMESPACE_COUNT=$(kubectl get namespaces --no-headers | wc -l)
export TOTAL_PODS=$(kubectl get pods --all-namespaces --no-headers | wc -l)

# Extract specific resource information
export FAILED_PODS=$(kubectl get pods --all-namespaces -o json | jq -r '[.items[] | select(.status.phase != "Running" and .status.phase != "Succeeded")] | length')
export SYSTEM_PODS=$(kubectl get pods -n kube-system --no-headers | wc -l)
export DEFAULT_STORAGE_CLASS=$(kubectl get storageclass -o json | jq -r '.items[] | select(.metadata.annotations["storageclass.kubernetes.io/is-default-class"] == "true") | .metadata.name')

# Database connection details from secrets
export DB_HOST=$(kubectl get secret db-credentials -o json 2>/dev/null | jq -r '.data.host // empty' | base64 -d || echo "localhost")
export DB_PORT=$(kubectl get secret db-credentials -o json 2>/dev/null | jq -r '.data.port // empty' | base64 -d || echo "5432")

# Generate comprehensive cluster report using environment variables
cat > data/cluster-report.json <<EOF
{
  "cluster_information": {
    "name": "$CLUSTER_NAME",
    "region": "$REGION", 
    "kubernetes_version": "$CLUSTER_VERSION",
    "report_generated": "$(date -Iseconds)"
  },
  "resource_summary": {
    "nodes": {
      "total": $NODE_COUNT,
      "status": "$(kubectl get nodes --no-headers | grep -c Ready)/$(kubectl get nodes --no-headers | wc -l) Ready"
    },
    "namespaces": {
      "total": $NAMESPACE_COUNT,
      "active": $(kubectl get namespaces -o json | jq '[.items[] | select(.status.phase == "Active")] | length')
    },
    "pods": {
      "total": $TOTAL_PODS,
      "running": $(($TOTAL_PODS - $FAILED_PODS)),
      "failed": $FAILED_PODS,
      "system_pods": $SYSTEM_PODS
    }
  },
  "storage_configuration": {
    "default_storage_class": "$DEFAULT_STORAGE_CLASS",
    "total_pvs": $(kubectl get pv --no-headers 2>/dev/null | wc -l || echo 0),
    "total_pvcs": $(kubectl get pvc --all-namespaces --no-headers 2>/dev/null | wc -l || echo 0)
  },
  "database_config": {
    "host": "$DB_HOST",
    "port": "$DB_PORT",
    "connection_configured": $([ -n "$DB_HOST" ] && [ "$DB_HOST" != "localhost" ] && echo "true" || echo "false")
  }
}
EOF

# Create monitoring configuration template
cat > manifests/monitoring-config.yaml <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: cluster-monitoring-config
  namespace: monitoring
data:
  cluster.json: |
    {
      "cluster_name": "$CLUSTER_NAME",
      "monitoring_targets": {
        "nodes": $NODE_COUNT,
        "system_pods": $SYSTEM_PODS,
        "total_namespaces": $NAMESPACE_COUNT
      },
      "alerting_thresholds": {
        "pod_failure_rate": 0.1,
        "node_cpu_threshold": 80,
        "memory_threshold": 85
      },
      "grafana_version": "$GRAFANA_VERSION",
      "last_updated": "$(date -Iseconds)"
    }
  prometheus.yml: |
    global:
      scrape_interval: 15s
      external_labels:
        cluster: '$CLUSTER_NAME'
        region: '$REGION'
    scrape_configs:
    - job_name: 'kubernetes-nodes'
      kubernetes_sd_configs:
      - role: node
      relabel_configs:
      - source_labels: [__address__]
        regex: '(.*):10250'
        target_label: __address__
        replacement: '\${1}:9100'
EOF

# Generate deployment manifest with variables
cat > manifests/app-deployment.yaml <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: webapp-deployment
  namespace: production
  labels:
    app: webapp
    version: "1.0"
spec:
  replicas: $(echo "scale=0; $NODE_COUNT * 2" | bc)
  selector:
    matchLabels:
      app: webapp
  template:
    metadata:
      labels:
        app: webapp
        cluster: $CLUSTER_NAME
    spec:
      containers:
      - name: webapp
        image: nginx:1.21
        ports:
        - containerPort: 80
        env:
        - name: CLUSTER_NAME
          value: "$CLUSTER_NAME"
        - name: NODE_COUNT
          value: "$NODE_COUNT"
        - name: POD_COUNT
          value: "$TOTAL_PODS"
        resources:
          limits:
            cpu: "500m"
            memory: "512Mi"
          requests:
            cpu: "250m"
            memory: "256Mi"
EOF
```

## Output Organization: Sorting JSON Files by Properties

### Example 1: Organizing Pods by Namespace and Status

```bash
# Create directory structure based on namespace and status
mkdir -p outputs/pods/{namespaces,statuses,images}

# Organize pods by namespace
for namespace in $(kubectl get namespaces -o json | jq -r '.items[].metadata.name'); do
    mkdir -p "outputs/pods/namespaces/$namespace"
    kubectl get pods -n "$namespace" -o json | jq '.items[]' > "outputs/pods/namespaces/$namespace/pods.json"
    
    # Create namespace summary
    POD_COUNT=$(kubectl get pods -n "$namespace" --no-headers | wc -l)
    RUNNING_COUNT=$(kubectl get pods -n "$namespace" --no-headers | grep -c Running || echo 0)
    
    cat > "outputs/pods/namespaces/$namespace/summary.json" <<EOF
{
  "namespace": "$namespace",
  "total_pods": $POD_COUNT,
  "running_pods": $RUNNING_COUNT,
  "pod_distribution": $(kubectl get pods -n "$namespace" -o json | jq '[.items[] | .status.phase] | group_by(.) | map({status: .[0], count: length})')
}
EOF
done

# Organize pods by status
for status in Running Pending Failed Succeeded; do
    mkdir -p "outputs/pods/statuses/$status"
    kubectl get pods --all-namespaces -o json | jq ".items[] | select(.status.phase == \"$status\")" > "outputs/pods/statuses/$status/pods.json" 2>/dev/null || echo "[]" > "outputs/pods/statuses/$status/pods.json"
done
```

### Example 2: Organizing Services by Type and Port Configuration

```bash
# Create service organization structure
mkdir -p outputs/services/{types,ports,protocols}

# Organize services by type
for svc_type in ClusterIP NodePort LoadBalancer ExternalName; do
    mkdir -p "outputs/services/types/$svc_type"
    kubectl get services --all-namespaces -o json | jq ".items[] | select(.spec.type == \"$svc_type\" or (.spec.type == null and \"$svc_type\" == \"ClusterIP\"))" > "outputs/services/types/$svc_type/services.json" 2>/dev/null || echo "[]" > "outputs/services/types/$svc_type/services.json"
    
    # Create type-specific summary
    SERVICE_COUNT=$(kubectl get services --all-namespaces -o json | jq "[.items[] | select(.spec.type == \"$svc_type\" or (.spec.type == null and \"$svc_type\" == \"ClusterIP\"))] | length")
    
    cat > "outputs/services/types/$svc_type/summary.json" <<EOF
{
  "service_type": "$svc_type",
  "count": $SERVICE_COUNT,
  "namespaces": $(kubectl get services --all-namespaces -o json | jq "[.items[] | select(.spec.type == \"$svc_type\" or (.spec.type == null and \"$svc_type\" == \"ClusterIP\")) | .metadata.namespace] | unique"),
  "port_ranges": $(kubectl get services --all-namespaces -o json | jq "[.items[] | select(.spec.type == \"$svc_type\" or (.spec.type == null and \"$svc_type\" == \"ClusterIP\")) | .spec.ports[]?.port] | sort | unique")
}
EOF
done

# Organize services by common ports
for port in 80 443 8080 3000 5432 6379; do
    mkdir -p "outputs/services/ports/$port"
    kubectl get services --all-namespaces -o json | jq ".items[] | select(.spec.ports[]?.port == $port)" > "outputs/services/ports/$port/services.json"
done
```

### Example 3: Organizing Nodes by Labels and Conditions

```bash
# Create node organization structure
mkdir -p outputs/nodes/{labels,conditions,zones}

# Organize nodes by node role labels
for role in master worker; do
    mkdir -p "outputs/nodes/labels/$role"
    kubectl get nodes -o json | jq ".items[] | select(.metadata.labels[\"kubernetes.io/role\"] == \"$role\" or .metadata.labels[\"node-role.kubernetes.io/$role\"] != null)" > "outputs/nodes/labels/$role/nodes.json"
done

# Organize nodes by availability zones (AKS specific)
kubectl get nodes -o json | jq -r '.items[].metadata.labels["topology.kubernetes.io/zone"] // "unknown"' | sort | uniq | while read zone; do
    mkdir -p "outputs/nodes/zones/$zone"
    kubectl get nodes -o json | jq ".items[] | select(.metadata.labels[\"topology.kubernetes.io/zone\"] == \"$zone\")" > "outputs/nodes/zones/$zone/nodes.json"
    
    # Create zone summary
    NODE_COUNT=$(kubectl get nodes -o json | jq "[.items[] | select(.metadata.labels[\"topology.kubernetes.io/zone\"] == \"$zone\")] | length")
    TOTAL_CPU=$(kubectl get nodes -o json | jq "[.items[] | select(.metadata.labels[\"topology.kubernetes.io/zone\"] == \"$zone\") | .status.capacity.cpu | tonumber] | add")
    
    cat > "outputs/nodes/zones/$zone/summary.json" <<EOF
{
  "availability_zone": "$zone",
  "node_count": $NODE_COUNT,
  "total_cpu_capacity": $TOTAL_CPU,
  "node_types": $(kubectl get nodes -o json | jq "[.items[] | select(.metadata.labels[\"topology.kubernetes.io/zone\"] == \"$zone\") | .metadata.labels[\"beta.kubernetes.io/instance-type\"]] | group_by(.) | map({type: .[0], count: length})")
}
EOF
done

# Organize by node conditions
for condition in Ready MemoryPressure DiskPressure PIDPressure NetworkUnavailable; do
    mkdir -p "outputs/nodes/conditions/$condition"
    kubectl get nodes -o json | jq ".items[] | select(.status.conditions[] | select(.type == \"$condition\" and .status == \"True\"))" > "outputs/nodes/conditions/$condition/nodes.json"
done
```

## Background Jobs for Monitoring and Maintenance

### Continuous Cluster Monitoring with Background Jobs

```bash
# Create monitoring script for background execution
cat > scripts/monitor-cluster.sh <<'EOF'
#!/bin/bash
set -euo pipefail

MONITORING_DIR="outputs/monitoring/$(date +%Y%m%d)"
mkdir -p "$MONITORING_DIR"

# Function to collect cluster metrics
collect_metrics() {
    while true; do
        TIMESTAMP=$(date -Iseconds)
        
        # Collect node metrics
        kubectl top nodes --no-headers > "$MONITORING_DIR/node-metrics-$(date +%H%M%S).txt" 2>/dev/null || echo "Metrics server unavailable" > "$MONITORING_DIR/node-metrics-$(date +%H%M%S).txt"
        
        # Collect pod metrics
        kubectl top pods --all-namespaces --no-headers > "$MONITORING_DIR/pod-metrics-$(date +%H%M%S).txt" 2>/dev/null || echo "Metrics server unavailable" > "$MONITORING_DIR/pod-metrics-$(date +%H%M%S).txt"
        
        # Check for problematic pods
        FAILED_PODS=$(kubectl get pods --all-namespaces -o json | jq -r '.items[] | select(.status.phase != "Running" and .status.phase != "Succeeded") | "\(.metadata.namespace)/\(.metadata.name)"')
        
        if [ -n "$FAILED_PODS" ]; then
            echo "$TIMESTAMP: Failed pods detected:" >> "$MONITORING_DIR/alerts.log"
            echo "$FAILED_PODS" >> "$MONITORING_DIR/alerts.log"
            
            # Collect detailed information about failed pods
            echo "$FAILED_PODS" | while read pod_info; do
                if [ -n "$pod_info" ]; then
                    namespace=$(echo "$pod_info" | cut -d'/' -f1)
                    pod_name=$(echo "$pod_info" | cut -d'/' -f2)
                    kubectl describe pod "$pod_name" -n "$namespace" > "$MONITORING_DIR/failed-pod-${pod_name}-$(date +%H%M%S).txt"
                fi
            done
        fi
        
        # Monitor cluster events
        kubectl get events --all-namespaces --sort-by=.metadata.creationTimestamp -o json | jq '.items[-10:]' > "$MONITORING_DIR/recent-events-$(date +%H%M%S).json"
        
        sleep 60
    done
}

# Function to monitor storage
monitor_storage() {
    while true; do
        # Check PVC status
        kubectl get pvc --all-namespaces -o json | jq '.items[] | select(.status.phase != "Bound")' > "$MONITORING_DIR/unbound-pvcs-$(date +%H%M%S).json"
        
        # Monitor storage usage (if metrics available)
        kubectl get --raw /apis/metrics.k8s.io/v1beta1/nodes 2>/dev/null | jq '.items[] | {name: .metadata.name, storage: .usage.storage}' > "$MONITORING_DIR/storage-usage-$(date +%H%M%S).json" || echo "Storage metrics unavailable"
        
        sleep 300  # Check every 5 minutes
    done
}

# Run monitoring functions in background
collect_metrics &
METRICS_PID=$!

monitor_storage &
STORAGE_PID=$!

# Trap to cleanup background processes
trap "kill $METRICS_PID $STORAGE_PID 2>/dev/null; exit" INT TERM EXIT

echo "Cluster monitoring started"
echo "Metrics collection PID: $METRICS_PID"
echo "Storage monitoring PID: $STORAGE_PID" 
echo "Press Ctrl+C to stop monitoring"

# Wait for background processes
wait
EOF

chmod +x scripts/monitor-cluster.sh

# Start monitoring in background
nohup ./scripts/monitor-cluster.sh > outputs/monitoring.log 2>&1 &
MONITOR_PID=$!
echo "Background monitoring started with PID: $MONITOR_PID"

# Create script to check monitoring status
cat > scripts/check-monitoring.sh <<EOF
#!/bin/bash
if ps -p $MONITOR_PID > /dev/null; then
    echo "Monitoring is running (PID: $MONITOR_PID)"
    echo "Latest metrics files:"
    ls -lt outputs/monitoring/\$(date +%Y%m%d)/ | head -5
else
    echo "Monitoring is not running"
fi
EOF

chmod +x scripts/check-monitoring.sh
```

## Cron Job Scheduling for Production Tasks

### Database Backup and Maintenance CronJobs

```bash
# Create backup CronJob manifest
cat > manifests/backup-cronjob.yaml <<EOF
apiVersion: batch/v1
kind: CronJob
metadata:
  name: postgres-backup
  namespace: production
spec:
  schedule: "0 2 * * *"  # Daily at 2 AM
  timeZone: "America/New_York"
  jobTemplate:
    spec:
      template:
        spec:
          containers:
          - name: backup
            image: postgres:13
            command:
            - /bin/bash
            - -c
            - |
              set -euo pipefail
              BACKUP_DATE=\$(date +%Y%m%d-%H%M%S)
              BACKUP_FILE="/backup/backup-\${BACKUP_DATE}.sql"
              
              echo "Starting backup at \$(date)"
              pg_dump -h \$DB_HOST -U \$DB_USER \$DB_NAME > "\$BACKUP_FILE"
              
              # Create backup metadata
              cat > "/backup/backup-\${BACKUP_DATE}-metadata.json" <<METADATA
              {
                "backup_date": "\$(date -Iseconds)",
                "database": "\$DB_NAME",
                "host": "\$DB_HOST",
                "file_size": \$(stat -c%s "\$BACKUP_FILE"),
                "backup_type": "full",
                "compression": "none"
              }
              METADATA
              
              echo "Backup completed: \$BACKUP_FILE"
              
              # Cleanup old backups (keep last 7 days)
              find /backup -name "backup-*.sql" -type f -mtime +7 -delete
              find /backup -name "backup-*-metadata.json" -type f -mtime +7 -delete
              
              echo "Cleanup completed at \$(date)"
            env:
            - name: DB_HOST
              value: "postgres-service.production.svc.cluster.local"
            - name: DB_USER
              valueFrom:
                secretKeyRef:
                  name: postgres-credentials
                  key: username
            - name: DB_NAME
              value: "production_db"
            - name: PGPASSWORD
              valueFrom:
                secretKeyRef:
                  name: postgres-credentials
                  key: password
            volumeMounts:
            - name: backup-storage
              mountPath: /backup
          volumes:
          - name: backup-storage
            persistentVolumeClaim:
              claimName: backup-pvc
          restartPolicy: OnFailure
  successfulJobsHistoryLimit: 3
  failedJobsHistoryLimit: 1
EOF

# Log rotation and cleanup CronJob
cat > manifests/log-cleanup-cronjob.yaml <<EOF
apiVersion: batch/v1
kind: CronJob
metadata:
  name: log-cleanup
  namespace: kube-system
spec:
  schedule: "0 1 * * 0"  # Weekly on Sunday at 1 AM
  jobTemplate:
    spec:
      template:
        spec:
          containers:
          - name: cleanup
            image: busybox
            command:
            - /bin/sh
            - -c
            - |
              set -e
              echo "Starting log cleanup at \$(date)"
              
              # Find and remove old log files
              DELETED_COUNT=\$(find /var/log -name "*.log" -type f -mtime +14 -print | wc -l)
              find /var/log -name "*.log" -type f -mtime +14 -delete
              
              # Rotate current logs
              find /var/log -name "*.log" -type f -size +100M -exec sh -c 'mv "\$1" "\$1.\$(date +%Y%m%d)"' _ {} \;
              
              # Create cleanup report
              cat > /var/log/cleanup-report-\$(date +%Y%m%d).json <<REPORT
              {
                "cleanup_date": "\$(date -Iseconds)",
                "deleted_files": \$DELETED_COUNT,
                "disk_usage_before": "\$(df -h /var/log | tail -1 | awk '{print \$3}')",
                "disk_usage_after": "\$(df -h /var/log | tail -1 | awk '{print \$3}')"
              }
              REPORT
              
              echo "Cleanup completed. Deleted \$DELETED_COUNT files"
            volumeMounts:
            - name: log-volume
              mountPath: /var/log
          volumes:
          - name: log-volume
            hostPath:
              path: /var/log
          restartPolicy: OnFailure
EOF

# Cluster health check CronJob
cat > manifests/health-check-cronjob.yaml <<EOF
apiVersion: batch/v1
kind: CronJob
metadata:
  name: cluster-health-check
  namespace: monitoring
spec:
  schedule: "*/15 * * * *"  # Every 15 minutes
  jobTemplate:
    spec:
      template:
        spec:
          serviceAccountName: health-checker
          containers:
          - name: health-check
            image: bitnami/kubectl:1.30
            command:
            - /bin/bash
            - -c
            - |
              set -euo pipefail
              TIMESTAMP=\$(date -Iseconds)
              REPORT_FILE="/reports/health-check-\$(date +%Y%m%d-%H%M).json"
              
              # Check node status
              NODE_COUNT=\$(kubectl get nodes --no-headers | wc -l)
              READY_NODES=\$(kubectl get nodes --no-headers | grep -c " Ready " || echo 0)
              
              # Check critical system pods
              SYSTEM_POD_COUNT=\$(kubectl get pods -n kube-system --no-headers | wc -l)
              RUNNING_SYSTEM_PODS=\$(kubectl get pods -n kube-system --no-headers | grep -c " Running " || echo 0)
              
              # Check API server responsiveness
              API_RESPONSE_TIME=\$(time kubectl get --raw /api/v1 >/dev/null 2>&1 | grep real | awk '{print \$2}' || echo "timeout")
              
              # Generate health report
              cat > "\$REPORT_FILE" <<HEALTH_REPORT
              {
                "timestamp": "\$TIMESTAMP",
                "cluster_health": {
                  "nodes": {
                    "total": \$NODE_COUNT,
                    "ready": \$READY_NODES,
                    "health_percentage": \$(echo "scale=2; \$READY_NODES * 100 / \$NODE_COUNT" | bc)
                  },
                  "system_pods": {
                    "total": \$SYSTEM_POD_COUNT,
                    "running": \$RUNNING_SYSTEM_PODS,
                    "health_percentage": \$(echo "scale=2; \$RUNNING_SYSTEM_PODS * 100 / \$SYSTEM_POD_COUNT" | bc)
                  },
                  "api_server": {
                    "response_time": "\$API_RESPONSE_TIME",
                    "status": "\$([ "\$API_RESPONSE_TIME" != "timeout" ] && echo "healthy" || echo "unhealthy")"
                  }
                },
                "alerts": [
              \$(if [ \$READY_NODES -lt \$NODE_COUNT ]; then
                echo "    {\"type\": \"node_down\", \"message\": \"Some nodes are not ready\"}"
              fi)
              \$(if [ "\$API_RESPONSE_TIME" = "timeout" ]; then
                echo "    {\"type\": \"api_timeout\", \"message\": \"API server not responding\"}"
              fi)
                ]
              }
              HEALTH_REPORT
              
              echo "Health check completed. Report saved to \$REPORT_FILE"
            volumeMounts:
            - name: report-storage
              mountPath: /reports
          volumes:
          - name: report-storage
            persistentVolumeClaim:
              claimName: health-reports-pvc
          restartPolicy: OnFailure
EOF

# Apply CronJobs
kubectl apply -f manifests/backup-cronjob.yaml
kubectl apply -f manifests/log-cleanup-cronjob.yaml  
kubectl apply -f manifests/health-check-cronjob.yaml

# Check CronJob status
kubectl get cronjobs --all-namespaces -o wide
```

## Grafana OSS 12 Integration with AKS

### Deploying Grafana with AKS-Specific Dashboards

```bash
# Create Grafana configuration with AKS monitoring
cat > manifests/grafana-deployment.yaml <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: grafana
  namespace: monitoring
spec:
  replicas: 1
  selector:
    matchLabels:
      app: grafana
  template:
    metadata:
      labels:
        app: grafana
    spec:
      containers:
      - name: grafana
        image: grafana/grafana-oss:12.0.0
        ports:
        - containerPort: 3000
        env:
        - name: GF_SECURITY_ADMIN_PASSWORD
          valueFrom:
            secretKeyRef:
              name: grafana-credentials
              key: admin-password
        - name: GF_INSTALL_PLUGINS
          value: "grafana-azure-monitor-datasource,grafana-kubernetes-app"
        volumeMounts:
        - name: grafana-storage
          mountPath: /var/lib/grafana
        - name: grafana-config
          mountPath: /etc/grafana/grafana.ini
          subPath: grafana.ini
      volumes:
      - name: grafana-storage
        persistentVolumeClaim:
          claimName: grafana-pvc
      - name: grafana-config
        configMap:
          name: grafana-config
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: grafana-config
  namespace: monitoring
data:
  grafana.ini: |
    [server]
    http_port = 3000
    
    [database]
    type = sqlite3
    path = /var/lib/grafana/grafana.db
    
    [auth.azure_ad]
    enabled = true
    allow_sign_up = true
    client_id = $AZURE_CLIENT_ID
    client_secret = $AZURE_CLIENT_SECRET
    scopes = openid email profile
    auth_url = https://login.microsoftonline.com/$AZURE_TENANT_ID/oauth2/v2.0/authorize
    token_url = https://login.microsoftonline.com/$AZURE_TENANT_ID/oauth2/v2.0/token
    
    [feature_toggles]
    enable = publicDashboards,dashboardPreviews
EOF

# Create AKS-specific dashboard configuration
cat > configs/aks-dashboard.json <<'EOF'
{
  "dashboard": {
    "id": null,
    "title": "AKS Cluster Overview",
    "tags": ["kubernetes", "aks", "azure"],
    "timezone": "browser",
    "panels": [
      {
        "id": 1,
        "title": "Node Status",
        "type": "stat",
        "targets": [
          {
            "expr": "kube_node_status_condition{condition=\"Ready\",status=\"true\"}",
            "legendFormat": "Ready Nodes"
          }
        ],
        "fieldConfig": {
          "defaults": {
            "color": {
              "mode": "thresholds"
            },
            "thresholds": {
              "steps": [
                {"color": "red", "value": 0},
                {"color": "yellow", "value": 1},
                {"color": "green", "value": 2}
              ]
            }
          }
        }
      },
      {
        "id": 2,
        "title": "Pod Status Distribution",
        "type": "piechart",
        "targets": [
          {
            "expr": "sum by (phase) (kube_pod_status_phase)",
            "legendFormat": "{{phase}}"
          }
        ]
      },
      {
        "id": 3,
        "title": "CPU Usage by Node",
        "type": "timeseries",
        "targets": [
          {
            "expr": "100 - (avg by (instance) (irate(node_cpu_seconds_total{mode=\"idle\"}[5m])) * 100)",
            "legendFormat": "{{instance}}"
          }
        ]
      }
    ],
    "time": {
      "from": "now-1h",
      "to": "now"
    },
    "refresh": "30s"
  }
}
EOF

# Deploy Grafana
kubectl apply -f manifests/grafana-deployment.yaml

# Create Grafana service
kubectl expose deployment grafana --type=LoadBalancer --port=3000 -n monitoring
```

## Advanced JSON Processing Workflows

### Comprehensive Cluster Analysis Pipeline

```bash
# Create advanced JSON processing pipeline
cat > scripts/cluster-analysis.sh <<'EOF'
#!/bin/bash
set -euo pipefail

ANALYSIS_DIR="outputs/analysis/$(date +%Y%m%d-%H%M%S)"
mkdir -p "$ANALYSIS_DIR"/{raw,processed,reports}

echo "Starting comprehensive cluster analysis..."

# Step 1: Collect raw data
echo "Collecting raw cluster data..."
kubectl get nodes -o json > "$ANALYSIS_DIR/raw/nodes.json"
kubectl get pods --all-namespaces -o json > "$ANALYSIS_DIR/raw/pods.json"
kubectl get services --all-namespaces -o json > "$ANALYSIS_DIR/raw/services.json"
kubectl get deployments --all-namespaces -o json > "$ANALYSIS_DIR/raw/deployments.json"
kubectl get events --all-namespaces -o json > "$ANALYSIS_DIR/raw/events.json"

# Step 2: Process nodes data
echo "Processing nodes data..."
jq -r '
.items[] | 
{
  name: .metadata.name,
  labels: .metadata.labels,
  capacity: .status.capacity,
  allocatable: .status.allocatable,
  conditions: [.status.conditions[] | select(.status == "True") | .type],
  addresses: [.status.addresses[] | {type: .type, address: .address}],
  node_info: .status.nodeInfo,
  creation_timestamp: .metadata.creationTimestamp
}' "$ANALYSIS_DIR/raw/nodes.json" > "$ANALYSIS_DIR/processed/nodes-detailed.json"

# Calculate node resource summary
jq -r '
[.items[] | 
  {
    name: .metadata.name,
    cpu_capacity: (.status.capacity.cpu | tonumber),
    memory_capacity: (.status.capacity.memory | sub("[^0-9]"; "") | tonumber),
    pod_capacity: (.status.capacity.pods | tonumber)
  }
] | 
{
  total_nodes: length,
  total_cpu: (map(.cpu_capacity) | add),
  total_memory_gi: ((map(.memory_capacity) | add) / (1024*1024*1024) | floor),
  total_pod_capacity: (map(.pod_capacity) | add),
  avg_cpu_per_node: ((map(.cpu_capacity) | add) / length),
  avg_memory_per_node: (((map(.memory_capacity) | add) / length) / (1024*1024*1024) | floor)
}' "$ANALYSIS_DIR/raw/nodes.json" > "$ANALYSIS_DIR/processed/node-summary.json"

# Step 3: Process pods data
echo "Processing pods data..."
jq -r '
[.items[] | 
  {
    name: .metadata.name,
    namespace: .metadata.namespace,
    node: .spec.nodeName,
    phase: .status.phase,
    containers: [.spec.containers[] | {name: .name, image: .image}],
    restart_count: ([.status.containerStatuses[]?.restartCount // 0] | add),
    resource_requests: {
      cpu: ([.spec.containers[]?.resources.requests.cpu // "0"] | map(sub("[^0-9]"; "") | tonumber) | add),
      memory: ([.spec.containers[]?.resources.requests.memory // "0"] | map(sub("[^0-9]"; "") | tonumber) | add)
    },
    creation_timestamp: .metadata.creationTimestamp
  }
]' "$ANALYSIS_DIR/raw/pods.json" > "$ANALYSIS_DIR/processed/pods-detailed.json"

# Pod status analysis
jq -r '
.items | 
group_by(.metadata.namespace) | 
map({
  namespace: .[0].metadata.namespace,
  total_pods: length,
  status_distribution: (group_by(.status.phase) | map({status: .[0].status.phase, count: length})),
  restart_analysis: {
    high_restart_pods: [.[] | select(([.status.containerStatuses[]?.restartCount // 0] | add) > 5) | .metadata.name],
    total_restarts: ([.[] | [.status.containerStatuses[]?.restartCount // 0] | add] | add)
  }
})' "$ANALYSIS_DIR/raw/pods.json" > "$ANALYSIS_DIR/processed/namespace-pod-analysis.json"

# Step 4: Service analysis
echo "Processing services data..."
jq -r '
[.items[] | 
  {
    name: .metadata.name,
    namespace: .metadata.namespace,
    type: (.spec.type // "ClusterIP"),
    cluster_ip: .spec.clusterIP,
    external_ip: .status.loadBalancer.ingress[0].ip // null,
    ports: .spec.ports,
    selector: .spec.selector,
    endpoint_count: (.metadata.name as $svc | 
      if .spec.selector then 1 else 0 end)
  }
] | 
group_by(.type) | 
map({
  type: .[0].type,
  count: length,
  namespaces: [.[].namespace] | unique,
  services: [.[].name]
})' "$ANALYSIS_DIR/raw/services.json" > "$ANALYSIS_DIR/processed/service-type-analysis.json"

# Step 5: Event analysis
echo "Processing events data..."
jq -r '
.items | 
sort_by(.metadata.creationTimestamp) | 
reverse | 
.[0:50] | 
map({
  timestamp: .metadata.creationTimestamp,
  type: .type,
  reason: .reason,
  message: .message,
  object: {
    kind: .involvedObject.kind,
    name: .involvedObject.name,
    namespace: .involvedObject.namespace
  },
  count: .count // 1
}) | 
group_by(.reason) | 
map({
  reason: .[0].reason,
  count: length,
  latest_occurrence: .[0].timestamp,
  affected_objects: [.[].object | select(.name != null)] | unique_by(.name)
})' "$ANALYSIS_DIR/raw/events.json" > "$ANALYSIS_DIR/processed/event-analysis.json"

# Step 6: Generate comprehensive report
echo "Generating comprehensive analysis report..."
cat > "$ANALYSIS_DIR/reports/cluster-health-report.json" <<REPORT
{
  "analysis_metadata": {
    "timestamp": "$(date -Iseconds)",
    "cluster_name": "$CLUSTER_NAME",
    "analysis_version": "1.0"
  },
  "node_summary": $(cat "$ANALYSIS_DIR/processed/node-summary.json"),
  "namespace_analysis": $(cat "$ANALYSIS_DIR/processed/namespace-pod-analysis.json"),
  "service_distribution": $(cat "$ANALYSIS_DIR/processed/service-type-analysis.json"),
  "recent_events": $(cat "$ANALYSIS_DIR/processed/event-analysis.json"),
  "recommendations": [
$(jq -r '
if .total_nodes < 3 then
  "    {\"priority\": \"high\", \"category\": \"availability\", \"message\": \"Consider adding more nodes for high availability\"}"
else empty end' "$ANALYSIS_DIR/processed/node-summary.json")
$(jq -r '
if .[] | select(.restart_analysis.high_restart_pods | length > 0) then
  "    {\"priority\": \"medium\", \"category\": \"stability\", \"message\": \"Some pods have high restart counts - investigate container issues\"}"
else empty end' "$ANALYSIS_DIR/processed/namespace-pod-analysis.json")
  ]
}
REPORT

echo "Analysis completed successfully!"
echo "Results saved to: $ANALYSIS_DIR"
echo "Main report: $ANALYSIS_DIR/reports/cluster-health-report.json"

# Display summary
echo ""
echo "=== CLUSTER ANALYSIS SUMMARY ==="
jq -r '
"Cluster: " + .analysis_metadata.cluster_name,
"Nodes: " + (.node_summary.total_nodes | tostring),
"Total CPU Cores: " + (.node_summary.total_cpu | tostring),
"Total Memory (GB): " + (.node_summary.total_memory_gi | tostring),
"Namespaces with Issues: " + ([.namespace_analysis[] | select(.restart_analysis.high_restart_pods | length > 0) | .namespace] | length | tostring)
' "$ANALYSIS_DIR/reports/cluster-health-report.json"
EOF

chmod +x scripts/cluster-analysis.sh

# Run the analysis
./scripts/cluster-analysis.sh
```

## Go 1.21 Kubernetes Utilities

### Custom Kubernetes Monitoring Tool

```bash
# Create Go-based monitoring utility
mkdir -p go-tools/cluster-monitor
cd go-tools/cluster-monitor

# Initialize Go module
go mod init cluster-monitor

# Create main monitoring application
cat > main.go <<'EOF'
package main

import (
    "context"
    "encoding/json"
    "fmt"
    "log"
    "os"
    "path/filepath"
    "time"

    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/client-go/kubernetes"
    "k8s.io/client-go/rest"
    "k8s.io/client-go/tools/clientcmd"
)

type ClusterMetrics struct {
    Timestamp     time.Time     `json:"timestamp"`
    NodeCount     int           `json:"node_count"`
    PodCount      int           `json:"pod_count"`
    NamespaceCount int          `json:"namespace_count"`
    Nodes         []NodeInfo    `json:"nodes"`
    PodSummary    []PodSummary  `json:"pod_summary"`
}

type NodeInfo struct {
    Name       string            `json:"name"`
    Status     string            `json:"status"`
    Capacity   map[string]string `json:"capacity"`
    Labels     map[string]string `json:"labels"`
}

type PodSummary struct {
    Namespace string `json:"namespace"`
    Running   int    `json:"running"`
    Pending   int    `json:"pending"`
    Failed    int    `json:"failed"`
}

func main() {
    clientset, err := createClientset()
    if err != nil {
        log.Fatalf("Failed to create clientset: %v", err)
    }

    metrics, err := collectMetrics(clientset)
    if err != nil {
        log.Fatalf("Failed to collect metrics: %v", err)
    }

    outputPath := fmt.Sprintf("../../outputs/go-metrics/metrics-%s.json", 
        time.Now().Format("20060102-150405"))
    
    if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
        log.Fatalf("Failed to create output directory: %v", err)
    }

    file, err := os.Create(outputPath)
    if err != nil {
        log.Fatalf("Failed to create output file: %v", err)
    }
    defer file.Close()

    encoder := json.NewEncoder(file)
    encoder.SetIndent("", "  ")
    if err := encoder.Encode(metrics); err != nil {
        log.Fatalf("Failed to encode metrics: %v", err)
    }

    fmt.Printf("Metrics collected and saved to: %s\n", outputPath)
    fmt.Printf("Nodes: %d, Pods: %d, Namespaces: %d\n", 
        metrics.NodeCount, metrics.PodCount, metrics.NamespaceCount)
}

func createClientset() (*kubernetes.Clientset, error) {
    config, err := rest.InClusterConfig()
    if err != nil {
        // Fallback to kubeconfig
        kubeconfig := filepath.Join(os.Getenv("HOME"), ".kube", "config")
        config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
        if err != nil {
            return nil, err
        }
    }

    return kubernetes.NewForConfig(config)
}

func collectMetrics(clientset *kubernetes.Clientset) (*ClusterMetrics, error) {
    ctx := context.TODO()
    
    // Get nodes
    nodes, err := clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
    if err != nil {
        return nil, fmt.Errorf("failed to list nodes: %v", err)
    }

    // Get pods
    pods, err := clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
    if err != nil {
        return nil, fmt.Errorf("failed to list pods: %v", err)
    }

    // Get namespaces
    namespaces, err := clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
    if err != nil {
        return nil, fmt.Errorf("failed to list namespaces: %v", err)
    }

    // Process nodes
    var nodeInfos []NodeInfo
    for _, node := range nodes.Items {
        status := "Unknown"
        for _, condition := range node.Status.Conditions {
            if condition.Type == "Ready" {
                if condition.Status == "True" {
                    status = "Ready"
                } else {
                    status = "NotReady"
                }
                break
            }
        }

        nodeInfos = append(nodeInfos, NodeInfo{
            Name:     node.Name,
            Status:   status,
            Capacity: node.Status.Capacity,
            Labels:   node.Labels,
        })
    }

    // Process pods by namespace
    podsByNamespace := make(map[string]map[string]int)
    for _, pod := range pods.Items {
        ns := pod.Namespace
        if podsByNamespace[ns] == nil {
            podsByNamespace[ns] = make(map[string]int)
        }
        podsByNamespace[ns][string(pod.Status.Phase)]++
    }

    var podSummaries []PodSummary
    for ns, statusCounts := range podsByNamespace {
        podSummaries = append(podSummaries, PodSummary{
            Namespace: ns,
            Running:   statusCounts["Running"],
            Pending:   statusCounts["Pending"],
            Failed:    statusCounts["Failed"],
        })
    }

    return &ClusterMetrics{
        Timestamp:      time.Now(),
        NodeCount:      len(nodes.Items),
        PodCount:       len(pods.Items),
        NamespaceCount: len(namespaces.Items),
        Nodes:          nodeInfos,
        PodSummary:     podSummaries,
    }, nil
}
EOF

# Create go.mod dependencies
go mod tidy

# Build the monitoring tool
go build -o cluster-monitor

# Run the monitoring tool
./cluster-monitor

cd ../..
```

## Conclusion

This comprehensive CKA guide provides practical, terminal-based approaches to Kubernetes administration on AKS using the specified software versions. **The emphasis on troubleshooting aligns with the 2025 CKA exam structure, where troubleshooting comprises 30% of the assessment** - the largest single domain.

Key capabilities covered include advanced JSON manipulation with jq for cluster analysis, background job monitoring for continuous operations, and cron-based automation for production maintenance tasks. The integration of Grafana OSS 12 with AKS provides enterprise-grade monitoring capabilities, while Go 1.21 utilities offer custom tooling for specific operational needs.

The file organization strategies and environment variable templating techniques enable scalable cluster management workflows that can adapt to various production scenarios. All examples emphasize practical, exam-relevant scenarios while maintaining the terminal-focused, scriptable approach essential for CKA certification success.