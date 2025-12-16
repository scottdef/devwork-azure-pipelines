# Amazon Bedrock to Dynatrace Logging Pipeline

This Terraform configuration creates a complete logging pipeline to stream Amazon Bedrock model invocation logs to Dynatrace for monitoring, analysis, and observability.

## Architecture

```
Amazon Bedrock
    ↓ (Model Invocations)
CloudWatch Logs
    ↓ (Subscription Filter)
Kinesis Firehose
    ↓ (Optional Lambda Transformation)
Dynatrace Logs API
```

## Features

- ✅ **Automated Bedrock Logging**: Automatically configures Bedrock model invocation logging
- ✅ **Real-time Streaming**: Logs flow in near real-time to Dynatrace
- ✅ **Lambda Transformation**: Enriches and transforms logs for better analysis
- ✅ **Error Handling**: Failed deliveries backed up to S3
- ✅ **Cost Optimization**: Configurable buffering and compression
- ✅ **Token Tracking**: Captures input/output token counts for cost analysis
- ✅ **Multi-Model Support**: Works with all Bedrock models (Claude, Llama, Titan, etc.)
- ✅ **Flexible Configuration**: Enable/disable features as needed

## Prerequisites

1. **AWS Account** with permissions to create:
   - CloudWatch Logs
   - Kinesis Firehose
   - Lambda functions
   - IAM roles and policies
   - S3 buckets
   - Bedrock model access

2. **Dynatrace Environment**:
   - Dynatrace environment URL
   - API token with `logs.ingest` permission

3. **Terraform**: Version >= 1.0

4. **AWS CLI**: Configured with appropriate credentials

## Quick Start

### Step 1: Clone or Download Configuration

Save all the Terraform files to a directory:
```bash
mkdir bedrock-dynatrace-logging
cd bedrock-dynatrace-logging
# Copy all .tf files to this directory
```

### Step 2: Configure Variables

Create a `terraform.tfvars` file:

```hcl
# Copy from terraform.tfvars.example and update values
aws_region     = "us-east-1"
aws_account_id = "123456789012"

dynatrace_url       = "https://your-env.live.dynatrace.com"
dynatrace_api_token = "dt0c01.YOUR_TOKEN_HERE"

environment  = "prod"
project_name = "bedrock-dynatrace"
```

### Step 3: Get Your Dynatrace API Token

1. Log in to Dynatrace
2. Go to **Settings → Access tokens**
3. Click **Generate new token**
4. Name: `Bedrock Logs Ingestion`
5. Scopes: Select **Ingest logs** (`logs.ingest`)
6. Click **Generate token** and copy it

### Step 4: Initialize Terraform

```bash
terraform init
```

### Step 5: Review the Plan

```bash
terraform plan
```

Review the resources that will be created.

### Step 6: Apply Configuration

```bash
terraform apply
```

Type `yes` when prompted.

### Step 7: Verify Setup

The setup is complete! Test it by invoking a Bedrock model:

```python
import boto3
import json

bedrock = boto3.client('bedrock-runtime', region_name='us-east-1')

response = bedrock.invoke_model(
    modelId='anthropic.claude-3-sonnet-20240229-v1:0',
    body=json.dumps({
        'anthropic_version': 'bedrock-2023-05-31',
        'max_tokens': 100,
        'messages': [
            {
                'role': 'user',
                'content': 'Hello! This is a test for logging.'
            }
        ]
    })
)

print("Model invoked - check Dynatrace logs in 1-2 minutes")
```

## Configuration Options

### Essential Variables

| Variable | Description | Required | Default |
|----------|-------------|----------|---------|
| `aws_region` | AWS region | Yes | `us-east-1` |
| `aws_account_id` | Your AWS account ID | Yes | - |
| `dynatrace_url` | Dynatrace environment URL | Yes | - |
| `dynatrace_api_token` | Dynatrace API token | Yes | - |

### Optional Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `environment` | Environment name | `prod` |
| `project_name` | Project name prefix | `bedrock-dynatrace` |
| `firehose_buffer_size` | Buffer size in MB (1-5) | `1` |
| `firehose_buffer_interval` | Buffer interval in seconds (60-900) | `60` |
| `log_retention_days` | CloudWatch log retention | `7` |
| `enable_lambda_transformation` | Enable log transformation | `true` |
| `enable_bedrock_logging` | Auto-configure Bedrock | `true` |
| `enable_large_data_delivery` | Enable S3 for large data | `false` |
| `log_text_data` | Log text prompts/completions | `true` |
| `log_image_data` | Log image data | `false` |
| `log_embedding_data` | Log embedding data | `false` |

