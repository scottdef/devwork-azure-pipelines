#!/bin/bash
set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}Bedrock to Dynatrace Logging Setup${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""

# Check prerequisites
echo -e "${YELLOW}Checking prerequisites...${NC}"

# Check Terraform
if ! command -v terraform &> /dev/null; then
    echo -e "${RED}Error: Terraform is not installed${NC}"
    echo "Install from: https://www.terraform.io/downloads"
    exit 1
fi
echo -e "${GREEN}✓ Terraform installed: $(terraform version -json | jq -r '.terraform_version')${NC}"

# Check AWS CLI
if ! command -v aws &> /dev/null; then
    echo -e "${RED}Error: AWS CLI is not installed${NC}"
    echo "Install from: https://aws.amazon.com/cli/"
    exit 1
fi
echo -e "${GREEN}✓ AWS CLI installed${NC}"

# Check AWS credentials
if ! aws sts get-caller-identity &> /dev/null; then
    echo -e "${RED}Error: AWS credentials not configured${NC}"
    echo "Run: aws configure"
    exit 1
fi

AWS_ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text)
AWS_REGION=$(aws configure get region)
echo -e "${GREEN}✓ AWS credentials configured${NC}"
echo -e "  Account ID: ${AWS_ACCOUNT_ID}"
echo -e "  Region: ${AWS_REGION}"

# Check if terraform.tfvars exists
if [ ! -f "terraform.tfvars" ]; then
    echo ""
    echo -e "${YELLOW}Creating terraform.tfvars from example...${NC}"
    cp terraform.tfvars.example terraform.tfvars
    
    # Auto-populate AWS values
    sed -i "s/aws_account_id = \".*\"/aws_account_id = \"${AWS_ACCOUNT_ID}\"/" terraform.tfvars
    sed -i "s/aws_region = \".*\"/aws_region = \"${AWS_REGION}\"/" terraform.tfvars
    
    echo -e "${GREEN}✓ Created terraform.tfvars${NC}"
    echo ""
    echo -e "${YELLOW}⚠ IMPORTANT: Edit terraform.tfvars and add:${NC}"
    echo -e "  1. Your Dynatrace URL"
    echo -e "  2. Your Dynatrace API token"
    echo ""
    echo -e "${YELLOW}To get a Dynatrace API token:${NC}"
    echo -e "  1. Log in to Dynatrace"
    echo -e "  2. Go to Settings → Access tokens"
    echo -e "  3. Generate new token with 'Ingest logs' scope"
    echo ""
    read -p "Press Enter after updating terraform.tfvars..."
fi

# Validate Dynatrace configuration
echo ""
echo -e "${YELLOW}Validating Dynatrace configuration...${NC}"

DYNATRACE_URL=$(grep "dynatrace_url" terraform.tfvars | cut -d'"' -f2)
DYNATRACE_TOKEN=$(grep "dynatrace_api_token" terraform.tfvars | cut -d'"' -f2)

if [[ "$DYNATRACE_URL" == *"abc12345"* ]] || [[ -z "$DYNATRACE_URL" ]]; then
    echo -e "${RED}Error: Please update dynatrace_url in terraform.tfvars${NC}"
    exit 1
fi

if [[ "$DYNATRACE_TOKEN" == *"****"* ]] || [[ -z "$DYNATRACE_TOKEN" ]]; then
    echo -e "${RED}Error: Please update dynatrace_api_token in terraform.tfvars${NC}"
    exit 1
fi

# Test Dynatrace connectivity
echo -e "${YELLOW}Testing Dynatrace connectivity...${NC}"
HTTP_STATUS=$(curl -s -o /dev/null -w "%{http_code}" \
    -X POST "${DYNATRACE_URL}/api/v2/logs/ingest" \
    -H "Authorization: Api-Token ${DYNATRACE_TOKEN}" \
    -H "Content-Type: application/json" \
    -d '{"content":"test"}')

if [ "$HTTP_STATUS" -eq 200 ] || [ "$HTTP_STATUS" -eq 204 ]; then
    echo -e "${GREEN}✓ Dynatrace connection successful${NC}"
else
    echo -e "${RED}Error: Failed to connect to Dynatrace (HTTP ${HTTP_STATUS})${NC}"
    echo -e "${YELLOW}Please verify your URL and API token${NC}"
    exit 1
fi

# Initialize Terraform
echo ""
echo -e "${YELLOW}Initializing Terraform...${NC}"
terraform init

# Run Terraform plan
echo ""
echo -e "${YELLOW}Running Terraform plan...${NC}"
terraform plan -out=tfplan

# Ask for confirmation
echo ""
echo -e "${YELLOW}========================================${NC}"
echo -e "${YELLOW}Ready to deploy!${NC}"
echo -e "${YELLOW}========================================${NC}"
echo ""
echo "This will create the following resources:"
echo "  - CloudWatch Log Groups"
echo "  - Kinesis Firehose Delivery Stream"
echo "  - Lambda Function (for transformation)"
echo "  - S3 Buckets (for failed logs)"
echo "  - IAM Roles and Policies"
echo "  - CloudWatch Subscription Filter"
echo ""
read -p "Do you want to apply these changes? (yes/no): " CONFIRM

if [ "$CONFIRM" != "yes" ]; then
    echo -e "${YELLOW}Deployment cancelled${NC}"
    exit 0
fi

# Apply Terraform
echo ""
echo -e "${YELLOW}Applying Terraform configuration...${NC}"
terraform apply tfplan

# Clean up plan file
rm -f tfplan

# Display success message
echo ""
echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}✓ Setup Complete!${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""
echo "Your Bedrock to Dynatrace logging pipeline is now active!"
echo ""
echo "Next steps:"
echo "  1. Invoke a Bedrock model to generate logs"
echo "  2. Check Dynatrace in 1-2 minutes for logs"
echo "  3. Use query: log.source=\"aws.bedrock\""
echo ""
echo "Test with this Python snippet:"
echo ""
cat << 'EOF'
import boto3
import json

bedrock = boto3.client('bedrock-runtime', region_name='us-east-1')
response = bedrock.invoke_model(
    modelId='anthropic.claude-3-sonnet-20240229-v1:0',
    body=json.dumps({
        'anthropic_version': 'bedrock-2023-05-31',
        'max_tokens': 100,
        'messages': [{'role': 'user', 'content': 'Hello!'}]
    })
)
print("✓ Model invoked - check Dynatrace logs!")
EOF
echo ""
echo "For more information, see README.md"
echo ""
