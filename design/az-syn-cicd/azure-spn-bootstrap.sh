#!/usr/bin/env bash
# =============================================================================
# azure-spn-bootstrap.sh
#
# 1. Login to Azure as Service Principal A
# 2. Retrieve dev-clientID and dev-clientSecret for SPN B from dev-akv01
# 3. Login as Service Principal B using retrieved credentials
# 4. Show role assignments for SPN B
# 5. List Synapse workspaces accessible to SPN B
#
# Required environment variables (SPN A credentials):
#   SPN_A_CLIENT_ID      — App ID of service principal A
#   SPN_A_CLIENT_SECRET  — Client secret of service principal A
#   SPN_A_TENANT_ID      — Azure AD tenant ID
#   SPN_A_SUBSCRIPTION_ID — Azure subscription ID (used by both SPNs)
#
# Optional:
#   AKV_NAME             — Key Vault name           (default: dev-akv01)
#   SECRET_CLIENT_ID     — Secret name for SPN B ID  (default: dev-clientID)
#   SECRET_CLIENT_SECRET — Secret name for SPN B sec  (default: dev-clientSecret)
# =============================================================================
set -euo pipefail
IFS=$'\n\t'

# ── Colour helpers ─────────────────────────────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
RESET='\033[0m'

log()     { echo -e "${CYAN}[$(date -u +%H:%M:%S)]${RESET} $*"; }
success() { echo -e "${GREEN}[$(date -u +%H:%M:%S)] ✓${RESET} $*"; }
warn()    { echo -e "${YELLOW}[$(date -u +%H:%M:%S)] ⚠${RESET}  $*"; }
die()     { echo -e "${RED}[$(date -u +%H:%M:%S)] ✗ FATAL:${RESET} $*" >&2; exit 1; }
section() { echo -e "\n${BOLD}━━━ $* ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${RESET}"; }

# ── Dependency check ───────────────────────────────────────────────────────────
section "Preflight checks"
command -v az  >/dev/null 2>&1 || die "Azure CLI (az) is not installed."
command -v jq  >/dev/null 2>&1 || die "jq is not installed."
log "Azure CLI version: $(az version --query '"azure-cli"' -o tsv 2>/dev/null)"

# ── Configuration ──────────────────────────────────────────────────────────────
AKV_NAME="${AKV_NAME:-dev-akv01}"
SECRET_CLIENT_ID="${SECRET_CLIENT_ID:-dev-clientID}"
SECRET_CLIENT_SECRET="${SECRET_CLIENT_SECRET:-dev-clientSecret}"

# SPN A — must be set in environment (never hard-code credentials)
SPN_A_CLIENT_ID="${SPN_A_CLIENT_ID:?SPN_A_CLIENT_ID must be set}"
SPN_A_CLIENT_SECRET="${SPN_A_CLIENT_SECRET:?SPN_A_CLIENT_SECRET must be set}"
SPN_A_TENANT_ID="${SPN_A_TENANT_ID:?SPN_A_TENANT_ID must be set}"
SPN_A_SUBSCRIPTION_ID="${SPN_A_SUBSCRIPTION_ID:?SPN_A_SUBSCRIPTION_ID must be set}"

log "Key Vault  : ${AKV_NAME}"
log "Secret (ID): ${SECRET_CLIENT_ID}"
log "Secret (sec): ${SECRET_CLIENT_SECRET}"

# ── Cleanup on exit: always log out of both sessions ──────────────────────────
cleanup() {
  local exit_code=$?
  echo ""
  log "Cleaning up Azure CLI sessions..."
  az logout 2>/dev/null || true
  az account clear 2>/dev/null || true
  if [[ $exit_code -ne 0 ]]; then
    echo -e "${RED}Script exited with code ${exit_code}${RESET}"
  fi
}
trap cleanup EXIT

# ══════════════════════════════════════════════════════════════════════════════
# STEP 1 — Login as Service Principal A
# ══════════════════════════════════════════════════════════════════════════════
section "Step 1 — Login as Service Principal A"

log "Logging in as SPN A (${SPN_A_CLIENT_ID})..."
az login \
  --service-principal \
  --username    "${SPN_A_CLIENT_ID}" \
  --password    "${SPN_A_CLIENT_SECRET}" \
  --tenant      "${SPN_A_TENANT_ID}" \
  --output none

az account set --subscription "${SPN_A_SUBSCRIPTION_ID}"

# Verify — show the signed-in identity
CURRENT_ACCOUNT=$(az account show --query '{sub:id, tenant:tenantId, user:user.name, type:user.type}' -o json)
success "Logged in as SPN A"
log "  Subscription : $(echo "$CURRENT_ACCOUNT" | jq -r '.sub')"
log "  Tenant       : $(echo "$CURRENT_ACCOUNT" | jq -r '.tenant')"
log "  Identity     : $(echo "$CURRENT_ACCOUNT" | jq -r '.user') ($(echo "$CURRENT_ACCOUNT" | jq -r '.type'))"

