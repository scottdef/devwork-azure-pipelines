#!/bin/bash
# Import existing GitHub organization settings into Terraform state
# Usage: ./import-org-settings.sh <org-name>

set -e

ORG_NAME="${1:-${GITHUB_ORG}}"

if [[ -z "$ORG_NAME" ]]; then
    echo "Error: Organization name required"
    echo "Usage: $0 <org-name>"
    echo "   or: GITHUB_ORG=<org-name> $0"
    exit 1
fi

echo "==> Importing organization settings for: $ORG_NAME"

# Navigate to terraform directory if not already there
if [[ -d "terraform" ]]; then
    cd terraform
fi

# Verify terraform is initialized
if [[ ! -d ".terraform" ]]; then
    echo "==> Initializing Terraform..."
    terraform init
fi

# Import the organization settings resource
echo "==> Importing github_organization_settings.this"
terraform import github_organization_settings.this "$ORG_NAME"

echo "==> Import complete!"
echo ""
echo "Next steps:"
echo "1. Run: terraform plan"
echo "2. Adjust organization.tf to match your current settings"
echo "3. Run: terraform apply"
