#!/usr/bin/env bash
# bootstrap-azure.sh: one-time setup for foundry-custom-models.
#
# Azure: an Azure ML workspace, a CPU cluster for downloads, GPU clusters for
# fine-tuning, a deploy identity federated to GitHub (no secrets), a compute
# identity for Key Vault, and the role assignments both need.
# GitHub: environments ftm-plan / ftm-prod / ftm-verify, repository
# variables, and the IssueOps labels.
#
# Prints every command. Pass --apply to run them. Safe to re-run.
#
# You need Owner (or Contributor + Role Based Access Control Administrator) on
# the resource group, and admin on the GitHub repository.
set -euo pipefail

RG=${AZURE_RESOURCE_GROUP:-rg-coolgit-copilot}
LOC=${AZURE_LOCATION:-eastus2}
WS=${AML_WORKSPACE:-mlw-coolgit-foundry}
ACCOUNT=${FOUNDRY_ACCOUNT:-ais-coolgit-copilot-prod}
PROJECT=${FOUNDRY_PROJECT:-proj-coolgit-agents}
REPO=${FTM_REPO:-CoolGitOrg/foundry-custom-models}
KV_URL=${KEY_VAULT_URL:-}
DEPLOY_ID=uami-ftm-deploy
COMPUTE_ID=uami-ftm-compute
ISSUER=https://token.actions.githubusercontent.com

# Role definition ids, not names: Microsoft renamed the Foundry roles in 2026.
ROLE_AML_DATA_SCIENTIST=f6c7c914-8db3-469d-8ca1-694a8f32e121
ROLE_FOUNDRY_OWNER=c883944f-8b7b-4483-af10-35834be79c4a
ROLE_KV_SECRETS_USER=4633458b-17de-408a-b874-0445c86b69e6

APPLY=false
case "${1:-}" in
  --apply) APPLY=true ;;
  "") ;;
  -h | --help) sed -n '2,15p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
  *) echo "usage: $0 [--apply]" >&2; exit 2 ;;
esac

say() { printf '%s\n' "$*" >&2; }
run() {
  printf '+' >&2; printf ' %q' "$@" >&2; printf '\n' >&2
  if [[ "$APPLY" == true ]]; then "$@"; fi
}
exists() { "$@" -o none >/dev/null 2>&1; }

for t in az gh; do command -v "$t" >/dev/null || { say "missing: $t"; exit 1; }; done
[[ "$REPO" =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ ]] || { say "FTM_REPO must be OWNER/NAME"; exit 2; }
az extension add --name ml --upgrade --yes --only-show-errors >/dev/null
SUB=$(az account show --query id -o tsv)
TENANT=$(az account show --query tenantId -o tsv)
[[ "$APPLY" == true ]] || say "dry run: nothing changes (pass --apply)"
say "subscription $SUB  resource group $RG  region $LOC  repo $REPO"

# 1. Workspace.
exists az ml workspace show -n "$WS" -g "$RG" || run az ml workspace create -n "$WS" -g "$RG" -l "$LOC"

# 2. Identities.
for id in "$DEPLOY_ID" "$COMPUTE_ID"; do
  exists az identity show -g "$RG" -n "$id" || run az identity create -g "$RG" -n "$id" -l "$LOC"
done
field() { az identity show -g "$RG" -n "$1" --query "$2" -o tsv 2>/dev/null || echo "<$1-$2>"; }
DEPLOY_CLIENT=$(field "$DEPLOY_ID" clientId)
DEPLOY_PRINCIPAL=$(field "$DEPLOY_ID" principalId)
COMPUTE_RESOURCE=$(field "$COMPUTE_ID" id)
COMPUTE_PRINCIPAL=$(field "$COMPUTE_ID" principalId)

# 3. Federation: one subject per GitHub environment that logs in to Azure.
for env in ftm-plan ftm-prod ftm-verify; do
  exists az identity federated-credential show -g "$RG" --identity-name "$DEPLOY_ID" -n "gh-$env" ||
    run az identity federated-credential create -g "$RG" --identity-name "$DEPLOY_ID" -n "gh-$env" \
      --issuer "$ISSUER" --subject "repo:$REPO:environment:$env" --audiences api://AzureADTokenExchange
