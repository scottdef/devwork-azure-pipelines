#!/bin/bash
# =============================================================================
# SAMPLE KUBERNETES ADMINISTRATION SCRIPTS
# Place these in ./scripts/ directory before building
# =============================================================================

# -----------------------------------------------------------------------------
# File: scripts/check-aks-health.sh
# -----------------------------------------------------------------------------
cat > scripts/check-aks-health.sh << 'EOF'
#!/bin/bash
echo "=== AKS Cluster Health Check ==="

# Get cluster info
echo "Cluster Information:"
kubectl cluster-info

# Check node status
echo -e "\nNode Status:"
kubectl get nodes -o json | jq -r '.items[] | "\(.metadata.name): \(.status.conditions[] | select(.type=="Ready") | .status)"'

# Check system pods
echo -e "\nSystem Pods Status:"
kubectl get pods -n kube-system --field-selector=status.phase!=Running -o json | \
    jq -r '.items[] | "\(.metadata.name): \(.status.phase)"' || echo "All system pods running"

# Resource usage if metrics available
echo -e "\nResource Usage:"
kubectl top nodes 2>/dev/null || echo "Metrics server not available"

# Save health report
cat > /data/health-report-$(date +%Y%m%d-%H%M%S).json << EOJ
{
    "timestamp": "$(date -Iseconds)",
    "cluster_info": $(kubectl cluster-info dump --output-directory=/tmp/cluster-info 2>/dev/null && echo '{"status": "collected"}' || echo '{"status": "failed"}'),
    "nodes": $(kubectl get nodes -o json | jq '[.items[] | {name: .metadata.name, ready: (.status.conditions[] | select(.type=="Ready") | .status), version: .status.nodeInfo.kubeletVersion}]'),
    "unhealthy_pods": $(kubectl get pods -A --field-selector=status.phase!=Running -o json | jq '[.items[] | {name: .metadata.name, namespace: .metadata.namespace, phase: .status.phase}]')
}
EOJ

echo "Health report saved to /data/"
EOF

# -----------------------------------------------------------------------------
# File: scripts/monitor-deployments.sh
# -----------------------------------------------------------------------------
cat > scripts/monitor-deployments.sh << 'EOF'
#!/bin/bash
echo "=== Monitoring Deployments ==="

