#!/usr/bin/env bash
###############################################################################
# detect-drift.sh — Compare live Synapse workspace state against template
#
# Usage:
#   ./scripts/detect-drift.sh <workspace-name> <template-file>
#
# Compares live artifact names (pipelines, triggers, datasets, linked services)
# against the expected names in the ARM template.  Exits non-zero if drift is
# detected, which the drift-detection workflow uses to create a GitHub issue.
#
# Requires: az CLI authenticated, jq
###############################################################################
set -euo pipefail

WORKSPACE="${1:?Usage: $0 <workspace-name> <template-file>}"
TEMPLATE="${2:?Usage: $0 <workspace-name> <template-file>}"
DRIFT=0

echo "==> Checking drift for ${WORKSPACE} against ${TEMPLATE}"

# ── Helper: extract expected names from ARM template by resource type suffix ──
extract_expected() {
  local suffix="$1"
  jq -r --arg s "$suffix" \
    '.resources[] | select(.type | endswith($s)) | .name | split("/")[-1]' \
    "$TEMPLATE" 2>/dev/null | sort
}

# ── Helper: compare lists ────────────────────────────────────────────────────
compare() {
  local label="$1" expected="$2" live="$3"

  echo ""
  echo "==> ${label}:"

  local only_expected only_live
  only_expected=$(comm -23 <(echo "$expected") <(echo "$live") || true)
  only_live=$(comm -13 <(echo "$expected") <(echo "$live") || true)

  if [ -n "$only_expected" ]; then
    echo "    DRIFT: in template but not live:"
    echo "$only_expected" | sed 's/^/      - /'
    DRIFT=1
  fi
  if [ -n "$only_live" ]; then
    echo "    DRIFT: live but not in template (manual change?):"
    echo "$only_live" | sed 's/^/      - /'
    DRIFT=1
  fi
  if [ -z "$only_expected" ] && [ -z "$only_live" ]; then
    echo "    OK — no drift"
  fi
}

# ── Gather live state ────────────────────────────────────────────────────────
LIVE_PIPELINES=$(az synapse pipeline list --workspace-name "$WORKSPACE" \
  --query "[].name" -o tsv 2>/dev/null | sort)
LIVE_TRIGGERS=$(az synapse trigger list --workspace-name "$WORKSPACE" \
  --query "[].name" -o tsv 2>/dev/null | sort)
LIVE_DATASETS=$(az synapse dataset list --workspace-name "$WORKSPACE" \
  --query "[].name" -o tsv 2>/dev/null | sort)
LIVE_LS=$(az synapse linked-service list --workspace-name "$WORKSPACE" \
  --query "[].name" -o tsv 2>/dev/null | sort)

# ── Gather expected state from template ──────────────────────────────────────
EXP_PIPELINES=$(extract_expected "pipelines")
EXP_TRIGGERS=$(extract_expected "triggers")
EXP_DATASETS=$(extract_expected "datasets")
EXP_LS=$(extract_expected "linkedServices")

# ── Compare ──────────────────────────────────────────────────────────────────
compare "Pipelines"       "$EXP_PIPELINES" "$LIVE_PIPELINES"
compare "Triggers"        "$EXP_TRIGGERS"  "$LIVE_TRIGGERS"
compare "Datasets"        "$EXP_DATASETS"  "$LIVE_DATASETS"
compare "Linked Services" "$EXP_LS"        "$LIVE_LS"

echo ""
if [ "$DRIFT" -eq 1 ]; then
  echo "==> DRIFT DETECTED — review changes above"
  exit 1
else
  echo "==> No drift detected"
  exit 0
fi
