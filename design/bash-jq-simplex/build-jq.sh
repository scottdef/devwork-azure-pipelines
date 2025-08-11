#!/bin/bash

# =============================================================================
# BUILD AND RUN COMMANDS FOR KUBERNETES ADMIN CONTAINER
# =============================================================================

# 1. BUILD THE DOCKERFILE
echo "Building Docker image..."
docker build -t k8s-admin-tools:latest .

# Alternative build with build args if needed
docker build \
    --build-arg GO_VERSION=1.21.12 \
    -t k8s-admin-tools:latest .

# =============================================================================
# 2. RUN CONTAINER WITH DATA VOLUME MOUNT
# =============================================================================

# Create local data directory if it doesn't exist
mkdir -p ./data
mkdir -p ./scripts

# Run container with data volume mounted
docker run -it \
    --name k8s-admin \
    -v $(pwd)/data:/data \
    -v $(pwd)/scripts:/scripts \
    -v ~/.kube:/root/.kube \
    --network host \
    k8s-admin-tools:latest

# Run container in detached mode for background operations
docker run -d \
    --name k8s-admin-daemon \
    -v $(pwd)/data:/data \
    -v $(pwd)/scripts:/scripts \
    -v ~/.kube:/root/.kube \
    --network host \
    k8s-admin-tools:latest \
    tail -f /dev/null

# =============================================================================
# 3. EXECUTE COMMANDS IN RUNNING CONTAINER
# =============================================================================

# Execute curl and jq operations
docker exec -it k8s-admin bash -c "
    # Example: Query Kubernetes API and process with jq
    kubectl get nodes -o json | jq '.items[].metadata.name'

    # Example: Curl AKS metadata and process
    curl -s -H 'Metadata: true' \
        'http://169.254.169.254/metadata/instance?api-version=2021-02-01' \
        | jq '.compute.resourceGroupName'

    # Save processed data to volume
    kubectl get pods --all-namespaces -o json \
        | jq '[.items[] | {name: .metadata.name, namespace: .metadata.namespace, status: .status.phase}]' \
        > /data/pod-status.json
"

# =============================================================================
# 4. SAMPLE KUBERNETES ADMIN OPERATIONS
# =============================================================================

# Check AKS cluster health
docker exec -it k8s-admin bash -c "
    echo 'Checking AKS cluster health...'
    kubectl cluster-info
    kubectl get componentstatuses
    kubectl top nodes 2>/dev/null || echo 'Metrics server not available'
"

# Monitor cluster resources with jq processing
docker exec -it k8s-admin bash -c "
    echo 'Gathering cluster resource information...'
    kubectl get all --all-namespaces -o json \
        | jq '{
            total_pods: [.items[] | select(.kind==\"Pod\")] | length,
            running_pods: [.items[] | select(.kind==\"Pod\" and .status.phase==\"Running\")] | length,
            services: [.items[] | select(.kind==\"Service\")] | length,
            deployments: [.items[] | select(.kind==\"Deployment\")] | length
        }' > /data/cluster-summary.json

    cat /data/cluster-summary.json
"

# Continuous monitoring script
docker exec -d k8s-admin bash -c "
    while true; do
        echo \"\$(date): Checking cluster status\" >> /data/monitoring.log
        kubectl get pods --field-selector=status.phase!=Running -A \
            | grep -v 'No resources found' >> /data/failed-pods.log 2>/dev/null || true
        sleep 300
    done
"

# =============================================================================
# 5. GRAFANA-OSS INTEGRATION PREPARATION
# =============================================================================

# Prepare for Grafana OSS 12 deployment
docker exec -it k8s-admin bash -c "
    # Create namespace for monitoring
    kubectl create namespace monitoring --dry-run=client -o yaml > /data/monitoring-namespace.yaml

    # Generate Grafana deployment manifests
    cat > /data/grafana-deployment.yaml << 'EOF'
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
          value: admin123
        volumeMounts:
        - name: grafana-storage
          mountPath: /var/lib/grafana
      volumes:
      - name: grafana-storage
        emptyDir: {}
EOF
"

# =============================================================================
# 6. CLEANUP COMMANDS
# =============================================================================

# Stop and remove container
# docker stop k8s-admin && docker rm k8s-admin

# Remove image
# docker rmi k8s-admin-tools:latest

# Clean up data (BE CAREFUL!)
# rm -rf ./data/*

echo "Setup complete! Use the commands above to build and run your K8s admin container."
