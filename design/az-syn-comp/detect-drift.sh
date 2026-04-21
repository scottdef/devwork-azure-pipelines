#!/usr/bin/env bash
# =============================================================================
# detect-drift.sh  <workspace>  <artifacts-dir>
#
# Compares the names of live Azure Synapse artifacts against those encoded in
# the local artifact JSON files (as committed in the collaboration branch).
#
# Exit codes:
#   0 — no drift
#   1 — drift detected (or comparison failed)
#
# Outputs:
#   /tmp/drift-report-<workspace>.txt   — human-readable diff
#   Sets DRIFT_DETECTED=true in GITHUB_ENV when called from a workflow
# =============================================================================
set -euo pipefail

WORKSPACE="${1:?Usage: $0 <workspace> <artifacts-dir>}"
ARTIFACTS_DIR="${2:?artifacts-dir required}"
REPORT="/tmp/drift-report-${WORKSPACE}.txt"

log()  { echo "[$(date -u +%H:%M:%S)] $*"; }
warn() { echo "[WARN] $*" >&2; }

: > "$REPORT"   # reset report

drift_detected=false

# ── Helper: compare sorted name lists ────────────────────────────────────────
compare_artifact() {
  local artifact_type="$1"
  local list_cmd="$2"
  local expected_dir="${ARTIFACTS_DIR}/${artifact_type}"

  log "Checking ${artifact_type}..."

  # Expected: names from local JSON files (strip .json suffix)
  if [[ -d "$expected_dir" ]]; then
    EXPECTED=$(find "$expected_dir" -maxdepth 1 -name '*.json' \
                 | xargs -I{} basename {} .json | sort)
  else
    EXPECTED=""
    warn "No local directory for ${artifact_type} — skipping expected list"
  fi

  # Live: names from the running workspace
  LIVE=$(eval "$list_cmd" 2>/dev/null | sort || true)

  # Diff
  DIFF_OUT=$(diff \
    <(echo "$EXPECTED") \
    <(echo "$LIVE") 2>&1 || true)

  if [[ -n "$DIFF_OUT" ]]; then
    {
      echo "━━━━ DRIFT: ${artifact_type} ━━━━"
      echo "  Lines starting with < are in repo but NOT in workspace"
      echo "  Lines starting with > are in workspace but NOT in repo"
      echo "$DIFF_OUT"
      echo ""
    } >> "$REPORT"
    drift_detected=true
    log "  DRIFT detected in ${artifact_type}"
  else
    log "  ✓  ${artifact_type} matches"
  fi
}

# ── Run comparisons ───────────────────────────────────────────────────────────
compare_artifact "pipeline" \
  "az synapse pipeline list --workspace-name '$WORKSPACE' --query '[].name' -o tsv"

compare_artifact "dataset" \
  "az synapse dataset list --workspace-name '$WORKSPACE' --query '[].name' -o tsv"

compare_artifact "linkedService" \
  "az synapse linked-service list --workspace-name '$WORKSPACE' --query '[].name' -o tsv"

compare_artifact "trigger" \
  "az synapse trigger list --workspace-name '$WORKSPACE' --query '[].name' -o tsv"

compare_artifact "notebook" \
  "az synapse notebook list --workspace-name '$WORKSPACE' --query '[].name' -o tsv"

compare_artifact "dataflow" \
  "az synapse data-flow list --workspace-name '$WORKSPACE' --query '[].name' -o tsv"

compare_artifact "sqlscript" \
  "az synapse sql-script list --workspace-name '$WORKSPACE' --query '[].name' -o tsv"

# ── Emit results ──────────────────────────────────────────────────────────────
if [[ "$drift_detected" == true ]]; then
  log "==> DRIFT DETECTED — see ${REPORT}"
  cat "$REPORT"
  # Export for GitHub Actions
  echo "DRIFT_DETECTED=true"  >> "${GITHUB_ENV:-/dev/null}"
  echo "DRIFT_REPORT=${REPORT}" >> "${GITHUB_ENV:-/dev/null}"
  exit 1
else
  log "==> No drift detected"
  echo "DRIFT_DETECTED=false" >> "${GITHUB_ENV:-/dev/null}"
  exit 0
fi
