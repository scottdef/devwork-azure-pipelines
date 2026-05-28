#!/usr/bin/env bash
###############################################################################
# strip-linked-services.sh — Remove linked services from ARM templates
#
# Strips three things:
#   1. Linked service RESOURCES from the template
#   2. dependsOn ENTRIES referencing linked services from remaining resources
#   3. Linked service PARAMETERS from the parameters file
#
# This prevents the deployment action's dependency resolver from failing
# with "Could not figure out full dependency model" when it can't find
# stripped linked services in the template.
#
# Usage:
#   ./scripts/strip-linked-services.sh <template> [params]
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
  | if test("'"'"'/[^'"'"']+'"'"'") then
      capture("'"'"'/(?<n>[^'"'"']+)'"'"'").n
    elif contains("/") then
      split("/") | last
    else
      .
    end
' "$TEMPLATE")

# ── Step 1: Strip linked service resources ──
echo "  Step 1: Removing linked service resources..."
jq 'del(.resources[] | select(.type | endswith("/linkedServices")))' \
  "$TEMPLATE" > "${TEMPLATE}.tmp"
mv "${TEMPLATE}.tmp" "$TEMPLATE"

AFTER=$(jq '.resources | length' "$TEMPLATE")
echo "    Resources: ${BEFORE} → ${AFTER} (${LS_COUNT} linked services removed)"

if [ -n "$LS_NAMES" ]; then
  echo "    Removed:"
  echo "$LS_NAMES" | while IFS= read -r name; do
    [ -z "$name" ] && continue
    echo "      • ${name}"
  done
fi

# ── Step 2: Strip dependsOn entries referencing linked services ──
echo "  Step 2: Cleaning dependsOn references..."

DEPENDS_BEFORE=$(jq '[.resources[].dependsOn? // [] | .[]] | length' "$TEMPLATE")

jq '
  .resources |= [
    .[] |
    if .dependsOn then
      .dependsOn |= [
        .[] | select(contains("/linkedServices/") | not)
      ]
    else
      .
    end
  ]
' "$TEMPLATE" > "${TEMPLATE}.tmp"
mv "${TEMPLATE}.tmp" "$TEMPLATE"

DEPENDS_AFTER=$(jq '[.resources[].dependsOn? // [] | .[]] | length' "$TEMPLATE")
DEPENDS_STRIPPED=$((DEPENDS_BEFORE - DEPENDS_AFTER))
echo "    dependsOn entries: ${DEPENDS_BEFORE} → ${DEPENDS_AFTER} (${DEPENDS_STRIPPED} removed)"

# ── Step 3: Strip linked service parameters ──
if [ -n "$PARAMS" ] && [ -f "$PARAMS" ] && [ -n "$LS_NAMES" ]; then
  echo "  Step 3: Removing linked service parameters..."

  PARAMS_BEFORE=$(jq '.parameters | keys | length' "$PARAMS")

  FILTER="."
  while IFS= read -r name; do
    [ -z "$name" ] && continue
    FILTER="${FILTER} | .parameters |= with_entries(select(.key | startswith(\"${name}_\") | not))"
    FILTER="${FILTER} | del(.parameters.\"${name}\")"
  done <<< "$LS_NAMES"

  jq "$FILTER" "$PARAMS" > "${PARAMS}.tmp"
  mv "${PARAMS}.tmp" "$PARAMS"

  PARAMS_AFTER=$(jq '.parameters | keys | length' "$PARAMS")
  PARAMS_STRIPPED=$((PARAMS_BEFORE - PARAMS_AFTER))
  echo "    Parameters: ${PARAMS_BEFORE} → ${PARAMS_AFTER} (${PARAMS_STRIPPED} removed)"
fi

echo ""
echo "==> Done"

# ── GitHub Actions outputs ──
if [ -n "${GITHUB_OUTPUT:-}" ]; then
  echo "stripped_resources=${LS_COUNT}" >> "$GITHUB_OUTPUT"
  echo "stripped_depends=${DEPENDS_STRIPPED}" >> "$GITHUB_OUTPUT"
  echo "stripped_names=$(echo "$LS_NAMES" | tr '\n' ',' | sed 's/,$//')" >> "$GITHUB_OUTPUT"
fi