## Viewing Logs in Dynatrace

### Access Logs

1. Go to **Observe and explore → Logs**
2. Use queries to filter:

### Useful Log Queries

**All Bedrock logs:**
```
log.source="aws.bedrock"
```

**Filter by specific model:**
```
log.source="aws.bedrock" AND aws.bedrock.model_id="anthropic.claude-3-sonnet*"
```

**Find errors:**
```
log.source="aws.bedrock" AND status="ERROR"
```

**High token usage (>10k tokens):**
```
log.source="aws.bedrock" AND aws.bedrock.total_tokens>10000
```

**Logs from last hour:**
```
log.source="aws.bedrock" AND timestamp>now()-1h
```

**By model provider:**
```
log.source="aws.bedrock" AND aws.bedrock.provider="anthropic"
```

### Log Attributes

The Lambda transformer enriches logs with these attributes:

- `aws.bedrock.model_id` - Model identifier
- `aws.bedrock.provider` - Model provider (anthropic, amazon, meta, etc.)
- `aws.bedrock.request_id` - Request ID for tracing
- `aws.bedrock.operation` - Operation type (invoke, invoke-stream)
- `aws.bedrock.input_tokens` - Input token count
- `aws.bedrock.output_tokens` - Output token count
- `aws.bedrock.total_tokens` - Total tokens used
- `aws.bedrock.latency_ms` - Request latency
- `aws.account_id` - AWS account ID
- `cloud.region` - AWS region
- `error.message` - Error message (if error occurred)

## Monitoring and Alerts

### Create Dynatrace Alerts

1. **High Error Rate Alert:**
   - Query: `log.source="aws.bedrock" AND status="ERROR"`
   - Threshold: > 10 errors in 5 minutes

2. **High Token Usage Alert:**
   - Query: `log.source="aws.bedrock"`
   - Metric: `aws.bedrock.total_tokens`
   - Threshold: > 1M tokens per hour

3. **High Latency Alert:**
   - Query: `log.source="aws.bedrock"`
   - Metric: `aws.bedrock.latency_ms`
   - Threshold: p95 > 5000ms

### CloudWatch Metrics

Monitor these CloudWatch metrics:

**Firehose Metrics:**
- `IncomingRecords` - Records received
- `DeliveryToHttpEndpoint.Success` - Successful deliveries
- `DeliveryToHttpEndpoint.DataFreshness` - Age of oldest record
- `ExecuteProcessing.Duration` - Lambda transformation time

**Lambda Metrics:**
- `Invocations` - Number of invocations
- `Errors` - Lambda errors
- `Duration` - Execution time

## Cost Considerations

### Estimated Monthly Costs

For **10,000 Bedrock invocations/month**:

| Service | Cost |
|---------|------|
| CloudWatch Logs | ~$0.50 |
| Kinesis Firehose | ~$5.00 |
| Lambda (if enabled) | ~$0.20 |
| S3 (failed logs) | ~$0.10 |
| **Total** | **~$5.80/month** |

### Cost Optimization Tips

1. **Adjust buffer settings**: Increase `firehose_buffer_interval` to reduce API calls
2. **Disable transformation**: Set `enable_lambda_transformation = false` if not needed
3. **Reduce log retention**: Lower `log_retention_days` to minimum required
4. **Filter logs**: Modify subscription filter pattern to only log specific models
5. **Disable unnecessary data**: Set `log_image_data = false` and `log_embedding_data = false`

## Troubleshooting

### Logs Not Appearing in Dynatrace

1. **Check Firehose metrics:**
   ```bash
   aws cloudwatch get-metric-statistics \
     --namespace AWS/Firehose \
     --metric-name DeliveryToHttpEndpoint.Success \
     --dimensions Name=DeliveryStreamName,Value=bedrock-to-dynatrace \
     --start-time $(date -u -d '1 hour ago' +%Y-%m-%dT%H:%M:%S) \
     --end-time $(date -u +%Y-%m-%dT%H:%M:%S) \
     --period 300 \
     --statistics Sum
   ```

