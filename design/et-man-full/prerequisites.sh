#!/bin/bash
# Prerequisites Check Script

set -euo pipefail

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m'

echo -e "${BLUE}=== Checking Prerequisites ===${NC}"
echo ""

ALL_GOOD=true

# Function to check command existence
check_command() {
    local CMD=$1
    local NAME=$2
    local VERSION_CMD=${3:-""}
    
    if command -v "$CMD" &>/dev/null; then
        if [ -n "$VERSION_CMD" ]; then
            VERSION=$(eval "$VERSION_CMD" 2>&1 | head -1)
            echo -e "${GREEN}✓ ${NAME}${NC} - ${VERSION}"
        else
            echo -e "${GREEN}✓ ${NAME}${NC}"
        fi
        return 0
    else
        echo -e "${RED}✗ ${NAME}${NC} - not found"
        ALL_GOOD=false
        return 1
    fi
}

# Check required tools
echo -e "${BLUE}Required Tools:${NC}"
check_command "az" "Azure CLI" "az --version | head -1"
check_command "kubectl" "kubectl" "kubectl version --client --short 2>/dev/null || kubectl version --client"
check_command "kubelogin" "kubelogin" "kubelogin --version"
check_command "helm" "Helm" "helm version --short"
check_command "docker" "Docker" "docker --version"
check_command "git" "Git" "git --version"
check_command "make" "Make" "make --version | head -1"
check_command "jq" "jq" "jq --version"
check_command "curl" "curl" "curl --version | head -1"
echo ""

# Check optional tools
echo -e "${BLUE}Optional Tools:${NC}"
check_command "terraform" "Terraform" "terraform --version | head -1" || echo -e "${YELLOW}  (Optional - needed only for infrastructure provisioning)${NC}"
check_command "gh" "GitHub CLI" "gh --version | head -1" || echo -e "${YELLOW}  (Optional - needed only for GitHub operations)${NC}"
echo ""

# Check Azure login status
echo -e "${BLUE}Azure Authentication:${NC}"
if az account show &>/dev/null; then
    ACCOUNT=$(az account show --query name -o tsv)
    echo -e "${GREEN}✓ Logged in to Azure${NC} - $ACCOUNT"
else
    echo -e "${YELLOW}⚠ Not logged in to Azure${NC}"
    echo -e "${YELLOW}  Run: az login${NC}"
    ALL_GOOD=false
fi
echo ""

# Check Docker daemon
echo -e "${BLUE}Docker Status:${NC}"
if docker ps &>/dev/null; then
    echo -e "${GREEN}✓ Docker daemon is running${NC}"
else
    echo -e "${YELLOW}⚠ Docker daemon is not running or not accessible${NC}"
    echo -e "${YELLOW}  You may need to: sudo systemctl start docker${NC}"
    echo -e "${YELLOW}  Or add your user to docker group: sudo usermod -aG docker \$USER${NC}"
fi
echo ""

# Check kubectl context
echo -e "${BLUE}Kubernetes Context:${NC}"
if kubectl cluster-info &>/dev/null; then
    CONTEXT=$(kubectl config current-context)
    echo -e "${GREEN}✓ Connected to cluster${NC} - $CONTEXT"
    
    # Check if it's the target cluster
    if [[ "$CONTEXT" == *"prod-cus-aks-sre-lab-003"* ]]; then
        echo -e "${GREEN}✓ Connected to target AKS cluster${NC}"
    else
        echo -e "${YELLOW}⚠ Not connected to target cluster (prod-cus-aks-sre-lab-003)${NC}"
        echo -e "${YELLOW}  Run: make auth${NC}"
    fi
else
    echo -e "${YELLOW}⚠ No active kubectl context${NC}"
    echo -e "${YELLOW}  Run: make auth${NC}"
fi
echo ""

# Check if .env file exists
echo -e "${BLUE}Configuration:${NC}"
if [ -f .env ]; then
    echo -e "${GREEN}✓ .env file exists${NC}"
    source .env
    echo -e "  SUBSCRIPTION: ${SUBSCRIPTION:-not set}"
    echo -e "  CLUSTER_NAME: ${CLUSTER_NAME:-not set}"
    echo -e "  ACR_NAME: ${ACR_NAME:-not set}"
else
    echo -e "${YELLOW}⚠ .env file not found${NC}"
    echo -e "${YELLOW}  Copy .env.example to .env and configure:${NC}"
    echo -e "${YELLOW}  cp .env.example .env${NC}"
fi
echo ""

# Summary
echo -e "${BLUE}=== Summary ===${NC}"
if [ "$ALL_GOOD" = true ]; then
    echo -e "${GREEN}✓ All prerequisites met!${NC}"
    echo ""
    echo -e "${BLUE}Next steps:${NC}"
    echo -e "  1. Configure environment:    cp .env.example .env && vim .env"
    echo -e "  2. Authenticate to AKS:      make auth"
    echo -e "  3. Deploy EasyTrade:         make deploy-existing"
    exit 0
else
    echo -e "${YELLOW}⚠ Some prerequisites are missing${NC}"
    echo ""
    echo -e "${BLUE}Installation guides:${NC}"
    echo -e "  Ubuntu:  ./setup/ubuntu-setup.sh"
    echo -e "  Windows: ./setup/windows-setup.ps1"
    echo ""
    echo -e "Or install manually: https://docs.microsoft.com/en-us/azure/aks/kubernetes-walkthrough"
    exit 1
fi
