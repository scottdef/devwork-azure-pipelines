# Makefile for issueops-library. Plan 9 users: see mkfile.
GO      ?= go
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
BINS    := pagegen ledger tpl siteload
TARGETS := linux/amd64 linux/arm64 darwin/arm64 plan9/amd64
LDFLAGS := -s -w -X main.version=$(VERSION)
export GOFLAGS ?= -mod=mod
export CGO_ENABLED := 0

.PHONY: all build test vet lint cross release loadtest clean preflight site-sample fuzz

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

loadtest: bin/siteload
	scripts/loadtest.sh

# site-sample renders testdata/events/*.json into site-sample/ for review.
site-sample: build
	scripts/site-sample.sh

fuzz:
	$(GO) test ./internal/event -run "^$$" -fuzz FuzzDecode -fuzztime 30s

clean:
	rm -rf bin dist _site site.tmp
