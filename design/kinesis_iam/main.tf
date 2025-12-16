terraform {
  required_version = ">= 1.0"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
    archive = {
      source  = "hashicorp/archive"
      version = "~> 2.4"
    }
  }
}

provider "aws" {
  region = var.aws_region
}

# CloudWatch Log Group for Bedrock Model Invocations
resource "aws_cloudwatch_log_group" "bedrock_logs" {
  name              = "/aws/bedrock/modelinvocations"
  retention_in_days = var.log_retention_days

  tags = {
    Name        = "bedrock-model-invocations"
    Environment = var.environment
    ManagedBy   = "terraform"
  }
}

# CloudWatch Log Group for Firehose Delivery
resource "aws_cloudwatch_log_group" "firehose_logs" {
  name              = "/aws/kinesisfirehose/${var.firehose_stream_name}"
  retention_in_days = var.log_retention_days

  tags = {
    Name        = "firehose-delivery-logs"
    Environment = var.environment
    ManagedBy   = "terraform"
  }
}

resource "aws_cloudwatch_log_stream" "firehose_delivery" {
  name           = "delivery"
  log_group_name = aws_cloudwatch_log_group.firehose_logs.name
}

resource "aws_cloudwatch_log_stream" "firehose_backup" {
  name           = "backup"
  log_group_name = aws_cloudwatch_log_group.firehose_logs.name
}

# S3 Bucket for Failed Logs
resource "aws_s3_bucket" "failed_logs" {
  bucket = "${var.project_name}-bedrock-failed-logs-${var.aws_account_id}"

  tags = {
    Name        = "bedrock-failed-logs"
    Environment = var.environment
    ManagedBy   = "terraform"
  }
}

resource "aws_s3_bucket_versioning" "failed_logs" {
  bucket = aws_s3_bucket.failed_logs.id

  versioning_configuration {
    status = "Enabled"
  }
}

resource "aws_s3_bucket_server_side_encryption_configuration" "failed_logs" {
  bucket = aws_s3_bucket.failed_logs.id

  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }
  }
}

resource "aws_s3_bucket_lifecycle_configuration" "failed_logs" {
  bucket = aws_s3_bucket.failed_logs.id

  rule {
    id     = "expire-old-logs"
    status = "Enabled"

    expiration {
      days = var.failed_logs_retention_days
    }
  }
}

# S3 Bucket for Large Data Delivery (optional)
resource "aws_s3_bucket" "bedrock_large_data" {
  count  = var.enable_large_data_delivery ? 1 : 0
  bucket = "${var.project_name}-bedrock-large-data-${var.aws_account_id}"

  tags = {
    Name        = "bedrock-large-data"
    Environment = var.environment
    ManagedBy   = "terraform"
  }
}

resource "aws_s3_bucket_versioning" "bedrock_large_data" {
  count  = var.enable_large_data_delivery ? 1 : 0
  bucket = aws_s3_bucket.bedrock_large_data[0].id

  versioning_configuration {
    status = "Enabled"
  }
}

