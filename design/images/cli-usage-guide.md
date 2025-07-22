# Azure DevOps CLI - Complete Guide

A comprehensive CLI tool for managing Azure DevOps environments and analyzing deployment records.

## Features

✅ **Environment Management**: List and organize environments by levels (prod, stg, uat, qa, dev)  
✅ **Deployment Records**: Fetch deployment history for any environment  
✅ **Analytics**: Generate detailed deployment summaries with success rates  
✅ **Time Filtering**: Monthly summaries for recent deployment trends  
✅ **Batch Processing**: Full workflow automation for all environments  
✅ **Export**: JSON outputs for integration with other tools  

## Installation

### Prerequisites
- Go 1.21 or later
- Azure DevOps Personal Access Token
- Access to Azure DevOps environments

### Build
```bash
# Clone or download the ado-cli.go file
go build -o ado-cli ado-cli.go

# Or run directly
go run ado-cli.go <command>
```

## Configuration

### Environment Variables

Set these before using the CLI:

```bash
# Required: Personal Access Token
export AZURE_DEVOPS_PAT="your-pat-token-here"

# Optional: Override defaults
export ADO_ORGANIZATION="your-org-name"     # default: myorga
export ADO_PROJECT="your-project-name"      # default: project001
```

### Get Your PAT Token

1. Go to Azure DevOps → User Settings → Personal Access Tokens
2. Create token with these scopes:
   - **Environment (Read)** - Required for environment access
   - **Build (Read)** - May be required for deployment records
3. Copy the token value and set `AZURE_DEVOPS_PAT`

### Environment Levels File

Create `environment-levels.json`:
```json
["prod", "stg", "uat", "qa", "dev"]
```

## Commands

### 1. environments - Process Environments

Lists all environments and organizes them by levels:

```bash
# Use default environment-levels.json
./ado-cli environments

# Use custom levels file
./ado-cli environments -levels my-levels.json
```

**Output:** Creates `{level}-ids.json` files with environment mappings:
```json
{
  "dataengineering-etl": 264,
  "taxexchange": 8,
  "ofs": 33,
  "idList": [264, 8, 33]
}
```

### 2. deployments - Fetch Deployment Records

Gets deployment history for specific environments:

```bash
# Single environment
./ado-cli deployments -envs "8"

# Multiple environments
./ado-cli deployments -envs "8,10,15,20"

# From environment file (get IDs first)
ENV_IDS=$(jq -r '.idList | join(",")' prod-ids.json)
./ado-cli deployments -envs "$ENV_IDS"
```

**Output:** Creates `env-dep-rec-res-{id}.json` files with raw deployment data.

### 3. summary - Generate Analytics

Creates deployment summaries with success rates and statistics:

```bash
# Basic summary
./ado-cli summary -input env-dep-rec-res-8.json -env 8

# Monthly summary (last 30 days only)
./ado-cli summary -input env-dep-rec-res-8.json -env 8 -monthly

# Without environment ID in filename
./ado-cli summary -input env-dep-rec-res-8.json
```

**Output:** Creates summary files like `deploy-summary-8.json`:
```json
[
  {
    "pipeline-repo": "web-api-service",
    "total_deployments": 45,
    "total_succeeded": 42,
    "total_failed": 3,
    "percent_succeeded": 93.3,
    "latest": "2025-07-20T10:30:00Z"
  }
]
```

### 4. full - Complete Workflow

Runs the entire pipeline: environments → deployments → summaries:

```bash
# Complete workflow with monthly summaries
./ado-cli full -monthly

# Use custom levels file
./ado-cli full -levels my-levels.json -monthly

# All-time summaries only
./ado-cli full -levels environment-levels.json
```

This command:
1. Processes all environments by levels
2. Fetches deployment records for all found environments
3. Generates both all-time and monthly summaries (if `-monthly` flag used)

## Usage Examples

### Quick Start

```bash
# 1. Set up authentication
export AZURE_DEVOPS_PAT="your-token"
export ADO_ORGANIZATION="contoso"
export ADO_PROJECT="MyProject"

# 2. Create environment levels file
echo '["prod", "stg", "dev"]' > environment-levels.json

# 3. Run full workflow
./ado-cli full -monthly
```

