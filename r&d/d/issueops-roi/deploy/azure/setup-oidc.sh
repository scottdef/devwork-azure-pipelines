#!/usr/bin/env bash
# Creates the Azure identities used by the CoolEngOrg IssueOps platform:
#
#   GitHub Actions -> Azure (OIDC, no secrets). One Entra application per GitHub
#   environment, each with a federated credential whose subject is
#     repo:CoolEngOrg/issueops:environment:<environment>
#   so only jobs running in that environment (with its reviewers) can use it.
#
#     issueops-gh-foundry-dev|uat|prod  Cognitive Services Contributor on the
#                                       Foundry resource group + quota reader
#     issueops-gh-agentic-aks           AKS Cluster User + AKS RBAC Writer on
#                                       the issueops-agents namespace
#     issueops-gh-acr-push              AcrPush on the registry
#
#   AKS agent runner -> Foundry (Workload Identity). A user-assigned managed
#   identity federated to system:serviceaccount:issueops-agents:issueops-agent-runner
#   with Cognitive Services OpenAI User on the production Foundry account.
#
# Prints the GitHub variables to configure. Re-runnable: existing objects are reused.
# Requires: az CLI logged in as someone who can create app registrations and
# role assignments (Owner or User Access Administrator on the scopes).
set -euo pipefail

: "${SUBSCRIPTION_ID:?set SUBSCRIPTION_ID}"
ORG="${ORG:-CoolEngOrg}"
REPO="${REPO:-issueops}"
LOCATION="${LOCATION:-eastus2}"
# GitHub Enterprise Cloud with a customized OIDC issuer (enterprise setting)
# uses https://token.actions.githubusercontent.com/<enterprise-slug>.
OIDC_ISSUER="${OIDC_ISSUER:-https://token.actions.githubusercontent.com}"

declare -A FOUNDRY_RG=(
  [dev]="${FOUNDRY_RG_DEV:-rg-cooleng-ai-dev}"
  [uat]="${FOUNDRY_RG_UAT:-rg-cooleng-ai-uat}"
  [prod]="${FOUNDRY_RG_PROD:-rg-cooleng-ai-prod}"
)
PROD_ACCOUNT="${PROD_ACCOUNT:-aif-cooleng-prod-eus2}"
AKS_RG="${AKS_RG:-rg-cooleng-aks-prod}"
AKS_NAME="${AKS_NAME:-aks-cooleng-prod-eus2}"
ACR_NAME="${ACR_NAME:-acrcooleng}"
ACR_RG="${ACR_RG:-rg-cooleng-shared}"
MI_RG="${MI_RG:-$AKS_RG}"

az account set --subscription "$SUBSCRIPTION_ID"
TENANT_ID=$(az account show --query tenantId -o tsv)
SUB_SCOPE="/subscriptions/$SUBSCRIPTION_ID"

# app_for_environment <app-name> <github-environment> -> prints appId
app_for_environment() {
  local name=$1 env=$2 app_id
  app_id=$(az ad app list --display-name "$name" --query '[0].appId' -o tsv)
  if [[ -z "$app_id" ]]; then
    app_id=$(az ad app create --display-name "$name" --sign-in-audience AzureADMyOrg --query appId -o tsv)
    az ad sp create --id "$app_id" >/dev/null
  fi
  local fic_name="github-${env}"
  if ! az ad app federated-credential list --id "$app_id" --query "[?name=='$fic_name'] | [0].name" -o tsv | grep -q .; then
    az ad app federated-credential create --id "$app_id" --parameters "$(cat <<JSON
{
  "name": "$fic_name",
  "issuer": "$OIDC_ISSUER",
  "subject": "repo:$ORG/$REPO:environment:$env",
  "audiences": ["api://AzureADTokenExchange"],
  "description": "GitHub Actions $ORG/$REPO environment $env (IssueOps)"
}
JSON
)" >/dev/null
  fi
  echo "$app_id"
}

assign() { # assign <appId | principalObjectId> <role> <scope> [app|object]
  local who=$1 role=$2 scope=$3 kind=${4:-app} oid ptype=ServicePrincipal
  if [[ "$kind" == "app" ]]; then
    oid=$(az ad sp show --id "$who" --query id -o tsv)
  else
    oid=$who
  fi
  az role assignment create --assignee-object-id "$oid" --assignee-principal-type "$ptype" \
    --role "$role" --scope "$scope" --only-show-errors >/dev/null || true
}

