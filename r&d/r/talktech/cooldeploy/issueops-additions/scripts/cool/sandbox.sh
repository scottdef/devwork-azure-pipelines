#!/usr/bin/env bash
# The sandbox rehearsal: deploy a simulator scenario to sbx-eus2-appsim-aks-001.
#
# Not a request type. The platform team starts it by hand, to try a chart
# version, a mesh upgrade or the release machinery on apps that cannot hurt
# anyone. Scenarios live in sandbox/<name>/cool.json.
#
# Fields: scenario (steady, flaky, noisy), tag (image tag of sim/appsim),
# weight (optional: start a canary at this percent instead of replacing).
# shellcheck source=scripts/cool/lib.sh
. "$(dirname "$0")/lib.sh"

REGISTRY=${COOL_REGISTRY:-prdeus2coolopsacr001}
root=$(cd "$(dirname "$0")/../.." && pwd)

scenario=$(need scenario 'steady|flaky|noisy')
tag=$(need tag '[A-Za-z0-9._-]{1,64}')
weight=$(optional weight '[0-9]{1,3}')
manifest=$root/sandbox/$scenario/cool.json
[ -f "$manifest" ] || die "no scenario at sandbox/$scenario/cool.json"
app=sim/appsim-$scenario

if [ "$DRY_RUN" = 1 ]; then
	digest=sha256:$(printf '0%.0s' $(seq 1 64))
else
	digest=$(az acr repository show --name "$REGISTRY" --image "sim/appsim:$tag" --query digest --output tsv)
fi

if [ "$DRY_RUN" != 1 ] && ! coolctl status -app "$app" >/dev/null 2>&1; then
	cool register -env sbx -app "$app" -repo "${COOL_SIM_REPO:-CoolEngOrg/coolctl}" >&2
fi
# A release is named after a commit. The simulator's image tag is not one,
# so the release takes its name from the image digest instead.
out=$(cool deploy -env sbx -f "$manifest" -digest "$digest" -sha "${digest:7:40}")
if [ -n "$weight" ] && [ "$weight" -lt 100 ]; then
	out+=$'\n'$(cool weights -app "$app" -env sbx -weight "$weight")
fi

report "Sandbox: deployed scenario \`$scenario\` (\`sim/appsim:$tag\`) as \`$app\`."
report ""
report '```'
report "$out"
report '```'
