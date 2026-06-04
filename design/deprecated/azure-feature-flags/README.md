# Azure App Configuration Feature Flag Management

Infrastructure-as-Code management of Azure App Configuration feature flags using Terraform, driven by a JSON intent file and automated through GitHub Actions.

## Architecture

```
app-feature-flags.json          ← Single source of intent (what flags SHOULD be)
        │
        ├──[push to main]──→ feature-flag-update.yml
        │                       ├── compare-flags.py   (report drift + orphans)
        │                       ├── terraform plan      (diff against state)
        │                       └── terraform apply     (reconcile)
        │                               │
        │                               └──[on success]──→ feature-flag-export.yml
        │                                                    ├── az appconfig feature list
        │                                                    ├── write <instance>.json
        │                                                    └── git push → feature-flags branch
        │
        └── terraform/
              ├── providers.tf    (azurerm + OIDC backend)
              ├── variables.tf
              ├── main.tf         (azurerm_app_configuration_feature for_each)
              └── outputs.tf
```

## Behavior

**Update workflow** (triggered by changes to `app-feature-flags.json`):

- Imports existing flags from App Configuration via `az appconfig feature list`
- If a flag exists in App Config but NOT in the JSON → printed as "NOT PRESENT" (never deleted)
- If a flag exists in both and `enabled` differs → Terraform updates App Config to match JSON
- If a flag is in JSON but not in App Config → Terraform creates it

**Export workflow** (triggered after successful update, on schedule, or manually):

- Pulls all flags from App Configuration
- Writes them to `<app-config-name>.json` on the `feature-flags` branch
- File is named after the App Configuration instance

## Prerequisites

- Azure App Configuration instance
- User-assigned managed identity with `App Configuration Data Owner` role
- Federated credential for GitHub Actions OIDC
- Azure Storage Account for Terraform state backend

## Setup

### 1. Configure GitHub repository variables

```bash
gh variable set AZURE_CLIENT_ID       --body "<managed-identity-client-id>"
gh variable set AZURE_TENANT_ID       --body "<tenant-id>"
gh variable set AZURE_SUBSCRIPTION_ID --body "<subscription-id>"
gh variable set APP_CONFIG_NAME       --body "<app-configuration-name>"
gh variable set RESOURCE_GROUP        --body "<resource-group>"
gh variable set TF_BACKEND_RESOURCE_GROUP  --body "<tfstate-rg>"
gh variable set TF_BACKEND_STORAGE_ACCOUNT --body "<tfstate-storage>"
gh variable set TF_BACKEND_CONTAINER       --body "tfstate"
```

### 2. Create the `dev` environment

```bash
gh api repos/{owner}/{repo}/environments/dev --method PUT
```

### 3. Configure federated identity credential

```bash
az ad app federated-credential create \
  --id <app-registration-object-id> \
  --parameters '{
    "name": "github-actions-dev",
    "issuer": "https://token.actions.githubusercontent.com",
    "subject": "repo:<org>/<repo>:environment:dev",
    "audiences": ["api://AzureADTokenExchange"]
  }'
```

### 4. Local development

```bash
cp .env.example .env
# Edit .env with your values
az login
make compare   # see what would change
make plan      # terraform plan
make apply     # terraform apply
make export    # export flags to local file
```

## JSON Format

```json
[
  { "name": "enable-dark-mode",    "enabled": true  },
  { "name": "beta-dashboard",      "enabled": false },
  { "name": "new-checkout-flow",   "enabled": true  }
]
```

Each entry has exactly two keys: `name` (string) and `enabled` (boolean).

## Makefile Targets

| Target     | Description                                        |
|------------|----------------------------------------------------|
| `validate` | Validate JSON syntax and schema                    |
| `compare`  | Compare JSON against live App Configuration        |
| `init`     | Initialize Terraform                               |
| `plan`     | Run Terraform plan                                 |
| `apply`    | Apply Terraform changes                            |
| `export`   | Export flags from App Config to local file          |
| `clean`    | Remove local Terraform state and plan artifacts    |
