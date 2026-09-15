#!/usr/bin/env bash
# deploy-managed.sh — deploy a partner/community (Hugging Face) model on
# Foundry managed compute (dedicated GPU) via `az rest`.
#
# The managed-compute deployment body is the Foundry "managed_compute_deployments"
# shape. It is in public preview; the deploymentTemplate id and api-version move.
# Run with DRY_RUN=true first — it prints the exact PUT so you can diff it
# against the current docs before spending GPU-hours.
#
# Env: RESOURCE_GROUP ACCOUNT DEPLOYMENT MODEL_REGISTRY MODEL_VERSION
#      ACCELERATOR [CAPACITY=1] [DRY_RUN=false] [API_VERSION=2025-06-01]
set -euo pipefail
# shellcheck source=lib.sh
source "$(dirname "$0")/lib.sh"
need az jq

: "${RESOURCE_GROUP:?}" "${ACCOUNT:?}" "${DEPLOYMENT:?}" "${MODEL_REGISTRY:?}" "${ACCELERATOR:?}"
MODEL_VERSION="${MODEL_VERSION:-latest}"
CAPACITY="${CAPACITY:-1}"
DRY_RUN="${DRY_RUN:-false}"
API_VERSION="${API_VERSION:-2025-06-01}"

acct_id="$(az_id "$RESOURCE_GROUP" "$ACCOUNT")"

# registry model path → versioned asset id.  "latest" → highest label on the registry.
if [[ "$MODEL_VERSION" == "latest" ]]; then
  model_asset="${MODEL_REGISTRY}/labels/latest"
else
  model_asset="${MODEL_REGISTRY}/versions/${MODEL_VERSION}"
fi
# Template id follows the HF-on-Foundry convention: <model>--<ctx>-<accelerator>. Override with DEPLOYMENT_TEMPLATE.
tmpl_default="${MODEL_REGISTRY/models\//deploymenttemplates/}--$(tr '[:upper:]' '[:lower:]' <<<"$ACCELERATOR" | sed 's/_80gb//; s/_/-/g')/labels/latest"
DEPLOYMENT_TEMPLATE="${DEPLOYMENT_TEMPLATE:-$tmpl_default}"

body="$(jq -n --arg m "$model_asset" --arg t "$DEPLOYMENT_TEMPLATE" --arg acc "$ACCELERATOR" --argjson c "$CAPACITY" '{
  sku: { name: "GlobalManagedCompute", capacity: $c },
  properties: {
    model: { format: "HuggingFace", name: $m },
    deploymentTemplate: $t,
    acceleratorType: $acc
  }
}')"

url="https://management.azure.com${acct_id}/deployments/${DEPLOYMENT}?api-version=${API_VERSION}"
log "PUT $url"
jq . <<<"$body" >&2

if [[ "$DRY_RUN" == "true" ]]; then out provisioning_state dry-run; exit 0; fi

az rest --method put --url "$url" --body "$body" -o none
poll "az rest --method get --url '$url' --query properties.provisioningState -o tsv" 3600

out provisioning_state Succeeded
out endpoint "$(az cognitiveservices account show -g "$RESOURCE_GROUP" -n "$ACCOUNT" --query properties.endpoint -o tsv)"
out resolved_version "$model_asset"
