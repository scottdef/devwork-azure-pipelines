#!/usr/bin/env bash
# decide.sh - act on "/approve" or "/deny <reason>" left on a pending request.
#
# env: REPO ISSUE COMMENTER COMMENT ORG_TOKEN ENT_TOKEN [RUN_URL]
#
# Nothing from the webhook payload is trusted beyond who spoke and what they said.
# Labels, author, and the request itself are re-read from the API.
# shellcheck source=lib.sh
. "$(dirname "$0")/lib.sh"
need gh jq
: "${REPO:?}" "${ISSUE:?}" "${COMMENTER:?}" "${COMMENT?}"

read -r verb reason <<<"$(head -n1 <<<"$COMMENT" | tr -d '\r')" || true
case "$verb" in
/approve | /deny) ;;
*) echo "ignored: not a command"; exit 0 ;;
esac

# The run is serialised per issue; a second /approve arrives here after the label is gone.
has_label "budget:pending" || { echo "ignored: issue is not pending"; exit 0; }

if ! is_member "$APPROVER_TEAM" "$COMMENTER"; then
	comment "@$COMMENTER: only members of @$ORG/$APPROVER_TEAM can decide this request. Ignored."
	exit 0
fi

author=$(org "/repos/$REPO/issues/$ISSUE" --jq .user.login)
if [ "$(lc "$COMMENTER")" = "$(lc "$author")" ]; then
	comment "@$COMMENTER: you cannot decide your own request. Another member of @$ORG/$APPROVER_TEAM must."
	exit 0
fi

# Request of record: the last marker in a comment written by the app itself.
marker=$(org --paginate "/repos/$REPO/issues/$ISSUE/comments?per_page=100" \
	--jq ".[] | select(.user.type == \"Bot\" and .user.login == \"$APP_BOT_LOGIN\") | .body" |
	grep -o 'issueops-budget:{[^}]*}' | tail -n1 | sed 's/^issueops-budget://') || true
[ -n "$marker" ] || die "no request marker from $APP_BOT_LOGIN on issue $ISSUE"
user=$(jq -er .user <<<"$marker")
inc=$(jq -er .increase <<<"$marker")
[ "$(lc "$user")" = "$(lc "$author")" ] || die "marker user $user is not issue author $author"

if [ "$verb" = /deny ]; then
	unlabel "budget:pending"
	label "budget:denied"
	comment "### Request denied

@$user: denied by @$COMMENTER. ${reason:+Reason: $reason}

Nothing was changed."
	close_as not_planned
	exit 0
fi

# Membership is checked again: people change teams while requests sit in queues.
in_allowed_team "$user" >/dev/null || {
	unlabel "budget:pending"
	label "budget:rejected"
	comment "### Not applied

@$user is no longer in an allowed team. Nothing was changed."
	close_as not_planned
	exit 0
}

# The budget is recomputed now, from what the API says now, not from what it said at request time.
set +e
res=$("$HERE/budget.sh" apply "$user" "$inc" 2>/tmp/apply.err)
rc=$?
set -e
if [ $rc -ne 0 ]; then
	cat /tmp/apply.err >&2
	unlabel "budget:pending"
	label "budget:failed"
	comment "### Approved, but not applied

@$COMMENTER approved; the change failed:

\`\`\`
$(tail -n5 /tmp/apply.err)
\`\`\`

Nothing was changed. ${RUN_URL:+[Run log]($RUN_URL).} Fix the cause, restore \`budget:pending\`, and \`/approve\` again."
	exit 1
fi

cur=$(jq -r .current <<<"$res")
new=$(jq -r .new <<<"$res")
id=$(jq -r .budget_id <<<"$res")
reset=$(jq -r .reset_date <<<"$res")
exp=$(jq -r '.expires_at // "none"' <<<"$res")
used=$(jq -r 'if .consumed_usd == null then "n/a" else "$\(.consumed_usd)" end' <<<"$res")

report="### Approved and applied

@$user: your Copilot AI-credit budget has been raised. Approved by @$COMMENTER.

| | |
|---|---|
| Status | **success** — written and read back from the billing API |
| Previous monthly budget | $(money "$cur") |
| Increase | +$inc |
| **New monthly budget** | **$(money "$new")** |
| Used so far this cycle | $used |
| Usage resets | **$reset 00:00 UTC** — the budget stays, the meter returns to zero |
| Override expires | $exp |
| Budget ID | \`$id\` |
${RUN_URL:+| Run | [log]($RUN_URL) |}

Credits used against this budget are shown on the Copilot usage page in your GitHub settings."

unlabel "budget:pending"
label "budget:approved"
comment "$report"
close_as completed
[ -z "${GITHUB_STEP_SUMMARY:-}" ] || printf '%s\n\n```json\n%s\n```\n' "$report" "$(jq . <<<"$res")" >>"$GITHUB_STEP_SUMMARY"
echo "applied: $res"
