#!/bin/bash

# setup-infrastructure.sh
# Script to set up the complete infrastructure for Databricks Asset Bundle deployment

set -euo pipefail

# Configuration
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CONFIG_FILE="${SCRIPT_DIR}/config/deployment-config.json"
TERRAFORM_DIR="${SCRIPT_DIR}/terraform"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Logging functions
log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check prerequisites
check_prerequisites() {
    log_info "Checking prerequisites..."
    
    local missing_tools=()
    
    # Check required tools
    for tool in terraform az databricks jq go; do
        if ! command -v "$tool" &> /dev/null; then
            missing_tools+=("$tool")
        fi
    done
    
    if [ ${#missing_tools[@]} -ne 0 ]; then
        log_error "Missing required tools: ${missing_tools[*]}"
        log_error "Please install the missing tools and try again."
        exit 1
    fi
    
    # Check environment variables
    local required_vars=(
        "GITHUB_TOKEN"
        "AZURE_SUBSCRIPTION_ID"
        "AZURE_TENANT_ID"
        "DATABRICKS_TOKEN"
    )
    
    local missing_vars=()
    for var in "${required_vars[@]}"; do
        if [ -z "${!var:-}" ]; then
            missing_vars+=("$var")
        fi
    done
    
    if [ ${#missing_vars[@]} -ne 0 ]; then
        log_error "Missing required environment variables: ${missing_vars[*]}"
        exit 1
    fi
    
    log_info "Prerequisites check passed ✓"
}

# Set up Terraform
setup_terraform() {
    log_info "Setting up Terraform infrastructure..."
    
    cd "$TERRAFORM_DIR"
    
    # Initialize Terraform
    terraform init
    
    # Validate configuration
    terraform validate
    
    # Plan deployment
    terraform plan -out=tfplan \
        -var="organization=${GITHUB_ORGANIZATION:-your-org}" \
        -var="azure_subscription_id=$AZURE_SUBSCRIPTION_ID" \
        -var="azure_tenant_id=$AZURE_TENANT_ID" \
        -var="databricks_workspace_url=${DATABRICKS_WORKSPACE_URL}"
    
    # Apply if plan looks good
    read -p "Apply Terraform plan? (y/N): " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        terraform apply tfplan
        log_info "Terraform infrastructure deployed ✓"
    else
        log_warn "Terraform deployment skipped"
    fi
    
    cd - > /dev/null
}

# Set up Azure resources
setup_azure() {
    log_info "Setting up Azure resources..."
    
    # Login to Azure
    if ! az account show &> /dev/null; then
        log_info "Logging into Azure..."
        az login
    fi
    
    # Set subscription
    az account set --subscription "$AZURE_SUBSCRIPTION_ID"
    
    # Create resource groups if they don't exist
    for env in nonprod prod; do
        local rg_name="rg-databricks-$env"
        if ! az group show --name "$rg_name" &> /dev/null; then
            log_info "Creating resource group: $rg_name"
            az group create --name "$rg_name" --location "East US"
        fi
        
        # Create Document Intelligence resource
        local doc_intel_name="doc-intel-$env"
        if ! az cognitiveservices account show --name "$doc_intel_name" --resource-group "$rg_name" &> /dev/null; then
            log_info "Creating Document Intelligence service: $doc_intel_name"
            az cognitiveservices account create \
                --name "$doc_intel_name" \
                --resource-group "$rg_name" \
                --kind "FormRecognizer" \
                --sku "S0" \
                --location "East US" \
                --custom-domain "$doc_intel_name"
        fi
    done
    
    log_info "Azure resources setup completed ✓"
}

# Set up Databricks
setup_databricks() {
    log_info "Setting up Databricks configuration..."
    
    # Configure Databricks CLI
    mkdir -p ~/.databrickscfg
    
    cat > ~/.databrickscfg << EOF
[DEFAULT]
host = ${DATABRICKS_WORKSPACE_URL}
token = ${DATABRICKS_TOKEN}

[nonprod]
host = ${DATABRICKS_WORKSPACE_URL}
token = ${DATABRICKS_TOKEN}

[prod]
host = ${DATABRICKS_WORKSPACE_URL}
token = ${DATABRICKS_TOKEN}
EOF
    
    # Test connection
    if databricks workspace list > /dev/null; then
        log_info "Databricks connection test passed ✓"
    else
        log_error "Databricks connection test failed"
        exit 1
    fi
    
    # Create secret scopes
    for env in nonprod prod; do
        local scope_name="document-intelligence-$env"
        if ! databricks secrets list-scopes | grep -q "$scope_name"; then
            log_info "Creating secret scope: $scope_name"
            databricks secrets create-scope --scope "$scope_name"
        fi
    done
    
    log_info "Databricks setup completed ✓"
}

# Generate template files
generate_templates() {
    log_info "Generating template files..."
    
    # Build Go template processor
    if [ -f "$SCRIPT_DIR/process-templates.go" ]; then
        cd "$SCRIPT_DIR"
        go build -o process-templates process-templates.go
        
        # Generate sample templates
        ./process-templates --generate-samples
        
        log_info "Template files generated ✓"
        cd - > /dev/null
    else
        log_warn "Go template processor not found, skipping template generation"
    fi
}

# Main setup function
main() {
    log_info "Starting infrastructure setup..."
    
    check_prerequisites
    setup_terraform
    setup_azure
    setup_databricks
    generate_templates
    
    log_info "Infrastructure setup completed successfully! 🎉"
    log_info ""
    log_info "Next steps:"
    log_info "1. Review the generated configuration files"
    log_info "2. Customize the templates in your template repository"
    log_info "3. Test the GitHub Actions workflow"
    log_info "4. Deploy your first Databricks Asset Bundle"
}

# Script execution
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
    main "$@"
fi

# Additional utility functions
cleanup() {
    log_info "Cleaning up infrastructure..."
    
    if [ -d "$TERRAFORM_DIR" ]; then
        cd "$TERRAFORM_DIR"
        terraform destroy -auto-approve
        cd - > /dev/null
    fi
    
    # Clean up local files
    rm -f ~/.databrickscfg
    rm -f "$SCRIPT_DIR/process-templates"
    
    log_info "Cleanup completed ✓"
}

# Validate deployment function
validate_deployment() {
    log_info "Validating deployment..."
    
    local errors=0
    
    # Check GitHub repository
    if ! curl -s -H "Authorization: token $GITHUB_TOKEN" \
        "https://api.github.com/repos/${GITHUB_ORGANIZATION}/${REPOSITORY_NAME}" | jq -e '.id' > /dev/null; then
        log_error "GitHub repository not accessible"
        ((errors++))
    fi
    
    # Check Azure resources
    for env in nonprod prod; do
        local rg_name="rg-databricks-$env"
        if ! az group show --name "$rg_name" &> /dev/null; then
            log_error "Resource group $rg_name not found"
            ((errors++))
        fi
    done
    
    # Check Databricks connection
    if ! databricks workspace list > /dev/null 2>&1; then
        log_error "Databricks connection failed"
        ((errors++))
    fi
    
    if [ $errors -eq 0 ]; then
        log_info "Deployment validation passed ✓"
    else
        log_error "Deployment validation failed with $errors errors"
        exit 1
    fi
}

# Maintenance functions
update_dependencies() {
    log_info "Updating dependencies..."
    
    # Update Terraform providers
    cd "$TERRAFORM_DIR"
    terraform init -upgrade
    cd - > /dev/null
    
    # Update Databricks CLI
    pip install --upgrade databricks-cli
    
    # Update Azure CLI
    az upgrade --yes
    
    log_info "Dependencies updated ✓"
}

# Export functions for use in other scripts
export -f log_info log_warn log_error
export -f check_prerequisites setup_terraform setup_azure setup_databricks
export -f generate_templates cleanup validate_deployment update_dependencies