### Environment Analysis

```bash
# Find all production environments
./ado-cli environments
cat prod-ids.json | jq -r 'keys[]'

# Get deployment records for production
PROD_IDS=$(jq -r '.idList | join(",")' prod-ids.json)
./ado-cli deployments -envs "$PROD_IDS"
```

### Deployment Monitoring

```bash
# Generate monthly reports for all environments
for file in env-dep-rec-res-*.json; do
    env_id=$(echo $file | grep -o '[0-9]\+')
    ./ado-cli summary -input "$file" -env "$env_id" -monthly
done

# Find environments with low success rates
for file in deploy-summary-*-monthly.json; do
    success_rate=$(jq -r '.[0].percent_succeeded' "$file" 2>/dev/null)
    if (( $(echo "$success_rate < 90" | bc -l) )); then
        echo "Low success rate in $file: $success_rate%"
    fi
done
```

### Integration with Other Tools

```bash
# Export for Grafana dashboard
./ado-cli full
find . -name "deploy-summary-*.json" -exec cat {} \; | \
jq -s 'flatten | group_by(.["pipeline-repo"]) | map(.[0])' > grafana-data.json

# Generate CSV for Excel
echo "Pipeline,Environment,Deployments,Success Rate,Latest" > deployments.csv
for file in deploy-summary-*.json; do
    env=$(echo $file | grep -o '[0-9]\+')
    jq -r ".[] | \"\\(.[\\"pipeline-repo\\"]),${env},\\(.total_deployments),\\(.percent_succeeded),\\(.latest)\"" "$file" >> deployments.csv
done
```

## Output Files

After running commands, you'll have:

### Environment Files
- `{level}-ids.json` - Environment mappings per level
- Example: `prod-ids.json`, `dev-ids.json`

### Deployment Files  
- `env-dep-rec-res-{id}.json` - Raw deployment records per environment
- Example: `env-dep-rec-res-8.json`

### Summary Files
- `deploy-summary-{id}.json` - All-time deployment statistics
- `deploy-summary-{id}-monthly.json` - Last 30 days statistics
- Example: `deploy-summary-8.json`, `deploy-summary-8-monthly.json`

## Troubleshooting

### Authentication Issues
```bash
# Test your token
curl -H "Authorization: Basic $(echo -n $AZURE_DEVOPS_PAT: | base64)" \
     "https://dev.azure.com/$ADO_ORGANIZATION/$ADO_PROJECT/_apis/distributedtask/environments?api-version=7.2-preview"
```

### No Environments Found
- Check organization and project names in environment variables
- Verify PAT token has Environment (Read) permissions
- Confirm environments exist in the specified project

### Missing Deployment Records
- Environments may not have any deployments
- Check if environment IDs are correct
- Verify PAT token has sufficient permissions

### JSON Parsing Errors
- Check input files for corruption
- Verify API responses are complete
- Look for network timeout issues during large data fetches

## Advanced Usage

### Custom Configuration

```bash
# Use different API version
export ADO_API_VERSION="6.0-preview"

# Process specific environment patterns
./ado-cli environments -levels <(echo '["prod", "staging"]')
```

### Automation Scripts

Create a monitoring script:
```bash
#!/bin/bash
# daily-report.sh

export AZURE_DEVOPS_PAT="$1"
export ADO_ORGANIZATION="$2" 
export ADO_PROJECT="$3"

./ado-cli full -monthly

# Send results to monitoring system
for file in deploy-summary-*-monthly.json; do
    # Process and send to your monitoring tool
    curl -X POST -d @"$file" "https://monitoring.example.com/api/deployments"
done
```

### Kubernetes Integration

```bash
# Use with kubectl context switching
for level in prod stg dev; do
    if [ -f "${level}-ids.json" ]; then
        kubectl config use-context "${level}-cluster"
        # Deploy to environments in this level
    fi
done
```

## API Endpoints Used

- `distributedtask/environments` - List environments
- `distributedtask/environments/{id}/environmentdeploymentrecords` - Get deployment records

## License

This tool uses only Go standard library packages and is compatible with Go 1.21+ and kubectl 1.30 as specified.
