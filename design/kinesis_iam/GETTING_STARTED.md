# Getting Started with Bedrock to Dynatrace Logging

## 📦 What's Included

This package contains a complete Terraform infrastructure-as-code solution to stream Amazon Bedrock logs to Dynatrace:

### Core Files
- **main.tf** - Main Terraform configuration with all AWS resources
- **variables.tf** - Configurable input variables
- **outputs.tf** - Output values after deployment
- **iam.tf** - IAM roles and policies for secure access
- **lambda.tf** - Lambda function configuration
- **dashboard.tf** - Optional CloudWatch monitoring dashboard

### Lambda Code
- **lambda/transformer.py** - Python code to transform Bedrock logs for Dynatrace

### Helper Scripts
- **setup.sh** - Automated deployment script (recommended for first-time setup)
- **test_logging.py** - Script to test the logging pipeline

### Documentation
- **README.md** - Complete documentation with architecture, setup, and troubleshooting
- **QUICK_REFERENCE.md** - Quick reference for common commands and queries
- **terraform.tfvars.example** - Example configuration file

### Other
- **.gitignore** - Git ignore patterns for Terraform projects

---

## 🚀 Quick Start (5 Minutes)

### Option 1: Automated Setup (Recommended)

1. **Edit Configuration:**
   ```bash
   cp terraform.tfvars.example terraform.tfvars
   nano terraform.tfvars  # Add your Dynatrace URL and API token
   ```

2. **Run Setup Script:**
   ```bash
   chmod +x setup.sh
   ./setup.sh
   ```

3. **Test It:**
   ```bash
   chmod +x test_logging.py
   ./test_logging.py
   ```

That's it! Your logs will start flowing to Dynatrace.

---

### Option 2: Manual Setup

1. **Configure Variables:**
   ```bash
   cp terraform.tfvars.example terraform.tfvars
   # Edit terraform.tfvars with your values
   ```

2. **Deploy:**
   ```bash
   terraform init
   terraform plan
   terraform apply
   ```

3. **Verify:**
   - Invoke a Bedrock model
   - Check Dynatrace logs after 1-2 minutes
   - Query: `log.source="aws.bedrock"`

---

## 📋 Prerequisites

Before you begin, ensure you have:

- ✅ **AWS Account** with Bedrock access
- ✅ **Dynatrace Environment** (SaaS or Managed)
- ✅ **Dynatrace API Token** with `logs.ingest` permission
- ✅ **Terraform** installed (v1.0+)
- ✅ **AWS CLI** configured with credentials
- ✅ **Python 3** (for test script)

---

## 🔑 Getting a Dynatrace API Token

