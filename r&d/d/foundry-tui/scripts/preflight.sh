#!/usr/bin/env bash
# preflight.sh: check that this machine can run foundry-tui.
# Reads only. Exit status is the number of failed required checks.
set -euo pipefail

RESOURCE_GROUP="${FOUNDRY_RESOURCE_GROUP:-rg-coolgit-copilot}"
ACCOUNT="${FOUNDRY_ACCOUNT:-ais-coolgit-copilot-prod}"
REPO="${FOUNDRY_REPO:-CoolGitOrg/foundry-ops}"
ROLE_ID="c883944f-8b7b-4483-af10-35834be79c4a"
fails=0

ok()   { printf 'ok    %s\n' "$*"; }
warn() { printf 'warn  %s\n' "$*"; }
bad()  { printf 'FAIL  %s\n' "$*"; fails=$((fails + 1)); }

need() { # tool, why
  if command -v "$1" >/dev/null; then ok "$1: $(command -v "$1")"; else bad "$1 not found ($2)"; fi
}
want() {
  if command -v "$1" >/dev/null; then ok "$1: $(command -v "$1")"; else warn "$1 not found ($2)"; fi
}

need az "every read from Foundry"
need gh "every write, and the GitHub token"
want go "only to build from source"
want copilot "only for the Copilot CLI probe: npm install -g @github/copilot"
want actionlint "only for make lint"
want shellcheck "only for make lint"

if command -v az >/dev/null; then
  if user=$(az account show --query user.name -o tsv 2>/dev/null); then
    ok "az signed in as $user"
    if scope=$(az cognitiveservices account show -g "$RESOURCE_GROUP" -n "$ACCOUNT" --query id -o tsv 2>/dev/null); then
      ok "account $ACCOUNT is readable"
      n=$(az role assignment list --scope "$scope" --include-inherited --include-groups \
            --assignee "$user" --query "[?ends_with(roleDefinitionId, '$ROLE_ID')] | length(@)" -o tsv 2>/dev/null || echo 0)
      if [[ "$n" -gt 0 ]]; then
        ok "Foundry Owner is assigned to you at or above the account"
      else
        warn "no Foundry Owner assignment found for $user; a wider role also works, a narrower one will skip sections"
      fi
      if az account get-access-token --resource https://cognitiveservices.azure.com --query expiresOn -o tsv >/dev/null 2>&1; then
        ok "data-plane token can be issued (keyless probes)"
      else
        bad "cannot get a token for https://cognitiveservices.azure.com"
      fi
    else
      bad "cannot read $ACCOUNT in $RESOURCE_GROUP"
    fi
  else
    bad "az is not signed in: az login"
  fi
fi

if command -v gh >/dev/null; then
  if gh auth status >/dev/null 2>&1; then
    ok "gh is signed in"
    for wf in foundry-deploy.yml foundry-report.yml foundry-model-test.yml; do
      if gh api "repos/$REPO/actions/workflows/$wf" --silent 2>/dev/null; then
        ok "$REPO has $wf"
      else
        bad "$REPO lacks $wf: foundry-tui scaffold, commit, push"
      fi
    done
  else
    bad "gh is not signed in: gh auth login"
  fi
fi

echo
if [[ "$fails" -eq 0 ]]; then echo "ready"; else echo "$fails required check(s) failed"; fi
exit "$fails"
