#!/usr/bin/env bash
# Case study 02 preflight: runs both repos' own preflights (intake, library).
set -euo pipefail
cd "$(dirname "$0")/.."
for r in issueops-intake issueops-library; do
	echo "== $r: preflight"
	(cd "$r" && ./scripts/preflight.sh)
done
echo "case study 02: preflight ok"
