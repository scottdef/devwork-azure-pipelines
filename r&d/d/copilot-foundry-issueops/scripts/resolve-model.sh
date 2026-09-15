#!/usr/bin/env bash
# resolve-model.sh — validate a request against the registry and emit the
# concrete deployment parameters as KEY=VALUE (and $GITHUB_OUTPUT).
#
# Input: env vars MODEL_KEY TARGET_ACCOUNT [DEPLOYMENT] [SKU] [CAPACITY]
#        [RAI_POLICY] [MODEL_VERSION]
# Flags: --offline   skip `az` (no version resolution / quota check); used by IssueOps intake
set -euo pipefail
# shellcheck source=lib.sh
source "$(dirname "$0")/lib.sh"
need yq jq

offline=false; [[ "${1:-}" == "--offline" ]] && offline=true

: "${MODEL_KEY:?}"; : "${TARGET_ACCOUNT:?}"

reg_has .models   "$MODEL_KEY"      || die "unknown model_key '$MODEL_KEY' (see models/registry.yml)"
reg_has .accounts "$TARGET_ACCOUNT" || die "unknown target_account '$TARGET_ACCOUNT'"

m=".models[\"$MODEL_KEY\"]"; a=".accounts[\"$TARGET_ACCOUNT\"]"

kind="$(reg "$m.kind")"
format="$(reg "$m.format")"
name="$(reg "$m.name")"
sku="${SKU:-$(reg "$m.default_sku")}"
capacity="${CAPACITY:-$(reg "$m.default_capacity")}"
rai="${RAI_POLICY:-Microsoft.DefaultV2}"
version="${MODEL_VERSION:-$(reg "$m.version")}"
deployment="$(slug "${DEPLOYMENT:-$MODEL_KEY}")"

# validations
grep -qx "$sku" <<<"$(reg "$m.skus[]")"       || die "sku '$sku' not allowed for $MODEL_KEY (allowed: $(reg "$m.skus | join(\", \")"))"
grep -qx "$rai" <<<"$(reg ".rai_policies[]")" || die "unknown rai policy '$rai'"
[[ "$capacity" =~ ^[0-9]+$ && "$capacity" -gt 0 ]] || die "capacity must be a positive integer"
[[ -n "$deployment" ]]                   || die "deployment name empty after slugging"
[[ "$(reg "$m.copilot_byok")" == "true" ]] || warn "$MODEL_KEY is not Copilot-BYOK eligible (no tool calling). Deploying anyway; it will not work in Copilot agent/CLI."

rg="$(reg "$a.resource_group")"; account="$(reg "$a.account")"
location="$(reg "$a.location")"; environment="$(reg "$a.environment")"

if ! $offline && [[ "$version" == "latest" && "$kind" != "managed" ]]; then
  need az
  version="$(az cognitiveservices model list -l "$location" \
    --query "[?model.format=='$format' && model.name=='$name'] | sort_by(@, &model.version) | [-1].model.version" -o tsv)"
  [[ -n "$version" ]] || die "$format/$name not in the $location catalog. Run: az cognitiveservices model list -l $location"
fi

out kind            "$kind"
out model_format    "$format"
out model_name      "$name"
out model_version   "$version"
out model_registry  "$(reg "$m.registry // \"\"")"
out accelerator     "$(reg "$m.accelerator // \"\"")"
out sku             "$sku"
out capacity        "$capacity"
out rai_policy      "$rai"
out deployment      "$deployment"
out resource_group  "$rg"
out account         "$account"
out location        "$location"
out environment     "$environment"
out copilot_byok    "$(reg "$m.copilot_byok")"
out display         "$(reg "$m.display")"
