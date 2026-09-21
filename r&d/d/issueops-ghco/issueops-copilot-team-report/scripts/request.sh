#!/usr/bin/env bash
# request.sh - the front door. Decide whether this person may have a report on this
# team, then start the agentic workflow. The agent never sees the issue.
#
# env: REPO ISSUE AUTHOR BODY ORG_TOKEN [RUN_URL]
# A refusal is a successful run: comment, label, close, exit 0.
# shellcheck source=lib.sh
. "$(dirname "$0")/lib.sh"
need gh jq
: "${REPO:?}" "${ISSUE:?}" "${AUTHOR:?}" "${BODY?}"

reject() {
	comment "### Request refused

@$AUTHOR: $1

Nothing was collected."
	label "report:refused"
	close_as not_planned
	echo "refused: $1"
	exit 0
}

team=$("$HERE/parse-issue.sh" "team" <<<"$BODY") || reject "the form could not be read. Use the **Copilot team report** template."
days=$("$HERE/parse-issue.sh" "period" <<<"$BODY") || reject "the form could not be read. Use the **Copilot team report** template."
team=$(lc "${team#@}")            # accept "@CoolGitOrg/payments" and "payments"
team=${team#"$(lc "$ORG")"/}
days=${days%% *}                  # "28 days" -> "28"
valid_slug "$team" || reject "\`team\` must be a team slug: lowercase letters, digits, dots, dashes."
valid_days "$days" || reject "the period must be one of: $ALLOWED_DAYS days."

info=$(api "/orgs/$ORG/teams/$team" 2>/dev/null) || reject "there is no team \`$team\` in $ORG, or the app cannot see it."
name=$(jq -r .name <<<"$info")
size=$(jq -r .members_count <<<"$info")

# A maintainer of the team, or a member of the org-wide admins team.
role=$(team_role "$team" "$AUTHOR")
via="maintainer of \`$team\`"
if [ "$role" != maintainer ]; then
	[ -n "$REPORT_ADMINS_TEAM" ] && [ -n "$(team_role "$REPORT_ADMINS_TEAM" "$AUTHOR")" ] ||
		reject "only a maintainer of \`$team\`${REPORT_ADMINS_TEAM:+ or a member of \`$REPORT_ADMINS_TEAM\`} can request this report. Your role in the team: \`${role:-none}\`."
	via="member of \`$REPORT_ADMINS_TEAM\`"
fi

[ "$size" -ge "$MIN_TEAM_SIZE" ] ||
	reject "\`$team\` has $size members. Reports need at least $MIN_TEAM_SIZE, so that a team figure is not one person's figure."

# One run per team at a time. The label carries the team so the search is exact.
busy=$(api "/repos/$REPO/issues?state=open&labels=report:running,team:$team&per_page=100" \
	--jq "[.[] | select(.number != $ISSUE)] | length")
[ "$busy" -eq 0 ] || reject "a report for \`$team\` is already running."

jq -nc --arg ref "${REF:-main}" --arg t "$team" --arg d "$days" --arg i "$ISSUE" --arg by "$AUTHOR" \
	'{ref: $ref, inputs: {team: $t, days: $d, issue: $i, requested_by: $by}}' |
	api -X POST "/repos/$REPO/actions/workflows/$REPORT_WORKFLOW/dispatches" --input - >/dev/null ||
	die "could not start $REPORT_WORKFLOW"

label "report:running"
label "team:$team"
comment "### Collecting

@$AUTHOR: report on **$name** (\`$team\`, $size members) for the last $days days, requested as $via.

Data is gathered by script; an agent then reads the totals and writes the analysis. The HTML report arrives here as a download. ${RUN_URL:+[This run]($RUN_URL) only opened the door; the work is in the \`copilot-team-report\` run it started.}"
echo "dispatched: team=$team days=$days"
