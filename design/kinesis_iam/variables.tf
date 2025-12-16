variable "aws_region" {
  description = "AWS region where resources will be created"
  type        = string
  default     = "us-east-1"
}

variable "aws_account_id" {
  description = "AWS Account ID (used for unique S3 bucket naming)"
  type        = string
}

variable "environment" {
  description = "Environment name (e.g., dev, staging, prod)"
  type        = string
  default     = "prod"
}

variable "project_name" {
  description = "Project name for resource naming"
  type        = string
  default     = "bedrock-dynatrace"
}

variable "dynatrace_url" {
  description = "Dynatrace environment URL (e.g., https://abc12345.live.dynatrace.com)"
  type        = string
}

variable "dynatrace_api_token" {
  description = "Dynatrace API token with logs.ingest permission"
  type        = string
  sensitive   = true
}

variable "firehose_stream_name" {
  description = "Name of the Kinesis Firehose delivery stream"
  type        = string
  default     = "bedrock-to-dynatrace"
}

variable "firehose_buffer_size" {
  description = "Buffer size in MB for Firehose (1-5 MB recommended)"
  type        = number
  default     = 1
  validation {
    condition     = var.firehose_buffer_size >= 1 && var.firehose_buffer_size <= 5
    error_message = "Buffer size must be between 1 and 5 MB."
  }
}

variable "firehose_buffer_interval" {
  description = "Buffer interval in seconds for Firehose (60-900 seconds)"
  type        = number
  default     = 60
  validation {
    condition     = var.firehose_buffer_interval >= 60 && var.firehose_buffer_interval <= 900
    error_message = "Buffer interval must be between 60 and 900 seconds."
  }
}

variable "log_retention_days" {
  description = "CloudWatch Logs retention period in days"
  type        = number
  default     = 7
  validation {
    condition = contains([
      1, 3, 5, 7, 14, 30, 60, 90, 120, 150, 180, 365, 400, 545, 731, 1096, 1827, 2192, 2557, 2922, 3288, 3653
    ], var.log_retention_days)
    error_message = "Log retention days must be a valid CloudWatch Logs retention value."
  }
}

variable "failed_logs_retention_days" {
  description = "S3 lifecycle policy - days to retain failed logs"
  type        = number
  default     = 30
}

variable "enable_lambda_transformation" {
  description = "Enable Lambda transformation for log data"
  type        = bool
  default     = true
}

variable "enable_bedrock_logging" {
  description = "Enable Bedrock model invocation logging configuration"
  type        = bool
  default     = true
}

variable "enable_large_data_delivery" {
  description = "Enable S3 delivery for large data (images, embeddings)"
  type        = bool
  default     = false
}

variable "log_text_data" {
  description = "Enable logging of text data in Bedrock invocations"
  type        = bool
  default     = true
}

variable "log_image_data" {
  description = "Enable logging of image data in Bedrock invocations"
  type        = bool
  default     = false
}

variable "log_embedding_data" {
  description = "Enable logging of embedding data in Bedrock invocations"
  type        = bool
  default     = false
}

variable "lambda_memory_size" {
  description = "Memory size for Lambda function in MB"
  type        = number
  default     = 256
}

variable "lambda_timeout" {
  description = "Timeout for Lambda function in seconds"
  type        = number
  default     = 60
}

variable "tags" {
  description = "Additional tags to apply to all resources"
  type        = map(string)
  default     = {}
}
