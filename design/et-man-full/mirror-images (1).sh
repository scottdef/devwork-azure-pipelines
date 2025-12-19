#!/bin/bash
# scripts/mirror-images.sh
# Mirror EasyTrade images from upstream to private ACR

set -euo pipefail

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

# Configuration
ACR_NAME="${ACR_NAME:-prod-central-image-repo}"
ACR_LOGIN_SERVER="${ACR_LOGIN_SERVER:-prodcentralimagerepo.azurecr.io}"
SOURCE_REGISTRY="europe-docker.pkg.dev/dynatrace-demoability/docker/easytrade"
TARGET_REGISTRY="${ACR_LOGIN_SERVER}/easytrade"

# EasyTrade services to mirror
SERVICES=(
    "accountservice"
    "broker-service"
    "contentcreator"
    "credit-card-order-service"
    "engine"
    "factory"
    "feature-flag-service"
    "frontend"
    "frontendreverseproxy"
    "headless-loadgen"
    "loginservice"
    "manager"
    "offerservice"
    "pricingservice"
    "thirdpartyservice"
)

echo -e "${GREEN}=== EasyTrade Image Mirroring to ACR ===${NC}"
echo -e "Source: ${YELLOW}${SOURCE_REGISTRY}${NC}"
echo -e "Target: ${YELLOW}${TARGET_REGISTRY}${NC}"
echo -e "Services: ${#SERVICES[@]}\n"

# Login to ACR
echo -e "${YELLOW}Logging into ACR...${NC}"
az acr login --name "${ACR_NAME}"

if [ $? -ne 0 ]; then
    echo -e "${RED}Error: Failed to login to ACR${NC}"
    exit 1
fi

echo -e "${GREEN}✓ Logged into ACR${NC}\n"

# Import images
SUCCESS_COUNT=0
FAILED_COUNT=0
FAILED_SERVICES=()

for SERVICE in "${SERVICES[@]}"; do
    echo -e "${CYAN}Importing ${SERVICE}...${NC}"
    
    az acr import \
        --name "${ACR_NAME}" \
        --source "${SOURCE_REGISTRY}/${SERVICE}:latest" \
        --image "easytrade/${SERVICE}:latest" \
        --force \
        2>&1 | grep -v "WARNING" || true
    
    if [ ${PIPESTATUS[0]} -eq 0 ]; then
        echo -e "${GREEN}✓ ${SERVICE} imported successfully${NC}"
        ((SUCCESS_COUNT++))
    else
        echo -e "${RED}✗ ${SERVICE} import failed${NC}"
        ((FAILED_COUNT++))
        FAILED_SERVICES+=("${SERVICE}")
    fi
    echo ""
done

# Summary
echo -e "${GREEN}=== Import Summary ===${NC}"
echo -e "Successful: ${GREEN}${SUCCESS_COUNT}${NC}"
echo -e "Failed: ${RED}${FAILED_COUNT}${NC}"

if [ ${FAILED_COUNT} -gt 0 ]; then
    echo -e "\n${YELLOW}Failed services:${NC}"
    for SERVICE in "${FAILED_SERVICES[@]}"; do
        echo -e "  ${RED}✗ ${SERVICE}${NC}"
    done
fi

# List imported images
echo -e "\n${YELLOW}Imported images in ACR:${NC}"
az acr repository list --name "${ACR_NAME}" --output table | grep easytrade || true

# Tag with git commit if in git repo
if git rev-parse --git-dir > /dev/null 2>&1; then
    GIT_HASH=$(git rev-parse --short HEAD)
    echo -e "\n${YELLOW}Tagging images with git hash: ${GIT_HASH}${NC}"
    
    for SERVICE in "${SERVICES[@]}"; do
        # Tag with git hash
        az acr import \
            --name "${ACR_NAME}" \
            --source "${SOURCE_REGISTRY}/${SERVICE}:latest" \
            --image "easytrade/${SERVICE}:${GIT_HASH}" \
            --force \
            2>&1 | grep -v "WARNING" || true
    done
    
    echo -e "${GREEN}✓ Images tagged with ${GIT_HASH}${NC}"
fi

echo -e "\n${GREEN}✓ Image mirroring complete!${NC}"
echo -e "${YELLOW}Next steps:${NC}"
echo -e "  1. Verify images: az acr repository list --name ${ACR_NAME} --output table"
echo -e "  2. Deploy EasyTrade: make deploy-manifests USE_ACR_IMAGES=true"
