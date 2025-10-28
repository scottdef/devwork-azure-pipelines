#!/bin/bash
# ============================================================================
# Quick Start Script for GitHub Organization Settings
# ============================================================================
# This script sets up your environment and walks you through initial config
#
# Usage: ./quickstart.sh
#

set -e  # Exit on error

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
RESET='\033[0m'

echo -e "${BLUE}"
echo "╔═══════════════════════════════════════════════════════════╗"
echo "║  GitHub Organization Settings - Terraform Quick Start    ║"
echo "╚═══════════════════════════════════════════════════════════╝"
echo -e "${RESET}"

# Check prerequisites
echo -e "${BLUE}Checking prerequisites...${RESET}"

if ! command -v terraform &> /dev/null; then
    echo -e "${RED}✗ Terraform not found${RESET}"
    echo "Install from: https://www.terraform.io/downloads"
    exit 1
fi
echo -e "${GREEN}✓ Terraform installed: $(terraform version -json | grep -o '"version":"[^"]*' | cut -d'"' -f4)${RESET}"

if ! command -v make &> /dev/null; then
    echo -e "${RED}✗ Make not found${RESET}"
    echo "Install make using your package manager"
    exit 1
fi
echo -e "${GREEN}✓ Make installed${RESET}"

# Check environment variables
echo ""
echo -e "${BLUE}Checking environment variables...${RESET}"

if [ -z "$GITHUB_TOKEN" ]; then
    echo -e "${YELLOW}GITHUB_TOKEN not set${RESET}"
    echo ""
    echo "Create a Personal Access Token at:"
    echo "https://github.com/settings/tokens/new"
    echo ""
    echo "Required scopes: admin:org, repo"
    echo ""
    read -p "Enter your GitHub token: " GITHUB_TOKEN
    export GITHUB_TOKEN
    echo ""
    echo -e "${GREEN}✓ GITHUB_TOKEN set${RESET}"
else
    echo -e "${GREEN}✓ GITHUB_TOKEN already set${RESET}"
fi

if [ -z "$GITHUB_OWNER" ]; then
    echo -e "${YELLOW}GITHUB_OWNER not set${RESET}"
    echo ""
    read -p "Enter your GitHub organization name: " GITHUB_OWNER
    export GITHUB_OWNER
    echo ""
    echo -e "${GREEN}✓ GITHUB_OWNER set to: $GITHUB_OWNER${RESET}"
else
    echo -e "${GREEN}✓ GITHUB_OWNER already set: $GITHUB_OWNER${RESET}"
fi

# Create terraform.tfvars if it doesn't exist
echo ""
if [ ! -f terraform.tfvars ]; then
    echo -e "${BLUE}Creating terraform.tfvars from example...${RESET}"
    cp terraform.tfvars.example terraform.tfvars
    
    echo ""
    echo -e "${YELLOW}Please edit terraform.tfvars and update these required values:${RESET}"
    echo "  - org_billing_email"
    echo "  - org_company (optional)"
    echo "  - org_name (optional)"
    echo ""
    
    read -p "Do you want to edit terraform.tfvars now? [y/N] " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        ${EDITOR:-vi} terraform.tfvars
    else
        echo -e "${YELLOW}Remember to edit terraform.tfvars before applying!${RESET}"
    fi
else
    echo -e "${GREEN}✓ terraform.tfvars already exists${RESET}"
fi

# Initialize Terraform
echo ""
echo -e "${BLUE}Initializing Terraform...${RESET}"
make init

# Optionally import existing settings
echo ""
read -p "Do you want to import existing organization settings? [y/N] " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    make import
fi

# Generate plan
echo ""
echo -e "${BLUE}Generating Terraform plan...${RESET}"
make plan

# Summary
echo ""
echo -e "${GREEN}"
echo "╔═══════════════════════════════════════════════════════════╗"
echo "║                    Setup Complete!                        ║"
echo "╚═══════════════════════════════════════════════════════════╝"
echo -e "${RESET}"
echo ""
echo -e "${BLUE}Next steps:${RESET}"
echo "  1. Review the plan above"
echo "  2. Run: ${GREEN}make apply${RESET} to apply changes"
echo "  3. Run: ${GREEN}make help${RESET} to see all available commands"
echo ""
echo -e "${BLUE}Environment variables:${RESET}"
echo "  export GITHUB_TOKEN='$GITHUB_TOKEN'"
echo "  export GITHUB_OWNER='$GITHUB_OWNER'"
echo ""
echo -e "${YELLOW}Add these to your ~/.bashrc or ~/.zshrc for persistence${RESET}"
echo ""
