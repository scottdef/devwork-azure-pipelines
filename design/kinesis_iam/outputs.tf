output "bedrock_log_group_name" {
  description = "Name of the CloudWatch Log Group for Bedrock logs"
  value       = aws_cloudwatch_log_group.bedrock_logs.name
}

output "bedrock_log_group_arn" {
  description = "ARN of the CloudWatch Log Group for Bedrock logs"
  value       = aws_cloudwatch_log_group.bedrock_logs.arn
}

output "firehose_delivery_stream_name" {
  description = "Name of the Kinesis Firehose delivery stream"
  value       = aws_kinesis_firehose_delivery_stream.bedrock_to_dynatrace.name
}

output "firehose_delivery_stream_arn" {
  description = "ARN of the Kinesis Firehose delivery stream"
  value       = aws_kinesis_firehose_delivery_stream.bedrock_to_dynatrace.arn
}

output "failed_logs_s3_bucket" {
  description = "S3 bucket name for failed log deliveries"
  value       = aws_s3_bucket.failed_logs.id
}

output "failed_logs_s3_bucket_arn" {
  description = "ARN of S3 bucket for failed log deliveries"
  value       = aws_s3_bucket.failed_logs.arn
}

output "large_data_s3_bucket" {
  description = "S3 bucket name for large data delivery (if enabled)"
  value       = var.enable_large_data_delivery ? aws_s3_bucket.bedrock_large_data[0].id : null
}

output "lambda_transformer_function_name" {
  description = "Name of the Lambda transformer function (if enabled)"
  value       = var.enable_lambda_transformation ? aws_lambda_function.bedrock_transformer[0].function_name : null
}

output "lambda_transformer_function_arn" {
  description = "ARN of the Lambda transformer function (if enabled)"
  value       = var.enable_lambda_transformation ? aws_lambda_function.bedrock_transformer[0].arn : null
}

output "bedrock_logging_role_arn" {
  description = "ARN of the IAM role for Bedrock logging"
  value       = aws_iam_role.bedrock_logging.arn
}

output "firehose_role_arn" {
  description = "ARN of the IAM role for Firehose"
  value       = aws_iam_role.firehose_role.arn
}

output "cloudwatch_to_firehose_role_arn" {
  description = "ARN of the IAM role for CloudWatch to Firehose subscription"
  value       = aws_iam_role.cloudwatch_to_firehose.arn
}

output "subscription_filter_name" {
  description = "Name of the CloudWatch Logs subscription filter"
  value       = aws_cloudwatch_log_subscription_filter.bedrock_to_firehose.name
}

output "dynatrace_endpoint" {
  description = "Dynatrace logs ingestion endpoint"
  value       = "${var.dynatrace_url}/api/v2/logs/ingest"
}

output "setup_complete" {
  description = "Setup status message"
  value       = "Bedrock to Dynatrace logging pipeline successfully configured! Logs will flow: Bedrock → CloudWatch → Firehose → Dynatrace"
}
