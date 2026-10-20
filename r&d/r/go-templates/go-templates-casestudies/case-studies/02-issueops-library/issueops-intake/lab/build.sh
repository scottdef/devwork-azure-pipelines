#!/usr/bin/env bash
# build.sh: compile the lab's Go-wasm renderer into a site directory.
# usage: lab/build.sh [site-dir]   (default _site)
set -euo pipefail
cd "$(dirname "$0")/.."
out="${1:-_site}/lab"
mkdir -p "$out"
GOOS=js GOARCH=wasm CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o "$out/render.wasm" ./lab/wasm
root="$(go env GOROOT)"
# Go 1.24 moved wasm_exec.js to lib/wasm; older toolchains keep it in misc/wasm.
for f in "$root/lib/wasm/wasm_exec.js" "$root/misc/wasm/wasm_exec.js"; do
	if [[ -f "$f" ]]; then
		cp "$f" "$out/wasm_exec.js"
		echo "lab: $out/render.wasm ($(wc -c <"$out/render.wasm") bytes), wasm_exec.js from $f"
		exit 0
	fi
done
echo "lab: wasm_exec.js not found under $root" >&2
exit 1
