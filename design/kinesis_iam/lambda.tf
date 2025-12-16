# Lambda Function for Log Transformation
data "archive_file" "lambda_zip" {
  count       = var.enable_lambda_transformation ? 1 : 0
  type        = "zip"
  source_file = "${path.module}/lambda/transformer.py"
  output_path = "${path.module}/lambda/transformer.zip"
}

resource "aws_lambda_function" "bedrock_transformer" {
  count            = var.enable_lambda_transformation ? 1 : 0
  filename         = data.archive_file.lambda_zip[0].output_path
  function_name    = "${var.project_name}-log-transformer"
  role             = aws_iam_role.lambda_transformer[0].arn
  handler          = "transformer.lambda_handler"
  source_code_hash = data.archive_file.lambda_zip[0].output_base64sha256
  runtime          = "python3.11"
  timeout          = var.lambda_timeout
  memory_size      = var.lambda_memory_size

  environment {
    variables = {
      LOG_LEVEL      = "INFO"
      AWS_REGION     = var.aws_region
      ENVIRONMENT    = var.environment
    }
  }

  tags = merge(
    var.tags,
    {
      Name        = "bedrock-log-transformer"
      Environment = var.environment
      ManagedBy   = "terraform"
    }
  )
}

# CloudWatch Log Group for Lambda
resource "aws_cloudwatch_log_group" "lambda_transformer" {
  count             = var.enable_lambda_transformation ? 1 : 0
  name              = "/aws/lambda/${aws_lambda_function.bedrock_transformer[0].function_name}"
  retention_in_days = var.log_retention_days

  tags = merge(
    var.tags,
    {
      Name        = "lambda-transformer-logs"
      Environment = var.environment
      ManagedBy   = "terraform"
    }
  )
}

# Lambda Permission for Firehose
resource "aws_lambda_permission" "allow_firehose" {
  count         = var.enable_lambda_transformation ? 1 : 0
  statement_id  = "AllowExecutionFromFirehose"
  action        = "lambda:InvokeFunction"
  function_name = aws_lambda_function.bedrock_transformer[0].function_name
  principal     = "firehose.amazonaws.com"
  source_arn    = aws_kinesis_firehose_delivery_stream.bedrock_to_dynatrace.arn
}
