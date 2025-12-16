"""
Bedrock Log Transformer for Dynatrace
This Lambda function transforms CloudWatch Logs from Amazon Bedrock
into a format optimized for Dynatrace log ingestion.
"""

import json
import base64
import gzip
import os
from datetime import datetime
from typing import Dict, List, Any

# Environment variables
LOG_LEVEL = os.environ.get('LOG_LEVEL', 'INFO')
AWS_REGION = os.environ.get('AWS_REGION', 'us-east-1')
ENVIRONMENT = os.environ.get('ENVIRONMENT', 'prod')


def lambda_handler(event: Dict[str, Any], context: Any) -> Dict[str, List[Dict[str, Any]]]:
    """
    Main Lambda handler for Firehose data transformation.
    
    Args:
        event: Firehose event containing log records
        context: Lambda context object
        
    Returns:
        Dictionary containing transformed records
    """
    output_records = []
    
    for record in event['records']:
        try:
            # Decode the Firehose record
            payload = base64.b64decode(record['data'])
            
            # Decompress if gzipped
            try:
                decompressed = gzip.decompress(payload)
                log_data = json.loads(decompressed.decode('utf-8'))
            except (gzip.BadGzipFile, OSError):
                # Not gzipped, treat as plain text
                log_data = json.loads(payload.decode('utf-8'))
            
            # Process CloudWatch Logs data
            if 'logEvents' in log_data:
                # Multiple log events from CloudWatch subscription
                transformed_logs = process_cloudwatch_logs(log_data)
            else:
                # Single log event
                transformed_logs = [transform_bedrock_log(log_data)]
            
            # Prepare output for Dynatrace (newline-delimited JSON)
            output_data = '\n'.join([json.dumps(log) for log in transformed_logs])
            
            output_record = {
                'recordId': record['recordId'],
                'result': 'Ok',
                'data': base64.b64encode(output_data.encode('utf-8')).decode('utf-8')
            }
            
        except Exception as e:
            print(f"Error processing record {record['recordId']}: {str(e)}")
            # Return original record on error
            output_record = {
                'recordId': record['recordId'],
                'result': 'ProcessingFailed',
                'data': record['data']
            }
        
        output_records.append(output_record)
    
    return {'records': output_records}


def process_cloudwatch_logs(log_data: Dict[str, Any]) -> List[Dict[str, Any]]:
    """
    Process CloudWatch Logs subscription filter data.
    
    Args:
        log_data: CloudWatch Logs data structure
        
    Returns:
        List of transformed log entries
    """
    transformed_logs = []
    
    for log_event in log_data.get('logEvents', []):
        try:
            # Parse the log message
            message = log_event.get('message', '')
            
            # Try to parse as JSON (Bedrock logs are JSON)
            try:
                bedrock_log = json.loads(message)
                transformed_log = transform_bedrock_log(bedrock_log, log_event)
            except json.JSONDecodeError:
                # Not JSON, create basic log entry
                transformed_log = {
                    'content': message,
                    'timestamp': log_event.get('timestamp'),
                    'log.source': 'aws.bedrock',
                    'status': 'INFO'
                }
            
            transformed_logs.append(transformed_log)
            
        except Exception as e:
            print(f"Error processing log event: {str(e)}")
            continue
    
    return transformed_logs