# ══════════════════════════════════════════════════════════════════════════════
# STEP 2 — Retrieve SPN B credentials from Azure Key Vault
# ══════════════════════════════════════════════════════════════════════════════
section "Step 2 — Retrieve SPN B credentials from ${AKV_NAME}"

# Verify Key Vault is accessible before attempting secret retrieval
log "Verifying access to Key Vault '${AKV_NAME}'..."
az keyvault show --name "${AKV_NAME}" --query 'name' -o tsv >/dev/null \
  || die "Cannot access Key Vault '${AKV_NAME}'. Check SPN A's permissions (Key Vault Secrets User or access policy get/list)."
success "Key Vault '${AKV_NAME}' is accessible"

# Retrieve dev-clientID
log "Retrieving secret '${SECRET_CLIENT_ID}'..."
SPN_B_CLIENT_ID=$(az keyvault secret show \
  --vault-name "${AKV_NAME}" \
  --name       "${SECRET_CLIENT_ID}" \
  --query      "value" \
  --output     tsv 2>/dev/null) \
  || die "Failed to retrieve secret '${SECRET_CLIENT_ID}' from '${AKV_NAME}'. Verify the secret exists and SPN A has get permission."

[[ -z "${SPN_B_CLIENT_ID}" ]] && die "Secret '${SECRET_CLIENT_ID}' is empty."
success "Retrieved client ID for SPN B (${SPN_B_CLIENT_ID})"

# Retrieve dev-clientSecret
log "Retrieving secret '${SECRET_CLIENT_SECRET}'..."
SPN_B_CLIENT_SECRET=$(az keyvault secret show \
  --vault-name "${AKV_NAME}" \
  --name       "${SECRET_CLIENT_SECRET}" \
  --query      "value" \
  --output     tsv 2>/dev/null) \
  || die "Failed to retrieve secret '${SECRET_CLIENT_SECRET}' from '${AKV_NAME}'."

[[ -z "${SPN_B_CLIENT_SECRET}" ]] && die "Secret '${SECRET_CLIENT_SECRET}' is empty."
success "Retrieved client secret for SPN B"

# Sanity-check that B's client ID looks like a UUID
if [[ ! "${SPN_B_CLIENT_ID}" =~ ^[0-9a-fA-F-]{36}$ ]]; then
  warn "SPN B client ID '${SPN_B_CLIENT_ID}' does not look like a UUID — double-check the secret value."
fi

# ══════════════════════════════════════════════════════════════════════════════
# STEP 3 — Login as Service Principal B
# ══════════════════════════════════════════════════════════════════════════════
section "Step 3 — Login as Service Principal B"

# SPN B uses the same tenant and subscription as A unless overridden
SPN_B_TENANT_ID="${SPN_B_TENANT_ID:-${SPN_A_TENANT_ID}}"
SPN_B_SUBSCRIPTION_ID="${SPN_B_SUBSCRIPTION_ID:-${SPN_A_SUBSCRIPTION_ID}}"

log "Logging in as SPN B (${SPN_B_CLIENT_ID})..."
az login \
  --service-principal \
  --username    "${SPN_B_CLIENT_ID}" \
  --password    "${SPN_B_CLIENT_SECRET}" \
  --tenant      "${SPN_B_TENANT_ID}" \
  --output none

az account set --subscription "${SPN_B_SUBSCRIPTION_ID}"

CURRENT_ACCOUNT=$(az account show --query '{sub:id, tenant:tenantId, user:user.name, type:user.type}' -o json)
success "Logged in as SPN B"
log "  Subscription : $(echo "$CURRENT_ACCOUNT" | jq -r '.sub')"
log "  Tenant       : $(echo "$CURRENT_ACCOUNT" | jq -r '.tenant')"
log "  Identity     : $(echo "$CURRENT_ACCOUNT" | jq -r '.user') ($(echo "$CURRENT_ACCOUNT" | jq -r '.type'))"

# Resolve the Object ID of SPN B (needed for role assignment query)
log "Resolving object ID for SPN B..."
SPN_B_OBJECT_ID=$(az ad sp show \
  --id    "${SPN_B_CLIENT_ID}" \
  --query "id" \
  --output tsv 2>/dev/null) \
  || die "Could not resolve object ID for SPN B (${SPN_B_CLIENT_ID}). Ensure SPN exists in the tenant."

[[ -z "${SPN_B_OBJECT_ID}" ]] && die "Object ID for SPN B is empty."
success "SPN B object ID: ${SPN_B_OBJECT_ID}"

# ══════════════════════════════════════════════════════════════════════════════
# STEP 4 — Show role assignments for Service Principal B
# ══════════════════════════════════════════════════════════════════════════════
section "Step 4 — Role assignments for SPN B"

log "Querying role assignments (assignee: ${SPN_B_OBJECT_ID})..."
ROLE_JSON=$(az role assignment list \
  --assignee "${SPN_B_OBJECT_ID}" \
  --all \
  --output json 2>/dev/null) \
  || { warn "Could not list role assignments. SPN B may lack Microsoft.Authorization/roleAssignments/read."; ROLE_JSON="[]"; }

ROLE_COUNT=$(echo "${ROLE_JSON}" | jq 'length')
log "Found ${ROLE_COUNT} role assignment(s):"
echo ""

