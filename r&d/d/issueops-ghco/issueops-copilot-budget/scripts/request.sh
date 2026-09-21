#!/usr/bin/env bash
# request.sh - validate a newly opened budget request and hand it to the approvers.
#
# env: REPO ISSUE AUTHOR BODY ORG_TOKEN ENT_TOKEN [RUN_URL]
#
# A rejection is a successful run: comment, label, close, exit 0.
# Only faults (API down, bad token) exit non-zero.
# shellcheck source=lib.sh
. "$(dirname "$0")/lib.sh"
need gh jq
: "${REPO:?}" "${ISSUE:?}" "${AUTHOR:?}" "${BODY?}"

reject() {
	comment "### Request rejected

@$AUTHOR: $1

Nothing was changed. Open a new request once this is resolved."
	label "budget:rejected"
	close_as not_planned
	echo "rejected: $1"
	exit 0
}

inc=$("$HERE/parse-issue.sh" <<<"$BODY" 2>/dev/null) ||
	reject "the form could not be read. Use the **Copilot budget increase** template and leave its headings alone."

team=$(in_allowed_team "$AUTHOR") ||
	reject "you are not an active member of a team allowed to make this request (\`$ALLOWED_TEAMS\`)."

# One request in flight per user. Two approved at once would both add to the same
# starting figure, and the second write would silently swallow the first.
others=$(org "/repos/$REPO/issues?state=open&labels=budget:pending&creator=$AUTHOR&per_page=100" \
	--jq "[.[] | select(.number != $ISSUE)] | length")
[ "$others" -eq 0 ] || reject "you already have a request awaiting approval."

set +e
plan=$("$HERE/budget.sh" plan "$AUTHOR" "$inc" 2>/tmp/plan.err)
rc=$?
set -e
case $rc in
0) ;;
3) reject "$(sed 's/^refused: //' /tmp/plan.err)." ;;
*)
	cat /tmp/plan.err >&2
	comment "### Could not validate

@$AUTHOR: the billing API did not answer. This is not your fault. ${RUN_URL:+[Run log]($RUN_URL).}"
	label "budget:failed"
	exit 1
	;;
esac

cur=$(jq -r .current <<<"$plan")
new=$(jq -r .new <<<"$plan")
src=$(jq -r .source <<<"$plan")
reset=$("$HERE/budget.sh" reset-date)

# The marker is the request of record. The approval step reads it from this bot
# comment and never from the issue body, which the author can edit at any time.
marker=$(jq -nc --arg u "$AUTHOR" --argjson i "$inc" '{user: $u, increase: $i}')

comment "### Awaiting approval

| | |
|---|---|
| Requester | @$AUTHOR (team \`$team\`) |
| Current monthly budget | $(money "$cur") — source: \`$src\` |
| Requested increase | +$inc |
| **Proposed monthly budget** | **$(money "$new")** |
| Ceiling | $(money "$MAX") |
| Usage next resets | $reset 00:00 UTC |

@$ORG/$APPROVER_TEAM: reply \`/approve\` or \`/deny <reason>\`. The requester cannot approve their own request.

<!-- issueops-budget:$marker -->"
label "budget:pending"
echo "pending: $marker"
