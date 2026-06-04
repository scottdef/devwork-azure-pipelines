#!/usr/bin/env bash
###############################################################################
# filter-and-strip.sh — Build a delta ARM template from changed files
#
# Takes a full ARM template (from validate) and a list of changed file paths
# (from git diff), then:
#   1. Maps file paths → artifact names
#   2. Filters template to only matching resources
#   3. Strips all linked service resources (UI-managed)
#   4. Strips ALL dependsOn entries (prevents "dependency not found" errors
#      when referenced resources aren't in the delta template)
#   5. Cleans orphaned parameters
#
# Usage:
#   git diff --name-only origin/dev...HEAD -- pipeline/ dataset/ > changed.txt
#   ./scripts/filter-and-strip.sh template.json params.json changed.txt
#
# Requires: jq 1.6+
###############################################################################
set -euo pipefail

TEMPLATE="${1:?Usage: $0 <template.json> <params.json> <changed-files-list>}"
PARAMS="${2:?Usage: $0 <template.json> <params.json> <changed-files-list>}"
CHANGED_LIST="${3:?Usage: $0 <template.json> <params.json> <changed-files-list>}"

[ -f "$TEMPLATE" ]    || { echo "::error::Template not found: ${TEMPLATE}"; exit 1; }
[ -f "$CHANGED_LIST" ] || { echo "::error::Changed list not found: ${CHANGED_LIST}"; exit 1; }

