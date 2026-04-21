#!/usr/bin/env bash
###############################################################################
# bootstrap.sh
# One-shot setup for all GitHub secrets, variables, and environments across
# all three Synapse repos.
#
# Prerequisites:
#   - gh CLI authenticated with org admin or repo admin scope
#   - Three JSON files produced by `az ad sp create-for-rbac --sdk-auth`:
#       dev-spn.json, test-spn.json, prod-spn.json
#   - GitHub App private key file: synapse-cicd-app.pem
#
# Usage:
#   ./bootstrap.sh --org myorg \
#                  --app-id 123456 \
#                  --dev-spn dev-spn.json \
#                  --test-spn test-spn.json \
#                  --prod-spn prod-spn.json \
#                  --app-pem synapse-cicd-app.pem
###############################################################################
set -euo pipefail

# ── Argument parsing ──────────────────────────────────────────────────────────
ORG=""
APP_ID=""
DEV_SPN_FILE=""
TEST_SPN_FILE=""
PROD_SPN_FILE=""
APP_PEM_FILE=""

usage() {
  echo "Usage: $0 --org ORG --app-id APP_ID --dev-spn FILE --test-spn FILE --prod-spn FILE --app-pem FILE"
  exit 1
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --org)      ORG="$2";          shift 2 ;;
    --app-id)   APP_ID="$2";       shift 2 ;;
    --dev-spn)  DEV_SPN_FILE="$2"; shift 2 ;;
    --test-spn) TEST_SPN_FILE="$2";shift 2 ;;
    --prod-spn) PROD_SPN_FILE="$2";shift 2 ;;
    --app-pem)  APP_PEM_FILE="$2"; shift 2 ;;
    *)          usage ;;
  esac
done

[[ -z "$ORG" || -z "$APP_ID" || -z "$DEV_SPN_FILE" || -z "$TEST_SPN_FILE" \
   || -z "$PROD_SPN_FILE" || -z "$APP_PEM_FILE" ]] && usage

for f in "$DEV_SPN_FILE" "$TEST_SPN_FILE" "$PROD_SPN_FILE" "$APP_PEM_FILE"; do
  [[ -f "$f" ]] || { echo "ERROR: File not found: $f"; exit 1; }
done

log()  { echo "[$(date '+%H:%M:%S')] $*"; }
ok()   { echo "    ✓ $*"; }
sep()  { echo ""; echo "══════════════════════════════════════════"; }

# ── Helper: set secret ────────────────────────────────────────────────────────
set_secret() {
  local repo="$1" name="$2" value="$3"
  echo -n "$value" | gh secret set "$name" --repo "${ORG}/${repo}" --body - 2>/dev/null
  ok "Secret ${name} → ${repo}"
}

# ── Helper: set variable ──────────────────────────────────────────────────────
set_variable() {
  local repo="$1" name="$2" value="$3"
  gh variable set "$name" --repo "${ORG}/${repo}" --body "$value" 2>/dev/null
  ok "Variable ${name}=${value} → ${repo}"
}

# ── Helper: create environment ────────────────────────────────────────────────
create_env() {
  local repo="$1" env_name="$2"
  gh api --method PUT \
    "repos/${ORG}/${repo}/environments/${env_name}" \
    --input /dev/null \
    > /dev/null 2>&1 || true
  ok "Environment ${env_name} → ${repo}"
}

# ── Parse SPN files ───────────────────────────────────────────────────────────
log "Parsing service principal credential files..."

parse_spn() {
  local file="$1" field="$2"
  jq -r ".$field" "$file"
}

DEV_CLIENT_ID=$(parse_spn "$DEV_SPN_FILE" clientId)
DEV_CLIENT_SECRET=$(parse_spn "$DEV_SPN_FILE" clientSecret)
DEV_TENANT_ID=$(parse_spn "$DEV_SPN_FILE" tenantId)
DEV_SUBSCRIPTION_ID=$(parse_spn "$DEV_SPN_FILE" subscriptionId)

TEST_CLIENT_ID=$(parse_spn "$TEST_SPN_FILE" clientId)
TEST_CLIENT_SECRET=$(parse_spn "$TEST_SPN_FILE" clientSecret)
TEST_TENANT_ID=$(parse_spn "$TEST_SPN_FILE" tenantId)
TEST_SUBSCRIPTION_ID=$(parse_spn "$TEST_SPN_FILE" subscriptionId)

PROD_CLIENT_ID=$(parse_spn "$PROD_SPN_FILE" clientId)
PROD_CLIENT_SECRET=$(parse_spn "$PROD_SPN_FILE" clientSecret)
PROD_TENANT_ID=$(parse_spn "$PROD_SPN_FILE" tenantId)
PROD_SUBSCRIPTION_ID=$(parse_spn "$PROD_SPN_FILE" subscriptionId)

APP_PEM=$(cat "$APP_PEM_FILE")

ok "All SPN files parsed"

# ── synapse-repo-dev ──────────────────────────────────────────────────────────
sep
log "Configuring synapse-repo-dev..."

