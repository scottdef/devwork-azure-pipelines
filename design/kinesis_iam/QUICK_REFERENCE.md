# Quick Reference Guide

## Essential Commands

### Deployment
```bash
# Initial setup (automated)
./setup.sh

# Manual deployment
terraform init
terraform plan
terraform apply

# Destroy everything
terraform destroy
```

### Testing
```bash
# Run test script
./test_logging.py

# Invoke model manually
python3 << EOF
import boto3, json
bedrock = boto3.client('bedrock-runtime', region_name='us-east-1')
bedrock.invoke_model(
    modelId='anthropic.claude-3-sonnet-20240229-v1:0',
    body=json.dumps({
        'anthropic_version': 'bedrock-2023-05-31',
        'max_tokens': 50,
        'messages': [{'role': 'user', 'content': 'Test'}]
    })
)
EOF
```

### Monitoring

**CloudWatch Logs:**
```bash
# View Bedrock logs
aws logs tail /aws/bedrock/modelinvocations --follow

# View Firehose logs
aws logs tail /aws/kinesisfirehose/bedrock-to-dynatrace --follow

# View Lambda logs
aws logs tail /aws/lambda/bedrock-dynatrace-log-transformer --follow
```

**Firehose Metrics:**
```bash
# Check incoming records
aws cloudwatch get-metric-statistics \
  --namespace AWS/Firehose \
  --metric-name IncomingRecords \
  --dimensions Name=DeliveryStreamName,Value=bedrock-to-dynatrace \
  --start-time $(date -u -d '1 hour ago' +%Y-%m-%dT%H:%M:%S) \
  --end-time $(date -u +%Y-%m-%dT%H:%M:%S) \
  --period 300 \
  --statistics Sum

# Check delivery success
aws cloudwatch get-metric-statistics \
  --namespace AWS/Firehose \
  --metric-name DeliveryToHttpEndpoint.Success \
  --dimensions Name=DeliveryStreamName,Value=bedrock-to-dynatrace \
  --start-time $(date -u -d '1 hour ago' +%Y-%m-%dT%H:%M:%S) \
  --end-time $(date -u +%Y-%m-%dT%H:%M:%S) \
  --period 300 \
  --statistics Sum
```

**S3 Failed Logs:**
```bash
# List failed deliveries
aws s3 ls s3://bedrock-dynatrace-bedrock-failed-logs-YOUR_ACCOUNT_ID/ --recursive
```

## Dynatrace Log Queries

### Basic Queries
```
# All Bedrock logs
log.source="aws.bedrock"

# Specific model
log.source="aws.bedrock" AND aws.bedrock.model_id="anthropic.claude-3-sonnet*"

# Errors only
log.source="aws.bedrock" AND status="ERROR"

# Last hour
log.source="aws.bedrock" AND timestamp>now()-1h
```

### Advanced Queries
```
# High token usage
log.source="aws.bedrock" AND aws.bedrock.total_tokens>10000

# Slow requests (>5 seconds)
log.source="aws.bedrock" AND aws.bedrock.latency_ms>5000

# Specific provider
log.source="aws.bedrock" AND aws.bedrock.provider="anthropic"

# By AWS account
log.source="aws.bedrock" AND aws.account_id="123456789012"

# By region
log.source="aws.bedrock" AND cloud.region="us-east-1"
```

### Aggregations
```
# Total tokens by model
log.source="aws.bedrock" 
| summarize sum(aws.bedrock.total_tokens) by aws.bedrock.model_id

# Error rate
log.source="aws.bedrock" 
| summarize count() by status

# Average latency by model
log.source="aws.bedrock" 
| summarize avg(aws.bedrock.latency_ms) by aws.bedrock.model_id
```

## Common Issues

### Logs not appearing in Dynatrace

1. **Check Firehose delivery:**
   ```bash
   aws firehose describe-delivery-stream --delivery-stream-name bedrock-to-dynatrace
   ```

2. **Check failed logs in S3:**
   ```bash
   aws s3 ls s3://bedrock-dynatrace-bedrock-failed-logs-YOUR_ACCOUNT_ID/
   ```

3. **Verify Dynatrace token:**
   ```bash
   curl -X POST "https://YOUR_ENV.live.dynatrace.com/api/v2/logs/ingest" \
     -H "Authorization: Api-Token YOUR_TOKEN" \
     -H "Content-Type: application/json" \
     -d '{"content":"test"}'
   ```

### High data freshness

**Reduce buffer interval:**
```hcl
# In terraform.tfvars
firehose_buffer_interval = 60  # Minimum value
```

### Lambda errors

**View logs:**
```bash
aws logs tail /aws/lambda/bedrock-dynatrace-log-transformer --follow
```

**Common fixes:**
- Increase timeout: `lambda_timeout = 120`
- Increase memory: `lambda_memory_size = 512`

### Bedrock logging not enabled

**Check configuration:**
```bash
aws bedrock get-model-invocation-logging-configuration --region us-east-1
```

**Manually enable:**
```bash
aws bedrock put-model-invocation-logging-configuration \
  --region us-east-1 \
  --logging-config '{
    "cloudWatchConfig": {
      "logGroupName": "/aws/bedrock/modelinvocations",
      "roleArn": "arn:aws:iam::ACCOUNT_ID:role/bedrock-dynatrace-bedrock-logging-role"
    },
    "textDataDeliveryEnabled": true
  }'
```

## Useful AWS CLI Commands

### Bedrock
```bash
# List available models
aws bedrock list-foundation-models --region us-east-1

# Get logging configuration
aws bedrock get-model-invocation-logging-configuration --region us-east-1
```

### Firehose
```bash
# Describe delivery stream
aws firehose describe-delivery-stream --delivery-stream-name bedrock-to-dynatrace

# List delivery streams
aws firehose list-delivery-streams
```

### CloudWatch Logs
```bash
# List log groups
aws logs describe-log-groups --log-group-name-prefix /aws/bedrock

# Create subscription filter
aws logs put-subscription-filter \
  --log-group-name "/aws/bedrock/modelinvocations" \
  --filter-name "test-filter" \
  --filter-pattern "" \
  --destination-arn "arn:aws:firehose:REGION:ACCOUNT:deliverystream/NAME"
```

## Cost Optimization

### Reduce CloudWatch Logs retention
```hcl
log_retention_days = 3  # Minimum recommended
```

### Increase buffer settings (fewer API calls)
```hcl
firehose_buffer_size     = 5    # Maximum
firehose_buffer_interval = 900  # Maximum (15 minutes)
```

### Disable transformation
```hcl
enable_lambda_transformation = false
```

### Filter logs
```hcl
# In main.tf, modify subscription filter
filter_pattern = "[timestamp, request_id, event_type=*modelId=anthropic*]"
```

## File Structure

```
.
├── README.md                    # Full documentation
├── QUICK_REFERENCE.md          # This file
├── main.tf                     # Main resources
├── variables.tf                # Input variables
├── outputs.tf                  # Output values
├── iam.tf                      # IAM roles and policies
├── lambda.tf                   # Lambda function
├── dashboard.tf                # CloudWatch dashboard (optional)
├── terraform.tfvars.example    # Example configuration
├── setup.sh                    # Automated setup script
├── test_logging.py            # Test script
├── .gitignore                  # Git ignore file
└── lambda/
    └── transformer.py          # Lambda transformation code
```

## Support

- **Full Documentation**: See README.md
- **AWS Support**: For Bedrock/Firehose issues
- **Dynatrace Support**: For log ingestion issues
- **GitHub Issues**: For configuration issues