# Custom role: read Cognitive Services quota (az cognitiveservices usage list)
# and model catalogs at subscription scope, nothing else.
QUOTA_ROLE="IssueOps Foundry Quota Reader"
if ! az role definition list --name "$QUOTA_ROLE" --query '[0].name' -o tsv | grep -q .; then
  az role definition create --role-definition "$(cat <<JSON
{
  "Name": "$QUOTA_ROLE",
  "Description": "Read Azure AI Foundry / Cognitive Services quota and model availability (IssueOps pre-flight).",
  "Actions": [
    "Microsoft.CognitiveServices/locations/usages/read",
    "Microsoft.CognitiveServices/locations/models/read",
    "Microsoft.CognitiveServices/accounts/models/read"
  ],
  "AssignableScopes": ["$SUB_SCOPE"]
}
JSON
)" >/dev/null
fi

echo "## GitHub environment variables (Settings -> Environments -> <env> -> Variables)"
echo "Repository variables: AZURE_TENANT_ID=$TENANT_ID  AZURE_SUBSCRIPTION_ID=$SUBSCRIPTION_ID"
for env in dev uat prod; do
  app=$(app_for_environment "issueops-gh-foundry-$env" "foundry-$env")
  rg_scope="$SUB_SCOPE/resourceGroups/${FOUNDRY_RG[$env]}"
  assign "$app" "Cognitive Services Contributor" "$rg_scope"
  assign "$app" "$QUOTA_ROLE" "$SUB_SCOPE"
  echo "foundry-$env: AZURE_CLIENT_ID=$app"
done

aks_id=$(az aks show -g "$AKS_RG" -n "$AKS_NAME" --query id -o tsv)
app=$(app_for_environment "issueops-gh-agentic-aks" "agentic-aks")
assign "$app" "Azure Kubernetes Service Cluster User Role" "$aks_id"
assign "$app" "Azure Kubernetes Service RBAC Writer" "$aks_id/namespaces/issueops-agents"
echo "agentic-aks: AZURE_CLIENT_ID=$app  (object ID for deploy/aks/rbac.yaml: $(az ad sp show --id "$app" --query id -o tsv))"

acr_id=$(az acr show -g "$ACR_RG" -n "$ACR_NAME" --query id -o tsv)
app=$(app_for_environment "issueops-gh-acr-push" "acr-push")
assign "$app" "AcrPush" "$acr_id"
echo "acr-push: AZURE_CLIENT_ID=$app  ACR_NAME=$ACR_NAME"

# ---- agent runner workload identity -------------------------------------------
MI_NAME="id-issueops-agent-runner"
az identity show -g "$MI_RG" -n "$MI_NAME" >/dev/null 2>&1 || az identity create -g "$MI_RG" -n "$MI_NAME" -l "$LOCATION" >/dev/null
MI_CLIENT_ID=$(az identity show -g "$MI_RG" -n "$MI_NAME" --query clientId -o tsv)
MI_PRINCIPAL_ID=$(az identity show -g "$MI_RG" -n "$MI_NAME" --query principalId -o tsv)
AKS_OIDC=$(az aks show -g "$AKS_RG" -n "$AKS_NAME" --query oidcIssuerProfile.issuerUrl -o tsv)
if [[ -z "$AKS_OIDC" ]]; then
  echo "Enable the OIDC issuer and workload identity first:" >&2
  echo "  az aks update -g $AKS_RG -n $AKS_NAME --enable-oidc-issuer --enable-workload-identity" >&2
  exit 1
fi
az identity federated-credential show -g "$MI_RG" --identity-name "$MI_NAME" -n issueops-agent-runner >/dev/null 2>&1 ||
  az identity federated-credential create -g "$MI_RG" --identity-name "$MI_NAME" -n issueops-agent-runner \
    --issuer "$AKS_OIDC" --subject "system:serviceaccount:issueops-agents:issueops-agent-runner" \
    --audiences api://AzureADTokenExchange >/dev/null
prod_account_id=$(az cognitiveservices account show -g "${FOUNDRY_RG[prod]}" -n "$PROD_ACCOUNT" --query id -o tsv)
assign "$MI_PRINCIPAL_ID" "Cognitive Services OpenAI User" "$prod_account_id" object
echo "agent runner: set azure.workload.identity/client-id=$MI_CLIENT_ID in deploy/aks/serviceaccount.yaml"
