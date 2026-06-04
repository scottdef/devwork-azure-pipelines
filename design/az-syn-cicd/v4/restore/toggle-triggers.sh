#!/usr/bin/env bash
set -euo pipefail
ACTION="${1:?Usage: $0 <stop|start> <workspace>}"
WORKSPACE="${2:?Usage: $0 <stop|start> <workspace>}"
FILE="./active-triggers.txt"
case "$ACTION" in
  stop)
    az synapse trigger list --workspace-name "$WORKSPACE" \
      --query "[?properties.runtimeState=='Started'].name" -o tsv > "$FILE"
    COUNT=$(wc -l < "$FILE" | tr -d ' ')
    echo "Stopping ${COUNT} trigger(s)..."
    while IFS= read -r t; do [ -z "$t" ] && continue
      az synapse trigger stop --workspace-name "$WORKSPACE" --name "$t" --no-wait || true
    done < "$FILE"
    [ "$COUNT" -gt 0 ] && sleep 10 ;;
  start)
    sleep 15; [ ! -f "$FILE" ] && exit 0
    while IFS= read -r t; do [ -z "$t" ] && continue
      for i in 1 2 3; do
        az synapse trigger start --workspace-name "$WORKSPACE" --name "$t" 2>/dev/null && break; sleep 5
      done
    done < "$FILE" ;;
esac
