#!/usr/bin/env bash
###############################################################################
# sync-linked-services.sh — Quick wrapper for common sync operations
#
# Usage:
#   ./scripts/sync-linked-services.sh compare dev test
#   ./scripts/sync-linked-services.sh sync dev test prod
#   ./scripts/sync-linked-services.sh sync dev test --types linked-service dataset
#   ./scripts/sync-linked-services.sh sync-all dev test prod
###############################################################################
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
SYNC_SCRIPT="${SCRIPT_DIR}/sync-workspace-artifacts.py"
WORKSPACE_PREFIX="synapse-workspace-"

ACTION="${1:?Usage: $0 <compare|sync|sync-all> <source-env> <target-env> [target-env...] [--types type1 type2]}"
shift

# ── Parse environments and optional --types flag ──
ENVS=()
TYPES=("linked-service")
parsing_types=false

for arg in "$@"; do
  if [ "$arg" = "--types" ]; then
    parsing_types=true
    TYPES=()
    continue
  fi
  if $parsing_types; then
    TYPES+=("$arg")
  else
    ENVS+=("$arg")
  fi
done

SOURCE_ENV="${ENVS[0]:?Must provide at least source and one target environment}"
TARGET_ENVS=("${ENVS[@]:1}")

if [ "${#TARGET_ENVS[@]}" -eq 0 ]; then
  echo "Error: must provide at least one target environment" >&2
  echo "Usage: $0 <compare|sync|sync-all> <source-env> <target-env> [target-env...]" >&2
  exit 1
fi

SOURCE="${WORKSPACE_PREFIX}${SOURCE_ENV}"
TARGETS=()
for env in "${TARGET_ENVS[@]}"; do
  TARGETS+=("${WORKSPACE_PREFIX}${env}")
done

case "$ACTION" in
  compare)
    echo "==> Comparing ${SOURCE} against ${TARGETS[*]}"
    python3 "$SYNC_SCRIPT" \
      --source "$SOURCE" \
      --targets "${TARGETS[@]}" \
      --artifact-types "${TYPES[@]}" \
      --compare
    ;;

  sync)
    echo "==> Syncing ${TYPES[*]} from ${SOURCE} to ${TARGETS[*]}"
    python3 "$SYNC_SCRIPT" \
      --source "$SOURCE" \
      --targets "${TARGETS[@]}" \
      --artifact-types "${TYPES[@]}" \
      --direction source-to-target
    ;;

  sync-all)
    echo "==> Full bidirectional sync of ${TYPES[*]}"
    echo "    Source: ${SOURCE}"
    echo "    Targets: ${TARGETS[*]}"
    python3 "$SYNC_SCRIPT" \
      --source "$SOURCE" \
      --targets "${TARGETS[@]}" \
      --artifact-types "${TYPES[@]}" \
      --direction both
    ;;

  *)
    echo "Unknown action: ${ACTION}" >&2
    echo "Usage: $0 <compare|sync|sync-all> <source-env> <target-env> [target-env...]" >&2
    exit 1
    ;;
esac
