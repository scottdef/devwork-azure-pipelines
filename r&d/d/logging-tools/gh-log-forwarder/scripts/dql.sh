#!/usr/bin/env bash
# dql.sh - run a DQL query against Grail and print the records as JSON.
#
#   dql.sh 'fetch logs | filter log.source == "github-actions" | limit 5'
#   dql.sh -f dynatrace/queries/verify-job.dql
#
# Environment:
#   DT_PLATFORM_URL    https://abc12345.apps.dynatrace.com   (note: .apps.)
#   DT_PLATFORM_TOKEN  platform token or OAuth access token with
#                      storage:logs:read, storage:buckets:read
#
# Uses the Grail Query API (POST /platform/storage/query/v1/query:execute,
# then GET query:poll until SUCCEEDED). Ingest uses a different token.
set -euo pipefail
IFS=$'\n\t'

# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

need curl jq
need_env DT_PLATFORM_URL DT_PLATFORM_TOKEN
[[ "$DT_PLATFORM_URL" == https://* ]] || die "DT_PLATFORM_URL must be https"
[[ "$DT_PLATFORM_URL" == *.apps.* ]] || warn "DT_PLATFORM_URL usually is https://<env>.apps.dynatrace.com (Grail lives on .apps)"

if [[ "${1:-}" == "-f" ]]; then
  [[ -r "${2:-}" ]] || die "cannot read ${2:-<missing>}"
  query="$(grep -v '^[[:space:]]*//' "$2")"
else
  query="${1:?usage: dql.sh '<query>' | -f file.dql}"
fi
base="${DT_PLATFORM_URL%/}/platform/storage/query/v1"

# Keep the token out of argv (and thus ps): feed curl its header on stdin.
auth() { printf 'header = "Authorization: Bearer %s"\n' "$DT_PLATFORM_TOKEN"; }

body="$(jq -cn --arg q "$query" '{query: $q, requestTimeoutMilliseconds: 30000, maxResultRecords: 10000}')"
resp="$(auth | curl --silent --show-error --fail-with-body --config - \
  -H 'Content-Type: application/json' -H 'Accept: application/json' \
  --data "$body" "$base/query:execute")"

for _ in $(seq 1 60); do
  state="$(jq -r '.state // empty' <<<"$resp")"
  case "$state" in
    SUCCEEDED) jq '.result.records' <<<"$resp"; exit 0 ;;
    RUNNING | NOT_STARTED)
      token="$(jq -r '.requestToken' <<<"$resp")"
      sleep 1
      resp="$(auth | curl --silent --show-error --fail-with-body --config - -G \
        --data-urlencode "request-token=${token}" --data-urlencode 'request-timeout-milliseconds=5000' \
        "$base/query:poll")"
      ;;
    *) jq . <<<"$resp" >&2; die 1 "query ended in state '${state:-unknown}'" ;;
  esac
done
die 1 "query did not finish in time"
