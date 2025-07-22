# Azure DevOps Environment Processor

This Go script processes Azure DevOps environments and organizes them by environment levels (prod, stg, uat, qa, dev).

## Setup Instructions

### 1. Prerequisites

- Go 1.21 or later
- Azure DevOps Personal Access Token with Environment (Read) permissions
- Access to the Azure DevOps organization and project

### 2. Get Your Personal Access Token

1. Go to Azure DevOps → User Settings → Personal Access Tokens
2. Create a new token with **Environment (Read)** scope
3. Copy the token value

### 3. Configuration

Update the `getConfig()` function in the script with your Azure DevOps details:

```go
func getConfig() Config {
    return Config{
        Organization: "your-org-name",        // ← Change this
        Project:      "your-project-name",    // ← Change this
        PAT:          os.Getenv("AZURE_DEVOPS_PAT"),
        APIVersion:   "7.2-preview",
    }
}
```

## Usage

### 1. Set Environment Variable

```bash
export AZURE_DEVOPS_PAT="your-pat-token-here"
```

### 2. Prepare Environment Levels File

Create or use the provided `environment-levels.json` file:

```json
["prod", "stg", "uat", "qa", "dev"]
```

### 3. Run the Script

```bash
go run azure-devops-env-processor.go environment-levels.json
```

## Example Output

For each environment level (e.g., "dev"), the script creates a file like `dev-ids.json`:

```json
{
  "dataengineering-etl": 264,
  "dataoperations": 228,
  "enterprisevetting": 86,
  "ofs": 33,
  "taxexchange": 4,
  "tax-solutions": 26,
  "idList": [264, 228, 86, 33, 4, 26]
}
```

## How It Works

1. **API Call**: Fetches all environments from Azure DevOps using the `/environments` API
2. **Pattern Matching**: For each environment level, finds environments ending with `-{level}`
3. **Key Generation**: Removes the `-{level}` suffix to create clean keys
4. **Output**: Creates JSON files with mappings and ID lists

### Matching Examples

- `dataengineering-etl-dev` → key: `dataengineering-etl`, level: `dev`
- `taxexchange-prod` → key: `taxexchange`, level: `prod`
- `ofs-qa` → key: `ofs`, level: `qa`

## Output Files

For each environment level in your `environment-levels.json`, you'll get:

- `prod-ids.json` - All production environment mappings
- `stg-ids.json` - All staging environment mappings
- `uat-ids.json` - All UAT environment mappings
- `qa-ids.json` - All QA environment mappings
- `dev-ids.json` - All development environment mappings

Each file contains:
- Individual environment mappings (name → ID)
- `idList` array with all environment IDs for that level

## Troubleshooting

### Authentication Issues

```bash
# Error: AZURE_DEVOPS_PAT environment variable is required
export AZURE_DEVOPS_PAT="your-token-here"
```

### Configuration Issues

```bash
# Error: API call failed with status 404
# Check organization and project names in getConfig()
```

### File Issues

```bash
# Error: error reading file environment-levels.json
# Make sure the file exists and contains valid JSON array
```

### Common Patterns

The script handles these environment naming patterns:
- `service-name-{level}` (e.g., `taxexchange-prod`)
- `service-name-type-{level}` (e.g., `dataengineering-etl-dev`)

## Integration with Kubernetes

Use the generated ID mappings for kubectl context switching:

```bash
# Example: Switch to dev environment
ENV_ID=$(cat dev-ids.json | jq -r '.["your-service"]')
kubectl config use-context "env-${ENV_ID}"
```

## Grafana Integration

Use environment mappings to configure Grafana dashboards:

```bash
# Generate Grafana variables from environment files
for level in prod stg uat qa dev; do
    echo "Creating Grafana variables for ${level}..."
    jq -r 'keys[]' ${level}-ids.json > grafana-${level}-services.txt
done
```
