#!/usr/bin/env bash
# Standalone alternative to the repos component: mirror every enabled repo from
# SRC to TGT (all branches and tags; LFS objects when git-lfs is installed).
# The repos component does the same and also records the repo ID map that
# pipelines, policies and work item links need, so prefer it.
# The PAT is passed through GIT_CONFIG_* (git 2.31+), never on the command line.
set -euo pipefail
: "${ADO_PAT:?}" "${SRC:?}" "${TGT:?}"
ORG="${ORG:-${ADO_ORG:-CoolADO}}"
AUTH=$(printf ':%s' "$ADO_PAT" | base64 -w0)
export GIT_TERMINAL_PROMPT=0 GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=http.extraHeader GIT_CONFIG_VALUE_0="Authorization: Basic $AUTH"
api() { curl -fsS -H "Authorization: Basic $AUTH" -H 'Content-Type: application/json' "$@"; }
enc() { jq -rn --arg v "$1" '$v|@uri'; }
S=$(enc "$SRC"); T=$(enc "$TGT")
tgt_id=$(api "https://dev.azure.com/$ORG/_apis/projects/$T?api-version=7.1" | jq -r .id)

api "https://dev.azure.com/$ORG/$S/_apis/git/repositories?api-version=7.1" \
  | jq -r '.value[] | select(.isDisabled|not) | [.name, (.defaultBranch // "")] | @tsv' \
  | while IFS=$'\t' read -r name branch; do
      r=$(enc "$name")
      if ! api "https://dev.azure.com/$ORG/$T/_apis/git/repositories/$r?api-version=7.1" >/dev/null 2>&1; then
        api -X POST "https://dev.azure.com/$ORG/$T/_apis/git/repositories?api-version=7.1" \
          -d "$(jq -n --arg n "$name" --arg p "$tgt_id" '{name:$n, project:{id:$p}}')" >/dev/null
      fi
      [ -n "$branch" ] || { echo "empty: $name"; continue; }
      work=$(mktemp -d)
      git clone --mirror --quiet "https://dev.azure.com/$ORG/$S/_git/$r" "$work/r.git"
      # Branches and tags only: PR refs are read-only, and this never deletes target-only branches.
      git -C "$work/r.git" push --force --quiet "https://dev.azure.com/$ORG/$T/_git/$r" 'refs/heads/*:refs/heads/*' 'refs/tags/*:refs/tags/*'
      if command -v git-lfs >/dev/null 2>&1; then
        git -C "$work/r.git" lfs fetch --all origin && git -C "$work/r.git" lfs push --all "https://dev.azure.com/$ORG/$T/_git/$r"
      fi
      repo_id=$(api "https://dev.azure.com/$ORG/$T/_apis/git/repositories/$r?api-version=7.1" | jq -r .id)
      api -X PATCH "https://dev.azure.com/$ORG/$T/_apis/git/repositories/$repo_id?api-version=7.1" \
        -d "$(jq -n --arg b "$branch" '{defaultBranch:$b}')" >/dev/null
      rm -rf "$work"
      echo "mirrored: $name"
    done
