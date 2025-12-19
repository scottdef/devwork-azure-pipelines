# EasyTrade AKS Deployment Guide

Complete deployment solution for Dynatrace EasyTrade on private AKS clusters.

## Directory Structure

```
easytrade-deployment/
├── README.md                           # This file
├── Makefile                            # Automation for Ubuntu/Linux
├── terraform/                          # Infrastructure provisioning
│   ├── main.tf
│   ├── variables.tf
│   ├── outputs.tf
│   └── terraform.tfvars.example
├── kubernetes/                         # Raw Kubernetes manifests
│   ├── namespace.yaml
│   ├── configmap.yaml
│   ├── secrets.yaml
│   ├── database.yaml
│   ├── services/                       # Individual service deployments
│   └── ingress/                        # LoadBalancer and Istio configs
├── helm/                               # Helm chart
│   └── easytrade/
│       ├── Chart.yaml
│       ├── values.yaml
│       ├── values-production.yaml
│       └── templates/
├── scripts/                            # Setup and utility scripts
│   ├── ubuntu/
│   │   ├── 01-install-tools.sh
│   │   ├── 02-configure-aks.sh
│   │   ├── 03-deploy-manifests.sh
│   │   ├── 04-deploy-helm.sh
│   │   └── 05-setup-access.sh
│   ├── windows/
│   │   ├── 01-Install-Tools.ps1
│   │   ├── 02-Configure-AKS.ps1
│   │   ├── 03-Deploy-EasyTrade.ps1
│   │   └── 04-Setup-Access.ps1
│   ├── mirror-images.sh
│   ├── feature-flags.sh
│   └── health-check.sh
├── docs/                               # Documentation
│   ├── ubuntu-setup.md
│   ├── windows-setup.md
│   ├── feature-flags-guide.md
│   └── troubleshooting.md
└── .github/
    └── workflows/
        ├── terraform-plan.yml
        └── deploy.yml
```

## Quick Start

### Ubuntu 24.04 VM

```bash
# Clone repository
git clone https://github.com/your-org/easytrade-deployment.git
cd easytrade-deployment

# Install prerequisites
make ubuntu-install-tools

# Configure AKS access
make configure-aks

# Deploy (choose one)
make deploy-manifests    # Raw Kubernetes manifests
make deploy-helm         # Helm chart

# Setup access
make setup-portforward   # kubectl port-forward
make setup-loadbalancer  # Internal LoadBalancer
```

### Windows 11 AVD (Non-Admin)

```powershell
# Clone repository
git clone https://github.com/your-org/easytrade-deployment.git
cd easytrade-deployment

# Install prerequisites (uses user-scoped installers)
.\scripts\windows\01-Install-Tools.ps1

# Configure AKS access
.\scripts\windows\02-Configure-AKS.ps1

# Deploy
.\scripts\windows\03-Deploy-EasyTrade.ps1 -DeploymentType Manifests
# OR
.\scripts\windows\03-Deploy-EasyTrade.ps1 -DeploymentType Helm

# Setup access
.\scripts\windows\04-Setup-Access.ps1 -AccessMethod PortForward
```

## Environment Variables

Create `.env` file in repository root:

```bash
# Azure Configuration
export AZURE_SUBSCRIPTION_NAME="tango-CICD-platform-github-gitflow"
export AZURE_RESOURCE_GROUP="prod-cus-platform-base-rg-001"
export CLUSTER_NAME="prod-cus-aks-sre-lab-003"
export ACR_NAME="prod-central-image-repo"
export ACR_LOGIN_SERVER="prodcentralimagerepo.azurecr.io"

# EasyTrade Configuration
export EASYTRADE_NAMESPACE="easytrade"
export EASYTRADE_VERSION="latest"
export USE_ACR_IMAGES="true"  # Mirror to ACR vs use upstream
```

## Prerequisites

- Azure subscription with AKS cluster access
- kubectl, helm, azure-cli installed
- Private AKS cluster configured with kubelogin
- ACR attached to AKS cluster (for private images)

## Documentation

- [Ubuntu Setup Guide](docs/ubuntu-setup.md)
- [Windows Setup Guide](docs/windows-setup.md)
- [Feature Flags & Problem Patterns](docs/feature-flags-guide.md)
- [Troubleshooting](docs/troubleshooting.md)
