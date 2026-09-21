#!/usr/bin/env bash
# bootstrap-azure.sh: give the foundry-ops workflows an Azure identity.
#
# Creates a user-assigned managed identity, federates it to the GitHub
# repository (no secret is ever created), grants it Foundry Owner on the
# one Foundry account, and records the ids as repository variables.
#
# It prints what it would do. Pass --apply to do it. Every step is
# idempotent; running it twice changes nothing.
#
# You need: Owner, User Access Administrator or Role Based Access Control
# Administrator on the Foundry account (to assign the role), and admin on
# the GitHub repository (to set variables and environments).
set -euo pipefail

RESOURCE_GROUP="${FOUNDRY_RESOURCE_GROUP:-rg-coolgit-copilot}"
ACCOUNT="${FOUNDRY_ACCOUNT:-ais-coolgit-copilot-prod}"
LOCATION="${FOUNDRY_LOCATION:-eastus2}"
REPO="${FOUNDRY_REPO:-CoolGitOrg/foundry-ops}"
REF="${FOUNDRY_REF:-main}"
IDENTITY="${FOUNDRY_IDENTITY:-uami-foundry-deploy}"
# Foundry Owner (formerly Azure AI Owner). The id is stable; the name is not.
ROLE_ID="c883944f-8b7b-4483-af10-35834be79c4a"
ISSUER="https://token.actions.githubusercontent.com"
APPLY=false

usage() {
  sed -n '2,14p' "$0" | sed 's/^# \{0,1\}//'
  echo
  echo "usage: $0 [--apply]"
  echo "env:   FOUNDRY_RESOURCE_GROUP FOUNDRY_ACCOUNT FOUNDRY_LOCATION FOUNDRY_REPO FOUNDRY_REF FOUNDRY_IDENTITY"
}

for arg in "$@"; do
  case "$arg" in
    --apply) APPLY=true ;;
    -h|--help) usage; exit 0 ;;
    *) usage >&2; exit 2 ;;
  esac
done

say() { printf '%s\n' "$*" >&2; }

# run prints a command; with --apply it also runs it.
run() {
  printf '+' >&2; printf ' %q' "$@" >&2; printf '\n' >&2
  if [[ "$APPLY" == true ]]; then "$@"; fi
}

for tool in az gh jq; do
  command -v "$tool" >/dev/null || { say "missing: $tool"; exit 1; }
done

[[ "$REPO" =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ ]] || { say "FOUNDRY_REPO must be OWNER/NAME"; exit 2; }

sub=$(az account show --query id -o tsv)
tenant=$(az account show --query tenantId -o tsv)
scope=$(az cognitiveservices account show -g "$RESOURCE_GROUP" -n "$ACCOUNT" --query id -o tsv)
say "subscription $sub"
say "scope        $scope"
say "repository   $REPO@$REF"
[[ "$APPLY" == true ]] || say "dry run: nothing will change (pass --apply)"
say

# 1. The identity.
if ! az identity show -g "$RESOURCE_GROUP" -n "$IDENTITY" -o none 2>/dev/null; then
  run az identity create -g "$RESOURCE_GROUP" -n "$IDENTITY" -l "$LOCATION" -o none
fi
client_id=$(az identity show -g "$RESOURCE_GROUP" -n "$IDENTITY" --query clientId -o tsv 2>/dev/null || echo "<created-on-apply>")
principal_id=$(az identity show -g "$RESOURCE_GROUP" -n "$IDENTITY" --query principalId -o tsv 2>/dev/null || echo "<created-on-apply>")

# 2. Federated credentials: one subject per way a job can present itself.
#    Jobs with an environment use the environment subject; the report and
#    test workflows have none, so they present the branch.
federate() { # name subject
  local name=$1 subject=$2
  if az identity federated-credential show -g "$RESOURCE_GROUP" --identity-name "$IDENTITY" -n "$name" -o none 2>/dev/null; then
    return
  fi
  run az identity federated-credential create -g "$RESOURCE_GROUP" --identity-name "$IDENTITY" \
    -n "$name" --issuer "$ISSUER" --subject "$subject" --audiences api://AzureADTokenExchange -o none
}
federate gh-env-foundry-plan "repo:$REPO:environment:foundry-plan"
federate gh-env-foundry-prod "repo:$REPO:environment:foundry-prod"
federate gh-ref-main         "repo:$REPO:ref:refs/heads/$REF"

# 3. Foundry Owner on the one account, nothing wider.
have=$(az role assignment list --assignee "$principal_id" --scope "$scope" --role "$ROLE_ID" --query 'length(@)' -o tsv 2>/dev/null || echo 0)
if [[ "$have" == 0 ]]; then
  run az role assignment create --assignee-object-id "$principal_id" --assignee-principal-type ServicePrincipal \
    --role "$ROLE_ID" --scope "$scope" -o none
fi

# 4. Repository variables. These are ids, not secrets.
setvar() { run gh variable set "$1" --repo "$REPO" --body "$2"; }
setvar AZURE_CLIENT_ID        "$client_id"
setvar AZURE_TENANT_ID        "$tenant"
setvar AZURE_SUBSCRIPTION_ID  "$sub"
setvar FOUNDRY_ACCOUNT        "$ACCOUNT"
setvar FOUNDRY_RESOURCE_GROUP "$RESOURCE_GROUP"
setvar FOUNDRY_LOCATION       "$LOCATION"

# 5. Environments. foundry-plan is open; add required reviewers to
#    foundry-prod in the repository settings: that is the approval gate.
for env in foundry-plan foundry-prod; do
  run gh api --method PUT "repos/$REPO/environments/$env" --silent
done

say
say "next: Settings > Environments > foundry-prod > Required reviewers."
say "then: foundry-tui scaffold -dir <clone of $REPO>, commit, push."
