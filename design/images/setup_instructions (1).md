# Azure DevOps Deployment Records Processor

This solution consists of two Go scripts that work together to fetch and analyze Azure DevOps deployment records.

## Files Overview

1. **`deploy-summary.go`** - Processes JSON deployment records and generates summaries
2. **`azure-devops-processor.go`** - Orchestrates API calls and summary generation
3. **Setup files** - Configuration and usage instructions

## Setup Instructions

### 1. Prerequisites

- Go 1.21 or later installed
- Azure DevOps Personal Access Token (PAT) with appropriate permissions
- Access to Azure DevOps environments

### 2. Get Your Personal Access Token

1. Go to Azure DevOps → User Settings → Personal Access Tokens
2. Create a new token with these scopes:
   - **Environment (Read)** - Required for reading deployment records
   - **Build (Read)** - May be required depending on your setup
3. Copy the token value

### 3. Configuration

Edit `azure-devops-processor.go` and update the `getConfig()` function:

```go
func getConfig() Config {
    return Config{
        Organization: "your-org-name",    // Replace with your organization
        Project:      "your-project",     // Replace with your project
        PAT:          os.Getenv("AZURE_DEVOPS_PAT"),
        APIVersion:   "7.2-preview",
    }
}
```

Update the environment IDs list in the `main()` function:

```go
envIds := []int{8, 10, 15, 20} // Replace with your environment IDs
```

## Usage

### Option 1: Quick Start (Recommended)

```bash
# Set your PAT token
export AZURE_DEVOPS_PAT="your-pat-token-here"

# Run the processor
go run azure-devops-processor.go
```

### Option 2: Step by Step

```bash
# 1. Set environment variable
export AZURE_DEVOPS_PAT="your-pat-token-here"

# 2. Compile the summary script (optional, will auto-compile)
go build -o deploy-summary deploy-summary.go

# 3. Run the main processor
go run azure-devops-processor.go
```

### Option 3: Build and Run

```bash
# Build both scripts
go build -o azure-processor azure-devops-processor.go
go build -o deploy-summary deploy-summary.go

# Set PAT and run
export AZURE_DEVOPS_PAT="your-pat-token-here"
./azure-processor
```

## Output Files

For each environment ID (e.g., envId=8), the following files are generated:

- **`env-dep-rec-res-8.json`** - Raw API response from Azure DevOps
- **`deploy-summary-8.json`** - Complete deployment summary (all records)
- **`deploy-summary-8-monthly.json`** - Monthly summary (last 30 days only)

## Summary File Structure

Each summary file contains:

```json
[
  {
    "pipeline-repo": "web-api-example",
    "total_deployments": 25,
    "total_succeeded": 23,
    "total_failed": 2,
    "percent_succeeded": 92.0,
    "latest": "2025-06-27T10:30:00Z"
  }
]
```

## Manual Testing

To test the summary script independently:

```bash
# Test with existing data file
go run deploy-summary.go -input env-dep-rec-res.json

# Test monthly filtering
go run deploy-summary.go -input env-dep-rec-res.json -monthly
```

## Troubleshooting

### Authentication Issues
- Verify your PAT token has correct permissions
- Check that the token hasn't expired
- Ensure the organization and project names are correct

### API Issues
- Verify environment IDs exist and are accessible
- Check network connectivity to dev.azure.com
- Review API version compatibility

### File Issues
- Ensure Go has write permissions in the current directory
- Check disk space for output files
- Verify input file paths are correct

## Environment Variables

| Variable | Description | Required |
|----------|-------------|----------|
| `AZURE_DEVOPS_PAT` | Personal Access Token | Yes |

## Command Line Options

### deploy-summary.go
- `-input string` - Input JSON file path (default: "env-dep-rec-res.json")
- `-monthly` - Filter to last 30 days only (default: false)

### azure-devops-processor.go
- No command line options (configured in code)

## Example Workflow

1. **Setup**: Configure organization, project, and environment IDs
2. **Authentication**: Set AZURE_DEVOPS_PAT environment variable
3. **Execute**: Run `go run azure-devops-processor.go`
4. **Review**: Check generated summary files for insights
5. **Analyze**: Compare all-time vs monthly trends

## Next Steps

- Schedule regular execution for monitoring
- Integrate with dashboard tools (Grafana, Power BI)
- Add alerting based on success rate thresholds
- Extend with additional metrics and filtering options