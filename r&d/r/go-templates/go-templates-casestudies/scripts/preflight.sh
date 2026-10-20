#!/usr/bin/env bash
# Collection-wide preflight: runs each repo's own preflight, then the cookbook and the
# cheat-sheet verifier. Exits non-zero on the first failure. Works offline.
set -euo pipefail
cd "$(dirname "$0")/.."
export GOFLAGS=-mod=mod GOPROXY=off CGO_ENABLED=0

fail=0
for d in case-studies/01-tmplwatch case-studies/02-issueops-library case-studies/03-tmplforge; do
	echo "=== $d"
	if [ -x "$d/scripts/preflight.sh" ]; then
		(cd "$d" && ./scripts/preflight.sh) || { echo "FAIL: $d"; fail=1; }
	else
		echo "missing $d/scripts/preflight.sh"; fail=1
	fi
done

echo "=== cookbook"
(cd cookbook && go vet ./... && go test ./... && go run ./cmd/cookbook check) || { echo "FAIL: cookbook"; fail=1; }

echo "=== docs/verify"
(cd docs/verify && go run . | diff -u expected-output.txt -) || { echo "FAIL: cheat-sheet outputs drifted"; fail=1; }

echo "=== shell syntax"
while IFS= read -r f; do bash -n "$f" || { echo "FAIL: bash -n $f"; fail=1; }; done < <(find . -name '*.sh' -not -path '*/third_party/*')

echo "=== vendored checksums"
(cd third_party/htmx && sha256sum -c SHA256SUMS) || fail=1

if command -v terraform >/dev/null 2>&1; then
	while IFS= read -r d; do
		(cd "$d" && terraform fmt -check -recursive && terraform init -backend=false -input=false >/dev/null && terraform validate) || { echo "FAIL: terraform $d"; fail=1; }
	done < <(find . -type d -name terraform -not -path '*/third_party/*')
else
	echo "terraform not installed: skipping terraform fmt/validate (CI runs them)"
fi

[ "$fail" -eq 0 ] && echo "PREFLIGHT OK" || { echo "PREFLIGHT FAILED"; exit 1; }