2. **Check S3 for failed deliveries:**
   ```bash
   aws s3 ls s3://bedrock-dynatrace-bedrock-failed-logs-YOUR_ACCOUNT_ID/
   ```

3. **View Firehose CloudWatch Logs:**
   ```bash
   aws logs tail /aws/kinesisfirehose/bedrock-to-dynatrace --follow
   ```

4. **Verify Dynatrace token:**
   ```bash
   curl -X POST "https://your-env.live.dynatrace.com/api/v2/logs/ingest" \
     -H "Authorization: Api-Token YOUR_TOKEN" \
     -H "Content-Type: application/json" \
     -d '{"content":"test"}'
   ```

### Lambda Transformation Errors

View Lambda logs:
```bash
aws logs tail /aws/lambda/bedrock-dynatrace-log-transformer --follow
```

### Bedrock Logging Not Enabled

Verify configuration:
```bash
aws bedrock get-model-invocation-logging-configuration --region us-east-1
```

Manually enable if needed:
```bash
aws bedrock put-model-invocation-logging-configuration \
  --region us-east-1 \
  --logging-config file://bedrock-logging-config.json
```

### High Firehose Data Freshness

If `DataFreshness` metric is high:
- Reduce `firehose_buffer_interval`
- Increase `firehose_buffer_size`
- Check Dynatrace endpoint availability

## Advanced Configuration

### Custom Log Filtering

Modify the subscription filter to only log specific models:

```hcl
# In main.tf, update the subscription filter
resource "aws_cloudwatch_log_subscription_filter" "bedrock_to_firehose" {
  filter_pattern = "[timestamp, request_id, event_type=*modelId=anthropic*]"
  # This will only forward logs for Anthropic models
}
```

### Multi-Region Setup

Deploy in multiple regions:

```bash
# Region 1
terraform workspace new us-east-1
terraform apply -var="aws_region=us-east-1"

# Region 2
terraform workspace new eu-west-1
terraform apply -var="aws_region=eu-west-1"
```

### Custom Lambda Transformation

Modify `lambda/transformer.py` to add custom fields:

```python
# Add custom business logic
if 'modelId' in bedrock_log and 'claude' in bedrock_log['modelId']:
    dynatrace_log['business.unit'] = 'ai-research'
    dynatrace_log['cost.center'] = 'r-and-d'
```

Then reapply:
```bash
terraform apply
```

## Cleanup

To remove all resources:

```bash
terraform destroy
```

**Note:** This will:
- Delete CloudWatch log groups (logs will be lost)
- Remove Firehose stream
- Delete Lambda function
- Remove S3 buckets (must be empty)
- Delete IAM roles

## Security Best Practices

1. **API Token Management:**
   - Store token in AWS Secrets Manager or Parameter Store
   - Rotate tokens regularly
   - Use least-privilege scopes

2. **IAM Permissions:**
   - Follow principle of least privilege
   - Use IAM conditions where possible
   - Enable CloudTrail for audit logs

3. **Data Privacy:**
   - Review what data is logged (prompts may contain PII)
   - Consider disabling `log_text_data` for sensitive applications
   - Implement data retention policies

4. **Network Security:**
   - Firehose uses HTTPS for all communications
   - Enable VPC endpoints if needed
   - Use S3 bucket policies to restrict access

## Support and Contributing

### Getting Help

- **AWS Support:** For Bedrock, Firehose, or CloudWatch issues
- **Dynatrace Support:** For log ingestion or query issues
- **GitHub Issues:** For Terraform configuration issues

### Useful Links

- [Amazon Bedrock Documentation](https://docs.aws.amazon.com/bedrock/)
- [Kinesis Firehose Documentation](https://docs.aws.amazon.com/firehose/)
- [Dynatrace Logs API](https://docs.dynatrace.com/docs/dynatrace-api/environment-api/log-monitoring-v2)
- [Terraform AWS Provider](https://registry.terraform.io/providers/hashicorp/aws/latest/docs)

## License

This configuration is provided as-is under the MIT License.

## Changelog

### v1.0.0 (2024-12)
- Initial release
- Support for all Bedrock models
- Lambda transformation
- Error handling and S3 backup
- Comprehensive logging attributes
