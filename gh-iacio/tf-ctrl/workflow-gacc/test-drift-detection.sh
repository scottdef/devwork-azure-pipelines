#!/bin/bash
# ============================================================================
# Local Drift Detection Test Script
# ============================================================================
# Test the drift detection logic locally before deploying to GitHub Actions
#
# Usage: ./test-drift-detection.sh
#

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BLUE='\033[0;34m'
RESET='\033[0m'

echo -e "${BLUE}"
echo "╔═══════════════════════════════════════════════════════════╗"
echo "║     Organization Settings Drift Detection - Local Test   ║"
echo "╚═══════════════════════════════════════════════════════════╝"
echo -e "${RESET}"

# Check prerequisites
echo -e "${BLUE}Checking prerequisites...${RESET}"

if ! command -v terraform &> /dev/null; then
    echo -e "${RED}✗ Terraform not found${RESET}"
    exit 1
fi
echo -e "${GREEN}✓ Terraform found${RESET}"

if ! command -v python3 &> /dev/null; then
    echo -e "${RED}✗ Python3 not found${RESET}"
    exit 1
fi
echo -e "${GREEN}✓ Python3 found${RESET}"

if ! command -v jq &> /dev/null; then
    echo -e "${RED}✗ jq not found (required for JSON parsing)${RESET}"
    exit 1
fi
echo -e "${GREEN}✓ jq found${RESET}"

# Check environment
echo ""
echo -e "${BLUE}Checking environment...${RESET}"

if [ -z "$GITHUB_TOKEN" ]; then
    echo -e "${RED}✗ GITHUB_TOKEN not set${RESET}"
    exit 1
fi
echo -e "${GREEN}✓ GITHUB_TOKEN set${RESET}"

if [ -z "$GITHUB_OWNER" ]; then
    echo -e "${RED}✗ GITHUB_OWNER not set${RESET}"
    exit 1
fi
echo -e "${GREEN}✓ GITHUB_OWNER set: $GITHUB_OWNER${RESET}"

# Initialize if needed
if [ ! -d ".terraform" ]; then
    echo ""
    echo -e "${BLUE}Initializing Terraform...${RESET}"
    terraform init
fi

# Create temporary data source
echo ""
echo -e "${BLUE}Creating temporary data source...${RESET}"
cat > drift_check_data_source.tf << 'EOF'
# Temporary data source for drift detection testing
# This file is auto-generated and should not be committed
data "github_organization_settings" "current" {
  # Fetches current state from GitHub API
}

output "current_org_settings" {
  value     = data.github_organization_settings.current
  sensitive = false
}
EOF
echo -e "${GREEN}✓ Created drift_check_data_source.tf${RESET}"

# Run terraform plan
echo ""
echo -e "${BLUE}Running terraform plan...${RESET}"
set +e
terraform plan -detailed-exitcode -no-color -out=tfplan > plan_output.txt 2>&1
PLAN_EXIT_CODE=$?
set -e

if [ $PLAN_EXIT_CODE -eq 0 ]; then
    echo -e "${GREEN}✓ No changes detected${RESET}"
    DRIFT_DETECTED=false
elif [ $PLAN_EXIT_CODE -eq 2 ]; then
    echo -e "${YELLOW}⚠ Changes detected${RESET}"
    
    # Check if changes are in organization_settings
    if grep -q "github_organization_settings.main" plan_output.txt; then
        echo -e "${YELLOW}✓ Changes in organization_settings resource${RESET}"
        DRIFT_DETECTED=true
    else
        echo -e "${GREEN}✓ Changes not in organization_settings${RESET}"
        DRIFT_DETECTED=false
    fi
else
    echo -e "${RED}✗ Terraform plan failed with exit code $PLAN_EXIT_CODE${RESET}"
    cat plan_output.txt
    exit 1
fi

if [ "$DRIFT_DETECTED" = false ]; then
    echo ""
    echo -e "${GREEN}No drift detected. Cleaning up...${RESET}"
    rm -f drift_check_data_source.tf plan_output.txt tfplan
    exit 0
fi

