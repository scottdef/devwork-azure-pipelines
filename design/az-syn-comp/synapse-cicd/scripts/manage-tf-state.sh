#!/usr/bin/env bash
# scripts/manage-tf-state.sh — manage TF state stored as GitHub artifacts
# Usage: ./scripts/manage-tf-state.sh list|download|compare|info [env] [output-path]
set -euo pipefail

ACTION="${1:?Usage: $0 <list|download|compare|info> [env] [output]}"
ENV="${2:-}"
OUTPUT="${3:-./terraform.tfstate}"
REPO="${GITHUB_REPOSITORY:-$(gh repo view --json nameWithOwner -q .nameWithOwner)}"

case "$ACTION" in
  list)
    FILTER="${ENV:+terraform-state-${ENV}}"
    FILTER="${FILTER:-terraform-state-}"
    gh api "repos/${REPO}/actions/artifacts" \
      --jq ".artifacts[] | select(.name | startswith(\"${FILTER}\")) | {name,id,created_at,size_in_bytes}" \
      | jq -s 'sort_by(.created_at) | reverse | .[:10]'
    ;;
  download)
    [ -z "$ENV" ] && { echo "Error: env required"; exit 1; }
    AID=$(gh api "repos/${REPO}/actions/artifacts" \
      --jq ".artifacts[] | select(.name==\"terraform-state-${ENV}\" and .expired==false) | .id" | head -1)
    [ -z "$AID" ] && { echo "No artifact found for ${ENV}"; exit 1; }
    TMP=$(mktemp -d)
    gh api "repos/${REPO}/actions/artifacts/${AID}/zip" > "${TMP}/s.zip"
    unzip -o "${TMP}/s.zip" -d "$(dirname "$OUTPUT")"
    rm -rf "$TMP"
    echo "Downloaded state: $(wc -c < "$OUTPUT") bytes, $(jq '.resources|length' "$OUTPUT") resources"
    ;;
  compare)
    [ -z "$ENV" ] && { echo "Error: env required"; exit 1; }
    ARTS=$(gh api "repos/${REPO}/actions/artifacts" \
      --jq "[.artifacts[]|select(.name==\"terraform-state-${ENV}\" and .expired==false)]|sort_by(.created_at)|reverse|.[:2]")
    [ "$(echo "$ARTS" | jq 'length')" -lt 2 ] && { echo "Need 2+ artifacts to compare"; exit 0; }
    TMP=$(mktemp -d)
    for i in 0 1; do
      AID=$(echo "$ARTS" | jq -r ".[$i].id")
      gh api "repos/${REPO}/actions/artifacts/${AID}/zip" > "${TMP}/s${i}.zip"
      mkdir -p "${TMP}/s${i}" && unzip -o "${TMP}/s${i}.zip" -d "${TMP}/s${i}"
    done
    diff <(jq -r '.resources[]|"\(.type).\(.name)"' "${TMP}/s1/terraform.tfstate"|sort) \
         <(jq -r '.resources[]|"\(.type).\(.name)"' "${TMP}/s0/terraform.tfstate"|sort) || true
    rm -rf "$TMP"
    ;;
  info)
    [ -z "$ENV" ] && { echo "Error: env required"; exit 1; }
    REF=".terraform-state-refs/${ENV}.json"
    [ -f "$REF" ] && jq '.' "$REF" || \
      gh api "repos/${REPO}/actions/artifacts" \
        --jq ".artifacts[]|select(.name==\"terraform-state-${ENV}\")|{name,id,created_at,expired}" | head -1 | jq '.'
    ;;
  *) echo "Usage: $0 <list|download|compare|info> [env] [output]" && exit 1 ;;
esac
