#!/usr/bin/env bash
# ────────────────────────────────────────────────────────────────────────────
# export-flags.sh — Export feature flags from Azure App Configuration
#                   and push them to the feature-flags branch.
#
# Usage:
#   ./scripts/export-flags.sh <app-config-name> <resource-group> [label]
#
# The output file is named after the App Configuration instance:
#   <app-config-name>.json
# ────────────────────────────────────────────────────────────────────────────
set -euo pipefail

readonly RED='\033[0;31m'
readonly GREEN='\033[0;32m'
readonly CYAN='\033[0;36m'
readonly BOLD='\033[1m'
readonly NC='\033[0m'

APP_CONFIG_NAME="${1:?Usage: $0 <app-config-name> <resource-group> [label]}"
RESOURCE_GROUP="${2:?Usage: $0 <app-config-name> <resource-group> [label]}"
LABEL="${3:-}"

OUTPUT_FILE="${APP_CONFIG_NAME}.json"

echo -e "${BOLD}Exporting feature flags from ${CYAN}${APP_CONFIG_NAME}${NC}"

# ── Fetch flags from App Configuration ────────────────────────────────────
LABEL_ARGS=()
if [[ -n "${LABEL}" ]]; then
  LABEL_ARGS=(--label "${LABEL}")
  echo -e "  Label filter: ${CYAN}${LABEL}${NC}"
fi

RAW_FLAGS=$(az appconfig feature list \
  --name "${APP_CONFIG_NAME}" \
  --resource-group "${RESOURCE_GROUP}" \
  "${LABEL_ARGS[@]}" \
  --output json 2>&1) || {
    echo -e "${RED}ERROR: az appconfig feature list failed${NC}" >&2
    echo "${RAW_FLAGS}" >&2
    exit 1
  }

# ── Transform into our canonical format: [{name, enabled}] ───────────────
# az cli returns objects with "name" and either "enabled" (bool) or
# "state" ("on"/"off") depending on API version. Handle both.
echo "${RAW_FLAGS}" | python3 -c "
import json, sys
flags = json.load(sys.stdin)
out = []
for f in flags:
    name = f.get('name', '')
    if 'enabled' in f:
        enabled = f['enabled']
    elif 'state' in f:
        enabled = f['state'].lower() == 'on'
    else:
        enabled = False
    out.append({'name': name, 'enabled': enabled})
out.sort(key=lambda x: x['name'])
print(json.dumps(out, indent=2))
" > "${OUTPUT_FILE}"

FLAG_COUNT=$(python3 -c "import json; print(len(json.load(open('${OUTPUT_FILE}'))))")
echo -e "${GREEN}Exported ${FLAG_COUNT} flags → ${OUTPUT_FILE}${NC}"

# ── Git operations — push to feature-flags branch ─────────────────────────
BRANCH="feature-flags"

git config user.name "github-actions[bot]"
git config user.email "github-actions[bot]@users.noreply.github.com"

# Fetch remote so we know if the branch exists
git fetch origin "${BRANCH}" 2>/dev/null || true

# Switch to or create the branch (orphan if brand new)
if git show-ref --verify --quiet "refs/remotes/origin/${BRANCH}"; then
  git checkout "${BRANCH}"
  git reset --hard "origin/${BRANCH}"
else
  git checkout --orphan "${BRANCH}"
  git rm -rf . 2>/dev/null || true
fi

# Stage only our export file
cp "${GITHUB_WORKSPACE:-..}/${OUTPUT_FILE}" "./${OUTPUT_FILE}" 2>/dev/null \
  || true  # already in place if cwd didn't change

git add "${OUTPUT_FILE}"

if git diff --cached --quiet; then
  echo -e "${CYAN}No changes detected — skipping commit${NC}"
else
  TIMESTAMP=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
  git commit -m "feature-flag export: ${APP_CONFIG_NAME} @ ${TIMESTAMP}"
  git push origin "${BRANCH}"
  echo -e "${GREEN}Pushed ${OUTPUT_FILE} to branch ${BRANCH}${NC}"
fi
