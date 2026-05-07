#!/usr/bin/env bash
# scripts/toggle-triggers.sh — stop or start Synapse triggers
# Usage: ./scripts/toggle-triggers.sh stop|start <workspace-name>
set -euo pipefail

ACTION="${1:?Usage: $0 <stop|start> <workspace-name>}"
WORKSPACE="${2:?Usage: $0 <stop|start> <workspace-name>}"
STATE_FILE="active-triggers.json"

case "$ACTION" in
  stop)
    echo "[triggers] Listing active triggers on ${WORKSPACE}..."
    triggers=$(az synapse trigger list \
      --workspace-name "$WORKSPACE" \
      --query "[?properties.runtimeState=='Started'].name" -o tsv)

    if [ -z "$triggers" ]; then
      echo "[triggers] No active triggers."
      echo "[]" > "$STATE_FILE"
      exit 0
    fi

    echo "$triggers" | jq -R -s 'split("\n") | map(select(. != ""))' > "$STATE_FILE"
    while IFS= read -r t; do
      [ -z "$t" ] && continue
      echo "[triggers]   Stopping: $t"
      az synapse trigger stop --workspace-name "$WORKSPACE" --name "$t" || true
    done <<< "$triggers"
    echo "[triggers] Done."
    ;;

  start)
    sleep 15
    [ ! -f "$STATE_FILE" ] && { echo "[triggers] No state file."; exit 0; }
    mapfile -t names < <(jq -r '.[]' "$STATE_FILE")
    for name in "${names[@]}"; do
      [ -z "$name" ] && continue
      for i in 1 2 3; do
        az synapse trigger start --workspace-name "$WORKSPACE" --name "$name" 2>/dev/null && break
        sleep 5
      done
    done
    echo "[triggers] Restart complete."
    ;;

  *)
    echo "Usage: $0 <stop|start> <workspace-name>" && exit 1
    ;;
esac
