#!/usr/bin/env bash
# cool-app-retire: remove an app from every environment.
# shellcheck source=scripts/cool/lib.sh
. "$(dirname "$0")/lib.sh"

app=$(need app "$RE_APP")
confirm=$(field confirm)
[ "$confirm" = "$app" ] || die "the confirmation \"$confirm\" does not match the app \"$app\""

out=$(cool retire -app "$app" -confirm "$confirm")

team=${app%%/*} name=${app##*/}
report "Retired \`$app\`."
report ""
report '```'
report "$out"
report '```'
report ""
report "Argo CD removes its namespaces in every environment, previews included, as it syncs."
report ""
report "Kept on purpose, to be removed by hand when nobody needs them:"
report ""
report "- the source repository and its history;"
report "- the images under \`$team/$name\` in the registry;"
report "- the managed identities \`<env>-eus2-$team-$name-id-001\`;"
report "- its secrets in the team Key Vaults, named \`$name-*\`;"
report "- any add-ons listed for it in \`cool-infra/stacks/70-addons/addons.json\`;"
report "- its API in \`cool-apim/apis/$team/$name\`, if it published one."
