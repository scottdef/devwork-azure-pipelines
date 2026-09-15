#!/usr/bin/env bash
# render-issue-template.sh — regenerate dropdown options in the issue forms
# from models/registry.yml so the form can never drift from the registry.
set -euo pipefail
# shellcheck source=lib.sh
source "$(dirname "$0")/lib.sh"
need yq

M="$(reg '.models | to_entries | map("\(.key) — \(.value.display)") | @json')"
A="$(reg '.accounts | keys | @json')"
R="$(reg '.rai_policies | @json')"
S="$(reg '[.models[].skus[]] | unique | @json')"
export M A R S

for f in .github/ISSUE_TEMPLATE/foundry-model-deploy.yml .github/ISSUE_TEMPLATE/foundry-model-decommission.yml; do
  [[ -f "$f" ]] || continue
  yq -i '
    (.body[] | select(.id=="model")          | .attributes.options) |= env(M) |
    (.body[] | select(.id=="target_account") | .attributes.options) |= env(A) |
    (.body[] | select(.id=="rai_policy")     | .attributes.options) |= env(R) |
    (.body[] | select(.id=="sku")            | .attributes.options) |= env(S)
    | (.. | select(tag=="!!seq")) style=""
  ' "$f"
  log "rendered $f"
done
