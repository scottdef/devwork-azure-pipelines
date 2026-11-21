SHELL := /usr/bin/env bash
.SHELLFLAGS := -euo pipefail -c
export CGO_ENABLED := 0

.PHONY: preflight test bench fuzz probe verify sums clean

preflight:
	bash scripts/preflight.sh

test:
	go vet ./...
	go test -count=1 -timeout 120s ./...
	go test -count=1 -tags noasm ./kernels/ ./pool/

bench:
	go test -run '^$$' -bench . -benchtime=1s -count=3 ./kernels/

fuzz:
	go test -run '^$$' -fuzz FuzzParse -fuzztime 20s ./gguf/
	go test -run '^$$' -fuzz FuzzParse -fuzztime 20s ./ninep/
	go test -run '^$$' -fuzz FuzzSplitIsLossless -fuzztime 20s ./pretok/

probe:
	go run ./cmd/probe

verify:
	sha256sum -c SHA256SUMS

sums:
	find . -type f ! -name SHA256SUMS ! -path './.git/*' | LC_ALL=C sort | xargs sha256sum > SHA256SUMS

clean:
	go clean ./...
