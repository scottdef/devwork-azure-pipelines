#!/usr/bin/env bash
# Converts .github/ISSUE_TEMPLATE/*.yml issue forms to config/forms/*.json.
# The Go CLI (check-forms, parse, simulate) reads the JSON copies because the
# platform uses the Go standard library only (no YAML parser).
#
# Works with mikefarah/yq v4 (preinstalled on GitHub-hosted Ubuntu runners),
# kislyuk/yq (pip), or python3 + PyYAML.
set -euo pipefail
cd "$(dirname "$0")/.."
src=.github/ISSUE_TEMPLATE
dst=config/forms
mkdir -p "$dst"

convert() {
  local in=$1 out=$2
  if command -v yq >/dev/null 2>&1 && yq --version 2>&1 | grep -qi mikefarah; then
    yq -o=json '.' "$in" > "$out"
  elif command -v yq >/dev/null 2>&1; then
    yq '.' "$in" > "$out"
  else
    python3 -c 'import json,sys,yaml; json.dump(yaml.safe_load(open(sys.argv[1])), sys.stdout, indent=2); print()' "$in" > "$out"
  fi
}

for f in "$src"/*.yml; do
  name=$(basename "$f" .yml)
  [[ "$name" == "config" ]] && continue
  convert "$f" "$dst/$name.json"
  echo "converted $f -> $dst/$name.json"
done
