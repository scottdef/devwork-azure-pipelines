#!/usr/bin/env bash
# wait-live.sh: wait until the live Pages site was generated at or after
# APPLIED_AT (RFC 3339, seconds, UTC). Whichever run deployed it, a site
# generated after our commit contains our change, because every build
# renders the whole of main.
#
# Environment: BASE_URL, APPLIED_AT, TRIES (default 40), PAUSE (default 15s).
# Exit 0 when live, 1 on timeout (the caller still comments, with a caveat).
set -euo pipefail
: "${BASE_URL:?}" "${APPLIED_AT:?}"
tries="${TRIES:-40}"
pause="${PAUSE:-15}"
for ((i = 1; i <= tries; i++)); do
	gen="$(curl -fsS -H 'Cache-Control: no-cache' "${BASE_URL%/}/library.json?t=$i$RANDOM" 2>/dev/null | jq -r '.generated // empty' || true)"
	# Same fixed-width format on both sides, so text order is time order.
	if [[ -n "$gen" && ! "$gen" < "$APPLIED_AT" ]]; then
		echo "live: site generated $gen >= applied $APPLIED_AT (try $i)"
		exit 0
	fi
	sleep "$pause"
done
echo "wait-live: site not updated after $tries tries" >&2
exit 1