1. Log in to your Dynatrace environment
2. Navigate to **Settings → Access tokens**
3. Click **Generate new token**
4. Name it: `Bedrock Logs Ingestion`
5. Enable scope: **Ingest logs** (`logs.ingest`)
6. Click **Generate token**
7. Copy the token (you won't see it again!)

---

## ⚙️ Configuration

### Minimal Configuration

The minimum you need to set in `terraform.tfvars`:

```hcl
aws_account_id      = "123456789012"  # Your AWS account ID
aws_region          = "us-east-1"     # Your AWS region
dynatrace_url       = "https://abc12345.live.dynatrace.com"
dynatrace_api_token = "dt0c01.YOUR_TOKEN_HERE"
```

### Recommended Configuration

For production use:

```hcl
# AWS Settings
aws_account_id = "123456789012"
aws_region     = "us-east-1"

# Dynatrace Settings
dynatrace_url       = "https://abc12345.live.dynatrace.com"
dynatrace_api_token = "dt0c01.YOUR_TOKEN_HERE"

# Project Settings
environment  = "prod"
project_name = "bedrock-dynatrace"

# Logging Options
log_retention_days         = 7
enable_lambda_transformation = true
log_text_data             = true
log_image_data            = false

# Buffer Settings (optimize for your use case)
firehose_buffer_size      = 1    # 1-5 MB
firehose_buffer_interval  = 60   # 60-900 seconds
```

---

## 🧪 Testing

After deployment, test your pipeline:

```bash
# Automated test
./test_logging.py

# Or manually invoke a model
python3 -c "
import boto3, json
bedrock = boto3.client('bedrock-runtime', region_name='us-east-1')
bedrock.invoke_model(
    modelId='anthropic.claude-3-sonnet-20240229-v1:0',
    body=json.dumps({
        'anthropic_version': 'bedrock-2023-05-31',
        'max_tokens': 50,
        'messages': [{'role': 'user', 'content': 'Hello!'}]
    })
)
print('✓ Model invoked - check Dynatrace in 1-2 minutes')
"
```

Then check Dynatrace:
1. Go to **Observe and explore → Logs**
2. Query: `log.source="aws.bedrock"`
3. You should see your log entry!

---

## 📊 What Gets Logged

Each Bedrock invocation logs:

- **Model Information**: Model ID, provider
- **Token Usage**: Input, output, and total tokens
- **Performance**: Request latency
- **Metadata**: Request ID, AWS account, region
- **Errors**: Error messages when they occur
- **Identity**: IAM role/user making the request

---

## 💰 Cost Estimate

For **10,000 Bedrock invocations per month**:

| Service           | Monthly Cost |
|-------------------|--------------|
| CloudWatch Logs   | ~$0.50       |
| Kinesis Firehose  | ~$5.00       |
| Lambda            | ~$0.20       |
| S3                | ~$0.10       |
| **Total**         | **~$5.80**   |

*Costs scale with your Bedrock usage.*

---

## 🔍 Monitoring Your Pipeline

### CloudWatch Dashboard

If you enabled the dashboard (`create_cloudwatch_dashboard = true`), you'll get:

- Firehose incoming records
- Delivery success rates
- Data freshness metrics
- Lambda performance
- Recent log entries
- Error tracking

Access it at:
```
https://console.aws.amazon.com/cloudwatch/home?region=YOUR_REGION#dashboards:name=bedrock-dynatrace-monitoring
```

### Key Metrics to Watch

- **IncomingRecords**: Should match your Bedrock invocations
- **DeliverySuccess**: Should be close to 100%
- **DataFreshness**: Should be < 60 seconds
- **Lambda Duration**: Should be < 1000ms

---

## 🆘 Common Issues

### "Logs not appearing in Dynatrace"

1. Wait 2-3 minutes (buffering delay)
2. Check Firehose metrics in CloudWatch
3. Look for failed deliveries in S3
4. Verify Dynatrace token is valid

### "Permission denied" errors

- Ensure your AWS credentials have necessary permissions
- Check IAM roles were created correctly
- Verify Bedrock service is available in your region

### "Module not found" in Lambda

- This shouldn't happen with our configuration
- If it does, the Lambda uses only Python standard library

---

## 📚 Next Steps

1. **Read the full documentation**: See `README.md`
2. **Explore Dynatrace queries**: Check `QUICK_REFERENCE.md`
3. **Set up alerts**: Create alerts for errors or high usage
4. **Optimize costs**: Adjust buffer settings based on your needs
5. **Scale to production**: Test with your actual Bedrock workloads

---

## 🔧 Customization

### Add Custom Log Fields

Edit `lambda/transformer.py` to add custom fields:

```python
# Add business context
dynatrace_log['business.unit'] = 'ai-research'
dynatrace_log['cost.center'] = 'r-and-d'
```

### Filter Specific Models

Edit `main.tf` subscription filter:

```hcl
filter_pattern = "[timestamp, request_id, event_type=*modelId=anthropic*]"
```

### Multi-Region Deployment

Use Terraform workspaces:

```bash
terraform workspace new us-west-2
terraform apply -var="aws_region=us-west-2"
```

---

## 🧹 Cleanup

To remove all resources:

```bash
terraform destroy
```

This will delete:
- All CloudWatch log groups
- Kinesis Firehose stream
- Lambda function
- S3 buckets (must be empty first)
- All IAM roles

---

## 📞 Support

- **Documentation**: README.md and QUICK_REFERENCE.md
- **AWS Issues**: AWS Support or AWS Forums
- **Dynatrace Issues**: Dynatrace Support Portal
- **Terraform Issues**: Check output of `terraform plan/apply`

---

## 🎯 Success Checklist

- [ ] Terraform configured with your values
- [ ] Successfully deployed with `terraform apply`
- [ ] Bedrock model invoked successfully
- [ ] Logs visible in Dynatrace
- [ ] CloudWatch dashboard accessible (if enabled)
- [ ] Test script runs without errors
- [ ] Monitoring alerts configured (recommended)

---

## 🌟 Pro Tips

1. **Start small**: Begin with low buffer intervals for testing, increase for production
2. **Monitor costs**: Watch your Firehose and CloudWatch costs in AWS Cost Explorer
3. **Use filters**: Filter logs to only what you need to reduce costs
4. **Set alerts**: Create Dynatrace alerts for errors and unusual patterns
5. **Document custom fields**: If you add custom fields, document them for your team

---

**Happy Logging! 🚀**

For questions or issues, refer to the comprehensive README.md or QUICK_REFERENCE.md files.
