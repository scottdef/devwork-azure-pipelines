#!/usr/bin/env bash
# overdue.sh: daily loan sweep. Warn borrowers whose loans are past due
# (once: the "overdue" label marks a warned issue) and auto-return loans
# past due + grace. The ledger change is pushed before any comment is
# posted, so a comment never describes a state that did not land.
#
# Environment: GH_TOKEN (App token with issues:write on the intake repo),
# INTAKE_REPO, GRACE (default 7d), BASE_URL, GITHUB_OUTPUT, RUNNER_TEMP.
set -euo pipefail

: "${GH_TOKEN:?}" "${INTAKE_REPO:?}"
GRACE="${GRACE:-7d}"
BASE_URL="${BASE_URL:-https://coolgitorg.github.io/issueops-library/}"
out="${GITHUB_OUTPUT:-/dev/stdout}"
work="${RUNNER_TEMP:-/tmp}/overdue-$$"
mkdir -p "$work"

git config user.name "github-actions[bot]"
git config user.email "41898282+github-actions[bot]@users.noreply.github.com"

changed=false
for attempt in 1 2 3 4 5; do
	git fetch --quiet origin main
	git reset --quiet --hard origin/main
	bin/ledger -f ledger/ledger.json -items items overdue -grace "$GRACE" >"$work/report.json"
	if [[ "$(jq '.notices | length' "$work/report.json")" -eq 0 ]]; then
		echo "overdue: nothing overdue"
		break
	fi
	changed=true
	if git diff --quiet -- ledger; then
		break # warnings only; no ledger change to push
	fi
	bin/pagegen -base "$BASE_URL" -site site -items items -ledger ledger/ledger.json >/dev/null
	git add -A ledger site
	git commit --quiet -m "library: overdue sweep ($GRACE grace)"
	if git push --quiet origin HEAD:main; then
		break
	fi
	sleep $((attempt * 2 + RANDOM % 3))
	if [[ $attempt -eq 5 ]]; then
		echo "overdue: giving up after $attempt attempts" >&2
		exit 1
	fi
done
printf 'changed=%s\n' "$changed" >>"$out"

jq -c --arg g "$GRACE" --arg b "$BASE_URL" '.notices[] | . + {grace: $g, base: $b}' "$work/report.json" |
	while IFS= read -r notice; do
		n="$(jq -r '.loan.borrow_issue' <<<"$notice")"
		act="$(jq -r '.action' <<<"$notice")"
		[[ "$n" =~ ^[0-9]+$ ]] || continue
		labels="$(gh issue view "$n" --repo "$INTAKE_REPO" --json labels,state --jq '[.state, (.labels[].name)] | join(" ")')"
		if [[ "$act" == warn && " $labels " == *" overdue "* ]]; then
			continue # already warned
		fi
		printf '%s' "$notice" >"$work/notice.json"
		bin/ledger -f ledger/ledger.json render -t templates/comments/overdue-warning.md.tmpl \
			-d "$work/notice.json" >"$work/comment.md"
		gh issue comment "$n" --repo "$INTAKE_REPO" --body-file "$work/comment.md"
		gh issue edit "$n" --repo "$INTAKE_REPO" --add-label overdue >/dev/null
		if [[ "$act" == return ]]; then
			gh issue close "$n" --repo "$INTAKE_REPO" --reason "not planned" >/dev/null || true
		fi
		sleep 1 # GitHub asks for at least a second between writes
	done
