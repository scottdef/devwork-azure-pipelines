#!/usr/bin/env bash
# preflight: everything CI checks, runnable offline on a laptop.
#   gofmt, go vet (host and every target), go test (with -race where
#   supported), generated docs up to date, bash -n on every script, JSON
#   configs parse, static cross-builds for all targets, SHA256SUMS.
# Exits non-zero on the first failure.
set -euo pipefail

cd "$(dirname "$0")/.."
export GOFLAGS="${GOFLAGS:--mod=mod}" GOPROXY="${GOPROXY:-off}" CGO_ENABLED="${CGO_ENABLED:-0}"
TARGETS=${TARGETS:-"linux/amd64 linux/arm64 darwin/arm64 plan9/amd64"}
VERSION=${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}
OUT=${OUT:-dist/preflight}

step() { printf '\n== %s\n' "$*"; }

step "go version"
go version

step "gofmt"
bad=$(gofmt -l .)
if [[ -n "$bad" ]]; then
	echo "gofmt needed:"; echo "$bad"; exit 1
fi
echo ok

step "go vet (host)"
go vet ./...
for t in $TARGETS; do
	step "go vet $t"
	GOOS=${t%/*} GOARCH=${t#*/} go vet ./...
done

step "go test"
if [[ "${PREFLIGHT_SKIP_TEST:-}" == 1 ]]; then
	echo "skipped (PREFLIGHT_SKIP_TEST=1)"
elif [[ "$(go env GOOS)" == linux ]] && CGO_ENABLED=1 go env CC >/dev/null 2>&1 && command -v "$(CGO_ENABLED=1 go env CC)" >/dev/null 2>&1; then
	CGO_ENABLED=1 go test -race -count=1 ./...
else
	echo "(race detector needs cgo on linux; running without -race)"
	go test -count=1 ./...
fi

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

step "generated docs"
go run ./internal/funcs/mkdoc -o "$work/FUNCS.md"
if ! cmp -s "$work/FUNCS.md" docs/FUNCS.md; then
	echo "docs/FUNCS.md is stale: run make docs"; exit 1
fi
echo ok

step "bash -n"
for f in scripts/*.sh; do
	bash -n "$f"
	echo "ok $f"
done

step "host binaries"
for bin in tmplwatch tpl; do
	go build -o "$work/$bin" "./cmd/$bin"
done
echo ok

step "JSON configs parse"
: >"$work/empty.tmpl"
for f in config/*.json; do
	"$work/tpl" -t "$work/empty.tmpl" -d "$f" >/dev/null
	echo "ok $f"
done

step "every template renders (-once -dry against testdata)"
"$work/tmplwatch" -c config/testdata.json -once -dry -a off >/dev/null
echo ok

step "terraform"
if command -v terraform >/dev/null 2>&1; then
	terraform -chdir=terraform fmt -check -recursive
	terraform -chdir=terraform init -backend=false -input=false >/dev/null
	terraform -chdir=terraform validate
else
	echo "terraform not installed: skipping fmt/validate (HCL is hand-formatted; CI has terraform)"
fi

step "cross-build ($VERSION)"
rm -rf "$OUT"
mkdir -p "$OUT"
for t in $TARGETS; do
	os=${t%/*} arch=${t#*/}
	for bin in tmplwatch tpl tmplload; do
		GOOS=$os GOARCH=$arch go build -trimpath -ldflags="-s -w -X main.version=$VERSION" \
			-o "$OUT/${bin}_${os}_${arch}" "./cmd/$bin"
	done
	echo "ok $t"
done
(cd "$OUT" && sha256sum * > SHA256SUMS && sha256sum -c --quiet SHA256SUMS)
ls -l "$OUT"

step "preflight passed"
