#!/usr/bin/env bash
###############################################################################
# strip-for-rollback.sh — Prepare ARM template for rollback deployment
#
# For rollback we deploy the FULL template (all artifacts at that version),
# not a delta.  We still strip linked services and dependsOn.
#
# Usage:
#   ./scripts/strip-for-rollback.sh <template.json> [params.json]
#
# Requires: jq
###############################################################################
set -euo pipefail

TEMPLATE="${1:?Usage: $0 <template.json> [params.json]}"
PARAMS="${2:-}"

JQ_EXTRACT='
  if test("'"'"'/[^'"'"']+'"'"'") then
    capture("'"'"'/(?<n>[^'"'"']+)'"'"'").n
  elif contains("/") then
    split("/") | last
  else . end
'

echo "==> Preparing rollback template"

BEFORE=$(jq '.resources | length' "$TEMPLATE")

# ── Strip linked services ──
LS_COUNT=$(jq '[.resources[] | select(.type | endswith("/linkedServices"))] | length' "$TEMPLATE")
LS_NAMES=$(jq -r '.resources[] | select(.type | endswith("/linkedServices")) | .name | '"$JQ_EXTRACT" "$TEMPLATE")

jq '.resources = [.resources[] | select(.type | endswith("/linkedServices") | not)]' \
  "$TEMPLATE" > "${TEMPLATE}.tmp"
mv "${TEMPLATE}.tmp" "$TEMPLATE"

# ── Strip ALL dependsOn ──
DEPS=$(jq '[.resources[].dependsOn? // [] | .[]] | length' "$TEMPLATE")
jq '.resources = [.resources[] | del(.dependsOn)]' "$TEMPLATE" > "${TEMPLATE}.tmp"
mv "${TEMPLATE}.tmp" "$TEMPLATE"

AFTER=$(jq '.resources | length' "$TEMPLATE")

# ── Strip linked service parameters ──
PARAMS_CLEANED=0
if [ -n "$PARAMS" ] && [ -f "$PARAMS" ] && [ -n "$LS_NAMES" ]; then
  PBEFORE=$(jq '.parameters | keys | length' "$PARAMS")
  FILTER="."
  while IFS= read -r name; do
    [ -z "$name" ] && continue
    FILTER="${FILTER} | .parameters |= with_entries(select(.key | startswith(\"${name}_\") | not))"
    FILTER="${FILTER} | del(.parameters.\"${name}\")"
  done <<< "$LS_NAMES"
  jq "$FILTER" "$PARAMS" > "${PARAMS}.tmp"
  mv "${PARAMS}.tmp" "$PARAMS"
  PAFTER=$(jq '.parameters | keys | length' "$PARAMS")
  PARAMS_CLEANED=$((PBEFORE - PAFTER))
fi

echo "  Resources:    ${BEFORE} → ${AFTER}"
echo "  Linked svcs:  ${LS_COUNT} stripped"
echo "  dependsOn:    ${DEPS} stripped"
echo "  Parameters:   ${PARAMS_CLEANED} cleaned"
echo ""

jq -r '.resources[] | "  \(.type | split("/") | last): \(.name | '"$JQ_EXTRACT"')"' "$TEMPLATE"

if [ -n "${GITHUB_OUTPUT:-}" ]; then
  echo "resource_count=${AFTER}" >> "$GITHUB_OUTPUT"
fi
