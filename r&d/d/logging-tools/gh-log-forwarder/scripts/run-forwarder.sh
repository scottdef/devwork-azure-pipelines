#!/usr/bin/env bash
# run-forwarder.sh - one shard of the forwarder, as the workflows run it.
#
#   run-forwarder.sh sweep  <repos|*>          scheduled sweep
#   run-forwarder.sh event  <owner/name> <run_id> [attempt]
#
# Environment: GH_TOKEN, DT_URL, DT_LOGS_TOKEN (unless GHLOG_DRY_RUN=true),
# GHLOG_STATE_DIR, optional GHLOG_LOOKBACK, GHLOG_CONFIG.
#
# Exit status follows ghlog2dt, except 3 (partial success: some records
# refused or too old) is reported as a warning and becomes 0, so one odd
# batch does not page anyone; the self-monitoring record still says WARN.
set -euo pipefail
IFS=$'\n\t'

# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

need ghlog2dt
need_env GH_TOKEN GHLOG_STATE_DIR
mode="${1:-}"
args=(run --log-format json --summary "${GHLOG_SUMMARY_FILE:-summary.json}")

dry="${GHLOG_DRY_RUN:-false}"
case "$dry" in
  true) args+=(--dry-run --records-out "${GHLOG_RECORDS_FILE:-records.ndjson}") ;;
  false) need_env DT_URL DT_LOGS_TOKEN ;;
  *) die "GHLOG_DRY_RUN must be true or false" ;;
esac

case "$mode" in
  sweep)
    targets="${2:-}"
    [[ -n "$targets" ]] || die "sweep needs a comma-separated repo list or '*'"
    if [[ "$targets" == "*" ]]; then
      args+=(--all)
    else
      IFS=',' read -r -a repos <<<"$targets"
      for r in "${repos[@]}"; do
        r="${r//[[:space:]]/}"
        [[ -z "$r" ]] && continue
        valid_repo "$r" || die "invalid repository in target list: '$r'"
        args+=(--repo "$r")
      done
    fi
    ;;
  event)
    repo="${2:-}" run_id="${3:-}" attempt="${4:-0}"
    valid_repo "$repo" || die "event: invalid repository '$repo'"
    [[ "$run_id" =~ ^[0-9]{1,20}$ ]] || die "event: invalid run id '$run_id'"
    [[ "$attempt" =~ ^[0-9]{1,4}$ ]] || die "event: invalid attempt '$attempt'"
    args+=(--repo "$repo" --run-id "$run_id" --run-attempt "$attempt")
    ;;
  *) die "usage: run-forwarder.sh sweep <repos|*> | event <repo> <run_id> [attempt]" ;;
esac

mkdir -p "$GHLOG_STATE_DIR"
rc=0
ghlog2dt "${args[@]}" || rc=$?
case "$rc" in
  0) ;;
  3) warn "ghlog2dt: partial success (some records refused or too old); see summary"; rc=0 ;;
  4) die 4 "ghlog2dt: authentication failed (GitHub App token or DT_LOGS_TOKEN / logs.ingest scope)" ;;
  5) die 5 "ghlog2dt: rate limit exhausted or Dynatrace budget (402); state preserved" ;;
  6) die 6 "ghlog2dt: checkpoint I/O failure" ;;
  *) die "$rc" "ghlog2dt exited $rc" ;;
esac
exit "$rc"
