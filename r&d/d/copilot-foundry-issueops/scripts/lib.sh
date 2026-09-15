#!/usr/bin/env bash
# lib.sh — shared helpers. `source` it; never execute it.
# Every script speaks KEY=VALUE on stdout and diagnostics on stderr.
set -euo pipefail

: "${REGISTRY:=models/registry.yml}"

log()  { printf '%s %s\n' "$(date -u +%FT%TZ)" "$*" >&2; }
warn() { printf '::warning::%s\n' "$*" >&2; }
die()  { printf '::error::%s\n' "$*" >&2; exit 1; }

need() {
  local c
  for c in "$@"; do command -v "$c" >/dev/null 2>&1 || die "missing tool: $c"; done
}

# out KEY VALUE — emit to stdout, and to $GITHUB_OUTPUT when running in Actions.
out() {
  printf '%s=%s\n' "$1" "$2"
  [[ -n "${GITHUB_OUTPUT:-}" ]] && printf '%s=%s\n' "$1" "$2" >> "$GITHUB_OUTPUT" || true
}

# reg <yq-expr> — read from the registry
reg() { yq -r "$1" "$REGISTRY"; }

# reg_has <yq-path-to-map> <key>
reg_has() { [[ "$(reg "$1 | has(\"$2\")")" == "true" ]]; }

# slug — lowercase alnum + hyphen, max 64 (Foundry deployment-name rules)
slug() { tr '[:upper:]' '[:lower:]' <<<"$1" | sed -E 's/[^a-z0-9-]+/-/g; s/^-+|-+$//g' | cut -c1-64; }

# az_id — full ARM id of the account
az_id() { az cognitiveservices account show -g "$1" -n "$2" --query id -o tsv; }

# poll <cmd-that-prints-state> [timeout-seconds]
poll() {
  local cmd="$1" timeout="${2:-1800}" start state
  start=$(date +%s)
  while :; do
    state="$(bash -c "$cmd" 2>/dev/null || echo Unknown)"
    log "provisioningState=$state"
    case "$state" in
      Succeeded) return 0 ;;
      Failed|Canceled) die "deployment ended in state $state" ;;
    esac
    (( $(date +%s) - start > timeout )) && die "timeout after ${timeout}s (state=$state)"
    sleep 20
  done
}
