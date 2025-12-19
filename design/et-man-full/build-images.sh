#!/bin/bash
# Build Custom EasyTrade Images and Push to ACR

set -euo pipefail

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Configuration
ACR_NAME="${ACR_NAME:-prod-central-image-repo}"
ACR_LOGIN_SERVER="${ACR_LOGIN_SERVER:-prodcentralimagerepo.azurecr.io}"
EASYTRADE_REPO="https://github.com/Dynatrace/easytrade.git"
WORK_DIR="/tmp/easytrade-build-$$"
CI_MODE="${1:-}"

# Services that can be built
BUILDABLE_SERVICES=(
    "accountservice"
    "broker-service"
    "contentcreator"
    "credit-card-order-service"
    "engine"
    "factory"
    "feature-flag-service"
    "frontend"
    "frontendreverseproxy"
    "loginservice"
    "manager"
    "offerservice"
    "pricingservice"
)

echo -e "${BLUE}=== EasyTrade Custom Image Build ===${NC}"

# Parse arguments
SPECIFIC_SERVICE=""
if [ -n "${1:-}" ] && [ "$1" != "--ci-mode" ]; then
    SPECIFIC_SERVICE="$1"
fi

# Clone repository
echo -e "${BLUE}Cloning EasyTrade repository...${NC}"
if [ -d "$WORK_DIR" ]; then
    rm -rf "$WORK_DIR"
fi

git clone "$EASYTRADE_REPO" "$WORK_DIR"
cd "$WORK_DIR"

GIT_HASH=$(git rev-parse --short HEAD)
BUILD_DATE=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

echo -e "${GREEN}Repository cloned. Commit: ${GIT_HASH}${NC}"

# Login to ACR
echo -e "${BLUE}Authenticating to ACR...${NC}"
az acr login --name "$ACR_NAME"

# Function to build a single service
build_service() {
    local SERVICE=$1
    local SRC_DIR="src/${SERVICE}"
    
    if [ ! -d "$SRC_DIR" ]; then
        echo -e "${YELLOW}Source directory not found: ${SRC_DIR}${NC}"
        return 1
    fi
    
    if [ ! -f "${SRC_DIR}/Dockerfile" ]; then
        echo -e "${YELLOW}Dockerfile not found for: ${SERVICE}${NC}"
        return 1
    fi
    
    echo -e "${BLUE}Building ${SERVICE}...${NC}"
    
    local IMAGE_NAME="${ACR_LOGIN_SERVER}/easytrade/${SERVICE}"
    local IMAGE_TAG_HASH="${IMAGE_NAME}:${GIT_HASH}"
    local IMAGE_TAG_LATEST="${IMAGE_NAME}:latest"
    
    if [ "$CI_MODE" == "--ci-mode" ]; then
        # Use ACR Tasks for CI/CD builds
        echo -e "${BLUE}Building on ACR (CI mode)...${NC}"
        az acr build \
            --registry "$ACR_NAME" \
            --image "easytrade/${SERVICE}:${GIT_HASH}" \
            --image "easytrade/${SERVICE}:latest" \
            --file "${SRC_DIR}/Dockerfile" \
            "$SRC_DIR"
    else
        # Local Docker build
        echo -e "${BLUE}Building locally with Docker...${NC}"
        docker build \
            --build-arg BUILD_DATE="$BUILD_DATE" \
            --build-arg VCS_REF="$GIT_HASH" \
            --tag "$IMAGE_TAG_HASH" \
            --tag "$IMAGE_TAG_LATEST" \
            --file "${SRC_DIR}/Dockerfile" \
            "$SRC_DIR"
        
        echo -e "${BLUE}Pushing ${SERVICE}...${NC}"
        docker push "$IMAGE_TAG_HASH"
        docker push "$IMAGE_TAG_LATEST"
    fi
    
    echo -e "${GREEN}✓ Successfully built and pushed: ${SERVICE}${NC}"
    return 0
}

# Build specific service or all services
if [ -n "$SPECIFIC_SERVICE" ]; then
    echo -e "${BLUE}Building specific service: ${SPECIFIC_SERVICE}${NC}"
    build_service "$SPECIFIC_SERVICE"
else
    echo -e "${BLUE}Building all services...${NC}"
    
    SUCCESSFUL=0
    FAILED=0
    
    for SERVICE in "${BUILDABLE_SERVICES[@]}"; do
        echo ""
        if build_service "$SERVICE"; then
            ((SUCCESSFUL++))
        else
            ((FAILED++))
        fi
    done
    
    # Summary
    echo ""
    echo -e "${BLUE}=== Build Summary ===${NC}"
    echo -e "${GREEN}Successful: ${SUCCESSFUL}${NC}"
    echo -e "${RED}Failed: ${FAILED}${NC}"
    
    if [ $FAILED -eq 0 ]; then
        echo ""
        echo -e "${GREEN}All images built and pushed successfully!${NC}"
    else
        echo ""
        echo -e "${YELLOW}Some builds failed. Check output above for details.${NC}"
    fi
fi

# Cleanup
cd /
rm -rf "$WORK_DIR"

echo ""
echo -e "${GREEN}Build complete!${NC}"
echo ""
echo -e "${BLUE}Verify images in ACR:${NC}"
echo -e "  az acr repository list --name ${ACR_NAME} --output table"
echo -e "  az acr repository show-tags --name ${ACR_NAME} --repository easytrade/broker-service"
