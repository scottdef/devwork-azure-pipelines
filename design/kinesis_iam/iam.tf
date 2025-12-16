# IAM Role for Bedrock Logging
resource "aws_iam_role" "bedrock_logging" {
  name = "${var.project_name}-bedrock-logging-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Principal = {
          Service = "bedrock.amazonaws.com"
        }
        Action = "sts:AssumeRole"
      }
    ]
  })

  tags = merge(
    var.tags,
    {
      Name        = "bedrock-logging-role"
      Environment = var.environment
      ManagedBy   = "terraform"
    }
  )
}

resource "aws_iam_role_policy" "bedrock_logging_policy" {
  name = "${var.project_name}-bedrock-logging-policy"
  role = aws_iam_role.bedrock_logging.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "logs:CreateLogStream",
          "logs:PutLogEvents"
        ]
        Resource = "${aws_cloudwatch_log_group.bedrock_logs.arn}:*"
      },
      {
        Effect = "Allow"
        Action = [
          "s3:PutObject"
        ]
        Resource = var.enable_large_data_delivery ? [
          "${aws_s3_bucket.bedrock_large_data[0].arn}/*"
        ] : []
      }
    ]
  })
}

# IAM Role for CloudWatch Logs to Firehose
resource "aws_iam_role" "cloudwatch_to_firehose" {
  name = "${var.project_name}-cloudwatch-to-firehose-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Principal = {
          Service = "logs.amazonaws.com"
        }
        Action = "sts:AssumeRole"
        Condition = {
          StringLike = {
            "aws:SourceArn" = "arn:aws:logs:${var.aws_region}:${var.aws_account_id}:*"
          }
        }
      }
    ]
  })

  tags = merge(
    var.tags,
    {
      Name        = "cloudwatch-to-firehose-role"
      Environment = var.environment
      ManagedBy   = "terraform"
    }
  )
}

resource "aws_iam_role_policy" "cloudwatch_to_firehose_policy" {
  name = "${var.project_name}-cloudwatch-to-firehose-policy"
  role = aws_iam_role.cloudwatch_to_firehose.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "firehose:PutRecord",
          "firehose:PutRecordBatch"
        ]
        Resource = aws_kinesis_firehose_delivery_stream.bedrock_to_dynatrace.arn
      }
    ]
  })
}

resource "aws_iam_role_policy_attachment" "cloudwatch_to_firehose_policy" {
  role       = aws_iam_role.cloudwatch_to_firehose.name
  policy_arn = aws_iam_policy.cloudwatch_to_firehose.arn
}

resource "aws_iam_policy" "cloudwatch_to_firehose" {
  name        = "${var.project_name}-cloudwatch-to-firehose-policy"
  description = "Policy for CloudWatch Logs to send data to Firehose"

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "firehose:PutRecord",
          "firehose:PutRecordBatch"
        ]
        Resource = aws_kinesis_firehose_delivery_stream.bedrock_to_dynatrace.arn
      }
    ]
  })

  tags = merge(
    var.tags,
    {
      Name        = "cloudwatch-to-firehose-policy"
      Environment = var.environment
      ManagedBy   = "terraform"
    }
  )
}

# IAM Role for Firehose
resource "aws_iam_role" "firehose_role" {
  name = "${var.project_name}-firehose-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Principal = {
          Service = "firehose.amazonaws.com"
        }
        Action = "sts:AssumeRole"
        Condition = {
          StringEquals = {
            "sts:ExternalId" = var.aws_account_id
          }
        }
      }
    ]
  })

  tags = merge(
    var.tags,
    {
      Name        = "firehose-role"
      Environment = var.environment
      ManagedBy   = "terraform"
    }
  )
}

resource "aws_iam_role_policy" "firehose_policy" {
  name = "${var.project_name}-firehose-policy"
  role = aws_iam_role.firehose_role.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "s3:AbortMultipartUpload",
          "s3:GetBucketLocation",
          "s3:GetObject",
          "s3:ListBucket",
          "s3:ListBucketMultipartUploads",
          "s3:PutObject"
        ]
        Resource = [
          aws_s3_bucket.failed_logs.arn,
          "${aws_s3_bucket.failed_logs.arn}/*"
        ]
      },
      {
        Effect = "Allow"
        Action = [
          "logs:PutLogEvents"
        ]
        Resource = "${aws_cloudwatch_log_group.firehose_logs.arn}:*"
      },
      {
        Effect = "Allow"
        Action = [
          "lambda:InvokeFunction",
          "lambda:GetFunctionConfiguration"
        ]
        Resource = var.enable_lambda_transformation ? [
          "${aws_lambda_function.bedrock_transformer[0].arn}:*"
        ] : []
      }
    ]
  })
}

resource "aws_iam_role_policy_attachment" "firehose_policy" {
  role       = aws_iam_role.firehose_role.name
  policy_arn = aws_iam_policy.firehose.arn
}

resource "aws_iam_policy" "firehose" {
  name        = "${var.project_name}-firehose-policy"
  description = "Policy for Firehose to deliver logs"

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "s3:AbortMultipartUpload",
          "s3:GetBucketLocation",
          "s3:GetObject",
          "s3:ListBucket",
          "s3:ListBucketMultipartUploads",
          "s3:PutObject"
        ]
        Resource = [
          aws_s3_bucket.failed_logs.arn,
          "${aws_s3_bucket.failed_logs.arn}/*"
        ]
      },
      {
        Effect = "Allow"
        Action = [
          "logs:PutLogEvents"
        ]
        Resource = "${aws_cloudwatch_log_group.firehose_logs.arn}:*"
      },
      {
        Effect = "Allow"
        Action = [
          "lambda:InvokeFunction",
          "lambda:GetFunctionConfiguration"
        ]
        Resource = var.enable_lambda_transformation ? [
          "${aws_lambda_function.bedrock_transformer[0].arn}:*"
        ] : []
      }
    ]
  })

  tags = merge(
    var.tags,
    {
      Name        = "firehose-policy"
      Environment = var.environment
      ManagedBy   = "terraform"
    }
  )
}

# IAM Role for Lambda Transformer
resource "aws_iam_role" "lambda_transformer" {
  count = var.enable_lambda_transformation ? 1 : 0
  name  = "${var.project_name}-lambda-transformer-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Principal = {
          Service = "lambda.amazonaws.com"
        }
        Action = "sts:AssumeRole"
      }
    ]
  })

  tags = merge(
    var.tags,
    {
      Name        = "lambda-transformer-role"
      Environment = var.environment
      ManagedBy   = "terraform"
    }
  )
}

resource "aws_iam_role_policy_attachment" "lambda_transformer_basic" {
  count      = var.enable_lambda_transformation ? 1 : 0
  role       = aws_iam_role.lambda_transformer[0].name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole"
}

resource "aws_iam_role_policy" "lambda_transformer_policy" {
  count = var.enable_lambda_transformation ? 1 : 0
  name  = "${var.project_name}-lambda-transformer-policy"
  role  = aws_iam_role.lambda_transformer[0].id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "logs:CreateLogGroup",
          "logs:CreateLogStream",
          "logs:PutLogEvents"
        ]
        Resource = "arn:aws:logs:${var.aws_region}:${var.aws_account_id}:*"
      }
    ]
  })
}
