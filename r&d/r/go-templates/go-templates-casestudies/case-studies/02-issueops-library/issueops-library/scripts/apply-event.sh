#!/usr/bin/env bash
# apply-event.sh: apply one repository_dispatch event to main.
#
# Optimistic concurrency: fetch main, apply the event, commit, push; if the
# push loses a race, throw the work away and apply again on the new main.
# Every step is a pure function of (main, event), so re-applying is
# correct, and no event waits in a concurrency queue where GitHub could
# cancel it.
#
# Environment: EVENT (toJSON(github.event)), INTAKE_REPO, BASE_URL,
# GITHUB_OUTPUT, RUNNER_TEMP. Outputs: ok, refused, changed, action, issue,
# item, applied_at, close, comment (multi-line).
# Exit 0 when the event was applied or refused by a library rule.
set -euo pipefail

: "${EVENT:?}" "${INTAKE_REPO:?}"
BASE_URL="${BASE_URL:-https://coolgitorg.github.io/issueops-library/}"
out="${GITHUB_OUTPUT:-/dev/stdout}"
work="${RUNNER_TEMP:-/tmp}/apply-$$"
mkdir -p "$work"
printf '%s' "$EVENT" >"$work/event.json"

# describe validates the whole event and prints only checked fields.
bin/pagegen -event "$work/event.json" -describe >"$work/describe"
get() { sed -n "s/^$1=//p" "$work/describe"; }
action="$(get action)" issue="$(get issue)" repo="$(get repo)"
requester="$(get requester)" item="$(get item)" days="$(get days)"
if [[ "$repo" != "$INTAKE_REPO" ]]; then
	echo "apply: event names repo $repo, expected $INTAKE_REPO" >&2
	exit 1
fi
reason="$(jq -r '.client_payload.reason // ""' "$work/event.json")"

emit() { printf '%s=%s\n' "$1" "$2" >>"$out"; }
emit_block() {
	local d
	d="EOF_$(head -c 12 /dev/urandom | od -An -tx1 | tr -d ' \n')"
	{ printf '%s<<%s\n' "$1" "$d"; cat "$2"; printf '%s\n' "$d"; } >>"$out"
}
emit action "$action"
emit issue "$issue"
emit item "$item"

git config user.name "github-actions[bot]"
git config user.email "41898282+github-actions[bot]@users.noreply.github.com"

changed=false
for attempt in 1 2 3 4 5; do
	git fetch --quiet origin main
	git reset --quiet --hard origin/main
	applied_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
	rc=0
	case "$action" in
	library-borrow)
		bin/ledger -f ledger/ledger.json -items items borrow -item "$item" -by "$requester" \
			-days "$days" -issue "$issue" -reason "$reason" >"$work/result.json" || rc=$?
		;;
	library-return)
		bin/ledger -f ledger/ledger.json -items items return -item "$item" -by "$requester" \
			>"$work/result.json" || rc=$?
		;;
	esac
	if [[ $rc -eq 0 ]]; then
		bin/pagegen -event "$work/event.json" -base "$BASE_URL" -site site -items items \
			-ledger ledger/ledger.json >"$work/page.json" || rc=$?
		if [[ "$action" == library-publish || $rc -ne 0 ]]; then
			cp "$work/page.json" "$work/result.json"
		fi
	fi
	if [[ $rc -eq 3 ]]; then
		# Refused by a library rule: nothing to commit, tell the requester.
		bin/tpl -d "$work/result.json" -t templates/comments/refused.md.tmpl >"$work/comment.md"
		emit ok false
		emit refused true
		emit changed false
		emit_block comment "$work/comment.md"
		exit 0
	fi
	if [[ $rc -ne 0 ]]; then
		exit "$rc"
	fi
	git add -A items ledger site
	if git diff --cached --quiet; then
		break
	fi
	git commit --quiet -m "library: ${action#library-} $item" -m "From $INTAKE_REPO#$issue by $requester."
	if git push --quiet origin HEAD:main; then
		changed=true
		break
	fi
	echo "apply: push lost a race (attempt $attempt); re-applying on new main" >&2
	sleep $((attempt * 2 + RANDOM % 3))
	if [[ $attempt -eq 5 ]]; then
		echo "apply: giving up after $attempt attempts" >&2
		exit 1
	fi
done

# Issues to close once the page is live: the request itself for publish
# and return; the borrow thread when its loan ends.
close=""
case "$action" in
library-publish)
	close="$issue $(jq -r '.closed_borrow_issue // empty' "$work/result.json")"
	tmpl=published
	;;
library-borrow) tmpl=borrowed ;;
library-return)
	close="$issue $(jq -r '.loan.borrow_issue // empty' "$work/result.json")"
	tmpl=returned
	;;
esac
bin/tpl -d "$work/result.json" -t "templates/comments/$tmpl.md.tmpl" >"$work/comment.md"
emit ok true
emit refused false
emit changed "$changed"
emit applied_at "$applied_at"
emit close "$(echo "$close" | xargs)"
emit_block comment "$work/comment.md"
