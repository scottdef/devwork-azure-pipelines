#!/usr/bin/env bash
# loadtest.sh: the intake site is one static page plus flags.json; the
# interesting load is on the library site. Delegate there when it sits next
# to this repo, else say what to run.
set -euo pipefail
here="$(cd "$(dirname "$0")/.." && pwd)"
lib="$here/../issueops-library"
if [[ -x "$lib/scripts/loadtest.sh" ]]; then
	BASE="${BASE:-https://coolgitorg.github.io/issueops-intake/}" PATHS="${PATHS:- flags.json static/intake.js}" exec "$lib/scripts/loadtest.sh" "$@"
fi
echo "loadtest: run issueops-library/scripts/loadtest.sh with BASE=https://coolgitorg.github.io/issueops-intake/" >&2
exit 2
