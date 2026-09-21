# shellcheck shell=bash
# lib.sh - shared by the shell scripts. Source it; do not run it.
set -euo pipefail

HERE=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=../config/report.env
. "${REPORT_ENV:-$HERE/../config/report.env}"

die()  { printf 'error: %s\n' "$*" >&2; exit 1; }
need() { local c; for c in "$@"; do command -v "$c" >/dev/null || die "missing command: $c"; done; }
lc()   { printf '%s' "$1" | tr '[:upper:]' '[:lower:]'; }

# One token, from the org installation of the app. GH_TOKEN is what gh reads.
api() { GH_TOKEN="${ORG_TOKEN:?ORG_TOKEN unset}" gh api -H "Accept: application/vnd.github+json" -H "X-GitHub-Api-Version: $API_VERSION" "$@"; }

# Slugs and day counts are the only request fields that travel. Both are checked at every hop.
valid_slug() { [[ "$1" =~ ^[a-z0-9][a-z0-9._-]{0,99}$ ]]; }
valid_days() { local d; for d in $ALLOWED_DAYS; do [ "$1" = "$d" ] && return 0; done; return 1; }

team_role()  { api "/orgs/$ORG/teams/$1/memberships/$2" --jq 'select(.state == "active") | .role' 2>/dev/null || true; }

# Issue plumbing. REPO and ISSUE come from the workflow environment.
comment()  { jq -nc --arg b "$1" '{body:$b}' | api -X POST "/repos/$REPO/issues/$ISSUE/comments" --input - >/dev/null; }
label()    { jq -nc --arg l "$1" '{labels:[$l]}' | api -X POST "/repos/$REPO/issues/$ISSUE/labels" --input - >/dev/null; }
unlabel()  { api -X DELETE "/repos/$REPO/issues/$ISSUE/labels/$1" >/dev/null 2>&1 || true; }
close_as() { jq -nc --arg r "$1" '{state:"closed",state_reason:$r}' | api -X PATCH "/repos/$REPO/issues/$ISSUE" --input - >/dev/null; }
