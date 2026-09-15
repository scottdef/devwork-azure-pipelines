#!/usr/bin/env bash
# bootstrap-azure-oidc.sh — one-time: Entra app + federated credentials for each
# GitHub Environment in the registry, RBAC on the account, and the GitHub
# environment variables. No secrets are created anywhere.
# Env: ORG=CoolGitOrg REPO=copilot-foundry [APP_NAME=gh-copilot-foundry]
set -euo pipefail
# shellcheck source=lib.sh
source "$(dirname "$0")/lib.sh"
need az gh jq yq
: "${ORG:=CoolGitOrg}" "${REPO:=copilot-foundry}" "${APP_NAME:=gh-copilot-foundry}"

sub="$(az account show --query id -o tsv)"; tenant="$(az account show --query tenantId -o tsv)"
app_id="$(az ad app list --display-name "$APP_NAME" --query '[0].appId' -o tsv)"
[[ -n "$app_id" ]] || app_id="$(az ad app create --display-name "$APP_NAME" --query appId -o tsv)"
az ad sp show --id "$app_id" -o none 2>/dev/null || az ad sp create --id "$app_id" -o none
sp_oid="$(az ad sp show --id "$app_id" --query id -o tsv)"
log "app $app_id sp $sp_oid"

for acct in $(reg '.accounts | keys[]'); do
  env_name="$(reg ".accounts[\"$acct\"].environment")"
  rg="$(reg ".accounts[\"$acct\"].resource_group")"; ais="$(reg ".accounts[\"$acct\"].account")"

  # federated credential per environment: subject repo:ORG/REPO:environment:ENV
  if ! az ad app federated-credential list --id "$app_id" --query "[?name=='$env_name']" -o tsv | grep -q .; then
    az ad app federated-credential create --id "$app_id" --parameters "$(jq -n --arg n "$env_name" --arg s "repo:$ORG/$REPO:environment:$env_name" \
    '{name:$n, issuer:"https://token.actions.githubusercontent.com", subject:$s, audiences:["api://AzureADTokenExchange"]}')" -o none
  fi
  log "federated credential: $env_name"

  # RBAC: deploy + call the endpoint keyless
  scope="/subscriptions/$sub/resourceGroups/$rg"
  for role in "Cognitive Services Contributor" "Cognitive Services User" "Azure AI Developer"; do
    az role assignment create --assignee-object-id "$sp_oid" --assignee-principal-type ServicePrincipal --role "$role" --scope "$scope" -o none 2>/dev/null || true
  done

  # GitHub environment + vars (creates the environment if missing; add required reviewers in the UI)
  gh api -X PUT "repos/$ORG/$REPO/environments/$env_name" -o /dev/null >/dev/null
  gh variable set AZURE_CLIENT_ID       --env "$env_name" --repo "$ORG/$REPO" --body "$app_id"
  gh variable set AZURE_TENANT_ID       --env "$env_name" --repo "$ORG/$REPO" --body "$tenant"
  gh variable set AZURE_SUBSCRIPTION_ID --env "$env_name" --repo "$ORG/$REPO" --body "$sub"
  log "github env $env_name → $rg/$ais"
done
log "done. Next: add required reviewers to foundry-prod; create team $ORG/foundry-approvers; make bootstrap-labels"
