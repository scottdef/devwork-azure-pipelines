#!/bin/bash
# AKS Authentication Helper Script
# Handles Azure and AKS cluster authentication with kubelogin

set -euo pipefail

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Configuration
SUBSCRIPTION="${SUBSCRIPTION:-tango-CICD-platform-github-gitflow}"
RESOURCE_GROUP="${RESOURCE_GROUP:-prod-cus-platform-base-rg-001}"
CLUSTER_NAME="${CLUSTER_NAME:-prod-cus-aks-sre-lab-003}"

echo -e "${BLUE}=== AKS Authentication ===${NC}"

# Check if az is logged in
echo -e "${BLUE}Checking Azure login status...${NC}"
if ! az account show &>/dev/null; then
    echo -e "${YELLOW}Not logged in to Azure. Starting login...${NC}"
    az login
else
    CURRENT_ACCOUNT=$(az account show --query name -o tsv)
    echo -e "${GREEN}Already logged in to Azure: ${CURRENT_ACCOUNT}${NC}"
fi

# Set subscription
echo -e "${BLUE}Setting subscription...${NC}"
az account set --subscription "$SUBSCRIPTION"
SUBSCRIPTION_NAME=$(az account show --query name -o tsv)
echo -e "${GREEN}Using subscription: ${SUBSCRIPTION_NAME}${NC}"

# Get AKS credentials
echo -e "${BLUE}Getting AKS cluster credentials...${NC}"
az aks get-credentials \
    --resource-group "$RESOURCE_GROUP" \
    --name "$CLUSTER_NAME" \
    --overwrite-existing

# Convert kubeconfig for kubelogin
echo -e "${BLUE}Converting kubeconfig for kubelogin...${NC}"
kubelogin convert-kubeconfig -l azurecli

# Verify cluster access
echo -e "${BLUE}Verifying cluster access...${NC}"
if kubectl cluster-info &>/dev/null; then
    echo -e "${GREEN}Successfully connected to cluster!${NC}"
    echo ""
    echo -e "${GREEN}=== Cluster Information ===${NC}"
    kubectl cluster-info
    echo ""
    echo -e "${GREEN}=== Nodes ===${NC}"
    kubectl get nodes
else
    echo -e "${RED}Failed to connect to cluster${NC}"
    exit 1
fi

echo ""
echo -e "${GREEN}Authentication complete!${NC}"
echo -e "${BLUE}You can now run kubectl commands against the cluster.${NC}"
