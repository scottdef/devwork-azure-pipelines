# shellcheck shell=bash
# lib.sh - shared by all scripts. Source it; do not run it.
set -euo pipefail

HERE=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=../config/budget.env
. "${BUDGET_ENV:-$HERE/../config/budget.env}"

die()  { printf 'error: %s\n' "$*" >&2; exit 1; }
need() { local c; for c in "$@"; do command -v "$c" >/dev/null || die "missing command: $c"; done; }
lc()   { printf '%s' "$1" | tr '[:upper:]' '[:lower:]'; }

# One app, two installations, two tokens.
#   ORG_TOKEN: issues and team membership.   ENT_TOKEN: enterprise billing.
# Neither can do the other's job, so a leak of one is half a leak.
_api() { gh api -H "Accept: application/vnd.github+json" -H "X-GitHub-Api-Version: $API_VERSION" "$@"; }
org()  { GH_TOKEN="${ORG_TOKEN:?ORG_TOKEN unset}" _api "$@"; }
ent()  { GH_TOKEN="${ENT_TOKEN:?ENT_TOKEN unset}" _api "$@"; }

# is_member TEAM USER: true for an active member. The API counts child teams.
is_member() { [ "$(org "/orgs/$ORG/teams/$1/memberships/$2" --jq .state 2>/dev/null)" = active ]; }

# Prints the first allowed team USER belongs to.
in_allowed_team() {
	local t
	for t in $ALLOWED_TEAMS; do
		if is_member "$t" "$1"; then echo "$t"; return 0; fi
	done
	return 1
}

# Issue plumbing. REPO and ISSUE come from the workflow environment.
comment()  { jq -nc --arg b "$1" '{body:$b}' | org -X POST "/repos/$REPO/issues/$ISSUE/comments" --input - >/dev/null; }
label()    { jq -nc --arg l "$1" '{labels:[$l]}' | org -X POST "/repos/$REPO/issues/$ISSUE/labels" --input - >/dev/null; }
unlabel()  { org -X DELETE "/repos/$REPO/issues/$ISSUE/labels/$1" >/dev/null 2>&1 || true; }
close_as() { jq -nc --arg r "$1" '{state:"closed",state_reason:$r}' | org -X PATCH "/repos/$REPO/issues/$ISSUE" --input - >/dev/null; }
has_label() { org "/repos/$REPO/issues/$ISSUE" --jq '.labels[].name' | grep -Fxq "$1"; }

usd()     { echo $(( $1 * CENTS_PER_UNIT / 100 )); }   # units -> whole dollars
credits() { echo $(( $1 * CENTS_PER_UNIT )); }         # units -> AI credits
money()   { printf '$%s (%s AI credits)' "$(usd "$1")" "$(credits "$1")"; }
