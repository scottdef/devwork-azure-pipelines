#!/usr/bin/env bash
# Package the whole collection as a zip next to this tree, with SHA256SUMS inside and a
# checksum of the zip itself printed for the release notes.
set -euo pipefail
cd "$(dirname "$0")/.."
./scripts/checksums.sh
out="go-templates-casestudies.zip"
rm -f "$out"
(cd .. && zip -qr "go-templates-casestudies/$out" go-templates-casestudies \
	-x 'go-templates-casestudies/.git/*' 'go-templates-casestudies/*/dist/*' 'go-templates-casestudies/*/bin/*' "go-templates-casestudies/$out")
sha256sum "$out"
