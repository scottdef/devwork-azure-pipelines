#!/usr/bin/env bash
# deploy-serverless.sh — deploy a "Foundry Models sold by Azure" model
# (serverless pay-per-token OR ProvisionedManaged PTU) with az CLI. Idempotent (PUT).
#
# Env: RESOURCE_GROUP ACCOUNT DEPLOYMENT MODEL_FORMAT MODEL_NAME MODEL_VERSION
#      SKU CAPACITY [RAI_POLICY=Microsoft.DefaultV2] [DRY_RUN=false] [LOCATION]
set -euo pipefail
# shellcheck source=lib.sh
source "$(dirname "$0")/lib.sh"
need az jq

: "${RESOURCE_GROUP:?}" "${ACCOUNT:?}" "${DEPLOYMENT:?}" "${MODEL_FORMAT:?}" \
  "${MODEL_NAME:?}" "${MODEL_VERSION:?}" "${SKU:?}" "${CAPACITY:?}"
RAI_POLICY="${RAI_POLICY:-Microsoft.DefaultV2}"
DRY_RUN="${DRY_RUN:-false}"

log "target  : $RESOURCE_GROUP/$ACCOUNT/deployments/$DEPLOYMENT"
log "model   : $MODEL_FORMAT/$MODEL_NAME@$MODEL_VERSION  sku=$SKU cap=$CAPACITY rai=$RAI_POLICY"

# ── pre-flight: account exists, quota headroom ───────────────────────────
az cognitiveservices account show -g "$RESOURCE_GROUP" -n "$ACCOUNT" -o none \
  || die "account $ACCOUNT not found in $RESOURCE_GROUP"

LOCATION="${LOCATION:-$(az cognitiveservices account show -g "$RESOURCE_GROUP" -n "$ACCOUNT" --query location -o tsv)}"
usage_json="$(az cognitiveservices usage list -l "$LOCATION" -o json 2>/dev/null || echo '[]')"
quota_name="$(jq -r --arg f "$MODEL_FORMAT" --arg n "$MODEL_NAME" --arg s "$SKU" \
  '[.[] | select(.name.value | test("\($f).\($s).\($n)"; "i"))][0].name.value // empty' <<<"$usage_json")"
if [[ -n "$quota_name" ]]; then
  read -r cur lim < <(jq -r --arg q "$quota_name" '.[] | select(.name.value==$q) | "\(.currentValue) \(.limit)"' <<<"$usage_json")
  log "quota   : $quota_name current=$cur limit=$lim requested=$CAPACITY"
  (( cur + CAPACITY <= lim )) || die "quota exceeded: $cur+$CAPACITY > $lim for $quota_name (request an increase or lower capacity)"
else
  warn "no matching quota entry found for $MODEL_FORMAT/$SKU/$MODEL_NAME in $LOCATION — proceeding without quota pre-check"
fi

# ── existing deployment? (upsert semantics, but say so) ──────────────────
if az cognitiveservices account deployment show -g "$RESOURCE_GROUP" -n "$ACCOUNT" \
     --deployment-name "$DEPLOYMENT" -o none 2>/dev/null; then
  log "deployment exists → update in place"
  out action updated
else
  out action created
fi

if [[ "$DRY_RUN" == "true" ]]; then
  log "DRY RUN — command that would run:"
  cat >&2 <<CMD
az cognitiveservices account deployment create -g $RESOURCE_GROUP -n $ACCOUNT \\
  --deployment-name $DEPLOYMENT --model-format "$MODEL_FORMAT" --model-name "$MODEL_NAME" \\
  --model-version "$MODEL_VERSION" --sku-name $SKU --sku-capacity $CAPACITY --rai-policy-name $RAI_POLICY
CMD
  out provisioning_state dry-run
  exit 0
fi

# ── deploy ───────────────────────────────────────────────────────────────
# `--rai-policy-name` exists in current az; older CLIs lack it. Guard, fall back to az rest.
if az cognitiveservices account deployment create --help 2>/dev/null | grep -q -- '--rai-policy-name'; then
  az cognitiveservices account deployment create -g "$RESOURCE_GROUP" -n "$ACCOUNT" \
    --deployment-name "$DEPLOYMENT" \
    --model-format "$MODEL_FORMAT" --model-name "$MODEL_NAME" --model-version "$MODEL_VERSION" \
    --sku-name "$SKU" --sku-capacity "$CAPACITY" \
    --rai-policy-name "$RAI_POLICY" -o none
else
  warn "az CLI lacks --rai-policy-name; using az rest PUT"
  acct_id="$(az_id "$RESOURCE_GROUP" "$ACCOUNT")"
  body="$(jq -n --arg f "$MODEL_FORMAT" --arg n "$MODEL_NAME" --arg v "$MODEL_VERSION" \
              --arg s "$SKU" --argjson c "$CAPACITY" --arg r "$RAI_POLICY" \
    '{sku:{name:$s,capacity:$c},properties:{model:{format:$f,name:$n,version:$v},raiPolicyName:$r,versionUpgradeOption:"OnceNewDefaultVersionAvailable"}}')"
  az rest --method put \
    --url "https://management.azure.com${acct_id}/deployments/${DEPLOYMENT}?api-version=2025-06-01" \
    --body "$body" -o none
fi

poll "az cognitiveservices account deployment show -g $RESOURCE_GROUP -n $ACCOUNT --deployment-name $DEPLOYMENT --query properties.provisioningState -o tsv"

endpoint="$(az cognitiveservices account show -g "$RESOURCE_GROUP" -n "$ACCOUNT" --query properties.endpoint -o tsv)"
out provisioning_state Succeeded
out endpoint "$endpoint"
out resolved_version "$(az cognitiveservices account deployment show -g "$RESOURCE_GROUP" -n "$ACCOUNT" --deployment-name "$DEPLOYMENT" --query properties.model.version -o tsv)"
