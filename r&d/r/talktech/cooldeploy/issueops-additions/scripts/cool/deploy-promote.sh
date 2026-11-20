#!/usr/bin/env bash
# cool-deploy-promote: copy the live release of one environment to the next.
# shellcheck source=scripts/cool/lib.sh
. "$(dirname "$0")/lib.sh"

app=$(need app "$RE_APP")
from=$(need from 'dev|tst')
to=$(need environment 'tst|prd')
release=$(optional release "$RE_RELEASE")

case "$from>$to" in
"dev>tst" | "tst>prd") ;;
*) die "a release moves from dev to tst, or from tst to prd; not from $from to $to" ;;
esac

args=(-app "$app" -from "$from" -to "$to" -wait "${COOL_WAIT:-10m}")
if [ -n "$release" ]; then
	args+=(-release "$release")
fi
out=$(cool promote "${args[@]}")

report "Promoted \`$app\` from \`$from\` to \`$to\`."
report ""
report '```'
report "$out"
report '```'
