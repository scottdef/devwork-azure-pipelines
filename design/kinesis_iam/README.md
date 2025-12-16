# IAM Policies

This directory contains ready-to-use IAM policies for deploying the Bedrock to Dynatrace logging infrastructure.

## Available Policies

### 1. deployment-policy.json (Recommended)
**Use for:** Production deployments

**Description:** Comprehensive policy with all necessary permissions for deploying and managing the infrastructure. This is the recommended policy for most use cases.

**Permissions include:**
- Amazon Bedrock logging configuration
- CloudWatch Logs management
- Kinesis Firehose creation and management
- S3 bucket creation and management
- Lambda function deployment
- IAM role creation and management
- CloudWatch dashboard (optional)

**Apply with:**
```bash
aws iam create-policy \
  --policy-name BedrockDynatraceDeployment \
  --policy-document file://iam-policies/deployment-policy.json \
  --description "Permissions for Bedrock to Dynatrace logging deployment"

aws iam attach-user-policy \
  --user-name YOUR_USERNAME \
  --policy-arn arn:aws:iam::YOUR_ACCOUNT_ID:policy/BedrockDynatraceDeployment
```

---

### 2. minimal-policy.json
**Use for:** Highly restricted production environments

**Description:** Absolute minimum permissions required for deployment. Use this in environments with strict security requirements.

**Note:** This policy may require additional permissions depending on your specific AWS environment configuration.

**Apply with:**
```bash
aws iam create-policy \
  --policy-name BedrockDynatraceMinimal \
  --policy-document file://iam-policies/minimal-policy.json \
  --description "Minimal permissions for Bedrock to Dynatrace logging"

aws iam attach-user-policy \
  --user-name YOUR_USERNAME \
  --policy-arn arn:aws:iam::YOUR_ACCOUNT_ID:policy/BedrockDynatraceMinimal
```

---

### 3. admin-policy.json
**Use for:** Development and testing environments only

**Description:** Admin-level permissions for all required services. This policy is overly permissive and should NEVER be used in production.

**⚠️ WARNING:** This policy grants broad permissions. Use only for:
- Development environments
- Proof of concept deployments
- Troubleshooting permission issues

**Apply with:**
```bash
aws iam create-policy \
  --policy-name BedrockDynatraceAdmin \
  --policy-document file://iam-policies/admin-policy.json \
  --description "Admin permissions for Bedrock to Dynatrace (DEV ONLY)"

aws iam attach-user-policy \
  --user-name YOUR_USERNAME \
  --policy-arn arn:aws:iam::YOUR_ACCOUNT_ID:policy/BedrockDynatraceAdmin
```

---

## Quick Start

### Option 1: Use the Helper Script (Easiest)

```bash
chmod +x apply-iam-policy.sh
./apply-iam-policy.sh
```

The script will:
1. Check your AWS credentials
2. Let you choose which policy to apply
3. Create the policy
4. Attach it to your user/role
5. Verify permissions

---

### Option 2: Manual Application

1. **Choose your policy:**
   - `deployment-policy.json` - Recommended for most users
   - `minimal-policy.json` - For strict security requirements
   - `admin-policy.json` - Development/testing only

2. **Create the policy:**
   ```bash
   aws iam create-policy \
     --policy-name BedrockDynatraceDeployment \
     --policy-document file://iam-policies/deployment-policy.json
   ```

3. **Get the policy ARN from the output**

4. **Attach to your user:**
   ```bash
   aws iam attach-user-policy \
     --user-name YOUR_USERNAME \
     --policy-arn arn:aws:iam::ACCOUNT_ID:policy/BedrockDynatraceDeployment
   ```

---

## Verification

After applying the policy, verify your permissions:

```bash
# Test Bedrock
aws bedrock get-model-invocation-logging-configuration --region us-east-1

# Test CloudWatch Logs
aws logs describe-log-groups --log-group-name-prefix /aws/bedrock

# Test Firehose
aws firehose list-delivery-streams

# Test Lambda
aws lambda list-functions

# Test S3
aws s3 ls

# Test IAM
aws iam list-roles
```

If any command returns "AccessDenied", you may need additional permissions.

---

## Policy Comparison

| Feature | Minimal | Deployment (Recommended) | Admin |
|---------|---------|--------------------------|-------|
| Bedrock logging | ✓ | ✓ | ✓ |
| CloudWatch Logs | ✓ | ✓ | ✓ |
| Kinesis Firehose | ✓ | ✓ | ✓ |
| S3 buckets | ✓ | ✓ | ✓ |
| Lambda functions | ✓ | ✓ | ✓ |
| IAM roles | ✓ | ✓ | ✓ |
| CloudWatch dashboard | ✗ | ✓ | ✓ |
| Additional services | ✗ | ✗ | ✓ |
| Production ready | ⚠️ | ✓ | ✗ |

---

## Common Issues

### "Policy already exists"

If the policy already exists:
```bash
# Delete old policy
aws iam delete-policy --policy-arn arn:aws:iam::ACCOUNT_ID:policy/POLICY_NAME

# Recreate with new version
aws iam create-policy --policy-name ... --policy-document ...
```

### "Access Denied" errors during deployment

1. Verify the policy is attached:
   ```bash
   aws iam list-attached-user-policies --user-name YOUR_USERNAME
   ```

2. Wait 30 seconds for IAM propagation

3. Try with the admin policy temporarily to isolate the issue

4. Check CloudTrail for the specific permission that was denied

### "Cannot pass role to service"

Add the `iam:PassRole` permission with appropriate conditions. The deployment policy includes this.

---

## Security Best Practices

1. **Use Least Privilege**: Start with the minimal policy and add permissions as needed
2. **Tag Your Resources**: The policies use wildcards with "bedrock" in the name
3. **Regular Audits**: Review and remove unused permissions quarterly
4. **Separate Environments**: Use different policies for dev/staging/prod
5. **Enable CloudTrail**: Monitor API calls for security auditing
6. **Condition Keys**: Add additional condition keys in production

---

## Customization

To restrict policies further, you can:

1. **Replace wildcards with specific resources:**
   ```json
   "Resource": "arn:aws:logs:us-east-1:123456789012:log-group:/aws/bedrock/modelinvocations"
   ```

2. **Add IP-based conditions:**
   ```json
   "Condition": {
     "IpAddress": {
       "aws:SourceIp": ["203.0.113.0/24"]
     }
   }
   ```

3. **Add MFA requirements:**
   ```json
   "Condition": {
     "Bool": {
       "aws:MultiFactorAuthPresent": "true"
     }
   }
   ```

---

## Need Help?

- **Full documentation:** See `IAM_PERMISSIONS_GUIDE.md` in the parent directory
- **Permission errors:** Check CloudTrail logs for denied actions
- **Questions:** Review the comprehensive IAM guide

---

## Files in This Directory

```
iam-policies/
├── README.md                    # This file
├── deployment-policy.json       # Recommended policy for production
├── minimal-policy.json          # Minimal permissions policy
└── admin-policy.json            # Admin policy (dev/test only)
```

---

## Quick Reference

**View policy content:**
```bash
cat iam-policies/deployment-policy.json
```

**Create policy:**
```bash
aws iam create-policy --policy-name NAME --policy-document file://iam-policies/POLICY.json
```

**Attach to user:**
```bash
aws iam attach-user-policy --user-name USER --policy-arn ARN
```

**List attached policies:**
```bash
aws iam list-attached-user-policies --user-name USER
```

**Detach policy:**
```bash
aws iam detach-user-policy --user-name USER --policy-arn ARN
```

**Delete policy:**
```bash
aws iam delete-policy --policy-arn ARN
```
