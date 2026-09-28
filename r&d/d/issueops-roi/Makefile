# CoolEngOrg IssueOps platform — developer tasks.
# Requires Go 1.21 (go.mod) and, for validator tests, Node 22+.

GO        ?= go
ISSUEOPS  ?= bin/issueops
VERSION   ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS   := -s -w -X main.Version=$(VERSION)
SCENARIOS := $(wildcard examples/*/scenario*.json)

.PHONY: all build test vet fmt lint forms check-forms examples fixtures validators runner-image clean help

all: fmt vet test build ## Format, vet, test and build

build: ## Build bin/issueops and bin/agent-runner
	$(GO) build -trimpath -ldflags "$(LDFLAGS)" -o bin/issueops ./cmd/issueops
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o bin/agent-runner ./cmd/agent-runner

test: ## Unit tests + end-to-end scenarios
	$(GO) test -count=1 ./...

vet: ## go vet
	$(GO) vet ./...

fmt: ## gofmt
	gofmt -w ./cmd ./internal

lint: ## actionlint (docker)
	docker run --rm -v "$(CURDIR):/repo" -w /repo rhysd/actionlint:1.7.12 -color

forms: ## Convert .github/ISSUE_TEMPLATE/*.yml to config/forms/*.json
	scripts/forms-to-json.sh

check-forms: build ## Verify forms against the registry
	$(ISSUEOPS) check-forms

validators: ## Run custom validator tests
	node --test '.github/validator/tests/*.test.js'

fixtures: ## Regenerate the Copilot usage-metrics fixtures
	$(GO) run examples/copilot-usage-report/fixtures/generate.go -out examples/copilot-usage-report/fixtures -start 2026-08-17 -end 2026-09-20

examples: build ## Re-run every example scenario offline into examples/<type>/output/<scenario>/
	@set -e; for s in $(SCENARIOS); do \
	  dir=$$(dirname $$s); name=$$(basename $$s .json); out=$$dir/output/$$name; \
	  rm -rf $$out; mkdir -p $$out; \
	  $(ISSUEOPS) simulate --scenario $$s --out $$out; \
	done

runner-image: ## Build the agent-runner image locally
	docker build -f deploy/agent-runner/Dockerfile --build-arg VERSION=$(VERSION) -t issueops/agent-runner:$(VERSION) .

clean: ## Remove build output
	rm -rf bin out

help: ## Show targets
	@grep -E '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  %-14s %s\n", $$1, $$2}'
