# IAM Permissions Guide for Bedrock to Dynatrace Logging

## Overview

This document details all IAM permissions required to deploy and operate the Bedrock to Dynatrace logging pipeline.

## Table of Contents
1. [Deployment User Permissions](#deployment-user-permissions)
2. [Service Role Permissions](#service-role-permissions)
3. [Minimal Permissions Policy](#minimal-permissions-policy)
4. [Admin-Level Policy (Recommended)](#admin-level-policy-recommended)
5. [Troubleshooting Permissions](#troubleshooting-permissions)

---

## Deployment User Permissions

These are the permissions required for the **IAM user or role** running Terraform to deploy the infrastructure.

### Required Permissions by Service

#### 1. Amazon Bedrock Permissions
```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "BedrockLoggingConfiguration",
      "Effect": "Allow",
      "Action": [
        "bedrock:GetModelInvocationLoggingConfiguration",
        "bedrock:PutModelInvocationLoggingConfiguration",
        "bedrock:DeleteModelInvocationLoggingConfiguration",
        "bedrock:ListFoundationModels"
      ],
      "Resource": "*"
    }
  ]
}
```

**Why needed:**
- `GetModelInvocationLoggingConfiguration` - Check current logging configuration
- `PutModelInvocationLoggingConfiguration` - Enable/configure Bedrock logging
- `DeleteModelInvocationLoggingConfiguration` - Remove logging (for cleanup)
- `ListFoundationModels` - Verify model availability

---

#### 2. CloudWatch Logs Permissions
```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "CloudWatchLogsManagement",
      "Effect": "Allow",
      "Action": [
        "logs:CreateLogGroup",
        "logs:DeleteLogGroup",
        "logs:DescribeLogGroups",
        "logs:PutRetentionPolicy",
        "logs:DeleteRetentionPolicy",
        "logs:TagLogGroup",
        "logs:UntagLogGroup",
        "logs:ListTagsLogGroup"
      ],
      "Resource": [
        "arn:aws:logs:*:*:log-group:/aws/bedrock/*",
        "arn:aws:logs:*:*:log-group:/aws/kinesisfirehose/*",
        "arn:aws:logs:*:*:log-group:/aws/lambda/*"
      ]
    },
    {
      "Sid": "CloudWatchLogsStreams",
      "Effect": "Allow",
      "Action": [
        "logs:CreateLogStream",
        "logs:DeleteLogStream",
        "logs:DescribeLogStreams"
      ],
      "Resource": [
        "arn:aws:logs:*:*:log-group:/aws/bedrock/*:*",
        "arn:aws:logs:*:*:log-group:/aws/kinesisfirehose/*:*",
        "arn:aws:logs:*:*:log-group:/aws/lambda/*:*"
      ]
    },
    {
      "Sid": "CloudWatchLogsSubscription",
      "Effect": "Allow",
      "Action": [
        "logs:PutSubscriptionFilter",
        "logs:DeleteSubscriptionFilter",
        "logs:DescribeSubscriptionFilters"
      ],
      "Resource": "arn:aws:logs:*:*:log-group:/aws/bedrock/*"
    }
  ]
}
```

**Why needed:**
- Create and manage log groups for Bedrock, Firehose, and Lambda
- Create log streams within those groups
- Set retention policies
- Create subscription filters to send logs to Firehose

---

#### 3. Kinesis Firehose Permissions
```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "FirehoseManagement",
      "Effect": "Allow",
      "Action": [
        "firehose:CreateDeliveryStream",
        "firehose:DeleteDeliveryStream",
        "firehose:DescribeDeliveryStream",
        "firehose:UpdateDestination",
        "firehose:ListDeliveryStreams",
        "firehose:TagDeliveryStream",
        "firehose:UntagDeliveryStream",
        "firehose:ListTagsForDeliveryStream"
      ],
      "Resource": "arn:aws:firehose:*:*:deliverystream/*"
    }
  ]
}
```

**Why needed:**
- Create Firehose delivery stream
- Configure HTTP endpoint destination (Dynatrace)
- Update configuration
- Manage tags

---

#### 4. S3 Permissions
```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "S3BucketManagement",
      "Effect": "Allow",
      "Action": [
        "s3:CreateBucket",
        "s3:DeleteBucket",
        "s3:ListBucket",
        "s3:GetBucketLocation",
        "s3:GetBucketVersioning",
        "s3:PutBucketVersioning",
        "s3:GetBucketTagging",
        "s3:PutBucketTagging",
        "s3:GetBucketPublicAccessBlock",
        "s3:PutBucketPublicAccessBlock",
        "s3:GetBucketPolicy",
        "s3:PutBucketPolicy",
        "s3:DeleteBucketPolicy",
        "s3:GetEncryptionConfiguration",
        "s3:PutEncryptionConfiguration",
        "s3:GetLifecycleConfiguration",
        "s3:PutLifecycleConfiguration",
        "s3:DeleteLifecycleConfiguration"
      ],
      "Resource": "arn:aws:s3:::*bedrock*"
    },
    {
      "Sid": "S3ObjectManagement",
      "Effect": "Allow",
      "Action": [
        "s3:PutObject",
        "s3:GetObject",
        "s3:DeleteObject",
        "s3:ListBucketVersions",
        "s3:GetObjectVersion"
      ],
      "Resource": "arn:aws:s3:::*bedrock*/*"
    }
  ]
}
```

**Why needed:**
- Create S3 buckets for failed log deliveries
- Configure bucket policies, versioning, and lifecycle rules
- Configure server-side encryption

---

#### 5. Lambda Permissions
```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "LambdaFunctionManagement",
      "Effect": "Allow",
      "Action": [
        "lambda:CreateFunction",
        "lambda:DeleteFunction",
        "lambda:GetFunction",
        "lambda:GetFunctionConfiguration",
        "lambda:UpdateFunctionCode",
        "lambda:UpdateFunctionConfiguration",
        "lambda:PublishVersion",
        "lambda:ListVersionsByFunction",
        "lambda:TagResource",
        "lambda:UntagResource",
        "lambda:ListTags"
      ],
      "Resource": "arn:aws:lambda:*:*:function:*bedrock*"
    },
    {
      "Sid": "LambdaPermissions",
      "Effect": "Allow",
      "Action": [
        "lambda:AddPermission",
        "lambda:RemovePermission",
        "lambda:GetPolicy"
      ],
      "Resource": "arn:aws:lambda:*:*:function:*bedrock*"
    }
  ]
}
```

**Why needed:**
- Create Lambda function for log transformation
- Update function code and configuration
- Grant Firehose permission to invoke Lambda

---

#### 6. IAM Permissions
```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "IAMRoleManagement",
      "Effect": "Allow",
      "Action": [
        "iam:CreateRole",
        "iam:DeleteRole",
        "iam:GetRole",
        "iam:ListRoles",
        "iam:UpdateRole",
        "iam:TagRole",
        "iam:UntagRole",
        "iam:ListRoleTags"
      ],
      "Resource": [
        "arn:aws:iam::*:role/*bedrock*",
        "arn:aws:iam::*:role/*firehose*",
        "arn:aws:iam::*:role/*cloudwatch*",
        "arn:aws:iam::*:role/*lambda*"
      ]
    },
    {
      "Sid": "IAMPolicyManagement",
      "Effect": "Allow",
      "Action": [
        "iam:CreatePolicy",
        "iam:DeletePolicy",
        "iam:GetPolicy",
        "iam:GetPolicyVersion",
        "iam:ListPolicies",
        "iam:ListPolicyVersions",
        "iam:CreatePolicyVersion",
        "iam:DeletePolicyVersion",
        "iam:TagPolicy",
        "iam:UntagPolicy"
      ],
      "Resource": "arn:aws:iam::*:policy/*bedrock*"
    },
    {
      "Sid": "IAMRolePolicyAttachment",
      "Effect": "Allow",
      "Action": [
        "iam:AttachRolePolicy",
        "iam:DetachRolePolicy",
        "iam:PutRolePolicy",
        "iam:DeleteRolePolicy",
        "iam:GetRolePolicy",
        "iam:ListAttachedRolePolicies",
        "iam:ListRolePolicies"
      ],
      "Resource": [
        "arn:aws:iam::*:role/*bedrock*",
        "arn:aws:iam::*:role/*firehose*",
        "arn:aws:iam::*:role/*cloudwatch*",
        "arn:aws:iam::*:role/*lambda*"
      ]
    },
    {
      "Sid": "IAMPassRole",
      "Effect": "Allow",
      "Action": "iam:PassRole",
      "Resource": [
        "arn:aws:iam::*:role/*bedrock*",
        "arn:aws:iam::*:role/*firehose*",
        "arn:aws:iam::*:role/*cloudwatch*",
        "arn:aws:iam::*:role/*lambda*"
      ],
      "Condition": {
        "StringEquals": {
          "iam:PassedToService": [
            "bedrock.amazonaws.com",
            "firehose.amazonaws.com",
            "logs.amazonaws.com",
            "lambda.amazonaws.com"
          ]
        }
      }
    }
  ]
}
```

**Why needed:**
- Create IAM roles for each service
- Create and attach policies
- Pass roles to AWS services (Bedrock, Firehose, Lambda, CloudWatch)

---

#### 7. CloudWatch Metrics & Dashboard (Optional)
```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "CloudWatchDashboard",
      "Effect": "Allow",
      "Action": [
        "cloudwatch:PutDashboard",
        "cloudwatch:DeleteDashboards",
        "cloudwatch:GetDashboard",
        "cloudwatch:ListDashboards"
      ],
      "Resource": "*"
    },
    {
      "Sid": "CloudWatchMetrics",
      "Effect": "Allow",
      "Action": [
        "cloudwatch:GetMetricStatistics",
        "cloudwatch:ListMetrics",
        "cloudwatch:GetMetricData"
      ],
      "Resource": "*"
    }
  ]
}
```

**Why needed:**
- Create monitoring dashboard (if enabled)
- View metrics for monitoring

---

## Minimal Permissions Policy

This is the **absolute minimum** policy for deployment:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "BedrockLogging",
      "Effect": "Allow",
      "Action": [
        "bedrock:*ModelInvocationLoggingConfiguration"
      ],
      "Resource": "*"
    },
    {
      "Sid": "CloudWatchLogs",
      "Effect": "Allow",
      "Action": [
        "logs:CreateLogGroup",
        "logs:DeleteLogGroup",
        "logs:DescribeLogGroups",
        "logs:PutRetentionPolicy",
        "logs:CreateLogStream",
        "logs:PutSubscriptionFilter",
        "logs:DeleteSubscriptionFilter",
        "logs:DescribeSubscriptionFilters",
        "logs:TagLogGroup"
      ],
      "Resource": [
        "arn:aws:logs:*:*:log-group:/aws/bedrock/*",
        "arn:aws:logs:*:*:log-group:/aws/kinesisfirehose/*",
        "arn:aws:logs:*:*:log-group:/aws/lambda/*"
      ]
    },
    {
      "Sid": "Firehose",
      "Effect": "Allow",
      "Action": [
        "firehose:CreateDeliveryStream",
        "firehose:DeleteDeliveryStream",
        "firehose:DescribeDeliveryStream",
        "firehose:UpdateDestination",
        "firehose:TagDeliveryStream"
      ],
      "Resource": "arn:aws:firehose:*:*:deliverystream/*bedrock*"
    },
    {
      "Sid": "S3",
      "Effect": "Allow",
      "Action": [
        "s3:CreateBucket",
        "s3:DeleteBucket",
        "s3:PutBucketVersioning",
        "s3:PutBucketTagging",
        "s3:PutEncryptionConfiguration",
        "s3:PutLifecycleConfiguration",
        "s3:GetBucket*",
        "s3:ListBucket"
      ],
      "Resource": "arn:aws:s3:::*bedrock*"
    },
    {
      "Sid": "Lambda",
      "Effect": "Allow",
      "Action": [
        "lambda:CreateFunction",
        "lambda:DeleteFunction",
        "lambda:GetFunction",
        "lambda:UpdateFunctionCode",
        "lambda:UpdateFunctionConfiguration",
        "lambda:AddPermission",
        "lambda:RemovePermission",
        "lambda:TagResource"
      ],
      "Resource": "arn:aws:lambda:*:*:function:*bedrock*"
    },
    {
      "Sid": "IAM",
      "Effect": "Allow",
      "Action": [
        "iam:CreateRole",
        "iam:DeleteRole",
        "iam:GetRole",
        "iam:CreatePolicy",
        "iam:DeletePolicy",
        "iam:AttachRolePolicy",
        "iam:DetachRolePolicy",
        "iam:PutRolePolicy",
        "iam:DeleteRolePolicy",
        "iam:GetRolePolicy",
        "iam:PassRole",
        "iam:TagRole"
      ],
      "Resource": [
        "arn:aws:iam::*:role/*bedrock*",
        "arn:aws:iam::*:role/*firehose*",
        "arn:aws:iam::*:role/*cloudwatch*",
        "arn:aws:iam::*:role/*lambda*",
        "arn:aws:iam::*:policy/*bedrock*"
      ]
    }
  ]
}
```

---

## Admin-Level Policy (Recommended)

For easier deployment, especially in development/test environments:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "bedrock:*",
        "logs:*",
        "firehose:*",
        "s3:*",
        "lambda:*",
        "iam:*",
        "cloudwatch:*"
      ],
      "Resource": "*"
    }
  ]
}
```

**⚠️ Warning:** This policy is overly permissive. Use only for:
- Development environments
- Proof of concept
- When troubleshooting permission issues

For production, use the minimal policy above.

---

## Service Role Permissions

These roles are **created by Terraform** and do not need to be created manually.

### 1. Bedrock Logging Role

**Trust Policy:**
```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "Service": "bedrock.amazonaws.com"
      },
      "Action": "sts:AssumeRole"
    }
  ]
}
```

**Permissions Policy:**
```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "logs:CreateLogStream",
        "logs:PutLogEvents"
      ],
      "Resource": "arn:aws:logs:*:*:log-group:/aws/bedrock/modelinvocations:*"
    },
    {
      "Effect": "Allow",
      "Action": [
        "s3:PutObject"
      ],
      "Resource": "arn:aws:s3:::bedrock-large-data-bucket/*"
    }
  ]
}
```

---

### 2. CloudWatch to Firehose Role

**Trust Policy:**
```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "Service": "logs.amazonaws.com"
      },
      "Action": "sts:AssumeRole",
      "Condition": {
        "StringLike": {
          "aws:SourceArn": "arn:aws:logs:REGION:ACCOUNT_ID:*"
        }
      }
    }
  ]
}
```

**Permissions Policy:**
```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "firehose:PutRecord",
        "firehose:PutRecordBatch"
      ],
      "Resource": "arn:aws:firehose:*:*:deliverystream/bedrock-to-dynatrace"
    }
  ]
}
```

---

### 3. Firehose Delivery Role

**Trust Policy:**
```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "Service": "firehose.amazonaws.com"
      },
      "Action": "sts:AssumeRole",
      "Condition": {
        "StringEquals": {
          "sts:ExternalId": "YOUR_ACCOUNT_ID"
        }
      }
    }
  ]
}
```

**Permissions Policy:**
```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "s3:AbortMultipartUpload",
        "s3:GetBucketLocation",
        "s3:GetObject",
        "s3:ListBucket",
        "s3:ListBucketMultipartUploads",
        "s3:PutObject"
      ],
      "Resource": [
        "arn:aws:s3:::failed-logs-bucket",
        "arn:aws:s3:::failed-logs-bucket/*"
      ]
    },
    {
      "Effect": "Allow",
      "Action": [
        "logs:PutLogEvents"
      ],
      "Resource": "arn:aws:logs:*:*:log-group:/aws/kinesisfirehose/*:*"
    },
    {
      "Effect": "Allow",
      "Action": [
        "lambda:InvokeFunction",
        "lambda:GetFunctionConfiguration"
      ],
      "Resource": "arn:aws:lambda:*:*:function:*bedrock*transformer*"
    }
  ]
}
```

---

### 4. Lambda Execution Role

**Trust Policy:**
```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Principal": {
        "Service": "lambda.amazonaws.com"
      },
      "Action": "sts:AssumeRole"
    }
  ]
}
```

**Permissions Policy:**
```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "logs:CreateLogGroup",
        "logs:CreateLogStream",
        "logs:PutLogEvents"
      ],
      "Resource": "arn:aws:logs:*:*:log-group:/aws/lambda/*bedrock*:*"
    }
  ]
}
```

---

## Troubleshooting Permissions

### Common Permission Errors

#### 1. "AccessDenied: User is not authorized to perform: bedrock:PutModelInvocationLoggingConfiguration"

**Solution:**
Add Bedrock permissions:
```json
{
  "Effect": "Allow",
  "Action": "bedrock:PutModelInvocationLoggingConfiguration",
  "Resource": "*"
}
```

---

#### 2. "AccessDenied: User is not authorized to perform: iam:PassRole"

**Solution:**
Add PassRole permission with service condition:
```json
{
  "Effect": "Allow",
  "Action": "iam:PassRole",
  "Resource": "arn:aws:iam::*:role/*",
  "Condition": {
    "StringEquals": {
      "iam:PassedToService": [
        "bedrock.amazonaws.com",
        "firehose.amazonaws.com",
        "logs.amazonaws.com",
        "lambda.amazonaws.com"
      ]
    }
  }
}
```

---

#### 3. "AccessDenied: Cannot create S3 bucket"

**Solution:**
Ensure S3 permissions include bucket creation:
```json
{
  "Effect": "Allow",
  "Action": [
    "s3:CreateBucket",
    "s3:PutBucketVersioning",
    "s3:PutEncryptionConfiguration"
  ],
  "Resource": "arn:aws:s3:::*"
}
```

---

#### 4. "InvalidParameterException: Could not deliver test message to specified HTTP endpoint"

This is **NOT** a permissions issue. This means:
- Dynatrace URL is incorrect
- Dynatrace API token is invalid
- Network connectivity issue

**Solution:**
```bash
# Test Dynatrace connectivity
curl -X POST "https://your-env.live.dynatrace.com/api/v2/logs/ingest" \
  -H "Authorization: Api-Token YOUR_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"content":"test"}'
```

---

## How to Apply Permissions

### Option 1: Create IAM Policy (Recommended)

1. **Create the policy:**
```bash
aws iam create-policy \
  --policy-name BedrockDynatraceDeployment \
  --policy-document file://deployment-policy.json \
  --description "Permissions for deploying Bedrock to Dynatrace logging"
```

2. **Attach to your user:**
```bash
aws iam attach-user-policy \
  --user-name YOUR_USERNAME \
  --policy-arn arn:aws:iam::YOUR_ACCOUNT_ID:policy/BedrockDynatraceDeployment
```

### Option 2: Add to Existing Role/Group

```bash
aws iam put-user-policy \
  --user-name YOUR_USERNAME \
  --policy-name BedrockDynatraceLogging \
  --policy-document file://deployment-policy.json
```

### Option 3: Use AWS Console

1. Go to IAM → Policies → Create policy
2. Copy the JSON policy
3. Create and attach to your user/role

---

## Verification

Check your permissions before deploying:

```bash
# Check Bedrock permissions
aws bedrock get-model-invocation-logging-configuration --region us-east-1

# Check CloudWatch permissions
aws logs describe-log-groups --log-group-name-prefix /aws/bedrock

# Check Firehose permissions
aws firehose list-delivery-streams

# Check IAM permissions
aws iam get-user

# Check S3 permissions
aws s3 ls
```

If any command fails with "AccessDenied", you need to add those permissions.

---

## Security Best Practices

1. **Use Least Privilege**: Start with minimal permissions and add as needed
2. **Use Conditions**: Add condition keys to restrict resources
3. **Use Resource Tags**: Tag resources and use tag-based permissions
4. **Regular Audits**: Review and remove unused permissions
5. **Separate Environments**: Use different accounts/roles for dev/prod
6. **Enable CloudTrail**: Monitor API calls for security auditing
7. **MFA for Sensitive Actions**: Require MFA for IAM changes

---

## Quick Start Checklist

- [ ] Have admin access OR the minimal policy attached
- [ ] Can run `aws bedrock get-model-invocation-logging-configuration`
- [ ] Can create CloudWatch log groups
- [ ] Can create Firehose delivery streams
- [ ] Can create Lambda functions
- [ ] Can create IAM roles
- [ ] Can pass roles to AWS services
- [ ] Have valid Dynatrace URL and token

If all checkboxes are ticked, you're ready to deploy!

---

## Need Help?

**Permission Denied Errors:**
1. Check CloudTrail for the exact permission that was denied
2. Add that specific permission to your policy
3. Wait 30 seconds for IAM propagation
4. Retry

**Still Having Issues:**
- Use the admin policy temporarily to isolate the issue
- Once working, gradually restrict to minimal permissions
- Document which permissions were actually needed
