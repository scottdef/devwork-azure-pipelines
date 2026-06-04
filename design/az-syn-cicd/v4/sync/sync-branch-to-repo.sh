#!/usr/bin/env bash
###############################################################################
# sync-branch-to-repo.sh — Push a branch from dev repo to a target repo
#
# Uses the GitHub App token to authenticate, which bypasses branch protection
# rules when the app has org admin permissions.
#
# Usage:
#   ./scripts/sync-branch-to-repo.sh <branch> <target-repo> <token>
#
# Example:
#   ./scripts/sync-branch-to-repo.sh test org/synapse-repo-test "$GH_APP_TOKEN"
#   ./scripts/sync-branch-to-repo.sh prod org/synapse-repo-prod "$GH_APP_TOKEN"
#
# The script:
#   1. Configures git to use the app token for authentication
#   2. Adds the target repo as a remote
#   3. Pushes the source branch to the target repo's branch
#   4. Reports the sync result
###############################################################################
set -euo pipefail

BRANCH="${1:?Usage: $0 <branch> <target-repo> <token>}"
TARGET_REPO="${2:?Usage: $0 <branch> <target-repo> <token>}"
TOKEN="${3:?Usage: $0 <branch> <target-repo> <token>}"

# Target branch in the target repo (defaults to same name)
TARGET_BRANCH="${4:-$BRANCH}"

echo "═══════════════════════════════════════════════════════"
echo "  Branch Sync: ${BRANCH} → ${TARGET_REPO}:${TARGET_BRANCH}"
echo "═══════════════════════════════════════════════════════"

# ── Configure git ──
git config user.name "github-actions[bot]"
git config user.email "github-actions[bot]@users.noreply.github.com"

# ── Build authenticated URL ──
# The x-access-token mechanism with a GitHub App installation token
# bypasses branch protection when the app has admin permissions.
TARGET_URL="https://x-access-token:${TOKEN}@github.com/${TARGET_REPO}.git"

# ── Add target as remote ──
REMOTE_NAME="target-repo"
git remote remove "$REMOTE_NAME" 2>/dev/null || true
git remote add "$REMOTE_NAME" "$TARGET_URL"

# ── Fetch target to check current state ──
echo ""
echo "==> Fetching ${TARGET_REPO}..."
if git fetch "$REMOTE_NAME" "$TARGET_BRANCH" 2>/dev/null; then
  TARGET_SHA=$(git rev-parse "refs/remotes/${REMOTE_NAME}/${TARGET_BRANCH}" 2>/dev/null || echo "unknown")
  echo "  Target current HEAD: ${TARGET_SHA:0:8}"
else
  echo "  Target branch '${TARGET_BRANCH}' does not exist yet — will create"
fi

# ── Get source state ──
SOURCE_SHA=$(git rev-parse "refs/heads/${BRANCH}" 2>/dev/null || git rev-parse HEAD)
echo "  Source HEAD: ${SOURCE_SHA:0:8}"

# ── Push ──
echo ""
echo "==> Pushing ${BRANCH} → ${TARGET_REPO}:${TARGET_BRANCH}..."
if git push "$REMOTE_NAME" "refs/heads/${BRANCH}:refs/heads/${TARGET_BRANCH}" --force 2>&1; then
  echo ""
  echo "  ✓ Sync complete"
  echo "  Source: ${SOURCE_SHA:0:8}"
  echo "  Target: ${TARGET_REPO}:${TARGET_BRANCH}"
  SYNC_STATUS="success"
else
  echo ""
  echo "  ✗ Push failed"
  SYNC_STATUS="failed"
fi

# ── Clean up remote ──
git remote remove "$REMOTE_NAME" 2>/dev/null || true

# ── GitHub Actions outputs ──
if [ -n "${GITHUB_OUTPUT:-}" ]; then
  echo "sync_status=${SYNC_STATUS}" >> "$GITHUB_OUTPUT"
  echo "source_sha=${SOURCE_SHA}" >> "$GITHUB_OUTPUT"
  echo "target_repo=${TARGET_REPO}" >> "$GITHUB_OUTPUT"
  echo "target_branch=${TARGET_BRANCH}" >> "$GITHUB_OUTPUT"
fi

echo ""
echo "═══════════════════════════════════════════════════════"

[ "$SYNC_STATUS" = "success" ] || exit 1
