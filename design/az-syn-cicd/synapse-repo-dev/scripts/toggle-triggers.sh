#!/usr/bin/env bash
###############################################################################
# toggle-triggers.sh
# Stop or start all active Synapse triggers before/after a deployment.
#
# Usage:
#   toggle-triggers.sh stop  <workspace-name>  [state-file]
#   toggle-triggers.sh start <workspace-name>  [state-file]
#
# The state-file (default: /tmp/synapse-active-triggers.json) records which
# triggers were running so we can restart exactly those after deployment.
###############################################################################
set -euo pipefail

ACTION="$1"
WORKSPACE="$2"
STATE_FILE="${3:-/tmp/synapse-active-triggers.json}"
MAX_RETRIES=5
RETRY_DELAY=10

# ── helpers ──────────────────────────────────────────────────────────────────

log()  { echo "[$(date -u '+%Y-%m-%dT%H:%M:%SZ')] $*"; }
err()  { echo "[$(date -u '+%Y-%m-%dT%H:%M:%SZ')] ERROR: $*" >&2; }
die()  { err "$*"; exit 1; }

az_trigger_state() {
  az synapse trigger show \
    --workspace-name "$WORKSPACE" \
    --name "$1" \
    --query "properties.runtimeState" -o tsv 2>/dev/null || echo "Unknown"
}

wait_for_trigger_state() {
  local name="$1" target_state="$2"
  for i in $(seq 1 $MAX_RETRIES); do
    local current
    current=$(az_trigger_state "$name")
    if [ "$current" = "$target_state" ]; then
      log "  ✓ $name is $target_state"
      return 0
    fi
    log "  ⏳ $name current=$current target=$target_state (attempt $i/$MAX_RETRIES)"
    sleep "$RETRY_DELAY"
  done
  err "Trigger $name did not reach $target_state after $MAX_RETRIES retries"
  return 1
}

# ── stop ─────────────────────────────────────────────────────────────────────

stop_triggers() {
  log "==> Fetching active triggers from workspace: $WORKSPACE"

  local active_json
  active_json=$(az synapse trigger list \
    --workspace-name "$WORKSPACE" \
    --query "[?properties.runtimeState=='Started'].name" \
    -o json 2>/dev/null || echo "[]")

  echo "$active_json" > "$STATE_FILE"

  local count
  count=$(echo "$active_json" | jq 'length')
  log "==> Found $count active trigger(s)"

  if [ "$count" -eq 0 ]; then
    log "==> Nothing to stop."
    return 0
  fi

  echo "$active_json" | jq -r '.[]' | while IFS= read -r trigger; do
    [ -z "$trigger" ] && continue
    log "  Stopping: $trigger"
    az synapse trigger stop \
      --workspace-name "$WORKSPACE" \
      --name "$trigger" \
      --no-wait || err "Failed to issue stop for $trigger (continuing)"
  done

  log "==> Waiting for triggers to reach Stopped state..."
  echo "$active_json" | jq -r '.[]' | while IFS= read -r trigger; do
    [ -z "$trigger" ] && continue
    wait_for_trigger_state "$trigger" "Stopped" || true
  done

  log "==> All triggers stopped. State saved to: $STATE_FILE"
}

# ── start ─────────────────────────────────────────────────────────────────────

start_triggers() {
  if [ ! -f "$STATE_FILE" ]; then
    log "==> No state file found at $STATE_FILE — skipping trigger restart"
    return 0
  fi

  local names
  names=$(jq -r '.[]' "$STATE_FILE" 2>/dev/null || echo "")

  local count
  count=$(jq 'length' "$STATE_FILE" 2>/dev/null || echo 0)
  log "==> Restarting $count trigger(s)..."

  # Brief pause for deployment to fully settle
  sleep 20

  echo "$names" | while IFS= read -r trigger; do
    [ -z "$trigger" ] && continue
    local started=false
    for i in $(seq 1 $MAX_RETRIES); do
      log "  Starting: $trigger (attempt $i)"
      if az synapse trigger start \
          --workspace-name "$WORKSPACE" \
          --name "$trigger" 2>/dev/null; then
        wait_for_trigger_state "$trigger" "Started" && started=true && break
      fi
      sleep "$RETRY_DELAY"
    done
    if [ "$started" = "false" ]; then
      err "Failed to start trigger $trigger after $MAX_RETRIES attempts"
    fi
  done

  log "==> Trigger restart complete"
  rm -f "$STATE_FILE"
}

# ── main ──────────────────────────────────────────────────────────────────────

case "$ACTION" in
  stop)  stop_triggers  ;;
  start) start_triggers ;;
  *)     die "Usage: $0 {stop|start} <workspace-name> [state-file]" ;;
esac