if [[ "${ROLE_COUNT}" -eq 0 ]]; then
  warn "No role assignments found for SPN B at this subscription scope."
  warn "It may have roles at management group or resource-group scope, or no roles at all."
else
  # Pretty-print a human-readable table
  printf "${BOLD}  %-40s %-35s %s${RESET}\n" "ROLE" "SCOPE" "SCOPE TYPE"
  printf "  %s\n" "$(printf '%.0s─' {1..100})"

  echo "${ROLE_JSON}" | jq -c '.[]' | while IFS= read -r assignment; do
    ROLE=$(echo "${assignment}"  | jq -r '.roleDefinitionName')
    SCOPE=$(echo "${assignment}" | jq -r '.scope')
    # Infer scope type from the scope string
    if   echo "${SCOPE}" | grep -q '/resourceGroups/[^/]*/providers/'; then
      SCOPE_TYPE="Resource"
    elif echo "${SCOPE}" | grep -q '/resourceGroups/'; then
      SCOPE_TYPE="Resource Group"
    elif echo "${SCOPE}" | grep -qE '^/subscriptions/[^/]+$'; then
      SCOPE_TYPE="Subscription"
    elif echo "${SCOPE}" | grep -q '/managementGroups/'; then
      SCOPE_TYPE="Management Group"
    else
      SCOPE_TYPE="Unknown"
    fi
    printf "  %-40s %-35s %s\n" "${ROLE}" "$(echo "${SCOPE}" | sed 's|.*/||')" "${SCOPE_TYPE}"
  done

  echo ""
  log "Full role assignment JSON written below (verbose):"
  echo "${ROLE_JSON}" | jq '[.[] | {
      role:          .roleDefinitionName,
      principalType: .principalType,
      scope:         .scope,
      createdOn:     .createdOn
  }]'
fi

# ══════════════════════════════════════════════════════════════════════════════
# STEP 5 — List Synapse workspaces accessible to Service Principal B
# ══════════════════════════════════════════════════════════════════════════════
section "Step 5 — Synapse workspaces accessible to SPN B"

log "Querying Azure Synapse workspaces in subscription ${SPN_B_SUBSCRIPTION_ID}..."
SYNAPSE_JSON=$(az synapse workspace list \
  --output json 2>/dev/null) \
  || { warn "az synapse workspace list failed. SPN B may lack Reader on the subscription or resource groups."; SYNAPSE_JSON="[]"; }

WS_COUNT=$(echo "${SYNAPSE_JSON}" | jq 'length')
log "Found ${WS_COUNT} Synapse workspace(s):"
echo ""

if [[ "${WS_COUNT}" -eq 0 ]]; then
  warn "No Synapse workspaces found. Either:"
  warn "  • SPN B lacks Reader role on subscriptions/resource groups containing workspaces."
  warn "  • No workspaces exist in subscription ${SPN_B_SUBSCRIPTION_ID}."
else
  # Header
  printf "${BOLD}  %-35s %-20s %-15s %-20s %s${RESET}\n" \
    "WORKSPACE" "RESOURCE GROUP" "LOCATION" "SQL ENDPOINT" "PROVISIONING"
  printf "  %s\n" "$(printf '%.0s─' {1..120})"

  echo "${SYNAPSE_JSON}" | jq -c '.[]' | while IFS= read -r ws; do
    NAME=$(echo "${ws}"        | jq -r '.name')
    RG=$(echo "${ws}"          | jq -r '.resourceGroup')
    LOCATION=$(echo "${ws}"    | jq -r '.location')
    SQL_EP=$(echo "${ws}"      | jq -r '.connectivityEndpoints.sql // "n/a"')
    PROVISION=$(echo "${ws}"   | jq -r '.provisioningState')
    printf "  %-35s %-20s %-15s %-20s %s\n" \
      "${NAME}" "${RG}" "${LOCATION}" "$(echo "${SQL_EP}" | sed 's/\.sql\.azuresynapse\.net.*//')" "${PROVISION}"
  done

  echo ""
  log "Detailed workspace output:"
  echo "${SYNAPSE_JSON}" | jq '[.[] | {
      name:              .name,
      resourceGroup:     .resourceGroup,
      location:          .location,
      provisioningState: .provisioningState,
      managedResourceGroup: .managedResourceGroupName,
      sqlEndpoint:       .connectivityEndpoints.sql,
      devEndpoint:       .connectivityEndpoints.dev,
      sqlAdminLogin:     .sqlAdministratorLogin,
      identity:          .identity.type
  }]'
fi

# ══════════════════════════════════════════════════════════════════════════════
# Done
# ══════════════════════════════════════════════════════════════════════════════
echo ""
success "All steps completed successfully."
echo -e "${BOLD}  Summary${RESET}"
echo -e "  SPN A (${SPN_A_CLIENT_ID}) → retrieved credentials for SPN B from ${AKV_NAME}"
echo -e "  SPN B (${SPN_B_CLIENT_ID}) → role assignments: ${ROLE_COUNT}  |  Synapse workspaces: ${WS_COUNT}"
echo ""
