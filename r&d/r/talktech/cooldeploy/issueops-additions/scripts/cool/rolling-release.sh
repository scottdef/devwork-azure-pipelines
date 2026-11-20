#!/usr/bin/env bash
# cool-rolling-release: move traffic to a new release in stages.
#
# At each stage the new release gets a larger share of traffic. The script
# waits for the change to be live, lets traffic settle, and asks Prometheus
# whether the release is inside its error budget. If it is not, the release
# is taken out of traffic and left on standby, and the request fails.
#
# Reads from the environment:
#   COOL_PROM_URL    query endpoint of the Azure Monitor workspace
#   COOL_PROM_TOKEN  bearer token for it; fetched with az when unset
#   COOL_GATE=off    skip the checks (the sandbox rehearsal only)
# shellcheck source=scripts/cool/lib.sh
. "$(dirname "$0")/lib.sh"

app=$(need app "$RE_APP")
from=$(need from 'dev|tst')
env=$(need environment 'tst|prd')
stages=$(need stages '[0-9]{1,3}( *, *[0-9]{1,3}){0,9}')
settle=$(need settle '[0-9]{1,2}')
budget=$(need error_budget '[0-9]{1,2}(\.[0-9]{1,2})?')
release=$(optional release "$RE_RELEASE")
[ "$from" != "$env" ] || die "the release must come from another environment"

IFS=', ' read -r -a steps <<<"$stages"
last=0
for w in "${steps[@]}"; do
	if [ "$w" -le "$last" ] || [ "$w" -gt 100 ]; then
		die "stages must rise and end at 100; got \"$stages\""
	fi
	last=$w
done
[ "$last" = 100 ] || die "the last stage must be 100; got \"$stages\""

team=${app%%/*} name=${app##*/}
max_rate=$(jq -nM --arg b "$budget" '($b | tonumber) / 100')

if [ "${COOL_GATE:-on}" != off ] && [ -z "${COOL_PROM_TOKEN:-}" ] && [ "$DRY_RUN" != 1 ]; then
	COOL_PROM_TOKEN=$(az account get-access-token --resource https://prometheus.monitor.azure.com --query accessToken --output tsv)
	export COOL_PROM_TOKEN
fi

report "Rolling release of \`$app\` in \`$env\`, from \`$from\`."
report ""
report "| Stage | Result |"
report "| --- | --- |"

first=1 id=$release
outputs=$(mktemp)
for w in "${steps[@]}"; do
	args=(-app "$app" -env "$env" -weight "$w")
	if [ "$first" = 1 ]; then
		args+=(-from "$from")
		if [ -n "$release" ]; then
			args+=(-release "$release")
		fi
		first=0
	fi
	GITHUB_OUTPUT=$outputs cool weights "${args[@]}" >&2
	if [ -z "$id" ] && [ -s "$outputs" ]; then
		id=$(sed -n 's/^release=//p' "$outputs" | tail -1)
	fi
	if [ "${COOL_WAIT:-10m}" != 0 ]; then
		cool wait -app "$app" -env "$env" -timeout "${COOL_WAIT:-10m}" >&2
	fi

	# At 100% there is nothing left to protect by waiting: the previous
	# release is on standby, and a rollback request brings it back.
	if [ "$w" = 100 ]; then
		report "| $w% | live |"
		break
	fi
	if [ "${COOL_GATE:-on}" = off ]; then
		report "| $w% | not checked |"
		continue
	fi
	run sleep "$((settle * 60))"
	verdict=$(mktemp)
	if run coolctl gate -namespace "$team-$name" -workload "$name-$id" \
		-window "${settle}m" -max-error-rate "$max_rate" >"$verdict"; then
		report "| $w% | $(jq -rM '.reason // "inside the budget"' "$verdict" 2>/dev/null || echo "inside the budget") |"
	else
		reason=$(jq -rM '.reason // "the check failed"' "$verdict" 2>/dev/null || echo "the check failed")
		report "| $w% | **stopped:** $reason |"
		cool weights -app "$app" -env "$env" -weight 0 >&2
		report ""
		report "Release \`$id\` was taken out of traffic and left on standby. The previous release serves everything."
		die "release $id left its error budget at $w%: $reason"
	fi
done

report ""
report "Release \`$id\` now takes all traffic in \`$env\`. The previous release is on standby."
