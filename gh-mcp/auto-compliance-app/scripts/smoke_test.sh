#!/usr/bin/env bash
# Smoke test for the Compliance Evidence Center.
#
# Builds the site, serves it locally, and checks that the key routes return the
# expected HTTP status codes. Exits non-zero if any check fails.
#
# Usage: scripts/smoke_test.sh [PORT]   (default port: 8000)

set -euo pipefail

PORT="${1:-8000}"
BASE="http://localhost:${PORT}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"

cd "$ROOT"

# Build the site fresh.
python3 scripts/build_site.py --out _site

# Serve _site/ in the background, ensuring it is stopped on exit.
( cd _site && python3 -m http.server "$PORT" --bind 0.0.0.0 ) >/dev/null 2>&1 &
SERVER_PID=$!
trap 'kill "$SERVER_PID" 2>/dev/null || true' EXIT

# Wait for the server to accept connections.
for _ in $(seq 1 20); do
  curl -s -o /dev/null "$BASE/" && break
  sleep 0.25
done

# route -> expected HTTP status
declare -a CHECKS=(
  "/|200"
  "/controls/|200"
  "/controls/multi-factor-authentication/|200"
  "/controls/multi-factor-authentication/evidence/01-all-accounts-2fa.html|200"
  "/controls/multi-factor-authentication.zip|200"
  "/nope.html|404"
)

fail=0
for check in "${CHECKS[@]}"; do
  path="${check%%|*}"
  want="${check##*|}"
  got="$(curl -s -o /dev/null -w '%{http_code}' "${BASE}${path}")"
  if [ "$got" = "$want" ]; then
    printf 'PASS  %-65s HTTP %s\n' "$path" "$got"
  else
    printf 'FAIL  %-65s HTTP %s (expected %s)\n' "$path" "$got" "$want"
    fail=1
  fi
done

if [ "$fail" -ne 0 ]; then
  echo "Smoke test FAILED"
  exit 1
fi
echo "Smoke test PASSED"
