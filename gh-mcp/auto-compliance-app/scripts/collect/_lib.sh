#!/usr/bin/env bash
# Shared helpers for evidence collectors (scripts/collect/<control>.sh).
#
# Each collector sources this file, calls `parse_args "$@"`, then emits evidence
# fragments + provenance into evidence/<control>/ using emit / emit_manual /
# emit_asset. The build (scripts/build_site.py) injects whatever is present.
#
# Modes:
#   live   (default) — query real APIs; refresh automatable items only.
#                      Manual items (emit_manual) are left untouched.
#   sample (--sample or SAMPLE=1) — write realistic sample content for ALL items
#                      (automatable + manual), status: sample, WITHOUT clobbering
#                      files that already exist (use --force to overwrite).
#
# Env: ORG (defaults to GITHUB_REPOSITORY_OWNER, else "cla-org").

set -euo pipefail

LIB_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$LIB_DIR/../.." && pwd)"
EVIDENCE_ROOT="$REPO_ROOT/evidence"

MODE="${MODE:-live}"
FORCE="${FORCE:-0}"
ORG="${ORG:-${GITHUB_REPOSITORY_OWNER:-cla-org}}"

parse_args() {
  while [ $# -gt 0 ]; do
    case "$1" in
      --sample) MODE="sample" ;;
      --live)   MODE="live" ;;
      --force)  FORCE="1" ;;
      *) echo "unknown arg: $1" >&2 ;;
    esac
    shift
  done
  [ "${SAMPLE:-0}" = "1" ] && MODE="sample"
  return 0
}

is_sample() { [ "$MODE" = "sample" ]; }
today() { date -u +%F; }

# _skip_existing <path> — true (skip) if the file exists, not forced, sample mode.
_skip_existing() {
  [ -f "$1" ] && [ "$FORCE" != "1" ] && is_sample
}

# emit <control> <item> <by> <source> <status>   (HTML body on stdin)
#   Writes evidence/<control>/<item>.html + .meta.
#   live mode refreshes; sample mode writes but never clobbers existing files.
emit() {
  local control="$1" item="$2" by="$3" source="$4" status="$5"
  local dir="$EVIDENCE_ROOT/$control"
  mkdir -p "$dir"
  local html="$dir/$item.html"
  if _skip_existing "$html"; then
    cat >/dev/null
    echo "  skip (exists): $control/$item" >&2
    return 0
  fi
  cat > "$html"
  cat > "$dir/$item.meta" <<META
date: $(today)
by: $by
source: $source
status: $status
META
  echo "  wrote: $control/$item [$status]" >&2
}

# emit_manual: like emit, but a no-op in live mode (manual items are
# human-maintained — collectors must not overwrite them with stale data).
emit_manual() {
  if ! is_sample; then
    cat >/dev/null
    return 0
  fi
  emit "$@"
}

# emit_asset <control> <filename>   (asset bytes on stdin)
emit_asset() {
  local control="$1" name="$2"
  local dir="$EVIDENCE_ROOT/$control"
  mkdir -p "$dir"
  local path="$dir/$name"
  if _skip_existing "$path"; then
    cat >/dev/null
    return 0
  fi
  cat > "$path"
  echo "  asset: $control/$name" >&2
}

# gh_jq <api-path> <jq-filter> — echo the result, or empty string on failure.
gh_jq() {
  gh api "$1" --jq "$2" 2>/dev/null || echo ""
}
