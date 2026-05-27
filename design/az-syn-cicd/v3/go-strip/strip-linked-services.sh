#!/usr/bin/env bash
###############################################################################
# strip-linked-services.sh — Remove linked services from ARM templates
#
# Pure bash/jq alternative to the Go tool.  No compilation required.
#
# Usage:
#   ./scripts/strip-linked-services.sh <template> [params]
#   ./scripts/strip-linked-services.sh ExportedArtifacts/TemplateForWorkspace.json \
#                                       ExportedArtifacts/TemplateParametersForWorkspace.json
#
# Requires: jq
###############################################################################
set -euo pipefail

TEMPLATE="${1:?Usage: $0 <template.json> [params.json]}"
PARAMS="${2:-}"

echo "==> Stripping linked services from ${TEMPLATE}"

# ── Count before ──
BEFORE=$(jq '.resources | length' "$TEMPLATE")
LS_COUNT=$(jq '[.resources[] | select(.type | endswith("/linkedServices"))] | length' "$TEMPLATE")

# ── Extract linked service names (for parameter cleanup) ──
LS_NAMES=$(jq -r '
  .resources[]
  | select(.type | endswith("/linkedServices"))
  | .name
  | capture("'"'"'/(?<n>[^'"'"']+)'"'"'") // {n: .}
  | .n
' "$TEMPLATE")

# ── Strip from template ──
jq 'del(.resources[] | select(.type | endswith("/linkedServices")))' \
  "$TEMPLATE" > "${TEMPLATE}.tmp"
mv "${TEMPLATE}.tmp" "$TEMPLATE"

AFTER=$(jq '.resources | length' "$TEMPLATE")
echo "  Template: ${BEFORE} resources → ${AFTER} resources (${LS_COUNT} linked services stripped)"

if [ -n "$LS_NAMES" ]; then
  echo "  Removed:"
  echo "$LS_NAMES" | while IFS= read -r name; do
    [ -z "$name" ] && continue
    echo "    • ${name}"
  done
fi

# ── Strip from parameters ──
if [ -n "$PARAMS" ] && [ -f "$PARAMS" ] && [ -n "$LS_NAMES" ]; then
  echo ""
  echo "==> Stripping linked service parameters from ${PARAMS}"

  PARAMS_BEFORE=$(jq '.parameters | keys | length' "$PARAMS")

  # Build a jq filter that deletes parameter keys starting with each LS name
  FILTER="."
  while IFS= read -r name; do
    [ -z "$name" ] && continue
    # Delete any parameter whose key starts with the LS name followed by _
    FILTER="${FILTER} | .parameters |= with_entries(select(.key | startswith(\"${name}_\") | not))"
    # Also delete the exact key match
    FILTER="${FILTER} | del(.parameters.\"${name}\")"
  done <<< "$LS_NAMES"

  jq "$FILTER" "$PARAMS" > "${PARAMS}.tmp"
  mv "${PARAMS}.tmp" "$PARAMS"

  PARAMS_AFTER=$(jq '.parameters | keys | length' "$PARAMS")
  PARAMS_STRIPPED=$((PARAMS_BEFORE - PARAMS_AFTER))
  echo "  Parameters: ${PARAMS_BEFORE} → ${PARAMS_AFTER} (${PARAMS_STRIPPED} stripped)"
fi

echo ""
echo "==> Done"

# ── GitHub Actions outputs ──
if [ -n "${GITHUB_OUTPUT:-}" ]; then
  echo "stripped_resources=${LS_COUNT}" >> "$GITHUB_OUTPUT"
  echo "stripped_names=$(echo "$LS_NAMES" | tr '\n' ',' | sed 's/,$//')" >> "$GITHUB_OUTPUT"
fi
