#!/usr/bin/env bash
###############################################################################
# toggle-triggers.sh — Stop/start Synapse triggers for safe deployment
#
# Usage:
#   ./scripts/toggle-triggers.sh stop  <workspace-name>
#   ./scripts/toggle-triggers.sh start <workspace-name>
#
# On stop:  saves active trigger names to ./active-triggers.txt, then stops them.
# On start: reads ./active-triggers.txt and restarts each with retry logic.
#
# This is the single most important deployment safety measure.  If you deploy
# while triggers are running, the deployment fails with TriggerEnabledCannotUpdate.
#
# Requires: az CLI authenticated
###############################################################################
set -euo pipefail

ACTION="${1:?Usage: $0 <stop|start> <workspace-name>}"
WORKSPACE="${2:?Usage: $0 <stop|start> <workspace-name>}"
TRIGGER_FILE="./active-triggers.txt"
MAX_RETRIES=3

case "$ACTION" in
  stop)
    echo "==> Listing active triggers on ${WORKSPACE}..."
    az synapse trigger list \
      --workspace-name "$WORKSPACE" \
      --query "[?properties.runtimeState=='Started'].name" \
      -o tsv > "$TRIGGER_FILE"

    COUNT=$(wc -l < "$TRIGGER_FILE" | tr -d ' ')
    echo "==> Found ${COUNT} active trigger(s)"

    while IFS= read -r trigger; do
      [ -z "$trigger" ] && continue
      echo "    Stopping: ${trigger}"
      az synapse trigger stop \
        --workspace-name "$WORKSPACE" \
        --name "$trigger" \
        --no-wait || echo "    WARN: failed to stop ${trigger}"
    done < "$TRIGGER_FILE"

    # Wait for stops to propagate
    if [ "$COUNT" -gt 0 ]; then
      echo "==> Waiting 10s for trigger stops to propagate..."
      sleep 10
    fi
    echo "==> All triggers stopped"
    ;;

  start)
    echo "==> Waiting 15s for deployment to settle..."
    sleep 15

    if [ ! -f "$TRIGGER_FILE" ]; then
      echo "==> No trigger file found — nothing to restart"
      exit 0
    fi

    COUNT=$(wc -l < "$TRIGGER_FILE" | tr -d ' ')
    echo "==> Restarting ${COUNT} trigger(s) on ${WORKSPACE}..."

    while IFS= read -r trigger; do
      [ -z "$trigger" ] && continue
      for attempt in $(seq 1 $MAX_RETRIES); do
        if az synapse trigger start \
          --workspace-name "$WORKSPACE" \
          --name "$trigger" 2>/dev/null; then
          echo "    Started: ${trigger}"
          break
        else
          echo "    Retry ${attempt}/${MAX_RETRIES} for ${trigger}"
          sleep 5
        fi
      done
    done < "$TRIGGER_FILE"
    echo "==> All triggers restarted"
    ;;

  *)
    echo "Unknown action: ${ACTION}" >&2
    exit 1
    ;;
esac
