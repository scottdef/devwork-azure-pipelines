#!/usr/bin/env bash
# Standalone alternative to the wiki component's project-wiki step: the project
# wiki is a hidden Git repo (<project>.wiki, branch wikiMaster). Create the
# target project wiki if needed, then force-push wikiMaster into it.
# The PAT is passed through GIT_CONFIG_* (git 2.31+), never on the command line.
set -euo pipefail
: "${ADO_PAT:?}" "${SRC:?}" "${TGT:?}"
ORG="${ORG:-${ADO_ORG:-CoolADO}}"
AUTH=$(printf ':%s' "$ADO_PAT" | base64 -w0)
export GIT_TERMINAL_PROMPT=0 GIT_CONFIG_COUNT=1 GIT_CONFIG_KEY_0=http.extraHeader GIT_CONFIG_VALUE_0="Authorization: Basic $AUTH"
api() { curl -fsS -H "Authorization: Basic $AUTH" -H 'Content-Type: application/json' "$@"; }
enc() { jq -rn --arg v "$1" '$v|@uri'; }
project_wiki() { jq -c '[.value[] | select(.type=="projectWiki")][0] // empty'; }
S=$(enc "$SRC"); T=$(enc "$TGT")

src_wiki=$(api "https://dev.azure.com/$ORG/$S/_apis/wiki/wikis?api-version=7.1" | project_wiki)
if [ -z "$src_wiki" ]; then echo "source project has no project wiki; nothing to mirror"; exit 0; fi
tgt_wiki=$(api "https://dev.azure.com/$ORG/$T/_apis/wiki/wikis?api-version=7.1" | project_wiki)
if [ -z "$tgt_wiki" ]; then
  tgt_id=$(api "https://dev.azure.com/$ORG/_apis/projects/$T?api-version=7.1" | jq -r .id)
  tgt_wiki=$(api -X POST "https://dev.azure.com/$ORG/$T/_apis/wiki/wikis?api-version=7.1" \
    -d "$(jq -n --arg n "$TGT.wiki" --arg p "$tgt_id" '{type:"projectWiki", name:$n, projectId:$p}')")
fi

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
git clone --bare --quiet "$(jq -r .remoteUrl <<<"$src_wiki" | sed 's#//[^@/]*@#//#')" "$work/w.git"
git -C "$work/w.git" push --force --quiet "$(jq -r .remoteUrl <<<"$tgt_wiki" | sed 's#//[^@/]*@#//#')" refs/heads/wikiMaster:refs/heads/wikiMaster
echo "wiki mirrored: $SRC -> $TGT"
