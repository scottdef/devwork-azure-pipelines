#!/bin/bash
# scripts/ubuntu/01-prerequisites.sh
# Install all required tools on Ubuntu 24.04

set -euo pipefail

echo "=== Installing Prerequisites for Kubernetes Dashboard Deployment ==="

# Update package manager
sudo apt-get update

# Install basic tools
echo "Installing basic tools..."
sudo apt-get install -y \
    curl \
    wget \
    git \
    jq \
    make \
    build-essential \
    ca-certificates \
    gnupg \
    lsb-release

# Install Azure CLI
echo "Installing Azure CLI..."
if ! command -v az &> /dev/null; then
    curl -sL https://aka.ms/InstallAzureCLIDeb | sudo bash
else
    echo "Azure CLI already installed"
fi

# Install Docker
echo "Installing Docker..."
if ! command -v docker &> /dev/null; then
    sudo apt-get install -y docker.io
    sudo systemctl start docker
    sudo systemctl enable docker
    sudo usermod -aG docker $USER
    echo "Docker installed. You may need to log out and back in for group changes to take effect."
else
    echo "Docker already installed"
fi

# Install kubectl and kubelogin via Azure CLI
echo "Installing kubectl and kubelogin..."
sudo az aks install-cli

# Install Helm
echo "Installing Helm..."
if ! command -v helm &> /dev/null; then
    curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash
else
    echo "Helm already installed"
fi

# Verify installations
echo -e "\n=== Verification ==="
echo "Azure CLI: $(az version --output tsv --query '"azure-cli"')"
echo "kubectl: $(kubectl version --client -o json | jq -r '.clientVersion.gitVersion')"
echo "kubelogin: $(kubelogin --version)"
echo "Helm: $(helm version --short)"
echo "Docker: $(docker --version)"
echo "Git: $(git --version)"
echo "jq: $(jq --version)"

echo -e "\n=== Prerequisites Installation Complete ==="
echo "If you installed Docker for the first time, please log out and back in."
