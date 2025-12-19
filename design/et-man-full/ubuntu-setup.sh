#!/bin/bash
# Ubuntu 24.04 Setup Script for EasyTrade AKS Deployment
# This script installs all required tools for deploying EasyTrade to AKS

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Logging functions
log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check if running on Ubuntu
check_os() {
    if [[ ! -f /etc/os-release ]]; then
        log_error "Cannot determine OS. This script is for Ubuntu 24.04."
        exit 1
    fi
    
    . /etc/os-release
    if [[ "$ID" != "ubuntu" ]]; then
        log_error "This script is designed for Ubuntu. Detected: $ID"
        exit 1
    fi
    
    log_success "Running on Ubuntu $VERSION_ID"
}

# Update system packages
update_system() {
    log_info "Updating system packages..."
    sudo apt-get update
    sudo apt-get upgrade -y
    sudo apt-get install -y \
        ca-certificates \
        curl \
        gnupg \
        lsb-release \
        software-properties-common \
        apt-transport-https \
        wget \
        jq \
        git \
        make \
        build-essential \
        unzip
    log_success "System packages updated"
}

# Install Azure CLI
install_azure_cli() {
    log_info "Installing Azure CLI..."
    
    if command -v az &> /dev/null; then
        log_warning "Azure CLI already installed: $(az --version | head -1)"
        return 0
    fi
    
    curl -sL https://aka.ms/InstallAzureCLIDeb | sudo bash
    
    # Verify installation
    if command -v az &> /dev/null; then
        log_success "Azure CLI installed: $(az --version | head -1)"
    else
        log_error "Azure CLI installation failed"
        return 1
    fi
}

# Install kubectl via Azure CLI
install_kubectl() {
    log_info "Installing kubectl..."
    
    if command -v kubectl &> /dev/null; then
        log_warning "kubectl already installed: $(kubectl version --client --short 2>/dev/null || kubectl version --client)"
        return 0
    fi
    
    sudo az aks install-cli
    
    # Verify installation
    if command -v kubectl &> /dev/null; then
        log_success "kubectl installed: $(kubectl version --client --short 2>/dev/null || kubectl version --client)"
    else
        log_error "kubectl installation failed"
        return 1
    fi
}

# Install kubelogin
install_kubelogin() {
    log_info "Installing kubelogin..."
    
    if command -v kubelogin &> /dev/null; then
        log_warning "kubelogin already installed: $(kubelogin --version)"
        return 0
    fi
    
    # kubelogin is installed with az aks install-cli
    sudo az aks install-cli
    
    # Verify installation
    if command -v kubelogin &> /dev/null; then
        log_success "kubelogin installed: $(kubelogin --version)"
    else
        log_error "kubelogin installation failed"
        return 1
    fi
}

# Install Helm
install_helm() {
    log_info "Installing Helm 3..."
    
    if command -v helm &> /dev/null; then
        log_warning "Helm already installed: $(helm version --short)"
        return 0
    fi
    
    curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash
    
    # Verify installation
    if command -v helm &> /dev/null; then
        log_success "Helm installed: $(helm version --short)"
    else
        log_error "Helm installation failed"
        return 1
    fi
}

# Install Docker
install_docker() {
    log_info "Installing Docker..."
    
    if command -v docker &> /dev/null; then
        log_warning "Docker already installed: $(docker --version)"
        return 0
    fi
    
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
    
    log_success "Docker installed: $(docker --version)"
    log_warning "You may need to log out and back in for docker group membership to take effect"
}

# Install Terraform
install_terraform() {
    log_info "Installing Terraform..."
    
    if command -v terraform &> /dev/null; then
        log_warning "Terraform already installed: $(terraform --version | head -1)"
        return 0
    fi
    
    wget -O- https://apt.releases.hashicorp.com/gpg | sudo gpg --dearmor -o /usr/share/keyrings/hashicorp-archive-keyring.gpg
    echo "deb [signed-by=/usr/share/keyrings/hashicorp-archive-keyring.gpg] https://apt.releases.hashicorp.com $(lsb_release -cs) main" | sudo tee /etc/apt/sources.list.d/hashicorp.list
    sudo apt-get update
    sudo apt-get install -y terraform
    
    # Verify installation
    if command -v terraform &> /dev/null; then
        log_success "Terraform installed: $(terraform --version | head -1)"
    else
        log_error "Terraform installation failed"
        return 1
    fi
}

