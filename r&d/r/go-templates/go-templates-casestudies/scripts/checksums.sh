#!/usr/bin/env bash
# Write SHA256SUMS for every file in the collection (excluding git metadata and build output).
set -euo pipefail
cd "$(dirname "$0")/.."
find . -type f \
	-not -path './.git/*' -not -name SHA256SUMS -not -path '*/dist/*' -not -path '*/bin/*' \
	-not -name '*.zip' -print0 | sort -z | xargs -0 sha256sum > SHA256SUMS
echo "$(wc -l < SHA256SUMS) files hashed -> SHA256SUMS"
