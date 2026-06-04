#!/usr/bin/env bash
###############################################################################
# stage-changed-artifacts.sh — Build a minimal ArtifactsFolder from changed files
#
# Reads a list of changed file paths (from git diff) and copies ONLY those
# files to a staging directory, preserving the Synapse folder structure.
# The staging directory becomes the ArtifactsFolder for the validate operation.
#
# Usage:
#   ./scripts/stage-changed-artifacts.sh <source-dir> <staging-dir> <changed-files-list>
#
# Example:
#   git diff --name-only origin/dev...HEAD -- pipeline/ dataset/ ... > /tmp/changed.txt
#   ./scripts/stage-changed-artifacts.sh . ./staged /tmp/changed.txt
#
# The staging directory will contain:
#   staged/
#   ├── pipeline/
#   │   └── ChangedPipeline.json      (only changed pipelines)
#   ├── dataset/
#   │   └── ChangedDataset.json       (only changed datasets)
#   └── publish_config.json           (always included if present)
#
# Linked service files are EXCLUDED — they are managed via Synapse UI only.
###############################################################################
set -euo pipefail

SOURCE_DIR="${1:?Usage: $0 <source-dir> <staging-dir> <changed-files-list>}"
STAGING_DIR="${2:?Usage: $0 <source-dir> <staging-dir> <changed-files-list>}"
CHANGED_LIST="${3:?Usage: $0 <source-dir> <staging-dir> <changed-files-list>}"

[ -f "$CHANGED_LIST" ] || { echo "::error::Changed list not found: ${CHANGED_LIST}"; exit 1; }

# ── Clean and create staging directory ──
rm -rf "$STAGING_DIR"
mkdir -p "$STAGING_DIR"

# ── Synapse artifact directories we handle ──
ARTIFACT_DIRS="pipeline dataset notebook trigger dataflow sqlscript"

echo "==> Building staged ArtifactsFolder"
echo "  Source: ${SOURCE_DIR}"
echo "  Staging: ${STAGING_DIR}"
echo ""

STAGED_COUNT=0
SKIPPED_COUNT=0

while IFS= read -r filepath; do
  [ -z "$filepath" ] && continue

  # Extract the top-level directory
  top_dir=$(echo "$filepath" | cut -d'/' -f1)

  # Skip linked services entirely
  if [ "$top_dir" = "linkedService" ]; then
    SKIPPED_COUNT=$((SKIPPED_COUNT + 1))
    echo "  skip (linked service): ${filepath}"
    continue
  fi

  # Only stage files from known artifact directories
  is_artifact=false
  for d in $ARTIFACT_DIRS; do
    if [ "$top_dir" = "$d" ]; then
      is_artifact=true
      break
    fi
  done

  if ! $is_artifact; then
    echo "  skip (not artifact): ${filepath}"
    continue
  fi

  # Verify the source file exists
  if [ ! -f "${SOURCE_DIR}/${filepath}" ]; then
    echo "  skip (deleted): ${filepath}"
    continue
  fi

  # Copy preserving directory structure
  target_path="${STAGING_DIR}/${filepath}"
  mkdir -p "$(dirname "$target_path")"
  cp "${SOURCE_DIR}/${filepath}" "$target_path"
  STAGED_COUNT=$((STAGED_COUNT + 1))
  echo "  staged: ${filepath}"

done < "$CHANGED_LIST"

# ── Include workspace config files if they exist ──
# These are needed for validate to work correctly.
for config in publish_config.json template-parameters-definition.json; do
  if [ -f "${SOURCE_DIR}/${config}" ]; then
    cp "${SOURCE_DIR}/${config}" "${STAGING_DIR}/${config}"
    echo "  config: ${config}"
  fi
done

echo ""
echo "═══════════════════════════════════════════════════════"
echo "  STAGING REPORT"
echo "═══════════════════════════════════════════════════════"
echo "  Staged:  ${STAGED_COUNT} artifact(s)"
echo "  Skipped: ${SKIPPED_COUNT} linked service(s)"
echo ""

# List staged structure
if [ "$STAGED_COUNT" -gt 0 ]; then
  echo "  Staged directory:"
  find "$STAGING_DIR" -type f | sed "s|${STAGING_DIR}/|    |"
fi

echo "═══════════════════════════════════════════════════════"

# ── GitHub Actions outputs ──
if [ -n "${GITHUB_OUTPUT:-}" ]; then
  echo "staged_count=${STAGED_COUNT}" >> "$GITHUB_OUTPUT"
  echo "staging_dir=${STAGING_DIR}" >> "$GITHUB_OUTPUT"
  if [ "$STAGED_COUNT" -eq 0 ]; then
    echo "skip_deploy=true" >> "$GITHUB_OUTPUT"
  else
    echo "skip_deploy=false" >> "$GITHUB_OUTPUT"
  fi
fi
