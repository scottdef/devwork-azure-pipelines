#!/usr/bin/env bash
# Preflight for lm9 spike 0. Every check the plan calls a "gate" starts life as
# a line in a script like this one.
set -euo pipefail

cd "$(dirname "$0")/.."
export CGO_ENABLED=0

step() { printf '\n== %s\n' "$*"; }

step "toolchain"
go version

step "vet (includes the assembly frame-size check)"
go vet ./...

step "formatting"
test -z "$(gofmt -l .)" || { gofmt -l .; echo "gofmt needed" >&2; exit 1; }

step "tests on the host"
go test -count=1 -timeout 120s ./...

step "tests again with the portable tier forced (-tags noasm)"
go test -count=1 -tags noasm ./kernels/ ./pool/

step "cross-compile matrix, cgo off"
for t in linux/amd64 linux/arm64 darwin/arm64 windows/amd64 plan9/amd64 freebsd/amd64 js/wasm; do
  GOOS=${t%/*} GOARCH=${t#*/} go build ./... && printf '  %-14s ok\n' "$t"
done

step "no cgo and no dynamic loader in the CPU-only build"
out=$(mktemp -d)
trap 'rm -rf "$out"' EXIT
go build -trimpath -ldflags='-s -w' -o "$out/fmacheck" ./cmd/fmacheck
if command -v file >/dev/null; then file "$out/fmacheck" | sed 's/^[^:]*: /  /'; fi
if go version -m "$out/fmacheck" | grep -q 'CGO_ENABLED=1'; then
  echo "cgo is enabled" >&2
  exit 1
fi

step "floating-point fusion depends on the architecture"
# Reads the compiler's own output, so it needs no ARM machine: a fused
# multiply-add shows up as FMADDD (arm64) or VFMADD* (amd64).
fused() { # $1=GOARCH $2=function
  GOARCH=$1 go build -o /dev/null -gcflags=-S ./cmd/fmacheck 2>&1 |
    awk -v f="main.$2 STEXT" 'index($0,f){on=1;next} / STEXT /{on=0} on' |
    grep -cE 'FMADD' || true
}
for arch in amd64 arm64; do
  printf '  %-6s x*y+z fused: %s   float64(x*y)+z fused: %s\n' "$arch" \
    "$([ "$(fused "$arch" plain)" -gt 0 ] && echo yes || echo no)" \
    "$([ "$(fused "$arch" pinned)" -gt 0 ] && echo yes || echo no)"
done
[ "$(fused arm64 pinned)" -eq 0 ] || { echo "explicit conversion did not prevent fusion" >&2; exit 1; }
printf '  host run: '; go run ./cmd/fmacheck | head -1

printf '\npreflight passed\n'
