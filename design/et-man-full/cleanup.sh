#!/bin/bash
# EasyTrade Cleanup Script
# Removes all EasyTrade resources from the AKS cluster

set -euo pipefail

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Configuration
NAMESPACE="${NAMESPACE:-easytrade}"
HELM_RELEASE="${HELM_RELEASE:-easytrade}"

echo -e "${RED}=== EasyTrade Cleanup ===${NC}"
echo ""
echo -e "${RED}WARNING: This will delete all EasyTrade resources!${NC}"
echo -e "${RED}Namespace: ${NAMESPACE}${NC}"
echo ""

# Confirmation
read -p "Are you sure you want to continue? Type 'yes' to confirm: " CONFIRM

if [ "$CONFIRM" != "yes" ]; then
    echo -e "${YELLOW}Cleanup cancelled${NC}"
    exit 0
fi

echo ""
echo -e "${BLUE}Starting cleanup...${NC}"

# Check if Helm release exists
if helm list -n "$NAMESPACE" | grep -q "$HELM_RELEASE"; then
    echo -e "${BLUE}Uninstalling Helm release: ${HELM_RELEASE}${NC}"
    helm uninstall "$HELM_RELEASE" -n "$NAMESPACE"
    echo -e "${GREEN}✓ Helm release uninstalled${NC}"
else
    echo -e "${YELLOW}No Helm release found${NC}"
fi

# Delete any remaining resources
echo -e "${BLUE}Deleting remaining resources in namespace...${NC}"

# Delete deployments
if kubectl -n "$NAMESPACE" get deployments &>/dev/null; then
    kubectl -n "$NAMESPACE" delete deployments --all --wait=true
    echo -e "${GREEN}✓ Deployments deleted${NC}"
fi

# Delete statefulsets
if kubectl -n "$NAMESPACE" get statefulsets &>/dev/null; then
    kubectl -n "$NAMESPACE" delete statefulsets --all --wait=true
    echo -e "${GREEN}✓ StatefulSets deleted${NC}"
fi

# Delete services
if kubectl -n "$NAMESPACE" get services &>/dev/null; then
    kubectl -n "$NAMESPACE" delete services --all
    echo -e "${GREEN}✓ Services deleted${NC}"
fi

# Delete PVCs
if kubectl -n "$NAMESPACE" get pvc &>/dev/null; then
    kubectl -n "$NAMESPACE" delete pvc --all --wait=true
    echo -e "${GREEN}✓ PVCs deleted${NC}"
fi

# Delete ConfigMaps (excluding kube-root-ca.crt)
if kubectl -n "$NAMESPACE" get configmaps &>/dev/null; then
    kubectl -n "$NAMESPACE" delete configmaps --all --ignore-not-found=true
    echo -e "${GREEN}✓ ConfigMaps deleted${NC}"
fi

# Delete Secrets (excluding default token)
if kubectl -n "$NAMESPACE" get secrets &>/dev/null; then
    kubectl -n "$NAMESPACE" delete secrets --all --ignore-not-found=true
    echo -e "${GREEN}✓ Secrets deleted${NC}"
fi

# Delete Istio resources if they exist
if kubectl get gateway -n "$NAMESPACE" &>/dev/null; then
    echo -e "${BLUE}Deleting Istio resources...${NC}"
    kubectl -n "$NAMESPACE" delete gateway --all --ignore-not-found=true
    kubectl -n "$NAMESPACE" delete virtualservice --all --ignore-not-found=true
    kubectl -n "$NAMESPACE" delete destinationrule --all --ignore-not-found=true
    kubectl -n "$NAMESPACE" delete peerauthentication --all --ignore-not-found=true
    echo -e "${GREEN}✓ Istio resources deleted${NC}"
fi

# Optionally delete the namespace
echo ""
read -p "Delete the namespace '${NAMESPACE}' as well? (y/N): " DELETE_NS

if [[ "$DELETE_NS" =~ ^[Yy]$ ]]; then
    echo -e "${BLUE}Deleting namespace: ${NAMESPACE}${NC}"
    kubectl delete namespace "$NAMESPACE" --wait=true
    echo -e "${GREEN}✓ Namespace deleted${NC}"
else
    echo -e "${YELLOW}Namespace retained${NC}"
fi

# Optionally delete images from ACR
echo ""
read -p "Delete EasyTrade images from ACR? (y/N): " DELETE_IMAGES

if [[ "$DELETE_IMAGES" =~ ^[Yy]$ ]]; then
    echo -e "${BLUE}Deleting images from ACR...${NC}"
    ACR_NAME="${ACR_NAME:-prod-central-image-repo}"
    
    # List and delete repositories
    REPOS=$(az acr repository list --name "$ACR_NAME" --query "[?starts_with(@, 'easytrade/')]" -o tsv)
    
    for REPO in $REPOS; do
        echo -e "${BLUE}Deleting repository: ${REPO}${NC}"
        az acr repository delete --name "$ACR_NAME" --repository "$REPO" --yes
    done
    
    echo -e "${GREEN}✓ ACR images deleted${NC}"
else
    echo -e "${YELLOW}ACR images retained${NC}"
fi

echo ""
echo -e "${GREEN}=== Cleanup Complete ===${NC}"
echo ""
echo -e "${BLUE}To redeploy:${NC}"
echo -e "  make deploy-existing     # Deploy with existing images"
echo -e "  make deploy-custom       # Deploy with custom-built images"
