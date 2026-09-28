#!/usr/bin/env bash
# preflight.sh: read-only checks before the first deploy.
# Exit status is the number of failed required checks.
set -euo pipefail

RG=${AZURE_RESOURCE_GROUP:-rg-coolgit-copilot}
WS=${AML_WORKSPACE:-mlw-coolgit-foundry}
ACCOUNT=${FOUNDRY_ACCOUNT:-ais-coolgit-copilot-prod}
PROJECT=${FOUNDRY_PROJECT:-proj-coolgit-agents}
fails=0
ok() { printf 'ok    %s\n' "$*"; }
warn() { printf 'warn  %s\n' "$*"; }
bad() { printf 'FAIL  %s\n' "$*"; fails=$((fails + 1)); }

for t in az gh python3 curl; do
  if command -v "$t" >/dev/null; then ok "$t"; else bad "$t not found"; fi
done
if command -v copilot >/dev/null; then ok "copilot"; else warn "copilot not found (example 3 harness: npm install -g @github/copilot)"; fi

if python3 shared/tools/ftmcfg.py examples >/dev/null; then
  for ex in $(python3 shared/tools/ftmcfg.py examples); do
    if FTM_SET_HF_REVISION=0000000000000000000000000000000000000000 python3 shared/tools/ftmcfg.py json "$ex" >/dev/null; then
      ok "config $ex"
    else
      bad "config $ex"
    fi
  done
fi

if az account show -o none 2>/dev/null; then
  ok "az signed in to $(az account show --query name -o tsv)"
  if az ml workspace show -n "$WS" -g "$RG" -o none 2>/dev/null; then
    ok "workspace $WS"
    for c in cpu-fetch gpu-a100 gpu-h100x2; do
      if az ml compute show -n "$c" -g "$RG" -w "$WS" -o none 2>/dev/null; then
        ok "cluster $c"
      else
        warn "cluster $c missing (needed only by examples that use it)"
      fi
    done
  else
    bad "workspace $WS in $RG (scripts/bootstrap-azure.sh)"
  fi
  if az cognitiveservices account show -n "$ACCOUNT" -g "$RG" -o none 2>/dev/null; then
    ok "Foundry account $ACCOUNT"
  else
    bad "Foundry account $ACCOUNT in $RG"
  fi
  sub=$(az account show --query id -o tsv)
  if az rest --method get -o none --url "https://management.azure.com/subscriptions/$sub/resourceGroups/$RG/providers/Microsoft.CognitiveServices/accounts/$ACCOUNT/projects/$PROJECT?api-version=2025-04-01-preview" 2>/dev/null; then
    ok "Foundry project $PROJECT"
  else
    bad "Foundry project $PROJECT"
  fi
else
  bad "az is not signed in (az login)"
fi

if curl -fsS --max-time 15 https://huggingface.co/api/models/google/gemma-4-E4B-it -o /dev/null; then
  ok "huggingface.co reachable"
else
  warn "huggingface.co not reachable from here (workflows resolve revisions from the runner)"
fi

echo
if ((fails == 0)); then echo "ready"; else echo "$fails check(s) failed"; fi
exit "$fails"
