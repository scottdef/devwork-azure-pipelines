# foundry-tui: one interface for every task. `make help` lists them.
SHELL       := bash
.SHELLFLAGS := -euo pipefail -c
.DEFAULT_GOAL := help

BIN      := bin/foundry-tui
PKG      := ./cmd/foundry-tui
VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS  := -s -w -X main.version=$(VERSION)
GOFLAGS  ?= -trimpath
DIST     := dist
REPO_DIR ?= .

.PHONY: help deps build install test race vet fmt fmt-check lint shellcheck actionlint \
        check run report probe perms audit scaffold sample preflight checksums verify dist clean

help: ## list targets
	@awk -F':.*## ' '/^[a-z-]+:.*## /{printf "  %-12s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

deps: ## resolve modules and write go.sum (run once, commit go.sum)
	go mod tidy
	go mod verify

build: ## build $(BIN)
	CGO_ENABLED=0 go build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o $(BIN) $(PKG)

install: ## go install into GOBIN
	CGO_ENABLED=0 go install $(GOFLAGS) -ldflags '$(LDFLAGS)' $(PKG)

test: ## unit tests (fakes only; no Azure, no GitHub)
	go test -count=1 ./...

race: ## unit tests under the race detector
	go test -race -count=1 ./...

vet: ## go vet
	go vet ./...

fmt: ## gofmt -w
	gofmt -w .

fmt-check: ## fail if gofmt would change anything
	@out=$$(gofmt -l .); if [[ -n "$$out" ]]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi

shellcheck: ## shellcheck scripts/
	shellcheck scripts/*.sh

actionlint: ## actionlint (also shellchecks every run: block)
	actionlint -color

lint: fmt-check vet shellcheck actionlint ## all static checks

check: lint race ## what CI runs

run: build ## start the screen
	$(BIN)

report: build ## write an HTML + JSON record into report_dir
	$(BIN) report

probe: build ## probe every chat deployment through the API
	$(BIN) test -mode api

perms: build ## print what Foundry Owner covers
	$(BIN) perms

audit: build ## verify the local audit chain
	$(BIN) audit -verify

scaffold: build ## copy the workflows into REPO_DIR (default .)
	$(BIN) scaffold -dir $(REPO_DIR)

sample: ## render the fixture report into examples/sample-report
	mkdir -p examples/sample-report
	FOUNDRY_SAMPLE_DIR=$(CURDIR)/examples/sample-report go test -count=1 -run TestWrite ./internal/report

preflight: ## check tools, logins and role before first use
	scripts/preflight.sh

checksums: ## write SHA256SUMS over tracked source
	@{ git ls-files 2>/dev/null || find . -type f -not -path './.git/*' -not -path './bin/*' -not -path './dist/*' | sed 's|^\./||'; } \
	  | grep -v '^SHA256SUMS$$' | LC_ALL=C sort | xargs sha256sum > SHA256SUMS
	@wc -l < SHA256SUMS | xargs echo "SHA256SUMS: files"

verify: ## verify SHA256SUMS
	sha256sum -c --quiet SHA256SUMS

dist: build checksums ## binary + checksum in dist/
	mkdir -p $(DIST)
	cp $(BIN) $(DIST)/
	cd $(DIST) && sha256sum foundry-tui > foundry-tui.sha256

clean: ## remove build output
	rm -rf bin $(DIST)
