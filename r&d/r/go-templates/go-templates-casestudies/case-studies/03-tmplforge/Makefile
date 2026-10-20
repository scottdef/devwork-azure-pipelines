# tmplforge Makefile. Recipes are indented with TABS.
# Offline by design: GOFLAGS=-mod=mod GOPROXY=off (see BUILD-CONVENTIONS.md).

BIN      = tmplforge
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS  = -s -w -X main.version=$(VERSION)
TARGETS  = linux/amd64 linux/arm64 darwin/arm64 plan9/amd64
PKGS     = ./cmd/... ./internal/... ./catalog/...
GO       = GOFLAGS=-mod=mod GOPROXY=off go

all: vet test build

build:
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags="$(LDFLAGS)" -o bin/$(BIN) ./cmd/$(BIN)
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags="-s -w" -o bin/forgeload ./cmd/forgeload

test:
	$(GO) test $(PKGS)

race:
	$(GO) test -race $(PKGS)

vet:
	$(GO) vet $(PKGS)

lint: vet
	@out=$$(gofmt -l cmd internal catalog); if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi
	@for f in scripts/*.sh; do bash -n "$$f" || exit 1; done
	$(GO) run ./cmd/$(BIN) lint -t catalog/Dockerfile.tmpl >/dev/null

fuzz:
	$(GO) test ./internal/forge -run "^$$" -fuzz FuzzLint -fuzztime 30s
	$(GO) test ./internal/forge -run "^$$" -fuzz FuzzSandbox -fuzztime 30s

cross:
	@mkdir -p dist
	@for t in $(TARGETS); do \
		os=$${t%/*}; arch=$${t#*/}; \
		echo "build $$os/$$arch"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch $(GO) build -trimpath -ldflags="$(LDFLAGS)" -o dist/$(BIN)-$$os-$$arch ./cmd/$(BIN) || exit 1; \
	done

release: clean cross
	cd dist && sha256sum * > SHA256SUMS

golden:
	$(GO) test ./... -run "Golden" -update

docs:
	$(GO) generate ./internal/funcs
	$(GO) run ./catalog/gendoc -o docs/CATALOG.md
	$(GO) run ./cmd/$(BIN) render -t catalog/CHANGELOG.md.tmpl -d docs/changelog.json > CHANGELOG.md
	cp CHANGELOG.md docs/CHANGELOG.md

loadtest: build
	./scripts/loadtest.sh

preflight:
	./scripts/preflight.sh

clean:
	rm -rf bin dist

.PHONY: all build test race vet lint fuzz cross release golden docs loadtest preflight clean
