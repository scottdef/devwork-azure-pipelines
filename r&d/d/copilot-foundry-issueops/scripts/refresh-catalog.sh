#!/usr/bin/env bash
# refresh-catalog.sh — snapshot the live Foundry catalog per account location
# into models/catalog/<location>.json and report registry drift
# (models in the registry that the region no longer offers).
# Env: none (reads registry). Output: drift report on stdout; exit 2 on drift.
set -euo pipefail
# shellcheck source=lib.sh
source "$(dirname "$0")/lib.sh"
need az jq yq
mkdir -p models/catalog
drift=0
for loc in $(reg '.accounts[].location' | sort -u); do
  log "catalog: $loc"
  az cognitiveservices model list -l "$loc" -o json \
    | jq '[.[] | {format:.model.format, name:.model.name, version:.model.version, skus:[.model.skus[]?.name], deprecation:.model.deprecation}] | sort_by(.format,.name,.version)' \
    > "models/catalog/$loc.json"
  for key in $(reg '.models | keys[]'); do
    kind="$(reg ".models[\"$key\"].kind")"; [[ "$kind" == managed ]] && continue   # HF registry models aren't in this list
    f="$(reg ".models[\"$key\"].format")"; n="$(reg ".models[\"$key\"].name")"
    if ! jq -e --arg f "$f" --arg n "$n" 'map(select(.format==$f and .name==$n)) | length > 0' "models/catalog/$loc.json" >/dev/null; then
      echo "DRIFT: $key ($f/$n) not available in $loc"; drift=1
    fi
  done
done
exit $((drift ? 2 : 0))
