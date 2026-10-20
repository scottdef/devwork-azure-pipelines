# Makefile for the Go templates cookbook. Recipe lines start with a TAB.
GO       ?= go
VERSION  ?= $(shell git describe --tags --always 2>/dev/null || echo dev)
LDFLAGS  := -s -w -X main.version=$(VERSION)
TARGETS  := linux/amd64 linux/arm64 darwin/arm64 plan9/amd64
DOCS     := ../docs

# No module proxy in the build environment; the module has no dependencies.
export GOFLAGS ?= -mod=mod
export GOPROXY ?= off
export CGO_ENABLED = 0

.PHONY: all build test vet lint cross release loadtest docs check update preflight clean

all: vet test build

build:
	mkdir -p bin
	$(GO) build -trimpath -ldflags="$(LDFLAGS)" -o bin/cookbook ./cmd/cookbook

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

lint: vet
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi
	@for f in scripts/*.sh; do bash -n "$$f" || exit 1; done

cross:
	@mkdir -p dist
	@for t in $(TARGETS); do \
		os=$${t%/*}; arch=$${t#*/}; \
		echo "build $$os/$$arch"; \
		GOOS=$$os GOARCH=$$arch $(GO) build -trimpath -ldflags="$(LDFLAGS)" -o dist/cookbook-$$os-$$arch ./cmd/cookbook || exit 1; \
	done

release: cross
	cd dist && rm -f SHA256SUMS && sha256sum * > SHA256SUMS
	@echo 'publish with: gh release create "$$TAG" dist/* --verify-tag'

# The only load this module has is rendering every formula; measure that.
loadtest:
	$(GO) test ./formulas -run XXX_NONE -bench BenchmarkAll -benchmem -count 3

# docs regenerates the Markdown from the formulas. Never edit the output.
docs:
	$(GO) run ./cmd/cookbook gen -o $(DOCS)/COOKBOOK.md
	$(GO) run ./cmd/cookbook gen -funcs -o $(DOCS)/FUNCS-SHARED.md

check:
	$(GO) run ./cmd/cookbook check -docs $(DOCS)

# update rewrites testdata/out/*.txt and the docs after you change a formula
# AND have changed its Want to match.
update:
	$(GO) test ./formulas -update
	$(MAKE) docs

preflight:
	./scripts/preflight.sh

clean:
	rm -rf bin dist
