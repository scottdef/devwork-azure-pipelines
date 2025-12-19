#!/bin/bash
# Deploy EasyTrade using Kubernetes Manifests

set -euo pipefail

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Configuration
NAMESPACE="${NAMESPACE:-easytrade}"
ACR_LOGIN_SERVER="${ACR_LOGIN_SERVER:-prodcentralimagerepo.azurecr.io}"
UPSTREAM_REGISTRY="europe-docker.pkg.dev/dynatrace-demoability/docker/easytrade"
EASYTRADE_REPO="https://github.com/Dynatrace/easytrade.git"
WORK_DIR="/tmp/easytrade-deploy-$$"

echo -e "${BLUE}=== EasyTrade Manifest Deployment ===${NC}"

# Clone EasyTrade repository
echo -e "${BLUE}Cloning EasyTrade repository...${NC}"
if [ -d "$WORK_DIR" ]; then
    rm -rf "$WORK_DIR"
fi

git clone "$EASYTRADE_REPO" "$WORK_DIR"
cd "$WORK_DIR"

# Create namespace
echo -e "${BLUE}Creating namespace...${NC}"
kubectl create namespace "$NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -

# Patch manifests to use private ACR
echo -e "${BLUE}Patching manifests to use ${ACR_LOGIN_SERVER}...${NC}"

find kubernetes-manifests/release -name "*.yaml" -type f -exec \
    sed -i "s|${UPSTREAM_REGISTRY}|${ACR_LOGIN_SERVER}/easytrade|g" {} \;

# Apply manifests
echo -e "${BLUE}Applying manifests...${NC}"
kubectl -n "$NAMESPACE" apply -f kubernetes-manifests/release/

# Wait for deployments
echo -e "${BLUE}Waiting for deployments to be ready...${NC}"
echo -e "${YELLOW}This may take several minutes...${NC}"

TIMEOUT=300
ELAPSED=0
while [ $ELAPSED -lt $TIMEOUT ]; do
    READY=$(kubectl -n "$NAMESPACE" get deployments -o jsonpath='{.items[*].status.readyReplicas}' | wc -w)
    TOTAL=$(kubectl -n "$NAMESPACE" get deployments -o jsonpath='{.items[*].metadata.name}' | wc -w)
    
    echo -e "${BLUE}Ready: ${READY}/${TOTAL} deployments${NC}"
    
    if [ "$READY" -eq "$TOTAL" ]; then
        echo -e "${GREEN}All deployments ready!${NC}"
        break
    fi
    
    sleep 10
    ELAPSED=$((ELAPSED + 10))
done

if [ $ELAPSED -ge $TIMEOUT ]; then
    echo -e "${YELLOW}Warning: Deployment timeout. Some pods may still be starting.${NC}"
fi

# Display status
echo ""
echo -e "${GREEN}=== Deployment Status ===${NC}"
kubectl -n "$NAMESPACE" get pods
echo ""
kubectl -n "$NAMESPACE" get svc

# Cleanup
cd /
rm -rf "$WORK_DIR"

echo ""
echo -e "${GREEN}Deployment complete!${NC}"
echo ""
echo -e "${BLUE}Next steps:${NC}"
echo -e "  1. Verify deployment:  make verify"
echo -e "  2. Access application: make port-forward"
echo -e "  3. Enable problems:    make problem-enable PATTERN=high_cpu_usage"
