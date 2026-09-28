# foundry-byok - standard library only, Go 1.21
BINARY   := foundry-byok
PKG      := ./cmd/foundry-byok
VERSION  ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS  := -s -w -X main.version=$(VERSION)
INVENTORY ?= examples/inventory.json
OUT       ?= examples/output
IMAGE     ?= foundry-byok:$(VERSION)

export CGO_ENABLED ?= 0
# Never let a newer toolchain be downloaded behind your back: the project is
# pinned to Go 1.21 and must build with it.
export GOTOOLCHAIN ?= local

.PHONY: all build test race lint fmt generate check validate dist docker clean help

all: lint test build ## lint, test and build

build: ## build bin/foundry-byok
	go build -trimpath -ldflags '$(LDFLAGS)' -o bin/$(BINARY) $(PKG)

test: ## run unit tests
	go test ./...

race: ## run unit tests with the race detector (needs cgo)
	CGO_ENABLED=1 go test -race ./...

lint: ## gofmt + go vet
	@unformatted="$$(gofmt -l .)"; if [ -n "$$unformatted" ]; then echo "gofmt needed:"; echo "$$unformatted"; exit 1; fi
	go vet ./...

fmt: ## format the code
	gofmt -w cmd internal

validate: build ## validate $(INVENTORY)
	bin/$(BINARY) validate --inventory $(INVENTORY)

generate: build ## render $(INVENTORY) into $(OUT)
	bin/$(BINARY) generate --inventory $(INVENTORY) --out $(OUT)

check: build ## fail (exit 3) if $(OUT) has drifted from $(INVENTORY)
	bin/$(BINARY) generate --inventory $(INVENTORY) --out $(OUT) --check

dist: ## cross-compile release binaries into dist/
	@mkdir -p dist
	@for t in linux/amd64 linux/arm64 darwin/arm64 windows/amd64; do \
	  os=$${t%/*}; arch=$${t#*/}; ext=; [ "$$os" = windows ] && ext=.exe; \
	  echo "  $$os/$$arch"; \
	  GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags '$(LDFLAGS)' \
	    -o dist/$(BINARY)-$$os-$$arch$$ext $(PKG) || exit 1; \
	done
	@cd dist && sha256sum $(BINARY)-* > SHA256SUMS

docker: ## build the container image
	docker build --build-arg VERSION=$(VERSION) -t $(IMAGE) .

clean: ## remove build output
	rm -rf bin dist

help: ## show this help
	@grep -E '^[a-z]+:.*## ' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-10s %s\n", $$1, $$2}'
