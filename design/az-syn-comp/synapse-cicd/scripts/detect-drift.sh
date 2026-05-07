#!/usr/bin/env bash
# scripts/detect-drift.sh — compare live Synapse workspace against repo
# Usage: ./scripts/detect-drift.sh <workspace-name> [repo-root]
# Exit 1 = drift found, Exit 0 = clean
set -euo pipefail

WORKSPACE="${1:?Usage: $0 <workspace-name> [repo-root]}"
REPO_ROOT="${2:-.}"
DRIFT=false

extract_names() {
  local dir="$1"
  [ -d "${REPO_ROOT}/${dir}" ] && \
    find "${REPO_ROOT}/${dir}" -name '*.json' -exec basename {} .json \; | sort
}

compare() {
  local type="$1" expected="$2" live="$3"
  local added removed
  added=$(comm -13 <(echo "$expected") <(echo "$live") | grep -v '^$' || true)
  removed=$(comm -23 <(echo "$expected") <(echo "$live") | grep -v '^$' || true)
  [ -n "$added" ] && { DRIFT=true; echo "[drift] ${type}: unexpected live: ${added//$'\n'/, }"; }
  [ -n "$removed" ] && { DRIFT=true; echo "[drift] ${type}: missing live: ${removed//$'\n'/, }"; }
  [ -z "$added" ] && [ -z "$removed" ] && echo "[drift] ${type}: OK"
}

echo "[drift] Checking ${WORKSPACE}..."

compare "Pipelines" \
  "$(extract_names pipeline)" \
  "$(az synapse pipeline list --workspace-name "$WORKSPACE" --query '[].name' -o tsv 2>/dev/null | sort)"

compare "Triggers" \
  "$(extract_names trigger)" \
  "$(az synapse trigger list --workspace-name "$WORKSPACE" --query '[].name' -o tsv 2>/dev/null | sort)"

compare "Datasets" \
  "$(extract_names dataset)" \
  "$(az synapse dataset list --workspace-name "$WORKSPACE" --query '[].name' -o tsv 2>/dev/null | sort)"

compare "LinkedServices" \
  "$(extract_names linkedService)" \
  "$(az synapse linked-service list --workspace-name "$WORKSPACE" --query '[].name' -o tsv 2>/dev/null | sort)"

compare "Notebooks" \
  "$(extract_names notebook)" \
  "$(az synapse notebook list --workspace-name "$WORKSPACE" --query '[].name' -o tsv 2>/dev/null | sort)"

$DRIFT && { echo "[drift] DRIFT DETECTED."; exit 1; }
echo "[drift] Clean — no drift."
