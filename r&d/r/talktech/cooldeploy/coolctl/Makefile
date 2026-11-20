# coolctl: the Go tools of CoolDeploy. Standard library only.
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
BIN     ?= bin
REGISTRY ?= prdeus2coolopsacr001
MAJOR   ?= $(firstword $(subst ., ,$(VERSION)))
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: all build test vet fmt check images clean

all: check build

build:
	CGO_ENABLED=0 go build -trimpath -ldflags="$(LDFLAGS)" -o $(BIN)/ ./cmd/...

test:
	go test ./...

vet:
	go vet ./...

fmt:
	@test -z "$$(gofmt -l .)" || { echo "gofmt needed:"; gofmt -l .; exit 1; }

# check also proves the module has no dependencies: an empty module graph.
check: fmt vet test
	@test "$$(go list -m all | wc -l)" -eq 1 || { echo "coolctl must not depend on other modules"; go list -m all; exit 1; }

# Each image gets its full version and its major version as tags. Charts and
# Dockerfiles name the major tag, so a patch release needs no other change.
images:
	az acr build -r $(REGISTRY) -t platform/cool-api:$(VERSION)    -t platform/cool-api:$(MAJOR)    --target cool-api    --build-arg VERSION=$(VERSION) .
	az acr build -r $(REGISTRY) -t platform/coolctl:$(VERSION)     -t platform/coolctl:$(MAJOR)     --target coolctl     --build-arg VERSION=$(VERSION) .
	az acr build -r $(REGISTRY) -t platform/cool-static:$(VERSION) -t platform/cool-static:$(MAJOR) --target cool-static --build-arg VERSION=$(VERSION) .
	az acr build -r $(REGISTRY) -t sim/appsim:$(VERSION)           -t sim/appsim:$(MAJOR)           --target appsim      --build-arg VERSION=$(VERSION) .

clean:
	rm -rf $(BIN)
