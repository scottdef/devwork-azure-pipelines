#!/usr/bin/env bash
# bootstrap-uami.sh — one-time. User-assigned managed identity + federated
# credential bound to repo CoolGitOrg/copilot-foundry, environment foundry-prod.
# Then RBAC on the Foundry account and the GitHub environment vars. No secrets.
set -euo pipefail
ORG="${ORG:-CoolGitOrg}"; REPO="${REPO:-copilot-foundry}"; ENV_NAME="${ENV_NAME:-foundry-prod}"
RG="${RG:-rg-coolgit-copilot}"; UAMI="${UAMI:-uami-gh-copilot-foundry}"; LOC="${LOC:-eastus2}"
ACCOUNT="${ACCOUNT:-ais-coolgit-copilot-prod}"

sub="$(az account show --query id -o tsv)"; tenant="$(az account show --query tenantId -o tsv)"

az identity create -g "$RG" -n "$UAMI" -l "$LOC" -o none
client_id="$(az identity show -g "$RG" -n "$UAMI" --query clientId -o tsv)"
principal_id="$(az identity show -g "$RG" -n "$UAMI" --query principalId -o tsv)"

# Federated credential: subject must match what the workflow presents.
#   with `environment:` in the job → repo:ORG/REPO:environment:ENV
#   without                        → repo:ORG/REPO:ref:refs/heads/main
az identity federated-credential create -g "$RG" --identity-name "$UAMI" -n "gh-$ENV_NAME" \
  --issuer https://token.actions.githubusercontent.com \
  --subject "repo:$ORG/$REPO:environment:$ENV_NAME" \
  --audiences api://AzureADTokenExchange -o none

acct_id="$(az cognitiveservices account show -g "$RG" -n "$ACCOUNT" --query id -o tsv)"
for role in "Cognitive Services Contributor" "Cognitive Services User"; do   # deploy + keyless validate
  az role assignment create --assignee-object-id "$principal_id" --assignee-principal-type ServicePrincipal \
    --role "$role" --scope "$acct_id" -o none
done

gh api -X PUT "repos/$ORG/$REPO/environments/$ENV_NAME" >/dev/null
gh variable set UAMI_CLIENT_ID        --env "$ENV_NAME" --repo "$ORG/$REPO" --body "$client_id"
gh variable set AZURE_TENANT_ID       --env "$ENV_NAME" --repo "$ORG/$REPO" --body "$tenant"
gh variable set AZURE_SUBSCRIPTION_ID --env "$ENV_NAME" --repo "$ORG/$REPO" --body "$sub"
echo "UAMI $UAMI clientId=$client_id bound to repo:$ORG/$REPO:environment:$ENV_NAME"
echo "Add required reviewers to environment $ENV_NAME in the repo settings."
