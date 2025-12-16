#!/bin/bash
set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}IAM Policy Setup Helper${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""

# Function to print colored messages
print_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Check if AWS CLI is installed
if ! command -v aws &> /dev/null; then
    print_error "AWS CLI is not installed"
    echo "Install from: https://aws.amazon.com/cli/"
    exit 1
fi

# Check AWS credentials
print_info "Checking AWS credentials..."
if ! aws sts get-caller-identity &> /dev/null; then
    print_error "AWS credentials not configured"
    echo "Run: aws configure"
    exit 1
fi

AWS_ACCOUNT_ID=$(aws sts get-caller-identity --query Account --output text)
AWS_USER_ARN=$(aws sts get-caller-identity --query Arn --output text)
print_success "AWS credentials valid"
print_info "Account ID: ${AWS_ACCOUNT_ID}"
print_info "Identity: ${AWS_USER_ARN}"
echo ""

# Prompt user to select policy type
echo "Select the IAM policy level to apply:"
echo ""
echo "  1) Recommended - Standard deployment policy (recommended for production)"
echo "  2) Minimal - Absolute minimum permissions (most restrictive)"
echo "  3) Admin - Full admin access (development/testing only)"
echo "  4) Check current permissions (don't create policy)"
echo "  5) Exit"
echo ""
read -p "Enter your choice (1-5): " CHOICE

case $CHOICE in
    1)
        POLICY_FILE="iam-policies/deployment-policy.json"
        POLICY_NAME="BedrockDynatraceDeployment"
        POLICY_DESC="Standard permissions for deploying Bedrock to Dynatrace logging (recommended)"
        ;;
    2)
        POLICY_FILE="iam-policies/minimal-policy.json"
        POLICY_NAME="BedrockDynatraceMinimal"
        POLICY_DESC="Minimal permissions for deploying Bedrock to Dynatrace logging"
        ;;
    3)
        POLICY_FILE="iam-policies/admin-policy.json"
        POLICY_NAME="BedrockDynatraceAdmin"
        POLICY_DESC="Admin-level permissions for deploying Bedrock to Dynatrace logging (development only)"
        print_warning "Admin policy provides broad permissions. Use only for development/testing!"
        read -p "Are you sure you want to continue? (yes/no): " CONFIRM
        if [ "$CONFIRM" != "yes" ]; then
            print_info "Operation cancelled"
            exit 0
        fi
        ;;
    4)
        print_info "Checking current permissions..."
        echo ""
        
        # Test Bedrock permissions
        echo "Testing Bedrock permissions:"
        if aws bedrock get-model-invocation-logging-configuration --region us-east-1 &> /dev/null; then
            print_success "✓ Bedrock: Get logging configuration"
        else
            print_error "✗ Bedrock: Get logging configuration"
        fi
        
        # Test CloudWatch Logs permissions
        echo "Testing CloudWatch Logs permissions:"
        if aws logs describe-log-groups --log-group-name-prefix /aws/bedrock --max-items 1 &> /dev/null; then
            print_success "✓ CloudWatch Logs: Describe log groups"
        else
            print_error "✗ CloudWatch Logs: Describe log groups"
        fi
        
        # Test Firehose permissions
        echo "Testing Firehose permissions:"
        if aws firehose list-delivery-streams --max-items 1 &> /dev/null; then
            print_success "✓ Firehose: List delivery streams"
        else
            print_error "✗ Firehose: List delivery streams"
        fi
        
        # Test Lambda permissions
        echo "Testing Lambda permissions:"
        if aws lambda list-functions --max-items 1 &> /dev/null; then
            print_success "✓ Lambda: List functions"
        else
            print_error "✗ Lambda: List functions"
        fi
        
        # Test S3 permissions
        echo "Testing S3 permissions:"
        if aws s3 ls &> /dev/null; then
            print_success "✓ S3: List buckets"
        else
            print_error "✗ S3: List buckets"
        fi
        
        # Test IAM permissions
        echo "Testing IAM permissions:"
        if aws iam list-roles --max-items 1 &> /dev/null; then
            print_success "✓ IAM: List roles"
        else
            print_error "✗ IAM: List roles"
        fi
        
        echo ""
        print_info "Permission check complete"
        exit 0
        ;;
    5)
        print_info "Exiting"
        exit 0
        ;;
    *)
        print_error "Invalid choice"
        exit 1
        ;;
esac

# Check if policy file exists
if [ ! -f "$POLICY_FILE" ]; then
    print_error "Policy file not found: $POLICY_FILE"
    exit 1
fi

print_info "Selected policy: $POLICY_NAME"
print_info "Policy file: $POLICY_FILE"
echo ""

