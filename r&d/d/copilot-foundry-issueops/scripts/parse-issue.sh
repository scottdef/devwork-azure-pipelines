#!/usr/bin/env bash
# parse-issue.sh — GitHub issue-form markdown (stdin) -> JSON (stdout).
#
# Issue forms render as:   ### <Label>\n\n<value>\n\n
# Checkboxes render as:    - [x] <option>
# Empty fields render as:  _No response_
#
# Field labels are looked up by the exact text in the issue template.
# Usage: parse-issue.sh < body.md
set -euo pipefail
# shellcheck source=lib.sh
source "$(dirname "$0")/lib.sh"
need jq awk

body="$(cat)"

field() {  # field <Label> — first non-empty line after the heading
  awk -v f="$1" '
    $0 == "### " f { found=1; next }
    found && /^### /  { exit }
    found && NF && $0 != "_No response_" { sub(/^[ \t]+/,""); sub(/[ \t]+$/,""); print; exit }
  ' <<<"$body"
}
checks() { # checks <Label> — checked options, one per line
  awk -v f="$1" '
    $0 == "### " f { found=1; next }
    found && /^### / { exit }
    found && /^- \[[xX]\] / { sub(/^- \[[xX]\] /,""); print }
  ' <<<"$body"
}
para() {  # para <Label> — whole block (textarea)
  awk -v f="$1" '
    $0 == "### " f { found=1; next }
    found && /^### / { exit }
    found { print }
  ' <<<"$body" | sed '/./,$!d' | sed -e :a -e '/^\n*$/{$d;N;ba' -e '}'
}

model_raw="$(field "Model")"
model_key="${model_raw%% —*}"            # "qwen3-32b — Qwen3 32B ..." -> "qwen3-32b"
model_key="${model_key%% *}"

jq -n \
  --arg model_key      "$model_key" \
  --arg deployment     "$(field "Deployment name")" \
  --arg target_account "$(field "Target account")" \
  --arg sku            "$(field "SKU")" \
  --arg capacity       "$(field "Capacity")" \
  --arg rai_policy     "$(field "Content filter (RAI) policy")" \
  --arg model_version  "$(field "Model version")" \
  --arg justification  "$(para  "Justification")" \
  --argjson options    "$(checks "Options" | jq -R . | jq -s .)" \
  '{
    model_key:      $model_key,
    deployment:     ($deployment | if .=="" then $model_key else . end),
    target_account: $target_account,
    sku:            $sku,
    capacity:       ($capacity | if .=="" then null else tonumber end),
    rai_policy:     $rai_policy,
    model_version:  ($model_version | if .=="" then "latest" else . end),
    justification:  $justification,
    run_validation: ($options | index("Run endpoint validation after deploy") != null),
    notify:         ($options | index("Notify #platform-copilot on completion") != null),
    dry_run:        ($options | index("Dry run (plan only, no deploy)") != null)
  }'
