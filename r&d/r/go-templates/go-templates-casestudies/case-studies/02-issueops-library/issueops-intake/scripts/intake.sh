#!/usr/bin/env bash
# intake.sh: parse, validate and (optionally) dispatch one intake issue.
#
#   scripts/intake.sh check    KIND   # parse + validate; comment and label on failure
#   scripts/intake.sh dispatch KIND   # send to the library; comment and label on success
#
# KIND is request, borrow or return. Inputs come only from the environment,
# never from workflow interpolation: BODY, ISSUE (number), AUTHOR (login),
# GH_TOKEN, GITHUB_REPOSITORY, GITHUB_OUTPUT, LIBRARY_REPO.
# Exit status is 0 for "handled" (including an invalid request, which gets
# a comment), non-zero only for operator problems.
set -euo pipefail

cmd="${1:?usage: intake.sh check|dispatch KIND}"
kind="${2:?usage: intake.sh check|dispatch KIND}"
case "$kind" in
request) event=library-publish ;;
borrow) event=library-borrow ;;
return) event=library-return ;;
*) echo "intake: unknown kind $kind" >&2; exit 2 ;;
esac
: "${ISSUE:?}" "${GITHUB_REPOSITORY:?}"
work="${RUNNER_TEMP:-/tmp}/intake-$ISSUE"
mkdir -p "$work"
out="${GITHUB_OUTPUT:-/dev/null}"

case "$cmd" in
check)
	: "${BODY?}"
	if ! printf '%s' "$BODY" | bin/issueparse -form ".github/ISSUE_TEMPLATE/$kind.yml" >"$work/req.json" 2>"$work/parse.err"; then
		# Not even parseable: report as a single validation error.
		jq -n --rawfile e "$work/parse.err" '{ok:false, errors:[{field:"-", rule:"parse", message:($e|rtrimstr("\n"))}]}' >"$work/report.json"
	elif bin/validate -schema "schema/$kind.json" "$work/req.json" >"$work/report.json"; then
		gh issue edit "$ISSUE" --repo "$GITHUB_REPOSITORY" --remove-label invalid >/dev/null 2>&1 || true
		echo "valid=true" >>"$out"
		exit 0
	fi
	bin/pagegen -comment templates/comment-invalid.md.tmpl -d "$work/report.json" >"$work/comment.md"
	gh issue comment "$ISSUE" --repo "$GITHUB_REPOSITORY" --body-file "$work/comment.md"
	# Labels are housekeeping: the comment above is what the requester needs.
	gh issue edit "$ISSUE" --repo "$GITHUB_REPOSITORY" --add-label invalid --remove-label accepted >/dev/null || true
	echo "valid=false" >>"$out"
	;;
dispatch)
	: "${AUTHOR:?}" "${GH_TOKEN:?}" "${LIBRARY_REPO:?}"
	src=()
	if [[ "$kind" == request ]]; then
		src=(-source "$(jq -r '."source-id"' "$work/req.json")")
	fi
	bin/dispatch -repo "$LIBRARY_REPO" -event "$event" -issue "$ISSUE" -requester "$AUTHOR" \
		"${src[@]}" "$work/req.json" >"$work/receipt.json"
	bin/pagegen -comment templates/comment-dispatched.md.tmpl -d "$work/receipt.json" >"$work/comment.md"
	gh issue comment "$ISSUE" --repo "$GITHUB_REPOSITORY" --body-file "$work/comment.md"
	gh issue edit "$ISSUE" --repo "$GITHUB_REPOSITORY" --add-label accepted --remove-label invalid >/dev/null || true
	;;
*)
	echo "intake: unknown command $cmd" >&2
	exit 2
	;;
esac
