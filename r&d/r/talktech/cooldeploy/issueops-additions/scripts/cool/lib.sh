# shellcheck shell=bash
# shellcheck disable=SC2034  # the patterns below are used by the scripts that source this file
# Shared by the scripts that carry out CoolDeploy requests. Source it.
#
# A script is given the request as JSON in $COOL_FIELDS: the fields of the
# issue form, as the gate verified them. It writes a Markdown report to
# $COOL_REPORT for the finish step to post on the issue.
#
# Every field is text someone typed into an issue. Nothing here trusts it:
# each value is checked against a pattern before it reaches a command, and
# values reach commands only as quoted arguments.
set -euo pipefail

: "${COOL_FIELDS:?COOL_FIELDS is not set}"
COOL_REPORT=${COOL_REPORT:-/dev/stdout}
DRY_RUN=${DRY_RUN:-0}

die() {
	echo "::error::$*" >&2
	printf 'The request could not be carried out: %s\n' "$*" >>"$COOL_REPORT"
	exit 1
}

report() { printf '%s\n' "$*" >>"$COOL_REPORT"; }

# field NAME prints a field, trimmed, or nothing.
field() { jq -rM --arg k "$1" '.[$k] // "" | gsub("^\\s+|\\s+$"; "")' <<<"$COOL_FIELDS"; }

# need NAME PATTERN prints a field that must match an extended regular
# expression in full, and fails the request if it does not.
need() {
	local v
	v=$(field "$1")
	[[ $v =~ ^($2)$ ]] || die "field \"$1\" is \"$v\", which is not what the form allows"
	printf '%s' "$v"
}

# optional NAME PATTERN is need for a field that may be empty.
optional() {
	local v
	v=$(field "$1")
	[ -z "$v" ] || [[ $v =~ ^($2)$ ]] || die "field \"$1\" is \"$v\", which is not what the form allows"
	printf '%s' "$v"
}

RE_TEAM='[a-z][a-z0-9]{1,7}'
RE_SLUG='[a-z][a-z0-9-]{0,18}[a-z0-9]'
RE_APP="$RE_TEAM/$RE_SLUG"
RE_RELEASE='r[0-9a-f]{7}'
RE_KEY='[A-Z][A-Z0-9_]{0,63}'
RE_HOST='([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,}'

# run prints a command and runs it, unless this is a dry run.
run() {
	printf '+ %s\n' "$*" >&2
	if [ "$DRY_RUN" = 1 ]; then
		return 0
	fi
	"$@"
}

# cool ARGS... runs coolctl. If it fails, the request fails with coolctl's
# own last line as the reason, so the issue says what cool-api said.
cool() {
	local err
	err=$(mktemp)
	if ! run coolctl "$@" 2>"$err"; then
		cat "$err" >&2
		die "$(tail -1 "$err" | sed 's/^coolctl: //')"
	fi
	cat "$err" >&2
}

# cluster_of ENV prints the cluster an environment's apps run on.
cluster_of() {
	case $1 in
	sbx) echo sbx-eus2-appsim-aks-001 ;;
	dev | tst | prd) echo "$1-eus2-appdev-aks-001" ;;
	*) die "no cluster for environment \"$1\"" ;;
	esac
}
