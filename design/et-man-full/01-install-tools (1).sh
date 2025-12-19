#!/bin/bash
# scripts/ubuntu/01-install-tools.sh
# Install all required tools for EasyTrade deployment on Ubuntu 24.04

set -euo pipefail

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}=== Installing Tools for EasyTrade Deployment ===${NC}"

# Update package list
echo -e "${YELLOW}Updating package list...${NC}"
sudo apt-get update

# Install basic utilities
echo -e "${YELLOW}Installing basic utilities...${NC}"
sudo apt-get install -y \
    ca-certificates \
    curl \
    wget \
    gnupg \
    lsb-release \
    apt-transport-https \
    software-properties-common \
    git \
    jq \
    vim \
    unzip \
    make

# Install Azure CLI
if ! command -v az &> /dev/null; then
    echo -e "${YELLOW}Installing Azure CLI...${NC}"
    curl -sL https://aka.ms/InstallAzureCLIDeb | sudo bash
    echo -e "${GREEN}✓ Azure CLI installed${NC}"
else
    echo -e "${GREEN}✓ Azure CLI already installed${NC}"
    az version | head -1
fi

# Install kubectl and kubelogin via Azure CLI
if ! command -v kubectl &> /dev/null || ! command -v kubelogin &> /dev/null; then
    echo -e "${YELLOW}Installing kubectl and kubelogin...${NC}"
    sudo az aks install-cli
    echo -e "${GREEN}✓ kubectl and kubelogin installed${NC}"
else
    echo -e "${GREEN}✓ kubectl and kubelogin already installed${NC}"
fi

# Install Helm
if ! command -v helm &> /dev/null; then
    echo -e "${YELLOW}Installing Helm...${NC}"
    curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash
    echo -e "${GREEN}✓ Helm installed${NC}"
else
    echo -e "${GREEN}✓ Helm already installed${NC}"
    helm version --short
fi

# Install Docker
if ! command -v docker &> /dev/null; then
    echo -e "${YELLOW}Installing Docker...${NC}"
    
    # Add Docker's official GPG key
    sudo install -m 0755 -d /etc/apt/keyrings
    curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --dearmor -o /etc/apt/keyrings/docker.gpg
    sudo chmod a+r /etc/apt/keyrings/docker.gpg

    # Add the repository to Apt sources
    echo \
      "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu \
      $(. /etc/os-release && echo "$VERSION_CODENAME") stable" | \
      sudo tee /etc/apt/sources.list.d/docker.list > /dev/null
    
    sudo apt-get update
    sudo apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
    
    # Add current user to docker group
    sudo usermod -aG docker $USER
    
    echo -e "${GREEN}✓ Docker installed${NC}"
    echo -e "${YELLOW}Note: You may need to log out and back in for docker group membership to take effect${NC}"
else
    echo -e "${GREEN}✓ Docker already installed${NC}"
    docker --version
fi

# Verify installations
echo -e "\n${GREEN}=== Installation Summary ===${NC}"
echo -e "Azure CLI:   $(az version --output json | jq -r '.["azure-cli"]')"
echo -e "kubectl:     $(kubectl version --client --short 2>/dev/null || kubectl version --client | grep 'Client Version')"
echo -e "kubelogin:   $(kubelogin --version)"
echo -e "Helm:        $(helm version --short)"
echo -e "Docker:      $(docker --version)"
echo -e "Git:         $(git --version)"
echo -e "jq:          $(jq --version)"

echo -e "\n${GREEN}✓ All tools installed successfully!${NC}"
echo -e "${YELLOW}Next steps:${NC}"
echo -e "  1. Configure Azure: az login"
echo -e "  2. Get AKS credentials: make configure-aks"
echo -e "  3. Deploy EasyTrade: make deploy-manifests OR make deploy-helm"
