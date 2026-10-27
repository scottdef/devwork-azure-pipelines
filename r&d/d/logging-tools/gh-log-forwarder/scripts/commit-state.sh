#!/usr/bin/env bash
# commit-state.sh - fold checkpoint files from one or more shards into
# the state branch and push, from inside a checkout of that branch.
#
#   commit-state.sh <incoming-dir> [state-subdir]
#
# Checkpoints merge as max(high_water) + union(pending, shipped), so any
# number of writers converge; a push race is resolved by resetting to the
# remote branch and merging again. Pruning after the merge keeps the
# union from resurrecting entries a shard already expired.
set -euo pipefail
IFS=$'\n\t'

# shellcheck source=scripts/lib.sh
source "$(dirname "$(readlink -f "${BASH_SOURCE[0]}")")/lib.sh"

incoming="${1:?usage: commit-state.sh <incoming-dir> [state-subdir]}"
dest="${2:-state}"
branch="${GHLOG_STATE_BRANCH:-forwarder-state}"
need git ghlog2dt

shopt -s nullglob
files=("$incoming"/*.json)
if ((${#files[@]} == 0)); then
  note "no checkpoint files in $incoming; nothing to commit"
  exit 0
fi
for f in "${files[@]}"; do
  [[ "$(basename "$f")" =~ ^[A-Za-z0-9-]+__[A-Za-z0-9._-]+\.json$ ]] || die "unexpected file in $incoming: $(basename "$f")"
done

git config user.name "ghlog2dt[bot]"
git config user.email "ghlog2dt-bot@users.noreply.github.com"

merge_in() {
  mkdir -p "$dest"
  local f base repo
  for f in "${files[@]}"; do
    base="$(basename "$f")"
    if [[ -f "$dest/$base" ]]; then
      ghlog2dt checkpoint merge -o "$dest/$base" "$dest/$base" "$f"
    else
      cp -- "$f" "$dest/$base"
    fi
    repo="${base%.json}"
    repo="${repo/__//}"
    ghlog2dt checkpoint prune --repo "$repo" --state "$dest" 2>/dev/null
  done
}

for attempt in 1 2 3 4 5; do
  merge_in
  git add -- "$dest"
  if git diff --cached --quiet; then
    note "checkpoint unchanged"
    exit 0
  fi
  git commit --quiet -m "state: ${GITHUB_WORKFLOW:-manual} run ${GITHUB_RUN_ID:-0} attempt ${GITHUB_RUN_ATTEMPT:-1}"
  if git push --quiet origin "HEAD:refs/heads/${branch}"; then
    note "checkpoint pushed to ${branch} ($(git rev-parse --short HEAD))"
    exit 0
  fi
  warn "push to ${branch} raced (attempt ${attempt}); re-merging on top of the remote"
  sleep $((attempt * 3 + RANDOM % 3))
  git fetch --quiet origin "$branch"
  git reset --quiet --hard "origin/${branch}"
done
die 6 "could not push checkpoint to ${branch} after 5 attempts"
