#!/usr/bin/env bash
# cool-domain: add or remove a custom host name of an app.
# shellcheck source=scripts/cool/lib.sh
. "$(dirname "$0")/lib.sh"

app=$(need app "$RE_APP")
env=$(need environment 'dev|tst|prd')
action=$(need action 'add|remove')
domain=$(need domain "$RE_HOST")

case $domain in
*.coolapps.example) die "$domain is in the platform's own zone; apps get those names without asking" ;;
esac

out=$(cool config -app "$app" -env "$env" "-$action-domain" "$domain")

report "Custom domain \`$domain\`: $action for \`$app\` in \`$env\`."
report ""
report '```'
report "$out"
report '```'
if [ "$action" = add ]; then
	report ""
	report "The app's routes now answer for the host. Two things remain, and the platform team does both from this issue:"
	report ""
	report "1. add \`$domain\` to \`extraHosts\` in \`cool-gitops/clusters/$(cluster_of "$env")/platform-values.json\`, so the external gateway accepts it;"
	report "2. add the host and its certificate in \`cool-apim\`, which is where public traffic enters."
	report ""
	report "Point the domain's DNS at the gateway only after both are done."
fi
