#!/usr/bin/env bash
# publish.sh baseline|final|failed - tell the issue what happened.
#
# env: REPO ISSUE ORG_TOKEN [ARTIFACT_URL] [DATA] [HEADLINE] [RUN_URL]
# DATA is report-data.json; its totals are quoted so the comment is useful without the download.
# shellcheck source=lib.sh
. "$(dirname "$0")/lib.sh"
need gh jq
: "${REPO:?}" "${ISSUE:?}"
[[ "$ISSUE" =~ ^[0-9]+$ ]] || die "ISSUE must be a number"

figures() {
	jq -r '.totals as $t | .tiers as $r | .meta as $m |
	  def usd: if . == null then "n/a" else "$\(.)" end;
	  "| | |\n|---|---|\n" +
	  "| Team | `\($m.org)/\($m.team)`, \($t.members) members, \($t.active_users) active |\n" +
	  "| Window | \($m.since) to \($m.until) |\n" +
	  "| Tiers | power \($r.power), heavy \($r.heavy), medium \($r.medium), light \($r.light), inactive \($r.inactive) |\n" +
	  "| Acceptance | \(if $t.acceptance_rate == null then "n/a" else "\($t.acceptance_rate * 100 | floor)%" end) accepted; \($t.agent_users) members use an agent |\n" +
	  "| Cost | **\($t.cost_usd | usd)** for \($t.credits) AI credits |\n" +
	  "| Per merged PR to a protected branch | **\($t.cost_per_merged_pr | usd)** over \($t.merged_prs_protected) |\n" +
	  "| Per production deployment | **\($t.cost_per_prod_deployment | usd)** over \($t.prod_deployments) |"' "${DATA:?DATA unset}"
}
link="${ARTIFACT_URL:+[Download the report]($ARTIFACT_URL) (zip containing one HTML file; open it in any browser, no network needed). It expires in $RETENTION_DAYS days.}"

case "${1:-}" in
baseline)
	unlabel "report:running" # the lock covers collection only; a stalled agent must not block the team
	label "report:collected"
	comment "### Data collected

$(figures)

$link

This copy has every table and chart but no written analysis. An agent is reading the totals now; the annotated copy follows."
	;;
final)
	unlabel "report:collected"
	label "report:done"
	comment "### Report ready

${HEADLINE:+> $HEADLINE

}$(figures)

$link

Per-person usage data: handle it as you would a performance document."
	close_as completed
	;;
failed)
	unlabel "report:running"
	label "report:failed"
	comment "### Collection failed

The data could not be gathered, so no report was produced. ${RUN_URL:+[Run log]($RUN_URL).} Most often this is a missing app permission; the log's last line names the endpoint."
	;;
*) die "usage: publish.sh baseline|final|failed" ;;
esac
