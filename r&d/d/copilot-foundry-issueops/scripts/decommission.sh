#!/usr/bin/env bash
# decommission.sh — delete a Foundry deployment (serverless / provisioned / managed).
# Env: RESOURCE_GROUP ACCOUNT DEPLOYMENT [DRY_RUN=false]
set -euo pipefail
# shellcheck source=lib.sh
source "$(dirname "$0")/lib.sh"
need az
: "${RESOURCE_GROUP:?}" "${ACCOUNT:?}" "${DEPLOYMENT:?}"

if ! az cognitiveservices account deployment show -g "$RESOURCE_GROUP" -n "$ACCOUNT" \
      --deployment-name "$DEPLOYMENT" -o none 2>/dev/null; then
  warn "deployment $DEPLOYMENT not found — nothing to do"; out action absent; exit 0
fi
if [[ "${DRY_RUN:-false}" == "true" ]]; then
  log "DRY RUN: az cognitiveservices account deployment delete -g $RESOURCE_GROUP -n $ACCOUNT --deployment-name $DEPLOYMENT"
  out action dry-run; exit 0
fi
az cognitiveservices account deployment delete -g "$RESOURCE_GROUP" -n "$ACCOUNT" --deployment-name "$DEPLOYMENT" -o none
# delete is async for managed compute; wait until gone
for _ in $(seq 1 60); do
  az cognitiveservices account deployment show -g "$RESOURCE_GROUP" -n "$ACCOUNT" --deployment-name "$DEPLOYMENT" -o none 2>/dev/null || { out action deleted; exit 0; }
  sleep 10
done
die "deployment still present after 10 minutes"
