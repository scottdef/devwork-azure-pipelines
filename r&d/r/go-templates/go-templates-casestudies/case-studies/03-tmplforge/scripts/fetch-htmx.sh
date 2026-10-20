#!/usr/bin/env bash
# fetch-htmx.sh: refresh the vendored htmx in internal/web/static.
#
# htmx is vendored (committed) because the GUI must work offline and under
# CSP script-src 'self'. This script exists to re-fetch or upgrade it
# reproducibly; it refuses a file whose hash does not match the pin.
set -euo pipefail

VERSION=${HTMX_VERSION:-2.0.11}
SHA256=${HTMX_SHA256:-d6fdc75f204e6bdefa99b69bf1e6d4ac69b8a364f77929f45c13476b4000f717}
dest=${1:-"$(dirname "$0")/../internal/web/static"}
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

curl -fsSL --proto '=https' -o "$work/htmx.tgz" "https://registry.npmjs.org/htmx.org/-/htmx.org-${VERSION}.tgz"
tar -xzf "$work/htmx.tgz" -C "$work" package/dist/htmx.min.js package/LICENSE
echo "$SHA256  $work/package/dist/htmx.min.js" | sha256sum -c -
cp "$work/package/dist/htmx.min.js" "$dest/htmx.min.js"
cp "$work/package/LICENSE" "$dest/LICENSE"
(cd "$dest" && sha256sum htmx.min.js > SHA256SUMS)
echo "installed htmx ${VERSION} into $dest"