# Kinesis Firehose Delivery Stream
resource "aws_kinesis_firehose_delivery_stream" "bedrock_to_dynatrace" {
  name        = var.firehose_stream_name
  destination = "http_endpoint"

  http_endpoint_configuration {
    url                = "${var.dynatrace_url}/api/v2/logs/ingest"
    name               = "Dynatrace"
    access_key         = var.dynatrace_api_token
    buffering_size     = var.firehose_buffer_size
    buffering_interval = var.firehose_buffer_interval
    retry_duration     = 300
    s3_backup_mode     = "FailedDataOnly"

    request_configuration {
      content_encoding = "GZIP"

      common_attributes {
        name  = "log.source"
        value = "aws.bedrock"
      }

      common_attributes {
        name  = "cloud.provider"
        value = "aws"
      }

      common_attributes {
        name  = "cloud.region"
        value = var.aws_region
      }

      common_attributes {
        name  = "service.name"
        value = "bedrock"
      }
    }

    s3_configuration {
      role_arn           = aws_iam_role.firehose_role.arn
      bucket_arn         = aws_s3_bucket.failed_logs.arn
      buffering_size     = 5
      buffering_interval = 300
      compression_format = "GZIP"

      cloudwatch_logging_options {
        enabled         = true
        log_group_name  = aws_cloudwatch_log_group.firehose_logs.name
        log_stream_name = aws_cloudwatch_log_stream.firehose_backup.name
      }
    }

    cloudwatch_logging_options {
      enabled         = true
      log_group_name  = aws_cloudwatch_log_group.firehose_logs.name
      log_stream_name = aws_cloudwatch_log_stream.firehose_delivery.name
    }

    processing_configuration {
      enabled = var.enable_lambda_transformation

      dynamic "processors" {
        for_each = var.enable_lambda_transformation ? [1] : []
        content {
          type = "Lambda"

          parameters {
            parameter_name  = "LambdaArn"
            parameter_value = "${aws_lambda_function.bedrock_transformer[0].arn}:$LATEST"
          }

          parameters {
            parameter_name  = "BufferSizeInMBs"
            parameter_value = "3"
          }

          parameters {
            parameter_name  = "BufferIntervalInSeconds"
            parameter_value = "60"
          }
        }
      }
    }
  }

  tags = {
    Name        = "bedrock-to-dynatrace"
    Environment = var.environment
    ManagedBy   = "terraform"
  }

  depends_on = [
    aws_iam_role_policy_attachment.firehose_policy
  ]
}

# CloudWatch Logs Subscription Filter
resource "aws_cloudwatch_log_subscription_filter" "bedrock_to_firehose" {
  name            = "bedrock-to-firehose"
  log_group_name  = aws_cloudwatch_log_group.bedrock_logs.name
  filter_pattern  = ""
  destination_arn = aws_kinesis_firehose_delivery_stream.bedrock_to_dynatrace.arn
  role_arn        = aws_iam_role.cloudwatch_to_firehose.arn

  depends_on = [
    aws_iam_role_policy_attachment.cloudwatch_to_firehose_policy
  ]
}

# Bedrock Model Invocation Logging Configuration
resource "null_resource" "bedrock_logging_config" {
  count = var.enable_bedrock_logging ? 1 : 0

  provisioner "local-exec" {
    command = <<-EOT
      aws bedrock put-model-invocation-logging-configuration \
        --region ${var.aws_region} \
        --logging-config '{
          "cloudWatchConfig": {
            "logGroupName": "${aws_cloudwatch_log_group.bedrock_logs.name}",
            "roleArn": "${aws_iam_role.bedrock_logging.arn}",
            ${var.enable_large_data_delivery ? "\"largeDataDeliveryS3Config\": {\"bucketName\": \"${aws_s3_bucket.bedrock_large_data[0].bucket}\", \"keyPrefix\": \"bedrock-logs/\"}," : ""}
          },
          "textDataDeliveryEnabled": ${var.log_text_data},
          "imageDataDeliveryEnabled": ${var.log_image_data},
          "embeddingDataDeliveryEnabled": ${var.log_embedding_data}
        }'
    EOT
  }

  triggers = {
    log_group_name             = aws_cloudwatch_log_group.bedrock_logs.name
    role_arn                   = aws_iam_role.bedrock_logging.arn
    text_data_enabled          = var.log_text_data
    image_data_enabled         = var.log_image_data
    embedding_data_enabled     = var.log_embedding_data
    large_data_delivery_bucket = var.enable_large_data_delivery ? aws_s3_bucket.bedrock_large_data[0].bucket : ""
  }

  depends_on = [
    aws_cloudwatch_log_group.bedrock_logs,
    aws_iam_role.bedrock_logging
  ]
}
