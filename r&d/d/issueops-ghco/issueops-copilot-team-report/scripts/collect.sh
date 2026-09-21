#!/usr/bin/env bash
# collect.sh TEAM DAYS OUT_DIR - fetch everything the report needs, as files.
#
# env: ORG_TOKEN [REQUESTED_BY] [ISSUE]
#
# Writes OUT_DIR/raw/{meta,members,seats,repos,prs,deployments}.json and
# {usage,repo_copilot}.ndjson. The org-wide usage file is filtered to team members
# as it streams; the unfiltered file never touches disk, so it cannot leak into an artifact.
#
# Required sources fail the run. Optional ones (seats, repository report, PRs,
# deployments) degrade to empty with a note that ends up printed in the report.
# shellcheck source=lib.sh
. "$(dirname "$0")/lib.sh"
need gh jq curl date
[ $# -eq 3 ] || die "usage: collect.sh TEAM DAYS OUT_DIR"
team=$1 days=$2 raw=$3/raw
valid_slug "$team" || die "bad team slug"
valid_days "$days" || die "bad day count"
mkdir -p "$raw"

until_=$(date -u -d "${NOW:-now} -1 day" +%F)          # usage reports lag; yesterday is the last full day
since=$(date -u -d "$until_ -$((days - 1)) days" +%F)
notes=()
note() { notes+=("$1"); echo "note: $1" >&2; }

# Signed report URLs -> one JSON value per line, whether the file is NDJSON or an array.
fetch_reports() { local u; while read -r u; do [ -n "$u" ] && { curl -fsSL --retry 3 "$u"; echo; }; done | jq -c 'if type == "array" then .[] else . end'; }

# ---- team and members (required)
info=$(api "/orgs/$ORG/teams/$team") || die "team $team not found"
api --paginate "/orgs/$ORG/teams/$team/members?per_page=100" --jq '.[].login' |
	tr '[:upper:]' '[:lower:]' | sort -u | jq -Rnc '[inputs]' >"$raw/members.json"
n=$(jq length "$raw/members.json")
[ "$n" -ge "$MIN_TEAM_SIZE" ] || die "team has $n members; minimum is $MIN_TEAM_SIZE"
note "Membership is the team as it stands today, child teams included. Someone who joined last week brings their whole window with them."

# ---- Copilot usage, per user per day (required)
rep=$(api "/orgs/$ORG/copilot/metrics/reports/users-28-day/latest") ||
	die "usage report unavailable: enable the Copilot usage metrics policy and grant the app Organization Copilot metrics: read"
rstart=$(jq -r '.report_start_day // ""' <<<"$rep")
rend=$(jq -r '.report_end_day // ""' <<<"$rep")
[ -z "$rend" ] || [ "$rend" \> "$since" ] || die "latest usage report ends $rend, before the window starts"
jq -r '.download_links[]' <<<"$rep" | fetch_reports |
	jq -c --slurpfile m "$raw/members.json" --arg s "$since" --arg u "$until_" \
		'select(((.user_login // "") | ascii_downcase) as $l | $m[0] | index($l))
		 | select((.day // $s) >= $s and (.day // $u) <= $u)' >"$raw/usage.ndjson"
echo "usage rows: $(wc -l <"$raw/usage.ndjson")" >&2

# ---- seats (optional)
if ! api --paginate "/orgs/$ORG/copilot/billing/seats?per_page=100" \
	--jq '.seats[] | {login: (.assignee.login | ascii_downcase), plan_type, last_activity_at}' 2>/dev/null |
	jq -sc --slurpfile m "$raw/members.json" '[.[] | select(.login as $l | $m[0] | index($l))]' >"$raw/seats.json"; then
	echo '[]' >"$raw/seats.json"
	note "Seat assignments could not be read; the Seat column is unknown."
fi

# ---- team repositories (optional from here down)
api --paginate "/orgs/$ORG/teams/$team/repos?per_page=100" --jq '.[] | select(.archived | not) | .name' 2>/dev/null |
	head -n "$MAX_REPOS" | jq -Rnc '[inputs]' >"$raw/repos.json" || echo '[]' >"$raw/repos.json"
nrepos=$(jq length "$raw/repos.json")
[ "$nrepos" -lt "$MAX_REPOS" ] || note "The team reaches $MAX_REPOS or more repositories; only the first $MAX_REPOS were scanned."

# A base branch is protected if classic protection or any ruleset applies to it.
declare -A PROT
protected() {
	local k="$1/$2" p
	if [ -z "${PROT[$k]:-}" ]; then
		p=$(api "/repos/$ORG/$1/branches/$2" --jq .protected 2>/dev/null || echo false)
		[ "$p" = true ] || p=$(api "/repos/$ORG/$1/rules/branches/$2" --jq 'length > 0' 2>/dev/null || echo false)
		PROT[$k]=$p
	fi
	[ "${PROT[$k]}" = true ]
}

# Newest-first pages until a page ends before the window. $1 path, $2 date field, $3 jq row filter.
pages() {
	local p=1 page q='?'
	case $1 in *\?*) q='&' ;; esac
	while [ "$p" -le 10 ]; do
		page=$(api "$1${q}per_page=100&page=$p" 2>/dev/null) || return 1
		[ "$(jq length <<<"$page")" -gt 0 ] || break
		jq -c --arg s "$since" ".[] | $3" <<<"$page"
		[ "$(jq -r --arg s "$since" "(.[-1].$2 // \"\") < \$s" <<<"$page")" = true ] && break
		p=$((p + 1))
	done
}

: >"$raw/prs.ndjson"
: >"$raw/deployments.ndjson"
failed=0
while read -r r; do
	[ -n "$r" ] || continue
	pages "/repos/$ORG/$r/pulls?state=closed&sort=updated&direction=desc" updated_at \
		'select(.merged_at != null and .merged_at >= $s) | {number, author: .user.login, base: .base.ref, merged_at}' |
		while read -r pr; do
			if protected "$r" "$(jq -r .base <<<"$pr")"; then p=true; else p=false; fi
			jq -c --arg r "$r" --argjson p "$p" '. + {repo: $r, protected: $p}' <<<"$pr"
		done >>"$raw/prs.ndjson" || failed=$((failed + 1))

	pages "/repos/$ORG/$r/deployments" created_at \
		'select(.created_at >= $s) | {id, environment, created_at}' |
		jq -c --arg re "$PROD_ENV_REGEX" 'select(.environment | test($re; "i"))' |
		while read -r d; do
			ok=$(api "/repos/$ORG/$r/deployments/$(jq -r .id <<<"$d")/statuses?per_page=30" --jq 'any(.[]; .state == "success")' 2>/dev/null || echo false)
			jq -c --arg r "$r" --argjson ok "$ok" '. + {repo: $r, success: $ok}' <<<"$d"
		done >>"$raw/deployments.ndjson" || failed=$((failed + 1))
done < <(jq -r '.[]' "$raw/repos.json")
jq -sc . "$raw/prs.ndjson" >"$raw/prs.json"
jq -sc . "$raw/deployments.ndjson" >"$raw/deployments.json"
rm -f "$raw/prs.ndjson" "$raw/deployments.ndjson"
[ "$failed" -eq 0 ] || note "$failed repository scans failed; pull request and deployment counts are a floor."

# ---- GitHub's own per-repository Copilot PR report, one call per day (optional)
: >"$raw/repo_copilot.ndjson"
for ((i = 0; i < days; i++)); do
	d=$(date -u -d "$since +$i days" +%F)
	rep=$(api "/orgs/$ORG/copilot/metrics/reports/repos-1-day?day=$d" 2>/dev/null) || {
		[ "$i" -gt 0 ] || { note "GitHub's per-repository Copilot report was unavailable; Copilot review and cloud-agent columns are empty."; break; }
		continue
	}
	jq -r '.download_links[]?' <<<"$rep" | fetch_reports |
		jq -c --slurpfile rs "$raw/repos.json" 'select(.repo_name as $n | $rs[0] | index($n))' >>"$raw/repo_copilot.ndjson" || true
done

printf '%s\n' "${notes[@]}" | jq -Rnc --arg org "$ORG" --arg team "$team" --arg name "$(jq -r .name <<<"$info")" \
	--argjson days "$days" --arg since "$since" --arg until "$until_" --arg by "${REQUESTED_BY:-}" --arg issue "${ISSUE:-}" \
	--arg rs "$rstart" --arg re "$rend" \
	'{org: $org, team: $team, team_name: $name, days: $days, since: $since, until: $until,
	  report_start_day: $rs, report_end_day: $re, requested_by: $by, issue: $issue,
	  notes: [inputs | select(. != "")]}' >"$raw/meta.json"

echo "collected: members=$n repos=$nrepos prs=$(jq length "$raw/prs.json") deployments=$(jq length "$raw/deployments.json")" >&2