# Refresh and extract states
echo ""
echo -e "${BLUE}Extracting current states...${RESET}"
terraform init -upgrade > /dev/null 2>&1
terraform apply -refresh-only -auto-approve > /dev/null 2>&1

# Extract resource state
terraform state show -json github_organization_settings.main | \
    jq '.values' > resource_state.json
echo -e "${GREEN}✓ Extracted resource state${RESET}"

# Extract data source state
terraform state show -json 'data.github_organization_settings.current' | \
    jq '.values' > datasource_state.json
echo -e "${GREEN}✓ Extracted data source state${RESET}"

# Create org name file
echo "{\"name\": \"$GITHUB_OWNER\"}" > org_name.json

# Create comparison script
echo ""
echo -e "${BLUE}Creating comparison script...${RESET}"
cat > compare_drift.py << 'EOFPYTHON'
#!/usr/bin/env python3
"""
GitHub Organization Settings Drift Comparison Tool
Compares terraform resource definition against live GitHub state
"""
import json
import sys
from datetime import datetime

def load_json_file(filepath):
    """Load JSON from file"""
    try:
        with open(filepath, 'r') as f:
            return json.load(f)
    except FileNotFoundError:
        print(f"Error: {filepath} not found", file=sys.stderr)
        return None
    except json.JSONDecodeError as e:
        print(f"Error parsing {filepath}: {e}", file=sys.stderr)
        return None

def normalize_value(value):
    """Normalize values for comparison"""
    if value is None or value == "":
        return None
    if isinstance(value, bool):
        return value
    if isinstance(value, str):
        return value.strip()
    return value

def compare_settings(resource_state, data_source_state):
    """
    Compare resource state with data source state
    Returns dict of differences
    """
    differences = {}
    
    # List of all organization_settings arguments
    settings_fields = [
        'billing_email',
        'company',
        'blog',
        'email',
        'twitter_username',
        'location',
        'name',
        'description',
        'has_organization_projects',
        'has_repository_projects',
        'default_repository_permission',
        'members_can_create_repositories',
        'members_can_create_public_repositories',
        'members_can_create_private_repositories',
        'members_can_create_internal_repositories',
        'members_allowed_repository_creation_type',
        'members_can_create_pages',
        'members_can_create_public_pages',
        'members_can_create_private_pages',
        'members_can_fork_private_repositories',
        'web_commit_signoff_required',
        'advanced_security_enabled_for_new_repositories',
        'secret_scanning_enabled_for_new_repositories',
        'secret_scanning_push_protection_enabled_for_new_repositories',
        'dependabot_alerts_enabled_for_new_repositories',
        'dependabot_security_updates_enabled_for_new_repositories',
        'dependency_graph_enabled_for_new_repositories'
    ]
    
    for field in settings_fields:
        resource_val = normalize_value(resource_state.get(field))
        datasource_val = normalize_value(data_source_state.get(field))
        
        if resource_val != datasource_val:
            differences[field] = {
                'terraform_config': resource_val,
                'github_actual': datasource_val,
                'drift_type': 'modified' if resource_val is not None else 'unset_in_config'
            }
    
    return differences

