#!/usr/bin/env bash
# preflight: everything that must be true before this module is committed.
# No network, no terraform: neither is needed here (the module has no
# dependencies and writes no HCL to disk), so there is nothing to skip.
set -euo pipefail
cd "$(dirname "$0")/.."

export GOFLAGS="${GOFLAGS:--mod=mod}"
export GOPROXY="${GOPROXY:-off}"
GO="${GO:-go}"
step() { printf '== %s\n' "$*"; }

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

step gofmt
unformatted=$(gofmt -l .)
if [ -n "$unformatted" ]; then
	printf 'gofmt needed:\n%s\n' "$unformatted" >&2
	exit 1
fi

step vet
"$GO" vet ./...

step test
"$GO" test ./...

step "fuzz smoke (3s)"
"$GO" test ./funcs -run '^$' -fuzz FuzzToYAMLNeverPanics -fuzztime 3s >/dev/null

step "bash -n"
for f in scripts/*.sh; do
	bash -n "$f"
done

step "cross-compile: linux/amd64 linux/arm64 darwin/arm64 plan9/amd64"
for t in linux/amd64 linux/arm64 darwin/arm64 plan9/amd64; do
	os=${t%/*}
	arch=${t#*/}
	CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" "$GO" build -trimpath \
		-ldflags="-s -w -X main.version=preflight" \
		-o "$work/cookbook-$os-$arch" ./cmd/cookbook
done
# the library and its tests must at least type-check for Plan 9 as well
GOOS=plan9 GOARCH=amd64 "$GO" vet ./...

step checksums
(cd "$work" && sha256sum cookbook-* >SHA256SUMS && cat SHA256SUMS)

step "docs are deterministic"
"$GO" run ./cmd/cookbook gen -o "$work/a.md" >/dev/null
"$GO" run ./cmd/cookbook gen -o "$work/b.md" >/dev/null
cmp "$work/a.md" "$work/b.md"
"$GO" run ./cmd/cookbook gen -funcs -o "$work/fa.md" >/dev/null
"$GO" run ./cmd/cookbook gen -funcs -o "$work/fb.md" >/dev/null
cmp "$work/fa.md" "$work/fb.md"

step "every formula equals its Want, testdata/out and the committed docs"
if [ -d ../docs ]; then
	"$GO" run ./cmd/cookbook check -docs ../docs
else
	"$GO" run ./cmd/cookbook check
fi

echo "preflight ok"
