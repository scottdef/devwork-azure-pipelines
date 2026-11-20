#!/usr/bin/env bash
# Check what can be checked without Azure.
#
#   - the configuration is JSON and follows the naming rules
#   - no stack tries to manage API Management
#   - the Terraform is formatted, which also proves it parses
#   - every stack validates, when the providers can be downloaded
#
#   scripts/check.sh
#   TF=tofu scripts/check.sh
set -euo pipefail

cd "$(dirname "$0")/.."
TF=${TF:-terraform}
fail=0
bad() { echo "FAIL  $*" >&2; fail=1; }
ok() { echo "ok    $*"; }

jq -eM . config/*.json stacks/70-addons/addons.json >/dev/null || bad "a configuration file is not JSON"

# Names: <env>-<region>-<workload>-aks-<nnn>, and an environment that agrees.
region=$(jq -rM .region config/platform.json)
while IFS=$'\t' read -r name env workload; do
	[ "$name" = "$env-$region-$workload-aks-001" ] || bad "cluster $name should be named $env-$region-$workload-aks-001"
done < <(jq -rM 'to_entries[] | [.key, .value.environment, .value.workload] | @tsv' config/clusters.json)
[ "$(jq -rM '[.[] | select(.role == "hub")] | length' config/clusters.json)" = 1 ] || bad "there must be exactly one hub cluster"
jq -eM 'keys | all(test("^[a-z][a-z0-9]{1,7}$"))' config/teams.json >/dev/null || bad "a team slug is not 2 to 8 lower-case letters and digits"
jq -eM 'all(.addon | IN("redis", "postgres", "storage", "ai")) and all(.size | IN("small", "medium", "large")) and all(.environment | IN("dev", "tst", "prd"))' \
	stacks/70-addons/addons.json >/dev/null || bad "addons.json has an add-on, size or environment the stack does not know"
jq -eM '(map("\(.environment)/\(.team)/\(.app)/\(.addon)") | unique | length) == length' stacks/70-addons/addons.json >/dev/null ||
	bad "addons.json lists the same add-on twice"
[ "$fail" = 0 ] && ok "configuration"

# API Management belongs to cool-apim.
if grep -rnE --include='*.tf' 'azurerm_api_management|"Microsoft\.ApiManagement/' .; then
	bad "API Management is managed by cool-apim, with scripts. Remove it from Terraform."
else
	ok "no API Management in Terraform"
fi

if command -v "$TF" >/dev/null; then
	if "$TF" fmt -check -recursive . >/dev/null; then ok "terraform fmt"; else bad "run: $TF fmt -recursive"; fi
	for stack in stacks/*/; do
		if "$TF" -chdir="$stack" init -backend=false -input=false >/dev/null 2>&1; then
			if "$TF" -chdir="$stack" validate >/dev/null; then ok "validate $stack"; else bad "validate $stack"; fi
		else
			echo "skip  validate $stack: could not download providers"
		fi
	done
else
	echo "skip  terraform not found: set TF=tofu or install Terraform"
fi

[ "$fail" = 0 ] || exit 1
echo "cool-infra: checks passed"
