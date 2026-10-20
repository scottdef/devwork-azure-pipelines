#!/usr/bin/env bash
# preflight.sh: everything CI checks, runnable offline on a laptop.
# vet, gofmt, tests, generated-doc freshness, bash -n, cross-builds for
# every target, checksums of the cross-built binaries, and terraform when
# a terraform binary exists (it does not in the build sandbox: skipped).
set -euo pipefail

cd "$(dirname "$0")/.."
export GOFLAGS=-mod=mod GOPROXY=off CGO_ENABLED=0
PKGS=(./cmd/... ./internal/... ./catalog/...)
TARGETS=(linux/amd64 linux/arm64 darwin/arm64 plan9/amd64)
out=$(mktemp -d)
trap 'rm -rf "$out"' EXIT

step() { printf '== %s\n' "$*"; }

step "go version: $(go version)"

step "go vet (our packages; third_party is vendored upstream code)"
go vet "${PKGS[@]}"

step "gofmt"
bad=$(gofmt -l cmd internal catalog)
if [ -n "$bad" ]; then
	printf 'gofmt needed:\n%s\n' "$bad" >&2
	exit 1
fi

step "go test"
go test -count=1 "${PKGS[@]}"

if [ "${PREFLIGHT_RACE:-0}" = 1 ]; then
	step "go test -race"
	CGO_ENABLED=1 go test -race -count=1 "${PKGS[@]}"
fi

step "generated docs are current"
go run ./internal/funcs/gendoc -o "$out/FUNCS.md"
cmp -s "$out/FUNCS.md" docs/FUNCS.md || { echo "docs/FUNCS.md stale: make docs" >&2; exit 1; }
go run ./catalog/gendoc -o "$out/CATALOG.md"
cmp -s "$out/CATALOG.md" docs/CATALOG.md || { echo "docs/CATALOG.md stale: make docs" >&2; exit 1; }
go run ./cmd/tmplforge render -t catalog/CHANGELOG.md.tmpl -d docs/changelog.json > "$out/CHANGELOG.md"
cmp -s "$out/CHANGELOG.md" CHANGELOG.md || { echo "CHANGELOG.md stale: make docs" >&2; exit 1; }

step "bash -n scripts/*.sh"
for f in scripts/*.sh; do
	bash -n "$f"
done

step "vendored asset checksums"
(cd internal/web/static && sha256sum -c SHA256SUMS)

step "no network dependencies in go.mod"
if grep -E '^\s*require' -A20 go.mod | grep -vE 'go-echarts|^\s*\)|^\s*require|^--$|^\s*$' | grep -q .; then
	echo "unexpected requirement in go.mod" >&2
	exit 1
fi

step "cross-build ${TARGETS[*]}"
for t in "${TARGETS[@]}"; do
	os=${t%/*} arch=${t#*/}
	GOOS=$os GOARCH=$arch go build -trimpath -ldflags="-s -w -X main.version=preflight" \
		-o "$out/dist/tmplforge-$os-$arch" ./cmd/tmplforge
	printf '   %-14s %s bytes\n' "$t" "$(wc -c < "$out/dist/tmplforge-$os-$arch" | tr -d ' ')"
done
(cd "$out/dist" && sha256sum * > SHA256SUMS && sha256sum -c --quiet SHA256SUMS)
step "checksums ok"

step "smoke: built binary renders the catalog"
bin="$out/dist/tmplforge-$(go env GOOS)-$(go env GOARCH)"
if [ -x "$bin" ]; then
	"$bin" version
	"$bin" render -sandbox -t catalog/motd.txt.tmpl -d catalog/fixtures/motd.txt.json \
		| cmp -s - testdata/golden/catalog/motd.txt || { echo "smoke render differs from golden" >&2; exit 1; }
	printf 'c hello.go\nl\nq\n' | "$bin" sh >/dev/null
fi

if command -v terraform >/dev/null 2>&1; then
	step "terraform fmt + validate"
	terraform -chdir=terraform fmt -check
	terraform -chdir=terraform init -backend=false -input=false >/dev/null
	terraform -chdir=terraform validate
else
	step "terraform: not installed, skipping fmt/validate"
fi

step "preflight ok"
