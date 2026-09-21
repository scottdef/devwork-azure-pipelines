#!/usr/bin/env bash
# parse-issue.sh FIELD - issue-form body on stdin; first non-blank line under the
# first "### ...FIELD..." heading on stdout. Later headings with the same name are
# ignored: the form puts its own fields first, and free text comes last.
set -euo pipefail
[ $# -eq 1 ] || { echo "usage: parse-issue.sh FIELD" >&2; exit 2; }
tr -d '\r' | awk -v f="$(printf '%s' "$1" | tr '[:upper:]' '[:lower:]')" '
	/^### / { if (hit) exit; hit = index(tolower($0), f) > 0; next }
	hit && NF { sub(/^[ \t]+/, ""); sub(/[ \t]+$/, ""); print; found = 1; exit }
	END { exit !found }'
