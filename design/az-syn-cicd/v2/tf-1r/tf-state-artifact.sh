#!/usr/bin/env bash
###############################################################################
# tf-state-artifact.sh — Manage Terraform state via GitHub Actions artifacts
#
# Usage:
#   ./scripts/tf-state-artifact.sh download <environment>
#   ./scripts/tf-state-artifact.sh upload   <environment>
#
# The state artifact is named "tfstate-<environment>" and stored with 90-day
# retention.  The workflow downloads it before terraform init and uploads
# after terraform apply.  This keeps state durable without any remote backend.
#
# Requires: gh CLI authenticated (GITHUB_TOKEN or GH_TOKEN set)
###############################################################################
set -euo pipefail

ACTION="${1:?Usage: $0 <download|upload> <environment>}"
ENV="${2:?Usage: $0 <download|upload> <environment>}"
ARTIFACT_NAME="tfstate-${ENV}"
STATE_DIR="${GITHUB_WORKSPACE:-$(pwd)}/terraform"
STATE_FILE="${STATE_DIR}/terraform.tfstate"

case "$ACTION" in
  download)
    echo "==> Downloading state artifact: ${ARTIFACT_NAME}"
    mkdir -p "$STATE_DIR"

    # gh run download fetches the most recent artifact with that name.
    # If no artifact exists yet (first run), we skip gracefully.
    if gh run download --name "$ARTIFACT_NAME" --dir "$STATE_DIR" 2>/dev/null; then
      if [ -f "$STATE_FILE" ]; then
        echo "==> State restored ($(wc -c < "$STATE_FILE") bytes)"
      else
        echo "==> Artifact downloaded but no state file found — first run"
      fi
    else
      echo "==> No previous state artifact found — initializing fresh"
    fi
    ;;

  upload)
    echo "==> Uploading state artifact: ${ARTIFACT_NAME}"
    if [ ! -f "$STATE_FILE" ]; then
      echo "==> No state file at ${STATE_FILE} — nothing to upload"
      exit 0
    fi

    # Upload is handled by actions/upload-artifact in the workflow YAML.
    # This branch exists for local testing with gh CLI.
    echo "==> State file ready for upload ($(wc -c < "$STATE_FILE") bytes)"
    echo "    Artifact name: ${ARTIFACT_NAME}"
    echo "    Path: ${STATE_FILE}"
    ;;

  *)
    echo "Unknown action: ${ACTION}" >&2
    echo "Usage: $0 <download|upload> <environment>" >&2
    exit 1
    ;;
esac
