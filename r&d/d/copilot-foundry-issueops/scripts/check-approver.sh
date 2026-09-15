#!/usr/bin/env bash
# check-approver.sh — is $ACTOR allowed to /approve?
# Rule: member of org team $APPROVER_TEAM (needs a token with read:org — GH_ADMIN_TOKEN
# or a GitHub App token). Falls back to author_association OWNER/MEMBER when no such token.
# Env: ACTOR ORG [APPROVER_TEAM=foundry-approvers] [AUTHOR_ASSOCIATION] [GH_TOKEN]
set -euo pipefail
# shellcheck source=lib.sh
source "$(dirname "$0")/lib.sh"
need gh
: "${ACTOR:?}" "${ORG:?}"
APPROVER_TEAM="${APPROVER_TEAM:-foundry-approvers}"

if [[ -n "${GH_TOKEN:-}" ]] && gh api "orgs/$ORG/teams/$APPROVER_TEAM" -q .slug >/dev/null 2>&1; then
  state="$(gh api "orgs/$ORG/teams/$APPROVER_TEAM/memberships/$ACTOR" -q .state 2>/dev/null || echo none)"
  [[ "$state" == active ]] && { out approved true; out reason "team:$APPROVER_TEAM"; exit 0; }
  out approved false; out reason "not in team $APPROVER_TEAM"; exit 1
fi
warn "no team-capable token; falling back to author_association"
case "${AUTHOR_ASSOCIATION:-}" in
  OWNER|MEMBER) out approved true;  out reason "association:${AUTHOR_ASSOCIATION}" ;;
  *)            out approved false; out reason "association:${AUTHOR_ASSOCIATION:-none}"; exit 1 ;;
esac
