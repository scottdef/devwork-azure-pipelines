# ado-process-gitops Makefile
# Go 1.21 | Azure DevOps Process GitOps

APP_NAME   := adoproc
BUILD_DIR  := bin
VERSION    := $(shell git describe --tags --always 2>/dev/null || echo "dev")
COMMIT     := $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE       := $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS    := -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

.PHONY: build clean test validate diff sync report export init fmt lint help

build:  ## Build the adoproc binary
	@mkdir -p $(BUILD_DIR)
	go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(APP_NAME) ./cmd/adoproc/
	@echo "Built $(BUILD_DIR)/$(APP_NAME)"

clean:  ## Remove build artifacts
	rm -rf $(BUILD_DIR) reports/

test:  ## Run Go tests
	go test -v ./...

fmt:  ## Format Go source
	gofmt -s -w .

lint:  ## Run go vet
	go vet ./...

validate:  ## Validate config for all environments
	$(BUILD_DIR)/$(APP_NAME) validate

validate-env:  ## Validate config for ENV (make validate-env ENV=dev)
	$(BUILD_DIR)/$(APP_NAME) validate --env $(ENV)

diff:  ## Show drift for ENV (make diff ENV=dev)
	$(BUILD_DIR)/$(APP_NAME) diff --env $(ENV)

sync:  ## Sync ENV (make sync ENV=dev)
	$(BUILD_DIR)/$(APP_NAME) sync --env $(ENV)

sync-dry:  ## Dry-run sync for ENV (make sync-dry ENV=dev)
	$(BUILD_DIR)/$(APP_NAME) sync --env $(ENV) --dry-run

report:  ## Generate HTML drift report
	$(BUILD_DIR)/$(APP_NAME) report --output reports/drift-report.html

export:  ## Export process (make export PROCESS=CoolADOAgile-Dev)
	$(BUILD_DIR)/$(APP_NAME) export --process $(PROCESS) --output processes-export

init:  ## Scaffold new config directory
	$(BUILD_DIR)/$(APP_NAME) init

help:  ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*##' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'
