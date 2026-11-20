#!/usr/bin/env bash
# Merge the CoolDeploy request types and labels into the IssueOps registry.
#
#   scripts/merge-registry.sh /path/to/issueops
#
# Adds every entry of config/issueops.cool.json to request_types in
# config/issueops.json, and every entry of config/labels.cool.json to
# config/labels.json. An entry with the same id (or label name) is replaced,
# so running it twice changes nothing.
set -euo pipefail

here=$(cd "$(dirname "$0")/.." && pwd)
target=${1:?usage: merge-registry.sh /path/to/issueops}

for f in config/issueops.json config/labels.json; do
	[ -f "$target/$f" ] || { echo "merge-registry.sh: $target/$f not found" >&2; exit 1; }
done

tmp=$(mktemp)
jq -M --slurpfile add "$here/config/issueops.cool.json" '
	($add[0].request_types | map(.id)) as $ids
	| .request_types = [.request_types[] | select(.id as $id | $ids | index($id) | not)] + $add[0].request_types
' "$target/config/issueops.json" >"$tmp"
mv "$tmp" "$target/config/issueops.json"

tmp=$(mktemp)
jq -M --slurpfile add "$here/config/labels.cool.json" '
	($add[0] | map(.name)) as $names
	| [.[] | select(.name as $n | $names | index($n) | not)] + $add[0]
' "$target/config/labels.json" >"$tmp"
mv "$tmp" "$target/config/labels.json"

cp "$here"/.github/ISSUE_TEMPLATE/cool-*.yml "$target/.github/ISSUE_TEMPLATE/"
cp "$here"/.github/workflows/execute-cool-*.yml "$target/.github/workflows/"
mkdir -p "$target/scripts/cool" "$target/sandbox"
cp "$here"/scripts/cool/* "$target/scripts/cool/"
cp -R "$here"/sandbox/. "$target/sandbox/"

echo "merged $(jq -M '.request_types | length' "$here/config/issueops.cool.json") request types into $target"
echo "next: scripts/forms-to-json.sh, then wire the nine types into the comment router and add Go handlers"
