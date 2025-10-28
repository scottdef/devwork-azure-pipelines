#!/bin/bash
# Complete End-to-End Example: Import and Manage GitHub Organization Settings
# This demonstrates the entire workflow from import to management

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ORG_NAME="${1:-${GITHUB_ORG}}"

if [[ -z "$ORG_NAME" ]]; then
    echo "Usage: $0 <org-name>"
    exit 1
fi

echo "╔═══════════════════════════════════════════════════════════════╗"
echo "║ GitHub Organization Settings Management                      ║"
echo "║ Complete End-to-End Example                                  ║"
echo "╚═══════════════════════════════════════════════════════════════╝"
echo ""

# Step 1: Setup
echo "┌─ Step 1: Setup ─────────────────────────────────────────────┐"
echo "│ Organization: $ORG_NAME"
echo "│ Directory: $SCRIPT_DIR"
echo "└──────────────────────────────────────────────────────────────┘"
echo ""

# Step 2: Copy files to terraform directory
echo "┌─ Step 2: Copying files to terraform directory ──────────────┐"
if [[ ! -d "terraform" ]]; then
    mkdir -p terraform
fi

cp "$SCRIPT_DIR/organization.tf" terraform/
cp "$SCRIPT_DIR/organization_variables.tf" terraform/
echo "│ ✓ organization.tf"
echo "│ ✓ organization_variables.tf"
echo "└──────────────────────────────────────────────────────────────┘"
echo ""

# Step 3: Create terraform.tfvars if it doesn't exist
echo "┌─ Step 3: Configure variables ───────────────────────────────┐"
if [[ ! -f "terraform/terraform.tfvars" ]]; then
    cat > terraform/terraform.tfvars <<EOF
# GitHub Organization Settings
github_org = "$ORG_NAME"

# Organization profile
org_billing_email = "billing@example.com"
org_company       = "Example Corp"
org_email         = "opensource@example.com"
org_location      = "San Francisco, CA"
org_description   = "Building amazing software"
org_display_name  = "Example Corporation"
org_blog          = "https://blog.example.com"
EOF
    echo "│ ✓ Created terraform.tfvars with example values"
    echo "│ ⚠️  Edit terraform/terraform.tfvars with your actual values!"
else
    echo "│ ✓ terraform.tfvars already exists"
fi
echo "└──────────────────────────────────────────────────────────────┘"
echo ""

# Step 4: Initialize Terraform
echo "┌─ Step 4: Initialize Terraform ──────────────────────────────┐"
cd terraform
terraform init
echo "└──────────────────────────────────────────────────────────────┘"
echo ""

# Step 5: Fetch current settings from GitHub
echo "┌─ Step 5: Fetch current settings from GitHub ────────────────┐"
gh api "/orgs/$ORG_NAME" | jq '{
  name: .name,
  company: .company,
  blog: .blog,
  location: .location,
  email: .email,
  default_repository_permission: .default_repository_permission,
  members_can_create_repositories: .members_can_create_repositories,
  web_commit_signoff_required: .web_commit_signoff_required,
  two_factor_requirement_enabled: .two_factor_requirement_enabled
}' | tee current-settings.json
echo "└──────────────────────────────────────────────────────────────┘"
echo ""

# Step 6: Import existing settings
echo "┌─ Step 6: Import organization settings ──────────────────────┐"
if terraform state show github_organization_settings.this >/dev/null 2>&1; then
    echo "│ ⚠️  Settings already imported"
else
    terraform import github_organization_settings.this "$ORG_NAME"
    echo "│ ✓ Import complete"
fi
echo "└──────────────────────────────────────────────────────────────┘"
echo ""

# Step 7: Show current state
echo "┌─ Step 7: Show imported state ───────────────────────────────┐"
terraform state show github_organization_settings.this | head -30
echo "│ ... (truncated)"
echo "└──────────────────────────────────────────────────────────────┘"
echo ""

# Step 8: Plan changes
echo "┌─ Step 8: Plan changes ──────────────────────────────────────┐"
terraform plan -target=github_organization_settings.this -out=org.tfplan
echo "└──────────────────────────────────────────────────────────────┘"
echo ""

# Step 9: Instructions
echo "╔═══════════════════════════════════════════════════════════════╗"
echo "║ NEXT STEPS                                                    ║"
echo "╚═══════════════════════════════════════════════════════════════╝"
echo ""
echo "1. Review the plan above"
echo "2. Edit terraform/organization.tf to match your desired state"
echo "3. Edit terraform/terraform.tfvars with your actual values"
echo "4. Run: terraform plan -target=github_organization_settings.this"
echo "5. Run: terraform apply org.tfplan"
echo ""
echo "Or use make targets:"
echo "  make plan-org    # Review changes"
echo "  make apply-org   # Apply changes"
echo "  make show-org    # Show current state"
echo ""
echo "╔═══════════════════════════════════════════════════════════════╗"
echo "║ SECURITY CHECKLIST                                            ║"
echo "╚═══════════════════════════════════════════════════════════════╝"
echo ""
echo "Verify these security settings in organization.tf:"
echo "  [ ] default_repository_permission = \"read\""
echo "  [ ] members_can_create_repositories = false"
echo "  [ ] members_can_create_public_repositories = false"
echo "  [ ] web_commit_signoff_required = true"
echo "  [ ] advanced_security_enabled_for_new_repositories = true"
echo "  [ ] secret_scanning_enabled_for_new_repositories = true"
echo "  [ ] secret_scanning_push_protection_enabled... = true"
echo ""

# Step 10: Generate comparison report
echo "╔═══════════════════════════════════════════════════════════════╗"
echo "║ COMPARISON: GitHub vs Terraform                               ║"
echo "╚═══════════════════════════════════════════════════════════════╝"
echo ""
echo "Current GitHub Settings:"
cat current-settings.json
echo ""
echo "To see Terraform state:"
echo "  terraform state show github_organization_settings.this"
echo ""

# Cleanup
cd ..

echo "╔═══════════════════════════════════════════════════════════════╗"
echo "║ COMPLETE                                                      ║"
echo "╚═══════════════════════════════════════════════════════════════╝"
echo ""
echo "Organization settings imported and ready for management."
echo "Review the plan, adjust your configuration, then apply."
echo ""
