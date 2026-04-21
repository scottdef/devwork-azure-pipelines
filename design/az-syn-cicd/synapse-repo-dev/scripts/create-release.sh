#!/usr/bin/env bash
###############################################################################
# create-release.sh
# Creates a CalVer git tag and GitHub Release for the current commit.
#
# Usage:
#   create-release.sh <env-prefix> <repo> [notes]
#   env-prefix: dev | test | prod
#   repo:       owner/repo-name
#   notes:      optional release notes (defaults to commit message)
#
# Outputs GITHUB_OUTPUT variable: version=<tag>
###############################################################################
set -euo pipefail

ENV_PREFIX="${1:-dev}"
REPO="${2:-}"
EXTRA_NOTES="${3:-}"

if [ -z "$REPO" ]; then
  REPO="$GITHUB_REPOSITORY"
fi

# ── Generate CalVer tag ───────────────────────────────────────────────────────
# Format: <env>-YYYY.MM.DD.<run_number>
# e.g.   dev-2026.04.14.42
DATESTAMP="$(date -u '+%Y.%m.%d')"
RUN_NUM="${GITHUB_RUN_NUMBER:-0}"
TAG="${ENV_PREFIX}-${DATESTAMP}.${RUN_NUM}"

echo "==> Creating release tag: $TAG"
echo "==> Repository: $REPO"

# ── Compose release notes ─────────────────────────────────────────────────────
COMMIT_SHA="${GITHUB_SHA:-$(git rev-parse HEAD)}"
SHORT_SHA="${COMMIT_SHA:0:7}"
ACTOR="${GITHUB_ACTOR:-unknown}"
REF_NAME="${GITHUB_REF_NAME:-unknown}"
WORKFLOW="${GITHUB_WORKFLOW:-unknown}"

NOTES=$(cat <<EOF
## Synapse Workspace Release — ${TAG}

| Field         | Value |
|---------------|-------|
| Environment   | \`${ENV_PREFIX}\` |
| Commit        | \`${SHORT_SHA}\` |
| Branch        | \`${REF_NAME}\` |
| Triggered by  | \`${ACTOR}\` |
| Workflow run  | [\`${GITHUB_RUN_ID:-n/a}\`](${GITHUB_SERVER_URL:-https://github.com}/${REPO}/actions/runs/${GITHUB_RUN_ID:-0}) |

${EXTRA_NOTES}
EOF
)

# ── Create tag ────────────────────────────────────────────────────────────────
git config user.name  "github-actions[bot]"
git config user.email "github-actions[bot]@users.noreply.github.com"
git tag -a "$TAG" -m "Synapse release ${TAG}" || {
  echo "::warning::Tag ${TAG} already exists — skipping tag creation"
}
git push origin "$TAG" || echo "::warning::Tag push failed (may already exist)"

# ── Create GitHub Release ─────────────────────────────────────────────────────
gh release create "$TAG" \
  --repo "$REPO" \
  --title "Synapse ${TAG}" \
  --notes "$NOTES" \
  --latest=false \
  2>/dev/null || echo "::warning::Release may already exist for tag $TAG"

# ── Export version to GITHUB_OUTPUT ──────────────────────────────────────────
if [ -n "${GITHUB_OUTPUT:-}" ]; then
  echo "version=${TAG}" >> "$GITHUB_OUTPUT"
  echo "tag=${TAG}"     >> "$GITHUB_OUTPUT"
fi

echo "==> Release ${TAG} created successfully"
