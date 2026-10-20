#!/usr/bin/env bash
# preflight.sh: everything CI checks, runnable offline. Exit 0 means the
# repo is shippable: vet, gofmt, tests, script syntax, workflow sanity,
# cross builds for every target (including plan9/amd64), checksums.
set -euo pipefail
cd "$(dirname "$0")/.."
export GOFLAGS="${GOFLAGS:--mod=mod}" CGO_ENABLED=0
BINS=(issueparse validate dispatch pagegen)
TARGETS=(linux/amd64 linux/arm64 darwin/arm64 plan9/amd64)
step() { printf '== %s\n' "$*"; }

step "go version: $(go version)"
step vet
go vet ./...
step gofmt
bad="$(gofmt -l .)"
if [[ -n "$bad" ]]; then
	echo "gofmt needed:"
	echo "$bad"
	exit 1
fi
step test
go test -count=1 ./...
step "bash -n"
for f in scripts/*.sh lab/*.sh; do
	[[ -f "$f" ]] || continue
	bash -n "$f"
	head -1 "$f" | grep -qx '#!/usr/bin/env bash' || { echo "$f: wrong shebang"; exit 1; }
	grep -q 'set -euo pipefail' "$f" || { echo "$f: missing set -euo pipefail"; exit 1; }
done
step "workflow lint (grep-level)"
for f in .github/workflows/*.yml; do
	grep -q '^permissions:' "$f" || { echo "$f: no top-level permissions"; exit 1; }
	# Untrusted event text must arrive through env:, never inside run:.
	if grep -nE 'run:.*\$\{\{ *github\.event\.(issue|comment|client_payload)' "$f"; then
		echo "$f: event text interpolated into run:"
		exit 1
	fi
	if grep -nE 'uses: actions/(checkout|setup-go|configure-pages|upload-pages-artifact|deploy-pages|create-github-app-token)@' "$f" |
		grep -vE 'checkout@v6|setup-go@v6|configure-pages@v5|upload-pages-artifact@v5|deploy-pages@v5|create-github-app-token@v3'; then
		echo "$f: action major differs from BUILD-CONVENTIONS"
		exit 1
	fi
done
step "cross builds"
out="$(mktemp -d)"
trap 'rm -rf "$out"' EXIT
for t in "${TARGETS[@]}"; do
	for b in "${BINS[@]}"; do
		GOOS="${t%/*}" GOARCH="${t#*/}" go build -trimpath -ldflags="-s -w -X main.version=preflight" -o "$out/$b-${t%/*}-${t#*/}" "./cmd/$b"
	done
	echo "   $t ok"
done
(cd "$out" && sha256sum * >SHA256SUMS && sha256sum -c --quiet SHA256SUMS)
echo "   $(ls "$out" | grep -vc SHA256SUMS) binaries, checksums verified"
step "wasm lab (GOOS=js GOARCH=wasm)"
GOOS=js GOARCH=wasm go vet ./lab/wasm
lab/build.sh "$out/site" >/dev/null
echo "   render.wasm $(wc -c <"$out/site/lab/render.wasm") bytes + wasm_exec.js"
lab/wasm_test.sh
step "intake site renders"
go run ./cmd/pagegen -intake -o "$out/site" -time 2026-01-01T00:00:00Z
test -s "$out/site/index.html"
step "terraform"
if command -v terraform >/dev/null 2>&1 && [[ -d ../terraform ]]; then
	terraform -chdir=../terraform fmt -check && terraform -chdir=../terraform validate
else
	echo "   terraform not installed: skipping fmt/validate (see ../terraform/README.md)"
fi
echo "preflight: ok"
