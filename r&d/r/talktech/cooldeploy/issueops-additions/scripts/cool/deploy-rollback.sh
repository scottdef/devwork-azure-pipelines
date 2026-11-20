#!/usr/bin/env bash
# cool-deploy-rollback: swap the live release with the standby.
# shellcheck source=scripts/cool/lib.sh
. "$(dirname "$0")/lib.sh"

app=$(need app "$RE_APP")
env=$(need environment 'dev|tst|prd')
reason=$(field reason)
[ -n "$reason" ] || die "a rollback needs a reason"
# The reason goes into a commit message: one line, no control characters.
reason=$(tr -d '\r' <<<"$reason" | tr '\n' ' ' | tr -cd '[:print:]' | cut -c1-200)

out=$(cool rollback -app "$app" -env "$env" -reason "$reason")

report "Rolled back \`$app\` in \`$env\`."
report ""
report '```'
report "$out"
report '```'
report ""
report "The release that was live is now the standby. A second rollback request undoes this one."
