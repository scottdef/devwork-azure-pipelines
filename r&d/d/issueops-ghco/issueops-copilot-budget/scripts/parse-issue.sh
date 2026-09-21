#!/usr/bin/env bash
# parse-issue.sh - issue-form body on stdin, requested increase (integer units) on stdout.
#
# Issue forms render as "### <label>\n\n<value>". Take the first non-blank line
# under the heading that mentions "increase". Accept "+50", "50", "+50 ($50 ...)".
# Anything else is an error; the body is user-controlled and gets no benefit of doubt.
set -euo pipefail
v=$(tr -d '\r' | awk '/^### /{h=tolower($0); next} h ~ /increase/ && NF {print; exit}')
if [[ "$v" =~ ^\+?([0-9]{1,6})([^0-9]|$) ]]; then
	echo "${BASH_REMATCH[1]}"
else
	echo "error: no increase found in issue body" >&2
	exit 1
fi
