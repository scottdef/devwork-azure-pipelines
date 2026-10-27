#!/usr/bin/env bash
# bootstrap-state-branch.sh - create the orphan checkpoint branch with git
# plumbing, without touching the working tree or the current branch.
#
#   bootstrap-state-branch.sh [remote] [branch]
#
# Idempotent: does nothing if the branch already exists on the remote.
set -euo pipefail
IFS=$'\n\t'

# shellcheck source=scripts/lib.sh
source "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

remote="${1:-origin}"
branch="${2:-${GHLOG_STATE_BRANCH:-forwarder-state}}"
need git
git rev-parse --git-dir >/dev/null 2>&1 || die "not inside a git repository"
[[ "$branch" =~ ^[A-Za-z0-9._/-]+$ ]] || die "bad branch name: $branch"

if git ls-remote --exit-code --heads "$remote" "$branch" >/dev/null 2>&1; then
  note "branch $branch already exists on $remote; nothing to do"
  exit 0
fi

readme="$(mktemp)"
trap 'rm -f "$readme"' EXIT
cat >"$readme" <<'EOF'
# forwarder-state

Checkpoints for ghlog2dt: one `state/<owner>__<repo>.json` per target
repository. Written only by the forwarder workflows (commit-state.sh).

Do not edit by hand. To replay or skip work use
`ghlog2dt checkpoint rewind|mark` or the `checkpoint` workflow.
EOF

readme_blob="$(git hash-object -w "$readme")"
keep_blob="$(git hash-object -w /dev/null)"
state_tree="$(printf '100644 blob %s\t.gitkeep\n' "$keep_blob" | git mktree)"
root_tree="$(printf '100644 blob %s\tREADME.md\n040000 tree %s\tstate\n' "$readme_blob" "$state_tree" | git mktree)"
commit="$(git -c user.name='ghlog2dt[bot]' -c user.email='ghlog2dt-bot@users.noreply.github.com' \
  commit-tree "$root_tree" -m "state: bootstrap empty checkpoint branch")"
git push "$remote" "${commit}:refs/heads/${branch}"
note "created ${branch} at ${commit:0:12} on ${remote}"
