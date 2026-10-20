# Makefile for issueops-intake. Plan 9 users: see mkfile.
GO      ?= go
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
BINS    := issueparse validate dispatch pagegen
TARGETS := linux/amd64 linux/arm64 darwin/arm64 plan9/amd64
LDFLAGS := -s -w -X main.version=$(VERSION)
export GOFLAGS ?= -mod=mod
export CGO_ENABLED := 0

.PHONY: all build test vet lint cross release loadtest clean preflight site wasm fuzz

all: vet test build

build: $(addprefix bin/,$(BINS))

bin/%: FORCE
	$(GO) build -trimpath -ldflags="$(LDFLAGS)" -o $@ ./cmd/$*

FORCE:

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

lint: vet
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi
	@for f in scripts/*.sh lab/*.sh; do [ -f "$$f" ] && bash -n "$$f"; done; true

cross:
	@for t in $(TARGETS); do \
		os=$${t%/*}; arch=$${t#*/}; \
		for b in $(BINS); do \
			GOOS=$$os GOARCH=$$arch $(GO) build -trimpath -ldflags="$(LDFLAGS)" -o dist/$$b-$$os-$$arch ./cmd/$$b || exit 1; \
		done; \
		echo "cross: $$t ok"; \
	done

release: clean cross
	cd dist && sha256sum * > SHA256SUMS

preflight:
	scripts/preflight.sh

loadtest:
	@echo "intake is a static page; load-test the library site (issueops-library: make loadtest)"

# site renders the intake Pages site into _site (and the wasm lab).
site: bin/pagegen
	bin/pagegen -intake -flags flags.json -o _site
	lab/build.sh _site

wasm:
	GOOS=js GOARCH=wasm $(GO) build -trimpath -o /dev/null ./lab/wasm

fuzz:
	$(GO) test ./internal/issuebody -run "^$$" -fuzz FuzzParse -fuzztime 30s

clean:
	rm -rf bin dist _site site.tmp
