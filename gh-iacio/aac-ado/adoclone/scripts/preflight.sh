#!/usr/bin/env bash
# Preflight checks for adoclone:
#   - git (2.31+, for GIT_CONFIG_* auth), jq and curl are installed
#   - GET _apis/connectionData succeeds (the PAT is valid)
#   - the source project exists and the target project doesn't
#     (an existing target is allowed when a checkpoint is present, so reruns resume)
#   - the source uses Git (TFVC is a warning: work items can still be copied)
set -euo pipefail
: "${ADO_PAT:?ADO_PAT is required}" "${SRC:?SRC is required}" "${TGT:?TGT is required}"
ORG="${ORG:-${ADO_ORG:-CoolADO}}"
STATE="${STATE:-state.json}"

fail=0
ok()   { echo "ok    $*"; }
warn() { echo "warn  $*"; }
bad()  { echo "FAIL  $*"; fail=1; }

for t in git jq curl; do
  if command -v "$t" >/dev/null 2>&1; then ok "$t installed"; else bad "$t not installed"; fi
done
[ "$fail" -eq 0 ] || { echo "preflight failed"; exit 1; }
gv=$(git --version | awk '{print $3}')
if printf '2.31\n%s\n' "$gv" | sort -V -C; then ok "git $gv"; else warn "git $gv is older than 2.31; repo and wiki mirroring need 2.31+"; fi
command -v git-lfs >/dev/null 2>&1 && ok "git-lfs installed" || warn "git-lfs not installed; LFS objects won't be copied"

AUTH=$(printf ':%s' "$ADO_PAT" | base64 -w0)
code() { curl -sS -o /dev/null -w '%{http_code}' -H "Authorization: Basic $AUTH" "$1" || true; }
get()  { curl -fsS -H "Authorization: Basic $AUTH" "$1"; }
enc()  { jq -rn --arg v "$1" '$v|@uri'; }

c=$(code "https://dev.azure.com/$ORG/_apis/connectionData")
if [ "$c" = 200 ]; then ok "PAT accepted by $ORG"; else bad "connectionData returned HTTP $c (check ADO_PAT and ORG)"; fi

src_json=$(get "https://dev.azure.com/$ORG/_apis/projects/$(enc "$SRC")?includeCapabilities=true&api-version=7.1" 2>/dev/null || true)
if [ -n "$src_json" ] && jq -e '.id' >/dev/null 2>&1 <<<"$src_json"; then
  ok "source project '$SRC' exists"
  sct=$(jq -r '.capabilities.versioncontrol.sourceControlType // "unknown"' <<<"$src_json")
  if [ "$sct" = Git ]; then ok "source uses Git"
  else warn "source control type is '$sct'; repos need the TFVC route described in the guide"; fi
else
  bad "source project '$SRC' not found"
fi

tc=$(code "https://dev.azure.com/$ORG/_apis/projects/$(enc "$TGT")?api-version=7.1")
if [ "$tc" = 404 ]; then
  ok "target project '$TGT' does not exist yet"
elif [ "$tc" = 200 ] && { [ -s "$STATE" ] || [ -s "$STATE.journal" ]; }; then
  warn "target '$TGT' already exists and a checkpoint is present: treating this run as a resume"
elif [ "$tc" = 200 ]; then
  bad "target '$TGT' already exists and there is no checkpoint ($STATE) to resume from"
else
  bad "target lookup returned HTTP $tc"
fi

[ "$fail" -eq 0 ] || { echo "preflight failed"; exit 1; }
echo "preflight passed"
