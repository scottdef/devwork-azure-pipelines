#!/usr/bin/env bash
###############################################################################
# strip-template.sh — Clean generated ARM template for delta deployment
#
# After validate generates templates from the staged ArtifactsFolder:
#   1. Strips any linked services that leaked through
#   2. Strips ALL dependsOn entries (prevents dependency-not-found errors)
#   3. Removes orphaned parameters
#
# Usage:
#   ./scripts/strip-template.sh <template.json> [params.json]
#
# Requires: jq
###############################################################################
set -euo pipefail

TEMPLATE="${1:?Usage: $0 <template.json> [params.json]}"
PARAMS="${2:-}"

JQ_NAME='
  if test("'"'"'/[^'"'"']+'"'"'") then
    capture("'"'"'/(?<n>[^'"'"']+)'"'"'").n
  elif contains("/") then split("/") | last
  else . end
'

echo "==> Cleaning generated template"

BEFORE=$(jq '.resources | length' "$TEMPLATE")

# ── Strip any linked services ──
LS_COUNT=$(jq '[.resources[] | select(.type | endswith("/linkedServices"))] | length' "$TEMPLATE")
if [ "$LS_COUNT" -gt 0 ]; then
  LS_NAMES=$(jq -r '.resources[] | select(.type | endswith("/linkedServices")) | .name | '"$JQ_NAME" "$TEMPLATE")
  jq '.resources = [.resources[] | select(.type | endswith("/linkedServices") | not)]' \
    "$TEMPLATE" > "${TEMPLATE}.tmp"
  mv "${TEMPLATE}.tmp" "$TEMPLATE"
  echo "  Linked services stripped: ${LS_COUNT}"
else
  LS_NAMES=""
  echo "  No linked services (staging excluded them)"
fi

# ── Strip ALL dependsOn ──
DEPS=$(jq '[.resources[].dependsOn? // [] | .[]] | length' "$TEMPLATE")
jq '.resources = [.resources[] | del(.dependsOn)]' "$TEMPLATE" > "${TEMPLATE}.tmp"
mv "${TEMPLATE}.tmp" "$TEMPLATE"
echo "  dependsOn stripped: ${DEPS}"

# ── Clean parameters ──
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
  echo "  Parameters cleaned: $((PBEFORE - PAFTER))"
fi

AFTER=$(jq '.resources | length' "$TEMPLATE")
echo ""
echo "  Resources to deploy: ${AFTER}"
jq -r '.resources[] | "    \(.type | split("/") | last): \(.name | '"$JQ_NAME"')"' "$TEMPLATE"

if [ -n "${GITHUB_OUTPUT:-}" ]; then
  echo "resource_count=${AFTER}" >> "$GITHUB_OUTPUT"
fi