def format_console_report(differences, org_name):
    """Format differences for console output"""
    lines = []
    lines.append("=" * 70)
    lines.append("DRIFT DETECTION REPORT")
    lines.append("=" * 70)
    lines.append(f"Organization: {org_name}")
    lines.append(f"Detection Time: {datetime.utcnow().strftime('%Y-%m-%d %H:%M:%S UTC')}")
    lines.append(f"Drift Count: {len(differences)} setting(s) out of sync")
    lines.append("=" * 70)
    lines.append("")
    
    if not differences:
        lines.append("✅ No drift detected - all settings match Terraform configuration")
        return "\n".join(lines)
    
    # Group by category
    categories = {
        'Profile Information': [
            'billing_email', 'company', 'blog', 'email', 
            'twitter_username', 'location', 'name', 'description'
        ],
        'Repository Permissions': [
            'default_repository_permission',
            'members_can_create_repositories',
            'members_can_create_public_repositories',
            'members_can_create_private_repositories',
            'members_can_create_internal_repositories',
            'members_allowed_repository_creation_type'
        ],
        'Pages Settings': [
            'members_can_create_pages',
            'members_can_create_public_pages',
            'members_can_create_private_pages'
        ],
        'Project Settings': [
            'has_organization_projects',
            'has_repository_projects'
        ],
        'Security Settings': [
            'advanced_security_enabled_for_new_repositories',
            'secret_scanning_enabled_for_new_repositories',
            'secret_scanning_push_protection_enabled_for_new_repositories',
            'dependabot_alerts_enabled_for_new_repositories',
            'dependabot_security_updates_enabled_for_new_repositories',
            'dependency_graph_enabled_for_new_repositories'
        ],
        'Other Settings': [
            'members_can_fork_private_repositories',
            'web_commit_signoff_required'
        ]
    }
    
    for category, fields in categories.items():
        category_diffs = {k: v for k, v in differences.items() if k in fields}
        if not category_diffs:
            continue
        
        lines.append(f"\n{category}")
        lines.append("-" * 70)
        
        for field, diff in sorted(category_diffs.items()):
            lines.append(f"\n  {field}:")
            lines.append(f"    - Terraform Config: {diff['terraform_config']}")
            lines.append(f"    + GitHub Actual:    {diff['github_actual']}")
    
    lines.append("\n" + "=" * 70)
    lines.append("REMEDIATION")
    lines.append("=" * 70)
    lines.append("\nOption 1: Apply Terraform (Recommended)")
    lines.append("  make plan")
    lines.append("  make apply")
    lines.append("\nOption 2: Update terraform.tfvars to match GitHub:")
    for field, diff in sorted(differences.items()):
        if diff['github_actual'] is not None:
            if isinstance(diff['github_actual'], bool):
                lines.append(f"  {field} = {str(diff['github_actual']).lower()}")
            elif isinstance(diff['github_actual'], str):
                lines.append(f'  {field} = "{diff["github_actual"]}"')
            else:
                lines.append(f"  {field} = {diff['github_actual']}")
    
    return "\n".join(lines)

def main():
    # Load both state files
    resource_state = load_json_file('resource_state.json')
    datasource_state = load_json_file('datasource_state.json')
    org_info = load_json_file('org_name.json')
    
    if not resource_state or not datasource_state:
        sys.exit(1)
    
    # Compare states
    differences = compare_settings(resource_state, datasource_state)
    
    # Generate console report
    report = format_console_report(differences, org_info.get('name', 'Unknown'))
    print(report)
    
    # Write to file for GitHub Actions
    with open('drift_report_console.txt', 'w') as f:
        f.write(report)
    
    # Exit with appropriate code
    if differences:
        sys.exit(1)
    else:
        sys.exit(0)

if __name__ == '__main__':
    main()
EOFPYTHON

chmod +x compare_drift.py

# Run comparison
echo ""
echo -e "${BLUE}Running drift comparison...${RESET}"
set +e
python3 compare_drift.py
COMPARE_EXIT_CODE=$?
set -e

echo ""
if [ $COMPARE_EXIT_CODE -eq 0 ]; then
    echo -e "${GREEN}✅ No drift detected!${RESET}"
else
    echo -e "${YELLOW}⚠️  Drift detected! See report above.${RESET}"
fi

# Cleanup
echo ""
echo -e "${BLUE}Cleaning up temporary files...${RESET}"
rm -f drift_check_data_source.tf
rm -f compare_drift.py
rm -f resource_state.json
rm -f datasource_state.json
rm -f org_name.json
rm -f drift_report_console.txt
rm -f plan_output.txt
rm -f tfplan
echo -e "${GREEN}✓ Cleanup complete${RESET}"

echo ""
if [ $COMPARE_EXIT_CODE -ne 0 ]; then
    echo -e "${YELLOW}Drift was detected. Next steps:${RESET}"
    echo "  1. Review the drift report above"
    echo "  2. Run: make plan"
    echo "  3. Run: make apply (to sync GitHub with Terraform)"
    echo ""
    echo "Or update terraform.tfvars to match GitHub's current state"
    exit 1
fi

exit 0
