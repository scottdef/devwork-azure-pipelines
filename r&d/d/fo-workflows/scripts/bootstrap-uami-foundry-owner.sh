#!/usr/bin/env bash
# bootstrap-uami-foundry-owner.sh — UAMI + federated credential + ONE role:
# "Foundry Owner" (formerly "Azure AI Owner") on the Foundry account.
set -euo pipefail
ORG="${ORG:-CoolGitOrg}"; REPO="${REPO:-copilot-foundry}"; ENV_NAME="${ENV_NAME:-foundry-prod}"
RG="${RG:-rg-coolgit-copilot}"; UAMI="${UAMI:-uami-gh-copilot-foundry}"; LOC="${LOC:-eastus2}"
ACCOUNT="${ACCOUNT:-ais-coolgit-copilot-prod}"

sub="$(az account show --query id -o tsv)"; tenant="$(az account show --query tenantId -o tsv)"
az identity create -g "$RG" -n "$UAMI" -l "$LOC" -o none
client_id="$(az identity show -g "$RG" -n "$UAMI" --query clientId -o tsv)"
principal_id="$(az identity show -g "$RG" -n "$UAMI" --query principalId -o tsv)"

az identity federated-credential create -g "$RG" --identity-name "$UAMI" -n "gh-$ENV_NAME" \
  --issuer https://token.actions.githubusercontent.com \
  --subject "repo:$ORG/$REPO:environment:$ENV_NAME" --audiences api://AzureADTokenExchange -o none

# role name is mid-rename; resolve by whichever exists, and show what it grants
role_id="$(az role definition list --name "Foundry Owner" --query '[0].name' -o tsv)"
[[ -n "$role_id" ]] || role_id="$(az role definition list --name "Azure AI Owner" --query '[0].name' -o tsv)"
[[ -n "$role_id" ]] || { echo "neither 'Foundry Owner' nor 'Azure AI Owner' found in this tenant" >&2; exit 1; }
az role definition list --name "$role_id" --query '[0].permissions[0].{actions:actions,dataActions:dataActions}' -o yaml
for need in "Microsoft.CognitiveServices/accounts/deployments/write" "Microsoft.CognitiveServices/accounts/listKeys/action" "Microsoft.CognitiveServices/locations/models/read"; do
  az role definition list --name "$role_id" --query '[0].permissions[0].actions' -o tsv | tr '\t' '\n' | grep -qE "^(Microsoft\.CognitiveServices/\*|${need//\*/\\*})$" \
    || echo "WARNING: $need not obviously covered — verify" >&2
done

acct_id="$(az cognitiveservices account show -g "$RG" -n "$ACCOUNT" --query id -o tsv)"
az role assignment create --assignee-object-id "$principal_id" --assignee-principal-type ServicePrincipal --role "$role_id" --scope "$acct_id" -o none

gh api -X PUT "repos/$ORG/$REPO/environments/$ENV_NAME" >/dev/null
gh variable set UAMI_CLIENT_ID        --env "$ENV_NAME" --repo "$ORG/$REPO" --body "$client_id"
gh variable set AZURE_TENANT_ID       --env "$ENV_NAME" --repo "$ORG/$REPO" --body "$tenant"
gh variable set AZURE_SUBSCRIPTION_ID --env "$ENV_NAME" --repo "$ORG/$REPO" --body "$sub"
gh label create foundry-byok --color 1D76DB --force --repo "$ORG/$REPO" >/dev/null || true
echo "UAMI $UAMI ($client_id) → Foundry Owner on $acct_id"
echo "Optional: gh secret set GH_ADMIN_TOKEN --repo $ORG/$REPO   # PAT with repo scope so workflows can store the key as a secret"