done

# 4. Roles. Deploy identity: jobs, models, environments, endpoints in the workspace;
#    connections and agents in the Foundry project. Nothing at subscription scope.
WS_ID=/subscriptions/$SUB/resourceGroups/$RG/providers/Microsoft.MachineLearningServices/workspaces/$WS
PROJECT_ID=/subscriptions/$SUB/resourceGroups/$RG/providers/Microsoft.CognitiveServices/accounts/$ACCOUNT/projects/$PROJECT
assign() { # principal role scope
  local n
  n=$(az role assignment list --assignee "$1" --role "$2" --scope "$3" --query 'length(@)' -o tsv 2>/dev/null || echo 0)
  [[ "$n" != 0 ]] || run az role assignment create --assignee-object-id "$1" --assignee-principal-type ServicePrincipal \
    --role "$2" --scope "$3" -o none
}
assign "$DEPLOY_PRINCIPAL" "$ROLE_AML_DATA_SCIENTIST" "$WS_ID"
assign "$DEPLOY_PRINCIPAL" "$ROLE_FOUNDRY_OWNER" "$PROJECT_ID"
if [[ -n "$KV_URL" ]]; then
  KV_NAME=$(sed -E 's#https://([^.]+)\..*#\1#' <<<"$KV_URL")
  KV_ID=$(az keyvault show -n "$KV_NAME" --query id -o tsv)
  assign "$COMPUTE_PRINCIPAL" "$ROLE_KV_SECRETS_USER" "$KV_ID"
fi

# 5. Clusters: scale to zero, so idle costs nothing. The CPU size has a 600 GiB
#    temp disk because fetch jobs stage the snapshot locally before upload.
cluster() { # name size max
  exists az ml compute show -n "$1" -g "$RG" -w "$WS" ||
    run az ml compute create -n "$1" --type AmlCompute --size "$2" --min-instances 0 --max-instances "$3" \
      --idle-time-before-scale-down 900 --identity-type UserAssigned --user-assigned-identities "$COMPUTE_RESOURCE" \
      -g "$RG" -w "$WS"
}
cluster cpu-fetch Standard_D16ds_v5 2
cluster gpu-a100 Standard_NC24ads_A100_v4 1
cluster gpu-h100x2 Standard_NC80adis_H100_v5 1

# 6. GitHub: environments, variables, labels.
for env in ftm-plan ftm-prod ftm-verify; do run gh api --method PUT "repos/$REPO/environments/$env" --silent; done
setvar() { run gh variable set "$1" --repo "$REPO" --body "$2"; }
setvar AZURE_CLIENT_ID "$DEPLOY_CLIENT"
setvar AZURE_TENANT_ID "$TENANT"
setvar AZURE_SUBSCRIPTION_ID "$SUB"
setvar AZURE_RESOURCE_GROUP "$RG"
setvar AZURE_LOCATION "$LOC"
setvar AML_WORKSPACE "$WS"
setvar FOUNDRY_ACCOUNT "$ACCOUNT"
setvar FOUNDRY_PROJECT "$PROJECT"
[[ -z "$KV_URL" ]] || setvar KEY_VAULT_URL "$KV_URL"
for l in "model-request:1d76db" "needs-changes:fbca04" "applied:0e8a16" "apply-failed:d93f0b"; do
  run gh label create "${l%%:*}" --repo "$REPO" --color "${l#*:}" --force
done

say
say "next:"
say "  1. Settings > Environments > ftm-prod: add required reviewers; restrict all three to branch main."
say "  2. GPU quota for managed online endpoints in $LOC (NCADS_A100_v4, NCadsH100v5 families):"
say "     az ml compute list-usage -l $LOC --query \"[?contains(name.value, 'H100') || contains(name.value, 'A100')]\" -o table"
say "  3. Optional: vars.AUTOADMIN_APP_ID + secrets.AUTOADMIN_APP_PRIVATE_KEY so record commits pass rulesets."
say "  4. scripts/preflight.sh, then dispatch ex1-gemma4 with action=plan."
