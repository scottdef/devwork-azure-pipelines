#!/usr/bin/env bash
# bootstrap-uami-hub.sh — UAMI + federated credential for the LEGACY HUB stack.
# Roles: AzureML Data Scientist on the project (online/serverless endpoints),
#        Cognitive Services Contributor + User on the hub-connected AI Services account.
set -euo pipefail
ORG="${ORG:-CoolGitOrg}"; REPO="${REPO:-copilot-foundry}"; ENV_NAME="${ENV_NAME:-foundry-prod}"
RG="${RG:-rg-coolgit-copilot}"; UAMI="${UAMI:-uami-gh-copilot-foundry}"; LOC="${LOC:-eastus2}"
HUB="${HUB:-hub-coolgit-copilot}"; PROJECT="${PROJECT:-proj-coolgit-copilot}"

sub="$(az account show --query id -o tsv)"; tenant="$(az account show --query tenantId -o tsv)"
az extension add -n ml -y >/dev/null

az identity create -g "$RG" -n "$UAMI" -l "$LOC" -o none
client_id="$(az identity show -g "$RG" -n "$UAMI" --query clientId -o tsv)"
principal_id="$(az identity show -g "$RG" -n "$UAMI" --query principalId -o tsv)"

az identity federated-credential create -g "$RG" --identity-name "$UAMI" -n "gh-$ENV_NAME" \
  --issuer https://token.actions.githubusercontent.com \
  --subject "repo:$ORG/$REPO:environment:$ENV_NAME" \
  --audiences api://AzureADTokenExchange -o none

assign() { az role assignment create --assignee-object-id "$principal_id" --assignee-principal-type ServicePrincipal --role "$1" --scope "$2" -o none 2>/dev/null || true; }

proj_id="$(az ml workspace show -g "$RG" -n "$PROJECT" --query id -o tsv)"
hub_id="$(az ml workspace show -g "$RG" -n "$HUB" --query id -o tsv)"
assign "AzureML Data Scientist" "$proj_id"
assign "Reader" "$hub_id"                                  # list connections

acct_id="$(az rest --method get --url "https://management.azure.com${hub_id}/connections?api-version=2024-10-01" \
  --query "value[?properties.category=='AIServices' || properties.category=='AzureOpenAI'] | [0].properties.metadata.ResourceId" -o tsv)"
if [[ -n "$acct_id" ]]; then
  assign "Cognitive Services Contributor" "$acct_id"
  assign "Cognitive Services User" "$acct_id"
  echo "AI Services account: $acct_id"
else
  echo "WARNING: hub has no AIServices/AzureOpenAI connection — path A in deploy-qwen3-32b.yml will fail" >&2
fi

gh api -X PUT "repos/$ORG/$REPO/environments/$ENV_NAME" >/dev/null
gh variable set UAMI_CLIENT_ID        --env "$ENV_NAME" --repo "$ORG/$REPO" --body "$client_id"
gh variable set AZURE_TENANT_ID       --env "$ENV_NAME" --repo "$ORG/$REPO" --body "$tenant"
gh variable set AZURE_SUBSCRIPTION_ID --env "$ENV_NAME" --repo "$ORG/$REPO" --body "$sub"
echo "UAMI $UAMI clientId=$client_id → repo:$ORG/$REPO:environment:$ENV_NAME"