set_secret synapse-repo-dev AZURE_CLIENT_ID       "$DEV_CLIENT_ID"
set_secret synapse-repo-dev AZURE_CLIENT_SECRET   "$DEV_CLIENT_SECRET"
set_secret synapse-repo-dev AZURE_TENANT_ID       "$DEV_TENANT_ID"
set_secret synapse-repo-dev AZURE_SUBSCRIPTION_ID "$DEV_SUBSCRIPTION_ID"
set_secret synapse-repo-dev GH_APP_PRIVATE_KEY    "$APP_PEM"

set_variable synapse-repo-dev GH_APP_ID              "$APP_ID"
set_variable synapse-repo-dev SYNAPSE_WORKSPACE_DEV  "synapse-workspace-dev"
set_variable synapse-repo-dev RESOURCE_GROUP_DEV     "rg-synapse-dev"

create_env synapse-repo-dev synapse-dev
ok "synapse-repo-dev configured"

# ── synapse-repo-test ─────────────────────────────────────────────────────────
sep
log "Configuring synapse-repo-test..."

set_secret synapse-repo-test AZURE_CLIENT_ID       "$TEST_CLIENT_ID"
set_secret synapse-repo-test AZURE_CLIENT_SECRET   "$TEST_CLIENT_SECRET"
set_secret synapse-repo-test AZURE_TENANT_ID       "$TEST_TENANT_ID"
set_secret synapse-repo-test AZURE_SUBSCRIPTION_ID "$TEST_SUBSCRIPTION_ID"
set_secret synapse-repo-test GH_APP_PRIVATE_KEY    "$APP_PEM"

set_variable synapse-repo-test GH_APP_ID               "$APP_ID"
set_variable synapse-repo-test SYNAPSE_WORKSPACE_TEST   "synapse-workspace-test"
set_variable synapse-repo-test RESOURCE_GROUP_TEST      "rg-synapse-test"
set_variable synapse-repo-test DEV_REPO_NAME            "synapse-repo-dev"

create_env synapse-repo-test synapse-test
ok "synapse-repo-test configured"

# ── synapse-repo-prod ─────────────────────────────────────────────────────────
sep
log "Configuring synapse-repo-prod..."

set_secret synapse-repo-prod AZURE_CLIENT_ID       "$PROD_CLIENT_ID"
set_secret synapse-repo-prod AZURE_CLIENT_SECRET   "$PROD_CLIENT_SECRET"
set_secret synapse-repo-prod AZURE_TENANT_ID       "$PROD_TENANT_ID"
set_secret synapse-repo-prod AZURE_SUBSCRIPTION_ID "$PROD_SUBSCRIPTION_ID"
set_secret synapse-repo-prod GH_APP_PRIVATE_KEY    "$APP_PEM"

set_variable synapse-repo-prod GH_APP_ID               "$APP_ID"
set_variable synapse-repo-prod SYNAPSE_WORKSPACE_PROD   "synapse-workspace-prod"
set_variable synapse-repo-prod RESOURCE_GROUP_PROD      "rg-synapse-prod"
set_variable synapse-repo-prod DEV_REPO_NAME            "synapse-repo-dev"

create_env synapse-repo-prod synapse-prod

# Configure prod environment to require db-ops team approval
DB_OPS_TEAM_ID=$(gh api "orgs/${ORG}/teams/db-ops" --jq .id 2>/dev/null || echo "")
if [[ -n "$DB_OPS_TEAM_ID" ]]; then
  gh api --method PUT \
    "repos/${ORG}/synapse-repo-prod/environments/synapse-prod" \
    --field "reviewers=[{\"type\":\"Team\",\"id\":${DB_OPS_TEAM_ID}}]" \
    --field "deployment_branch_policy=null" \
    > /dev/null 2>&1 && ok "Production approval gate: db-ops team required" \
    || echo "    ⚠ Could not set prod reviewers via API — set manually in Settings → Environments"
else
  echo "    ⚠ db-ops team not found — set prod reviewers manually in Settings → Environments"
fi

ok "synapse-repo-prod configured"

# ── Summary ───────────────────────────────────────────────────────────────────
sep
echo ""
log "Bootstrap complete! Summary:"
echo ""
echo "  Repos configured:   synapse-repo-dev, synapse-repo-test, synapse-repo-prod"
echo "  Secrets set:        AZURE_CLIENT_ID, AZURE_CLIENT_SECRET, AZURE_TENANT_ID,"
echo "                      AZURE_SUBSCRIPTION_ID, GH_APP_PRIVATE_KEY"
echo "  Variables set:      GH_APP_ID, SYNAPSE_WORKSPACE_*, RESOURCE_GROUP_*, DEV_REPO_NAME"
echo "  Environments:       synapse-dev, synapse-test, synapse-prod"
echo ""
echo "  Next steps:"
echo "    1. Run: terraform apply  (in ./terraform/)"
echo "    2. Install the GitHub App on all 3 repos"
echo "    3. Configure Synapse Studio Git integration for each workspace"
echo "    4. Push workflow files to each repo"
echo ""
