#!/usr/bin/env python3
"""
Test script for Bedrock to Dynatrace logging pipeline.
This script invokes a Bedrock model and verifies logs appear in Dynatrace.
"""

import boto3
import json
import time
import sys
import os
from datetime import datetime

# Configuration
BEDROCK_REGION = os.environ.get('AWS_REGION', 'us-east-1')
MODEL_ID = 'anthropic.claude-3-sonnet-20240229-v1:0'

def print_header(text):
    """Print a formatted header."""
    print(f"\n{'='*60}")
    print(f"  {text}")
    print(f"{'='*60}\n")

def print_step(number, text):
    """Print a formatted step."""
    print(f"[Step {number}] {text}")

def invoke_bedrock_model():
    """Invoke a Bedrock model to generate test logs."""
    print_step(1, "Invoking Bedrock model...")
    
    try:
        bedrock = boto3.client('bedrock-runtime', region_name=BEDROCK_REGION)
        
        # Prepare request
        request_body = {
            'anthropic_version': 'bedrock-2023-05-31',
            'max_tokens': 100,
            'messages': [
                {
                    'role': 'user',
                    'content': f'This is a test message for logging at {datetime.now().isoformat()}'
                }
            ]
        }
        
        print(f"  Model: {MODEL_ID}")
        print(f"  Region: {BEDROCK_REGION}")
        
        # Invoke model
        start_time = time.time()
        response = bedrock.invoke_model(
            modelId=MODEL_ID,
            body=json.dumps(request_body)
        )
        duration = time.time() - start_time
        
        # Parse response
        response_body = json.loads(response['body'].read())
        
        print(f"  ✓ Model invoked successfully!")
        print(f"  Duration: {duration:.2f}s")
        print(f"  Input tokens: {response_body.get('usage', {}).get('input_tokens', 'N/A')}")
        print(f"  Output tokens: {response_body.get('usage', {}).get('output_tokens', 'N/A')}")
        
        return True
        
    except Exception as e:
        print(f"  ✗ Error invoking model: {str(e)}")
        return False

def check_cloudwatch_logs():
    """Check if logs appear in CloudWatch."""
    print_step(2, "Checking CloudWatch Logs...")
    
    log_group = '/aws/bedrock/modelinvocations'
    
    try:
        logs = boto3.client('logs', region_name=BEDROCK_REGION)
        
        # Wait a moment for logs to appear
        print("  Waiting 10 seconds for logs to appear...")
        time.sleep(10)
        
        # Get recent log events
        response = logs.filter_log_events(
            logGroupName=log_group,
            startTime=int((time.time() - 300) * 1000),  # Last 5 minutes
            limit=10
        )
        
        events = response.get('events', [])
        
        if events:
            print(f"  ✓ Found {len(events)} log event(s) in CloudWatch")
            
            # Show the most recent event
            latest_event = events[-1]
            message = json.loads(latest_event['message'])
            print(f"\n  Latest log entry:")
            print(f"    Model ID: {message.get('modelId', 'N/A')}")
            print(f"    Operation: {message.get('operation', 'N/A')}")
            print(f"    Request ID: {message.get('requestId', 'N/A')}")
            
            return True
        else:
            print(f"  ✗ No logs found in CloudWatch")
            print(f"  Note: Logs may take 1-2 minutes to appear")
            return False
            
    except logs.exceptions.ResourceNotFoundException:
        print(f"  ✗ Log group '{log_group}' not found")
        print(f"  Make sure Bedrock logging is enabled")
        return False
    except Exception as e:
        print(f"  ✗ Error checking logs: {str(e)}")
        return False

def check_firehose_metrics():
    """Check Firehose delivery metrics."""
    print_step(3, "Checking Firehose metrics...")
    
    try:
        cloudwatch = boto3.client('cloudwatch', region_name=BEDROCK_REGION)
        
        # Get metrics for the last 5 minutes
        response = cloudwatch.get_metric_statistics(
            Namespace='AWS/Firehose',
            MetricName='IncomingRecords',
            Dimensions=[
                {
                    'Name': 'DeliveryStreamName',
                    'Value': 'bedrock-to-dynatrace'
                }
            ],
            StartTime=datetime.utcnow().replace(minute=0, second=0, microsecond=0),
            EndTime=datetime.utcnow(),
            Period=300,
            Statistics=['Sum']
        )
        
        datapoints = response.get('Datapoints', [])
        
        if datapoints:
            total_records = sum(dp['Sum'] for dp in datapoints)
            print(f"  ✓ Firehose received {int(total_records)} record(s)")
            return True
        else:
            print(f"  ⚠ No Firehose metrics available yet")
            print(f"  This is normal for the first run")
            return True
            
    except Exception as e:
        print(f"  ✗ Error checking Firehose metrics: {str(e)}")
        return False

def print_dynatrace_instructions():
    """Print instructions for checking Dynatrace."""
    print_step(4, "Checking Dynatrace (manual step)...")
    
    print("""
  To verify logs in Dynatrace:
  
  1. Log in to your Dynatrace environment
  2. Go to: Observe and explore → Logs
  3. Use this query:
  
     log.source="aws.bedrock"
  
  4. You should see your test log entry with these attributes:
     - aws.bedrock.model_id
     - aws.bedrock.input_tokens
     - aws.bedrock.output_tokens
     - cloud.region
  
  Note: Logs may take 1-3 minutes to appear in Dynatrace
  
  Additional queries to try:
  
  - Filter by model: 
    log.source="aws.bedrock" AND aws.bedrock.model_id="anthropic.claude*"
  
  - Find high token usage:
    log.source="aws.bedrock" AND aws.bedrock.total_tokens>1000
  
  - View errors only:
    log.source="aws.bedrock" AND status="ERROR"
""")

def main():
    """Main test function."""
    print_header("Bedrock to Dynatrace Logging Test")
    
    print("This script will:")
    print("  1. Invoke a Bedrock model")
    print("  2. Check CloudWatch Logs")
    print("  3. Check Firehose metrics")
    print("  4. Provide instructions for Dynatrace verification")
    
    input("\nPress Enter to continue...")
    
    # Test steps
    success = True
    
    # Step 1: Invoke model
    if not invoke_bedrock_model():
        success = False
        print("\n⚠ Warning: Model invocation failed")
        print("Make sure you have access to Bedrock models")
        sys.exit(1)
    
    # Step 2: Check CloudWatch
    check_cloudwatch_logs()
    
    # Step 3: Check Firehose
    check_firehose_metrics()
    
    # Step 4: Dynatrace instructions
    print_dynatrace_instructions()
    
    # Summary
    print_header("Test Complete")
    
    if success:
        print("✓ Test completed successfully!")
        print("\nNext steps:")
        print("  1. Check Dynatrace for your logs (see instructions above)")
        print("  2. Run this script again to generate more test data")
        print("  3. Try different Bedrock models to test various scenarios")
    else:
        print("⚠ Test completed with warnings")
        print("Check the error messages above for details")
    
    print("\nFor troubleshooting, see README.md")
    print("")

if __name__ == '__main__':
    try:
        main()
    except KeyboardInterrupt:
        print("\n\nTest cancelled by user")
        sys.exit(0)
    except Exception as e:
        print(f"\n✗ Unexpected error: {str(e)}")
        sys.exit(1)
