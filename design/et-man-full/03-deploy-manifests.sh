#!/bin/bash
# scripts/ubuntu/03-deploy-manifests.sh
# Deploy EasyTrade using raw Kubernetes manifests

set -euo pipefail

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

# Configuration
NAMESPACE="${EASYTRADE_NAMESPACE:-easytrade}"
MANIFESTS_DIR="./kubernetes"
USE_ACR=false
ACR_LOGIN_SERVER="${ACR_LOGIN_SERVER:-prodcentralimagerepo.azurecr.io}"

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        --use-acr)
            USE_ACR=true
            shift
            ;;
        --namespace)
            NAMESPACE="$2"
            shift 2
            ;;
        *)
            echo -e "${RED}Unknown option: $1${NC}"
            exit 1
            ;;
    esac
done

echo -e "${GREEN}=== Deploying EasyTrade with Kubernetes Manifests ===${NC}"
echo -e "Namespace: ${YELLOW}${NAMESPACE}${NC}"
echo -e "Use ACR: ${YELLOW}${USE_ACR}${NC}"

# Create namespace
echo -e "${YELLOW}Creating namespace...${NC}"
kubectl create namespace ${NAMESPACE} --dry-run=client -o yaml | kubectl apply -f -

# Label for Istio injection (if Istio is available)
kubectl label namespace ${NAMESPACE} istio-injection=enabled --overwrite 2>/dev/null || true

# Deploy core resources
echo -e "${YELLOW}Deploying ConfigMap...${NC}"
kubectl -n ${NAMESPACE} apply -f ${MANIFESTS_DIR}/configmap.yaml

echo -e "${YELLOW}Deploying Database...${NC}"
kubectl -n ${NAMESPACE} apply -f ${MANIFESTS_DIR}/database.yaml

# Wait for database to be ready
echo -e "${YELLOW}Waiting for database to be ready...${NC}"
kubectl -n ${NAMESPACE} wait --for=condition=ready pod -l app=db --timeout=300s

# Deploy services
echo -e "${YELLOW}Deploying services...${NC}"
for SERVICE_FILE in ${MANIFESTS_DIR}/services/*.yaml; do
    if [ -f "$SERVICE_FILE" ]; then
        SERVICE_NAME=$(basename "$SERVICE_FILE" .yaml)
        echo -e "  Deploying ${SERVICE_NAME}..."
        
        if [ "$USE_ACR" = true ]; then
            # Replace image registry with ACR
            sed "s|europe-docker.pkg.dev/dynatrace-demoability/docker/easytrade|${ACR_LOGIN_SERVER}/easytrade|g" "$SERVICE_FILE" | \
                kubectl -n ${NAMESPACE} apply -f -
        else
            kubectl -n ${NAMESPACE} apply -f "$SERVICE_FILE"
        fi
    fi
done

# Deploy problem patterns (optional CronJobs)
if [ -f "${MANIFESTS_DIR}/problem-patterns/problem-patterns.yaml" ]; then
    echo -e "${YELLOW}Deploying problem patterns...${NC}"
    kubectl -n ${NAMESPACE} apply -f ${MANIFESTS_DIR}/problem-patterns/problem-patterns.yaml
fi

# Wait for all deployments to be ready
echo -e "${YELLOW}Waiting for all deployments to be ready...${NC}"
kubectl -n ${NAMESPACE} wait --for=condition=available --timeout=600s deployment --all

# Display deployment status
echo -e "\n${GREEN}=== Deployment Status ===${NC}"
kubectl -n ${NAMESPACE} get pods -o wide
echo ""
kubectl -n ${NAMESPACE} get svc

echo -e "\n${GREEN}✓ EasyTrade deployed successfully!${NC}"
echo -e "${YELLOW}Access options:${NC}"
echo -e "  1. Port-forward: make setup-portforward"
echo -e "  2. LoadBalancer: make setup-loadbalancer"
echo -e "  3. Check access info: make get-access-info"
