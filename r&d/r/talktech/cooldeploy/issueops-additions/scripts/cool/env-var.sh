#!/usr/bin/env bash
# cool-env-var: set or remove plain settings of an app in one environment.
#
# These are overrides: they win over the values in the app's cool.json and
# stay in place across deploys, until a request removes them.
# shellcheck source=scripts/cool/lib.sh
. "$(dirname "$0")/lib.sh"

app=$(need app "$RE_APP")
env=$(need environment 'dev|tst|prd')

args=(-app "$app" -env "$env")
changes=()

# A secret typed into an issue is a leaked secret. Refuse names that look
# like one, and say where secrets go.
secretish='(PASSWORD|PASSWD|SECRET|TOKEN|PRIVATE|CREDENTIAL|API_?KEY|CONNECTION_?STRING)'

while IFS= read -r line; do
	line=${line%$'\r'}
	[ -n "${line// /}" ] || continue
	key=${line%%=*} value=${line#*=}
	if [ "$key" = "$line" ]; then
		die "\"$line\" is not KEY=VALUE"
	fi
	[[ $key =~ ^$RE_KEY$ ]] || die "\"$key\" is not a variable name: use capital letters, digits and _"
	case $key in PORT | COOL_*) die "$key is set by the platform" ;; esac
	if [[ $key =~ $secretish ]]; then
		die "$key looks like a secret. Secrets do not go through issues: put it in the team Key Vault and list its name under \"secrets\" in cool.json"
	fi
	if [ "${#value}" -gt 1024 ]; then
		die "the value of $key is longer than 1024 characters"
	fi
	args+=(-set "$key=$value")
	changes+=("set \`$key\`")
done <<<"$(field set)"

while IFS= read -r key; do
	key=${key%$'\r'}
	key=${key// /}
	[ -n "$key" ] || continue
	[[ $key =~ ^$RE_KEY$ ]] || die "\"$key\" is not a variable name"
	args+=(-unset "$key")
	changes+=("removed \`$key\`")
done <<<"$(field unset)"

[ "${#changes[@]}" -gt 0 ] || die "the request sets nothing and removes nothing"

out=$(cool config "${args[@]}")

report "Changed the settings of \`$app\` in \`$env\`:"
report ""
for c in "${changes[@]}"; do report "- $c"; done
report ""
report '```'
report "$out"
report '```'
report ""
report "Pods restart with the new settings as Argo CD syncs the change."
