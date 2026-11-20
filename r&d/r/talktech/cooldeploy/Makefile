# CoolDeploy: every repository of the platform, side by side.
#
#   make demo     the whole story on this machine, no Azure and no GitHub
#   make test     every check that needs no cloud
#   make package  a zip of the tree
#
# Needs Go 1.24, git, curl and jq. shellcheck, actionlint, kubeconform, helm
# and terraform are used when they are installed and skipped when not.
SHELL := /bin/bash
BIN   := $(CURDIR)/coolctl/bin
export PATH := $(BIN):$(PATH)

GO_MODULES := coolctl examples/checkout cool-template-go-api cool-template-go-htmx
WORKFLOWS  := $(wildcard */.github/workflows/*.yml examples/*/.github/workflows/*.yml)
TF         ?= $(shell command -v terraform >/dev/null && echo terraform || echo tofu)

.PHONY: all build test go charts gitops apim infra issueops lint demo seed package clean

all: test

build:
	cd coolctl && CGO_ENABLED=0 go build -trimpath -o $(BIN)/ ./cmd/...

test: go charts gitops apim infra issueops lint
	@echo
	@echo "CoolDeploy: every check passed."

# Format, vet and test each Go module, and prove none has a dependency.
go:
	@for m in $(GO_MODULES); do \
		echo "== go: $$m"; \
		( cd $$m && test -z "$$(gofmt -l .)" && go vet ./... && go test ./... \
		  && test "$$(go list -m all | wc -l)" -eq 1 ) || { echo "FAIL  $$m"; exit 1; }; \
	done

charts: build
	@echo "== cool-charts"
	@cd cool-charts && test/render.sh

gitops: build
	@echo "== cool-gitops"
	@cd cool-gitops && CHARTS=../cool-charts test/check.sh

apim:
	@echo "== cool-apim"
	@cd cool-apim && test/check.sh

infra:
	@echo "== cool-infra"
	@cd cool-infra && TF=$(TF) scripts/check.sh

issueops: build
	@echo "== issueops-additions"
	@cd issueops-additions && test/check.sh

lint:
	@echo "== lint"
	@if command -v shellcheck >/dev/null; then \
		shellcheck -x demo/*.sh && \
		( cd cool-charts && shellcheck -x test/*.sh ) && \
		( cd cool-gitops && shellcheck -x test/*.sh bootstrap/*.sh ) && \
		( cd cool-apim && shellcheck -x scripts/*.sh test/az test/check.sh ) && \
		( cd cool-infra && shellcheck -x scripts/*.sh ) && \
		( cd issueops-additions && shellcheck -x scripts/*.sh scripts/cool/*.sh test/check.sh ) && \
		echo "ok    shellcheck"; \
	else echo "skip  shellcheck not found"; fi
	@if command -v actionlint >/dev/null; then actionlint $(WORKFLOWS) && echo "ok    actionlint ($(words $(WORKFLOWS)) workflows)"; else echo "skip  actionlint not found"; fi
	@for f in cool-template-static-js/js/*.js cool-portal/site/js/portal.js cool-portal/site/js/auth.js cool-portal/site/js/config.js; do \
		if command -v node >/dev/null; then node --check $$f || exit 1; fi; \
	done; echo "ok    javascript parses"

demo: build
	demo/run-local.sh

# seed regenerates the example apps in cool-gitops/apps through cool-api.
seed: build
	demo/seed.sh

package: clean
	rm -f ../cooldeploy.zip
	cd .. && zip -qr cooldeploy.zip cooldeploy -x '*/.terraform/*' -x '*/bin/coolctl*' -x '*.tfstate*'
	@echo "wrote $$(cd .. && pwd)/cooldeploy.zip"

clean:
	rm -rf $(BIN) cool-charts/rendered
	find . -name .terraform -type d -prune -exec rm -rf {} +
