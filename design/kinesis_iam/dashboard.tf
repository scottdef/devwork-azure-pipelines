# Optional CloudWatch Dashboard for Monitoring

resource "aws_cloudwatch_dashboard" "bedrock_logging" {
  count          = var.create_cloudwatch_dashboard ? 1 : 0
  dashboard_name = "${var.project_name}-monitoring"

  dashboard_body = jsonencode({
    widgets = [
      {
        type = "metric"
        properties = {
          title  = "Firehose - Incoming Records"
          region = var.aws_region
          metrics = [
            ["AWS/Firehose", "IncomingRecords", { stat = "Sum", period = 300 }],
            [".", "IncomingBytes", { stat = "Sum", period = 300 }]
          ]
          view    = "timeSeries"
          stacked = false
          period  = 300
        }
      },
      {
        type = "metric"
        properties = {
          title  = "Firehose - Delivery Success"
          region = var.aws_region
          metrics = [
            ["AWS/Firehose", "DeliveryToHttpEndpoint.Success", { stat = "Sum", period = 300 }],
            [".", "DeliveryToHttpEndpoint.Records", { stat = "Sum", period = 300 }]
          ]
          view    = "timeSeries"
          stacked = false
          period  = 300
        }
      },
      {
        type = "metric"
        properties = {
          title  = "Firehose - Data Freshness"
          region = var.aws_region
          metrics = [
            ["AWS/Firehose", "DeliveryToHttpEndpoint.DataFreshness", { stat = "Maximum", period = 300 }]
          ]
          view   = "timeSeries"
          period = 300
          yAxis = {
            left = {
              label = "Milliseconds"
            }
          }
        }
      },
      {
        type = "metric"
        properties = {
          title  = "Lambda - Invocations & Errors"
          region = var.aws_region
          metrics = var.enable_lambda_transformation ? [
            ["AWS/Lambda", "Invocations", { stat = "Sum", period = 300 }],
            [".", "Errors", { stat = "Sum", period = 300 }],
            [".", "Throttles", { stat = "Sum", period = 300 }]
          ] : []
          view    = "timeSeries"
          stacked = false
          period  = 300
        }
      },
      {
        type = "metric"
        properties = {
          title  = "Lambda - Duration"
          region = var.aws_region
          metrics = var.enable_lambda_transformation ? [
            ["AWS/Lambda", "Duration", { stat = "Average", period = 300 }],
            ["...", { stat = "Maximum", period = 300 }]
          ] : []
          view   = "timeSeries"
          period = 300
          yAxis = {
            left = {
              label = "Milliseconds"
            }
          }
        }
      },
      {
        type = "log"
        properties = {
          title  = "Recent Bedrock Logs"
          region = var.aws_region
          query  = <<-EOQ
            SOURCE '${aws_cloudwatch_log_group.bedrock_logs.name}'
            | fields @timestamp, modelId, operation, inputTokenCount, outputTokenCount
            | sort @timestamp desc
            | limit 20
          EOQ
        }
      },
      {
        type = "log"
        properties = {
          title  = "Firehose Delivery Errors"
          region = var.aws_region
          query  = <<-EOQ
            SOURCE '${aws_cloudwatch_log_group.firehose_logs.name}'
            | fields @timestamp, @message
            | filter @message like /ERROR/ or @message like /Failed/
            | sort @timestamp desc
            | limit 20
          EOQ
        }
      },
      {
        type = "metric"
        properties = {
          title  = "S3 - Failed Logs Storage"
          region = var.aws_region
          metrics = [
            ["AWS/S3", "BucketSizeBytes", { stat = "Average", period = 86400 }],
            [".", "NumberOfObjects", { stat = "Average", period = 86400 }]
          ]
          view    = "singleValue"
          period  = 86400
          stacked = false
        }
      }
    ]
  })
}

# Add this variable to variables.tf
variable "create_cloudwatch_dashboard" {
  description = "Create CloudWatch dashboard for monitoring"
  type        = bool
  default     = true
}

# Add this output to outputs.tf
output "cloudwatch_dashboard_url" {
  description = "URL to CloudWatch dashboard"
  value = var.create_cloudwatch_dashboard ? "https://console.aws.amazon.com/cloudwatch/home?region=${var.aws_region}#dashboards:name=${aws_cloudwatch_dashboard.bedrock_logging[0].dashboard_name}" : null
}
