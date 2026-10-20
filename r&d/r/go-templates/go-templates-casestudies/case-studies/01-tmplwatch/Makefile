# tmplwatch: see README.md. Plan 9 users: see mkfile.
# GNU make (uses ?=, $(shell) and export).

GO       ?= go
VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS  := -s -w -X main.version=$(VERSION)
TARGETS  := linux/amd64 linux/arm64 darwin/arm64 plan9/amd64
BINS     := tmplwatch tpl tmplload
DIST     := dist
export GOFLAGS ?= -mod=mod
export GOPROXY ?= off
export CGO_ENABLED ?= 0

.PHONY: all build test race vet lint fmt docs golden fuzz cross release preflight run loadtest docker clean

all: vet test build

build:
	@mkdir -p bin
	for b in $(BINS); do $(GO) build -trimpath -ldflags="$(LDFLAGS)" -o bin/$$b ./cmd/$$b || exit 1; done

test:
	$(GO) test -count=1 ./...

race:
	CGO_ENABLED=1 $(GO) test -race -count=1 ./...

vet:
	$(GO) vet ./...
	for t in $(TARGETS); do GOOS=$${t%/*} GOARCH=$${t#*/} $(GO) vet ./... || exit 1; done

lint: vet
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi
	for f in scripts/*.sh; do bash -n $$f || exit 1; done

fmt:
	gofmt -w .

docs:
	$(GO) generate ./internal/funcs

golden:
	$(GO) test ./templates -update
	git diff --stat -- testdata/golden 2>/dev/null || true

fuzz:
	$(GO) test ./templates -run '^$$' -fuzz FuzzRender -fuzztime 60s

cross:
	@mkdir -p $(DIST)
	for t in $(TARGETS); do \
		for b in $(BINS); do \
			GOOS=$${t%/*} GOARCH=$${t#*/} $(GO) build -trimpath -ldflags="$(LDFLAGS)" \
				-o $(DIST)/$${b}_$${t%/*}_$${t#*/} ./cmd/$$b || exit 1; \
		done; \
	done

release: clean cross
	cd $(DIST) && sha256sum * > SHA256SUMS

preflight:
	scripts/preflight.sh

run: build
	bin/tmplwatch -c config/testdata.json -v

loadtest:
	scripts/loadtest.sh

docker:
	docker build --build-arg VERSION=$(VERSION) -t tmplwatch:$(VERSION) .

clean:
	rm -rf bin $(DIST) out