# Install GitHub CLI
install_gh_cli() {
    log_info "Installing GitHub CLI..."
    
    if command -v gh &> /dev/null; then
        log_warning "GitHub CLI already installed: $(gh --version | head -1)"
        return 0
    fi
    
    type -p curl >/dev/null || (sudo apt update && sudo apt install curl -y)
    curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg | sudo dd of=/usr/share/keyrings/githubcli-archive-keyring.gpg
    sudo chmod go+r /usr/share/keyrings/githubcli-archive-keyring.gpg
    echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" | sudo tee /etc/apt/sources.list.d/github-cli.list > /dev/null
    sudo apt update
    sudo apt install gh -y
    
    # Verify installation
    if command -v gh &> /dev/null; then
        log_success "GitHub CLI installed: $(gh --version | head -1)"
    else
        log_error "GitHub CLI installation failed"
        return 1
    fi
}

# Create directory structure
create_directories() {
    log_info "Creating directory structure..."
    mkdir -p ~/easytrade-aks-deployment/{scripts,manifests,helm-chart,terraform,docs}
    log_success "Directory structure created"
}

# Verify all installations
verify_installations() {
    log_info "Verifying all installations..."
    
    local all_good=true
    
    declare -A tools=(
        ["az"]="Azure CLI"
        ["kubectl"]="kubectl"
        ["kubelogin"]="kubelogin"
        ["helm"]="Helm"
        ["docker"]="Docker"
        ["terraform"]="Terraform"
        ["git"]="Git"
        ["make"]="Make"
        ["jq"]="jq"
        ["gh"]="GitHub CLI"
    )
    
    for cmd in "${!tools[@]}"; do
        if command -v "$cmd" &> /dev/null; then
            echo -e "${GREEN}✓${NC} ${tools[$cmd]}"
        else
            echo -e "${RED}✗${NC} ${tools[$cmd]}"
            all_good=false
        fi
    done
    
    if [ "$all_good" = true ]; then
        log_success "All tools installed successfully!"
    else
        log_error "Some tools failed to install. Please check the output above."
        return 1
    fi
}

# Display next steps
show_next_steps() {
    echo ""
    log_success "Setup complete!"
    echo ""
    echo "Next steps:"
    echo "1. Log out and back in to activate Docker group membership:"
    echo "   exit"
    echo ""
    echo "2. Authenticate to Azure:"
    echo "   az login"
    echo "   az account set --subscription 'tango-CICD-platform-github-gitflow'"
    echo ""
    echo "3. Clone the EasyTrade deployment repository:"
    echo "   git clone <your-repo-url> ~/easytrade-aks-deployment"
    echo "   cd ~/easytrade-aks-deployment"
    echo ""
    echo "4. Configure environment:"
    echo "   cp .env.example .env"
    echo "   # Edit .env with your values"
    echo "   source .env"
    echo ""
    echo "5. Authenticate to AKS cluster:"
    echo "   make auth"
    echo ""
    echo "6. Deploy EasyTrade:"
    echo "   make deploy-existing     # Use existing images"
    echo "   # OR"
    echo "   make deploy-custom       # Build custom images"
    echo ""
    echo "7. Access the application:"
    echo "   make port-forward        # kubectl port-forward to localhost:8080"
    echo ""
}

# Main installation function
main() {
    log_info "Starting Ubuntu 24.04 setup for EasyTrade AKS deployment..."
    
    check_os
    update_system
    install_azure_cli
    install_kubectl
    install_kubelogin
    install_helm
    install_docker
    install_terraform
    install_gh_cli
    create_directories
    verify_installations
    show_next_steps
}

# Run main function
main "$@"
