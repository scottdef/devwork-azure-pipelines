# ghlog2dt - GitHub Actions job logs -> Dynatrace Log Monitoring API v2.
# One interface for humans and CI: `make help`.

SHELL       := bash
.SHELLFLAGS := -euo pipefail -c
.DEFAULT_GOAL := help
MAKEFLAGS   += --no-builtin-rules --warn-undefined-variables

BIN       := ghlog2dt
PKG       := ./cmd/ghlog2dt
VERSION   ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT    ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
LDFLAGS   := -s -w -buildid= -X main.version=$(VERSION) -X main.commit=$(COMMIT)
GOFLAGS   := -trimpath
PLATFORMS := linux/amd64 linux/arm64 darwin/arm64 darwin/amd64 windows/amd64
MAX_GO_MINOR := 24
SCRIPTS   := $(wildcard scripts/*.sh)
TF_DIR    := terraform
REPO      ?=
INTERVAL  ?= 15
OFFSET    ?= 7

export CGO_ENABLED := 0

.PHONY: help
help: ## list targets
	@awk 'BEGIN{FS=":.*## "} /^[a-zA-Z0-9_-]+:.*## /{printf "  \033[1m%-14s\033[0m %s\n",$$1,$$2}' $(MAKEFILE_LIST)

.PHONY: preflight
preflight: ## Go <= 1.24 and stdlib-only dependencies
	@v="$$(go env GOVERSION)"; minor="$$(sed -E 's/^go1\.([0-9]+).*/\1/' <<<"$$v")"; \
	  (( minor <= $(MAX_GO_MINOR) )) || { echo "::error::$$v > go1.$(MAX_GO_MINOR) (project cap)"; exit 2; }; \
	  echo "toolchain $$v ok"
	@grep -q '^go 1\.2[0-4]' go.mod || { echo "::error::go.mod go directive must be <= 1.24"; exit 2; }
	@deps="$$(go list -m all | tail -n +2)"; \
	  allowed='^(github\.com/go-echarts/|upspin\.io/|github\.com/upspin/)'; \
	  bad="$$(grep -Ev "$$allowed" <<<"$$deps" || true)"; \
	  [[ -z "$$bad" ]] || { echo "::error::dependencies outside the allow-list:"; echo "$$bad"; exit 2; }; \
	  echo "dependencies ok (standard library only)"

.PHONY: fmt
fmt: ## gofmt -w
	gofmt -w cmd internal

.PHONY: lint
lint: ## gofmt check, go vet, bash -n, shellcheck (if installed)
	@out="$$(gofmt -l cmd internal)"; [[ -z "$$out" ]] || { echo "gofmt needed:"; echo "$$out"; exit 1; }
	go vet ./...
	@for s in $(SCRIPTS); do bash -n "$$s"; done; echo "bash -n ok"
	@if command -v shellcheck >/dev/null; then shellcheck -x -S warning $(SCRIPTS) && echo "shellcheck ok"; \
	  else echo "shellcheck not installed; skipped"; fi

.PHONY: test
test: ## unit + integration tests (race detector when a C compiler exists)
	@if command -v gcc >/dev/null 2>&1; then CGO_ENABLED=1 go test -race -count=1 ./...; \
	  else echo "no C compiler: running without -race"; go test -count=1 ./...; fi

.PHONY: cover
cover: ## coverage report
	go test -count=1 -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out | tail -n 1

.PHONY: build
build: ## build bin/ghlog2dt for this machine
	go build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o bin/$(BIN) $(PKG)

.PHONY: install
install: build ## install into $$GOBIN or /usr/local/bin
	install -m 0755 bin/$(BIN) "$${GOBIN:-/usr/local/bin}/$(BIN)"

.PHONY: release
release: ## cross-compile static binaries into dist/
	@rm -rf dist && mkdir -p dist
	@for p in $(PLATFORMS); do \
	  os="$${p%/*}"; arch="$${p#*/}"; ext=""; [[ "$$os" == windows ]] && ext=.exe; \
	  GOOS="$$os" GOARCH="$$arch" go build $(GOFLAGS) -ldflags '$(LDFLAGS)' -o "dist/$(BIN)_$${os}_$${arch}$$ext" $(PKG); \
	  echo "built dist/$(BIN)_$${os}_$${arch}$$ext"; \
	done

.PHONY: checksums
checksums: ## dist/SHA256SUMS
	cd dist && sha256sum $(BIN)_* > SHA256SUMS && cat SHA256SUMS

.PHONY: verify
verify: ## verify dist/ against SHA256SUMS
	cd dist && sha256sum --check --strict SHA256SUMS

.PHONY: dry-run
dry-run: build ## REPO=owner/name: print records without shipping (needs GH_TOKEN)
	@[[ -n "$(REPO)" ]] || { echo "usage: make dry-run REPO=CoolGitOrg/app-a"; exit 2; }
	: "$${GH_TOKEN:?GH_TOKEN must be set}"
	bin/$(BIN) run --repo "$(REPO)" --dry-run --log-format text --state .state-dryrun | head -n 20

.PHONY: schedule
schedule: build ## INTERVAL=15 OFFSET=7: show cron + lookback and rewrite forward.yml
	bin/$(BIN) schedule --interval $(INTERVAL) --offset $(OFFSET) --rewrite .github/workflows/forward.yml

.PHONY: bootstrap-state
bootstrap-state: ## create the orphan forwarder-state branch on origin
	scripts/bootstrap-state-branch.sh origin forwarder-state

.PHONY: spike
spike: build ## Phase-0 probes against a non-prod tenant (DT_URL, DT_LOGS_TOKEN)
	PATH="$(CURDIR)/bin:$$PATH" scripts/phase0-spike.sh

.PHONY: tf-fmt
tf-fmt: ## terraform fmt -check
	terraform -chdir=$(TF_DIR) fmt -check -recursive

.PHONY: tf-validate
tf-validate: ## terraform init -backend=false && validate
	terraform -chdir=$(TF_DIR) init -backend=false -input=false >/dev/null
	terraform -chdir=$(TF_DIR) validate

.PHONY: ci
ci: preflight lint test build ## everything CI runs for Go and shell

.PHONY: clean
clean: ## remove build output
	rm -rf bin dist coverage.out .state-dryrun