# Extract username or role name from ARN
if [[ "$AWS_USER_ARN" == *"user/"* ]]; then
    IDENTITY_TYPE="user"
    IDENTITY_NAME=$(echo "$AWS_USER_ARN" | awk -F'user/' '{print $2}')
elif [[ "$AWS_USER_ARN" == *"assumed-role/"* ]]; then
    IDENTITY_TYPE="role"
    IDENTITY_NAME=$(echo "$AWS_USER_ARN" | awk -F'assumed-role/' '{print $2}' | cut -d'/' -f1)
else
    print_error "Unable to determine identity type from ARN: $AWS_USER_ARN"
    exit 1
fi

print_info "Identity type: $IDENTITY_TYPE"
print_info "Identity name: $IDENTITY_NAME"
echo ""

# Prompt for application method
echo "How would you like to apply this policy?"
echo ""
echo "  1) Create new managed policy and attach to current $IDENTITY_TYPE"
echo "  2) Create new managed policy only (don't attach)"
echo "  3) Create inline policy on current $IDENTITY_TYPE"
echo "  4) Show policy JSON (don't create)"
echo ""
read -p "Enter your choice (1-4): " APPLY_CHOICE

case $APPLY_CHOICE in
    1)
        print_info "Creating managed policy: $POLICY_NAME"
        
        # Check if policy already exists
        EXISTING_POLICY=$(aws iam list-policies --scope Local --query "Policies[?PolicyName=='$POLICY_NAME'].Arn" --output text)
        
        if [ -n "$EXISTING_POLICY" ]; then
            print_warning "Policy already exists: $EXISTING_POLICY"
            read -p "Do you want to delete and recreate it? (yes/no): " RECREATE
            if [ "$RECREATE" == "yes" ]; then
                print_info "Deleting existing policy..."
                aws iam delete-policy --policy-arn "$EXISTING_POLICY"
                print_success "Policy deleted"
            else
                POLICY_ARN="$EXISTING_POLICY"
            fi
        fi
        
        if [ -z "$POLICY_ARN" ]; then
            POLICY_ARN=$(aws iam create-policy \
                --policy-name "$POLICY_NAME" \
                --policy-document "file://$POLICY_FILE" \
                --description "$POLICY_DESC" \
                --query 'Policy.Arn' \
                --output text)
            print_success "Policy created: $POLICY_ARN"
        fi
        
        # Attach policy
        print_info "Attaching policy to $IDENTITY_TYPE: $IDENTITY_NAME"
        if [ "$IDENTITY_TYPE" == "user" ]; then
            aws iam attach-user-policy \
                --user-name "$IDENTITY_NAME" \
                --policy-arn "$POLICY_ARN"
        else
            aws iam attach-role-policy \
                --role-name "$IDENTITY_NAME" \
                --policy-arn "$POLICY_ARN"
        fi
        print_success "Policy attached successfully!"
        ;;
        
    2)
        print_info "Creating managed policy: $POLICY_NAME"
        
        POLICY_ARN=$(aws iam create-policy \
            --policy-name "$POLICY_NAME" \
            --policy-document "file://$POLICY_FILE" \
            --description "$POLICY_DESC" \
            --query 'Policy.Arn' \
            --output text)
        print_success "Policy created: $POLICY_ARN"
        print_info "Policy not attached. To attach manually:"
        echo ""
        echo "  aws iam attach-${IDENTITY_TYPE}-policy \\"
        echo "    --${IDENTITY_TYPE}-name YOUR_NAME \\"
        echo "    --policy-arn $POLICY_ARN"
        ;;
        
    3)
        if [ "$IDENTITY_TYPE" != "user" ]; then
            print_error "Inline policies can only be created for users, not roles"
            exit 1
        fi
        
        print_info "Creating inline policy on user: $IDENTITY_NAME"
        aws iam put-user-policy \
            --user-name "$IDENTITY_NAME" \
            --policy-name "$POLICY_NAME" \
            --policy-document "file://$POLICY_FILE"
        print_success "Inline policy created successfully!"
        ;;
        
    4)
        print_info "Policy JSON content:"
        echo ""
        cat "$POLICY_FILE"
        echo ""
        print_info "No changes made"
        exit 0
        ;;
        
    *)
        print_error "Invalid choice"
        exit 1
        ;;
esac

echo ""
print_success "========================================="
print_success "Policy setup complete!"
print_success "========================================="
echo ""
print_info "Next steps:"
echo "  1. Wait 30 seconds for IAM changes to propagate"
echo "  2. Run: ./setup.sh"
echo "  3. Deploy your infrastructure with Terraform"
echo ""
print_info "To verify permissions:"
echo "  ./apply-iam-policy.sh  (choose option 4)"
echo ""
