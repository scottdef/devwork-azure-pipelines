#!/usr/bin/env bash
# fetch-echarts.sh: install echarts.min.js for the /chart page.
#
# go-echarts emits <script src=AssetsHost+"echarts.min.js">; tmplforge sets
# AssetsHost to /static/echarts/ so the page works air-gapped and under
# CSP script-src 'self'. The file is not vendored (1 MB, Apache-2.0); fetch
# it once, verify it, and either rebuild (it is embedded) or serve it with
# `tmplforge serve -assets DIR`.
#
# Source: the npm registry tarball (registry.npmjs.org is reachable from
# most build networks; CDNs often are not). Pins: npm's own sha512
# integrity for the tarball, and sha256 of the extracted file.
set -euo pipefail

VERSION=5.6.0
TARBALL_URL="https://registry.npmjs.org/echarts/-/echarts-${VERSION}.tgz"
TARBALL_SHA256=e1b82487dd7f8d9f0438eec1f1433051148e0095ac7a15d4db890afda45cf610
TARBALL_SRI=sha512-oTbVTsXfKuEhxftHqL5xprgLoc0k7uScAwtryCgWF6hPYFLRwOUHiFmHGCBKP5NPFNkDVopOieyUqYGH8Fa3kA==
FILE_SHA256=bf4a223524e40b77c304bec67e1222cf551f14880cf42c69dc046558e11c07b1

dest=${1:-"$(dirname "$0")/../internal/web/static/echarts"}
mkdir -p "$dest"
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

curl -fsSL --proto '=https' -o "$work/echarts.tgz" "$TARBALL_URL"
echo "$TARBALL_SHA256  $work/echarts.tgz" | sha256sum -c -
sri="sha512-$(openssl dgst -sha512 -binary "$work/echarts.tgz" | base64 | tr -d '\n')"
[ "$sri" = "$TARBALL_SRI" ] || { echo "integrity mismatch: $sri" >&2; exit 1; }

tar -xzf "$work/echarts.tgz" -C "$work" package/dist/echarts.min.js package/LICENSE
echo "$FILE_SHA256  $work/package/dist/echarts.min.js" | sha256sum -c -
cp "$work/package/dist/echarts.min.js" "$dest/echarts.min.js"
cp "$work/package/LICENSE" "$dest/LICENSE"
(cd "$dest" && sha256sum echarts.min.js LICENSE > SHA256SUMS)
echo "installed echarts ${VERSION} into $dest"