# ─── Helper: extract artifact name from ARM resource name field ─────────────
# Handles three formats:
#   "[concat(parameters('workspaceName'), '/ArtifactName')]" → ArtifactName
#   "workspace/ArtifactName"                                 → ArtifactName
#   "ArtifactName"                                           → ArtifactName
JQ_EXTRACT_NAME='
  if test("'"'"'/[^'"'"']+'"'"'") then
    capture("'"'"'/(?<n>[^'"'"']+)'"'"'").n
  elif contains("/") then
    split("/") | last
  else . end
'

# ═════════════════════════════════════════════════════════════════════════════
# STEP 1: Map changed file paths → artifact names
# ═════════════════════════════════════════════════════════════════════════════
echo "==> Step 1: Mapping changed files to artifact names"

ARTIFACT_NAMES=""
CHANGED_COUNT=0

while IFS= read -r filepath; do
  [ -z "$filepath" ] && continue
  dir=$(echo "$filepath" | sed 's|/[^/]*$||; s|.*/||')
  base=$(basename "$filepath" .json)

  case "$dir" in
    pipeline|dataset|notebook|trigger|dataflow|sqlscript)
      ARTIFACT_NAMES="${ARTIFACT_NAMES}${base}"$'\n'
      CHANGED_COUNT=$((CHANGED_COUNT + 1))
      echo "    ${filepath} → ${base}"
      ;;
  esac
done < "$CHANGED_LIST"

ARTIFACT_NAMES=$(echo "$ARTIFACT_NAMES" | sort -u | sed '/^$/d')
UNIQUE_COUNT=$(echo "$ARTIFACT_NAMES" | grep -c . || true)

echo "  ${CHANGED_COUNT} files → ${UNIQUE_COUNT} unique artifacts"

if [ "$UNIQUE_COUNT" -eq 0 ]; then
  echo "  No deployable artifacts changed"
  [ -n "${GITHUB_OUTPUT:-}" ] && echo "artifact_count=0" >> "$GITHUB_OUTPUT"
  [ -n "${GITHUB_OUTPUT:-}" ] && echo "skip_deploy=true" >> "$GITHUB_OUTPUT"
  exit 0
fi

# ═════════════════════════════════════════════════════════════════════════════
# STEP 2: Filter template to changed resources only
# ═════════════════════════════════════════════════════════════════════════════
echo ""
echo "==> Step 2: Filtering template to changed resources"

BEFORE=$(jq '.resources | length' "$TEMPLATE")

# Build jq-compatible name array
JQ_NAMES=$(echo "$ARTIFACT_NAMES" | jq -R -s 'split("\n") | map(select(length > 0))')

jq --argjson names "$JQ_NAMES" '
  .resources = [
    .resources[] |
    (.name | '"$JQ_EXTRACT_NAME"') as $n |
    if (.type | endswith("/linkedServices")) then
      .   # keep — stripped in step 3
    elif ($names | index($n)) then
      .   # changed — keep
    else
      empty
    end
  ]
' "$TEMPLATE" > "${TEMPLATE}.tmp"
mv "${TEMPLATE}.tmp" "$TEMPLATE"

AFTER_FILTER=$(jq '.resources | length' "$TEMPLATE")
echo "  Resources: ${BEFORE} → ${AFTER_FILTER}"

# ═════════════════════════════════════════════════════════════════════════════
# STEP 3: Strip linked services
# ═════════════════════════════════════════════════════════════════════════════
echo ""
echo "==> Step 3: Stripping linked services"

LS_COUNT=$(jq '[.resources[] | select(.type | endswith("/linkedServices"))] | length' "$TEMPLATE")

LS_NAMES=$(jq -r '.resources[] | select(.type | endswith("/linkedServices")) | .name | '"$JQ_EXTRACT_NAME" "$TEMPLATE")

jq '.resources = [.resources[] | select(.type | endswith("/linkedServices") | not)]' \
  "$TEMPLATE" > "${TEMPLATE}.tmp"
mv "${TEMPLATE}.tmp" "$TEMPLATE"

AFTER_LS=$(jq '.resources | length' "$TEMPLATE")
echo "  Stripped ${LS_COUNT} linked service(s) → ${AFTER_LS} resources remaining"

if [ -n "$LS_NAMES" ]; then
  echo "$LS_NAMES" | while IFS= read -r n; do [ -n "$n" ] && echo "    • $n"; done
fi

# ═════════════════════════════════════════════════════════════════════════════
# STEP 4: Strip ALL dependsOn entries
# ═════════════════════════════════════════════════════════════════════════════
#
# In a delta template, most dependsOn references point to resources NOT in
# the template (because they didn't change and were filtered out in step 2).
# The deployment action's dependency resolver fails on any dangling reference:
#   "Could not figure out full dependency model"
#
# Since we deploy with DeleteArtifactsNotInTemplate: false, the referenced
# resources already exist in the workspace.  Deployment ordering is
# irrelevant — we're updating a subset of an existing workspace, not
# bootstrapping from scratch.  Stripping all dependsOn is safe.
# ═════════════════════════════════════════════════════════════════════════════
echo ""
echo "==> Step 4: Stripping dependsOn entries"

DEPENDS_COUNT=$(jq '[.resources[].dependsOn? // [] | .[]] | length' "$TEMPLATE")

jq '.resources = [.resources[] | del(.dependsOn)]' "$TEMPLATE" > "${TEMPLATE}.tmp"
mv "${TEMPLATE}.tmp" "$TEMPLATE"

echo "  Removed ${DEPENDS_COUNT} dependsOn entries"

# ═════════════════════════════════════════════════════════════════════════════
# STEP 5: Clean parameters
# ═════════════════════════════════════════════════════════════════════════════
echo ""
echo "==> Step 5: Cleaning parameters"

if [ -f "$PARAMS" ]; then
  PARAMS_BEFORE=$(jq '.parameters | keys | length' "$PARAMS")

  # Collect names of remaining resources (for keeping their params)
  KEEP_NAMES=$(jq -r '[.resources[].name | '"$JQ_EXTRACT_NAME"'] | unique | .[]' "$TEMPLATE")

  # Build jq filter: keep workspaceName + params matching remaining resources
  # Remove params matching stripped linked services
  FILTER=".parameters |= with_entries(
    select(
      .key == \"workspaceName\" or
      .key == \"workspaceId\" or
      ($(
        # Build OR conditions for remaining resource names
        prefix_conditions=""
        while IFS= read -r name; do
          [ -z "$name" ] && continue
          [ -n "$prefix_conditions" ] && prefix_conditions="${prefix_conditions} or "
          prefix_conditions="${prefix_conditions}(.key | startswith(\"${name}\"))"
        done <<< "$KEEP_NAMES"
        if [ -n "$prefix_conditions" ]; then
          echo "$prefix_conditions"
        else
          echo "false"
        fi
      ))
    )
  )"

  jq "$FILTER" "$PARAMS" > "${PARAMS}.tmp"
  mv "${PARAMS}.tmp" "$PARAMS"

  PARAMS_AFTER=$(jq '.parameters | keys | length' "$PARAMS")
  echo "  Parameters: ${PARAMS_BEFORE} → ${PARAMS_AFTER}"
else
  echo "  No params file — skipped"
fi

# ═════════════════════════════════════════════════════════════════════════════
# REPORT
# ═════════════════════════════════════════════════════════════════════════════
FINAL=$(jq '.resources | length' "$TEMPLATE")

echo ""
echo "═══════════════════════════════════════════════════════"
echo "  DELTA TEMPLATE READY"
echo "═══════════════════════════════════════════════════════"
echo "  Changed artifacts:     ${UNIQUE_COUNT}"
echo "  Resources to deploy:   ${FINAL}"
echo "  Linked svcs stripped:  ${LS_COUNT}"
echo "  dependsOn stripped:    ${DEPENDS_COUNT}"
echo "═══════════════════════════════════════════════════════"

jq -r '.resources[] | "    \(.type | split("/") | last): \(.name | '"$JQ_EXTRACT_NAME"')"' "$TEMPLATE"

echo ""

if [ -n "${GITHUB_OUTPUT:-}" ]; then
  echo "artifact_count=${UNIQUE_COUNT}" >> "$GITHUB_OUTPUT"
  echo "resource_count=${FINAL}" >> "$GITHUB_OUTPUT"
  echo "skip_deploy=false" >> "$GITHUB_OUTPUT"
fi

if [ -n "${GITHUB_STEP_SUMMARY:-}" ]; then
  {
    echo "## Delta Template Report"
    echo "| Metric | Count |"
    echo "|---|---|"
    echo "| Changed artifacts | ${UNIQUE_COUNT} |"
    echo "| Resources to deploy | ${FINAL} |"
    echo "| Linked services stripped | ${LS_COUNT} |"
    echo "| dependsOn stripped | ${DEPENDS_COUNT} |"
    echo ""
    echo "**Resources:**"
    jq -r '.resources[] | "- `\(.name | '"$JQ_EXTRACT_NAME"')` (\(.type | split("/") | last))"' "$TEMPLATE"
  } >> "$GITHUB_STEP_SUMMARY"
fi
