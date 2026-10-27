#!/usr/bin/env bash
# phase0-spike.sh - settle the behaviours Dynatrace does not document,
# against a NON-PRODUCTION environment, using the ghlog2dt binary itself.
#
#   1. plain JSON ingest with the classic token         (baseline)
#   2. Content-Encoding: gzip                           (--gzip)
#   3. X-Dynatrace-Attr request-level attributes        (--header-attrs path)
#   4. a record 25h old                                 (expect 400 all-stale)
#   5. Bearer platform token on the .live URL           (if DT_PLATFORM_INGEST_TOKEN set)
#
# Each probe carries spike.id; query them afterwards with
#   scripts/dql.sh 'fetch logs | filter isNotNull(spike.id) | sort timestamp desc'
# and compare builtin:billing.log.ingest.usage before/after for (3).
set -euo pipefail
IFS=$'\n\t'

# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

need ghlog2dt date
need_env DT_URL DT_LOGS_TOKEN
run_id="spike-$(date -u +%Y%m%dT%H%M%SZ)"
out="$(mktemp -d)"
trap 'rm -rf "$out"' EXIT
now="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
old="$(date -u -d '25 hours ago' +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date -u -v-25H +%Y-%m-%dT%H:%M:%SZ)"

probe() {
  local name="$1"; shift
  local rec="$1"; shift
  local rc=0
  printf '%s\n' "$rec" | ghlog2dt ship --self-log=false --log-format text "$@" >"$out/${name}.json" || rc=$?
  printf '%-14s exit=%d  %s\n' "$name" "$rc" "$(cat "$out/${name}.json")"
}

rec() { printf '{"timestamp":"%s","content":"ghlog2dt phase0 %s","severity":"INFO","log.source":"ghlog2dt.spike","spike.id":"%s","spike.probe":"%s"}' "$1" "$2" "$run_id" "$2"; }

probe baseline "$(rec "$now" baseline)"
probe gzip "$(rec "$now" gzip)" --gzip
probe stale "$(rec "$old" stale)"
if [[ -n "${DT_PLATFORM_INGEST_TOKEN:-}" ]]; then
  DT_LOGS_TOKEN="$DT_PLATFORM_INGEST_TOKEN" DT_AUTH_SCHEME=Bearer probe bearer "$(rec "$now" bearer)"
fi
note ""
note "Header attributes: run 'ghlog2dt run --repo <fixture> --header-attrs' and check that"
note "records carry vcs.repository.name although the body omits it."
note "Expected: baseline exit=0; stale exit=3 with stale=1; gzip exit=0 if supported (else 4xx)."
note "Query: scripts/dql.sh 'fetch logs | filter spike.id == \"$run_id\"'"