def transform_bedrock_log(bedrock_log: Dict[str, Any], log_event: Dict[str, Any] = None) -> Dict[str, Any]:
    """
    Transform a Bedrock log entry into Dynatrace format.
    
    Args:
        bedrock_log: Bedrock model invocation log data
        log_event: Optional CloudWatch log event metadata
        
    Returns:
        Transformed log entry for Dynatrace
    """
    # Base log structure
    dynatrace_log = {
        'content': json.dumps(bedrock_log),
        'timestamp': log_event.get('timestamp') if log_event else int(datetime.utcnow().timestamp() * 1000),
        'log.source': 'aws.bedrock',
        'cloud.provider': 'aws',
        'cloud.region': AWS_REGION,
        'service.name': 'bedrock',
        'environment': ENVIRONMENT
    }
    
    # Extract Bedrock-specific fields
    if 'modelId' in bedrock_log:
        dynatrace_log['aws.bedrock.model_id'] = bedrock_log['modelId']
        
        # Identify model provider
        if 'anthropic' in bedrock_log['modelId'].lower():
            dynatrace_log['aws.bedrock.provider'] = 'anthropic'
        elif 'amazon' in bedrock_log['modelId'].lower():
            dynatrace_log['aws.bedrock.provider'] = 'amazon'
        elif 'meta' in bedrock_log['modelId'].lower():
            dynatrace_log['aws.bedrock.provider'] = 'meta'
        elif 'cohere' in bedrock_log['modelId'].lower():
            dynatrace_log['aws.bedrock.provider'] = 'cohere'
        elif 'ai21' in bedrock_log['modelId'].lower():
            dynatrace_log['aws.bedrock.provider'] = 'ai21'
        elif 'stability' in bedrock_log['modelId'].lower():
            dynatrace_log['aws.bedrock.provider'] = 'stability'
    
    if 'requestId' in bedrock_log:
        dynatrace_log['aws.bedrock.request_id'] = bedrock_log['requestId']
    
    if 'operation' in bedrock_log:
        dynatrace_log['aws.bedrock.operation'] = bedrock_log['operation']
    
    # Token usage metrics
    if 'inputTokenCount' in bedrock_log:
        dynatrace_log['aws.bedrock.input_tokens'] = bedrock_log['inputTokenCount']
    
    if 'outputTokenCount' in bedrock_log:
        dynatrace_log['aws.bedrock.output_tokens'] = bedrock_log['outputTokenCount']
    
    if 'inputTokenCount' in bedrock_log and 'outputTokenCount' in bedrock_log:
        dynatrace_log['aws.bedrock.total_tokens'] = (
            bedrock_log['inputTokenCount'] + bedrock_log['outputTokenCount']
        )
    
    # Account and identity information
    if 'accountId' in bedrock_log:
        dynatrace_log['aws.account_id'] = bedrock_log['accountId']
    
    if 'identity' in bedrock_log:
        identity = bedrock_log['identity']
        if 'arn' in identity:
            dynatrace_log['aws.identity.arn'] = identity['arn']
    
    # Error handling
    if 'error' in bedrock_log:
        dynatrace_log['error'] = True
        dynatrace_log['error.message'] = bedrock_log['error']
        dynatrace_log['status'] = 'ERROR'
        dynatrace_log['severity'] = 'ERROR'
    else:
        dynatrace_log['status'] = 'INFO'
        dynatrace_log['severity'] = 'INFO'
    
    # Latency metrics
    if 'latency' in bedrock_log:
        dynatrace_log['aws.bedrock.latency_ms'] = bedrock_log['latency']
    
    # Input/Output content (optional - can be large)
    # Only include if specifically needed for debugging
    # if 'input' in bedrock_log:
    #     dynatrace_log['aws.bedrock.input'] = json.dumps(bedrock_log['input'])[:1000]  # Truncate to 1000 chars
    
    # if 'output' in bedrock_log:
    #     dynatrace_log['aws.bedrock.output'] = json.dumps(bedrock_log['output'])[:1000]  # Truncate to 1000 chars
    
    return dynatrace_log


def get_log_level_number(level: str) -> int:
    """Convert log level string to number."""
    levels = {
        'DEBUG': 10,
        'INFO': 20,
        'WARNING': 30,
        'ERROR': 40,
        'CRITICAL': 50
    }
    return levels.get(level.upper(), 20)


# Simple logging
def log(level: str, message: str):
    """Simple logging function."""
    if get_log_level_number(level) >= get_log_level_number(LOG_LEVEL):
        print(f"[{level}] {message}")