# Get all deployments across namespaces
kubectl get deployments -A -o json | jq '
    .items[] | {
        name: .metadata.name,
        namespace: .metadata.namespace,
        replicas: .spec.replicas,
        ready: .status.readyReplicas // 0,
        available: .status.availableReplicas // 0,
        health: (
            if (.status.readyReplicas // 0) == (.spec.replicas // 0)
            then "Healthy"
            else "Degraded"
            end
        )
    }' > /data/deployment-status.json

echo "Deployment status saved to /data/deployment-status.json"

# Show unhealthy deployments
echo -e "\nUnhealthy Deployments:"
cat /data/deployment-status.json | jq -r 'select(.health == "Degraded") | "\(.namespace)/\(.name): \(.ready)/\(.replicas)"'
EOF

# -----------------------------------------------------------------------------
# File: scripts/collect-logs.sh
# -----------------------------------------------------------------------------
cat > scripts/collect-logs.sh << 'EOF'
#!/bin/bash
NAMESPACE=${1:-default}
POD_NAME=${2}
LINES=${3:-100}

echo "=== Collecting Logs from $NAMESPACE ==="

if [ -z "$POD_NAME" ]; then
    # Collect logs from all pods in namespace
    kubectl get pods -n $NAMESPACE -o json | jq -r '.items[].metadata.name' | while read pod; do
        echo "Collecting logs for $pod..."
        kubectl logs -n $NAMESPACE $pod --tail=$LINES > /data/logs-${NAMESPACE}-${pod}-$(date +%Y%m%d-%H%M%S).log 2>&1
    done
else
    # Collect logs from specific pod
    kubectl logs -n $NAMESPACE $POD_NAME --tail=$LINES > /data/logs-${NAMESPACE}-${POD_NAME}-$(date +%Y%m%d-%H%M%S).log 2>&1
fi

echo "Logs collected in /data/"
EOF

# -----------------------------------------------------------------------------
# File: scripts/aks-api-query.sh
# -----------------------------------------------------------------------------
cat > scripts/aks-api-query.sh << 'EOF'
#!/bin/bash
echo "=== Querying AKS API Directly ==="

# Get API server URL
API_SERVER=$(kubectl config view --minify -o jsonpath='{.clusters[0].cluster.server}')
echo "API Server: $API_SERVER"

# Get service account token
TOKEN=$(kubectl get secret $(kubectl get serviceaccount default -o jsonpath='{.secrets[0].name}') -o jsonpath='{.data.token}' | base64 -d 2>/dev/null || echo "")

if [ -n "$TOKEN" ]; then
    # Query namespaces via API
    curl -s -k -H "Authorization: Bearer $TOKEN" \
        "$API_SERVER/api/v1/namespaces" | \
        jq '.items[] | {name: .metadata.name, status: .status.phase}' > /data/api-namespaces.json

    echo "Namespace data collected via API"
else
    echo "Using kubectl proxy for API queries..."
    # Alternative using kubectl proxy
    kubectl proxy --port=8080 &
    PROXY_PID=$!
    sleep 2

    curl -s http://localhost:8080/api/v1/namespaces | \
        jq '.items[] | {name: .metadata.name, status: .status.phase}' > /data/api-namespaces.json

    kill $PROXY_PID
fi

echo "API query results saved to /data/api-namespaces.json"
EOF

# -----------------------------------------------------------------------------
# File: scripts/azure-metadata.sh
# -----------------------------------------------------------------------------
cat > scripts/azure-metadata.sh << 'EOF'
#!/bin/bash
echo "=== Collecting Azure Instance Metadata ==="

# Check if running on Azure
if curl -s -H "Metadata: true" --connect-timeout 2 \
    "http://169.254.169.254/metadata/instance?api-version=2021-02-01" > /tmp/azure-metadata.json 2>/dev/null; then

    echo "Collecting Azure metadata..."

    # Parse and save key information
    cat /tmp/azure-metadata.json | jq '{
        resourceGroup: .compute.resourceGroupName,
        vmName: .compute.name,
        location: .compute.location,
        vmSize: .compute.vmSize,
        subscriptionId: .compute.subscriptionId,
        osType: .compute.osType,
        network: {
            privateIpAddress: .network.interface[0].ipv4.ipAddress[0].privateIpAddress,
            publicIpAddress: .network.interface[0].ipv4.ipAddress[0].publicIpAddress
        }
    }' > /data/azure-metadata.json

    echo "Azure metadata saved to /data/azure-metadata.json"

    # Get AKS specific information if available
    RESOURCE_GROUP=$(cat /tmp/azure-metadata.json | jq -r '.compute.resourceGroupName')
    echo "Resource Group: $RESOURCE_GROUP"

else
    echo "Not running on Azure or metadata service unavailable"
fi
EOF

# -----------------------------------------------------------------------------
# Make all scripts executable
# -----------------------------------------------------------------------------
chmod +x scripts/*.sh

echo "Sample scripts created in ./scripts/ directory:"
echo "- check-aks-health.sh: Comprehensive cluster health check"
echo "- monitor-deployments.sh: Monitor deployment status"
echo "- collect-logs.sh: Collect pod logs"
echo "- aks-api-query.sh: Direct Kubernetes API queries"
echo "- azure-metadata.sh: Collect Azure instance metadata"
echo ""
echo "Usage after container is running:"
echo "docker exec -it k8s-admin /scripts/check-aks-health.sh"
echo "docker exec -it k8s-admin /scripts/monitor-deployments.sh"
echo "docker exec -it k8s-admin /scripts/collect-logs.sh kube-system"
