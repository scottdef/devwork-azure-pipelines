# Makefile: the one interface to build, test, package and install cos.
SHELL       := /bin/bash
.SHELLFLAGS := -euo pipefail -c
.DEFAULT_GOAL := help

GO      ?= go
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo 0.1.0-prototype)
LDFLAGS := -s -w -X main.version=$(VERSION)
DIST    := dist
BIN     ?= $(DIST)/cos
# linux/arm64: Pi 5, Pi 3 B+ (64-bit OS), Coral.  linux/arm: Pico i.MX7D,
# Pi Zero 2 W on a 32-bit OS.  linux/amd64: x86 mini PC or build host.
TARGETS := linux/arm64 linux/arm linux/amd64

.PHONY: help preflight fmt vet test race build cross checksums demo tools bom install clean

help: ## list targets
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  %-10s %s\n", $$1, $$2}'

preflight: ## check the Go version (1.22 to 1.24) and build tools
	@scripts/preflight.sh build

fmt: ## fail if any file is not gofmt-clean
	@out="$$(gofmt -l .)"; [[ -z "$$out" ]] || { echo "gofmt needed:"; echo "$$out"; exit 1; }

vet: ## go vet
	$(GO) vet ./...

test: fmt vet ## unit and end-to-end tests (converter tests skip if tesseract/poppler are absent)
	$(GO) test ./...

race: ## the same tests under the race detector
	$(GO) test -race ./...

build: ## build dist/cos for this machine
	CGO_ENABLED=0 $(GO) build -trimpath -ldflags '$(LDFLAGS)' -o $(DIST)/cos ./cmd/cos

cross: ## static binaries for every board: dist/cos-linux-{arm64,arm,amd64}
	@for t in $(TARGETS); do \
	  os=$${t%/*}; arch=$${t#*/}; \
	  echo "building $$os/$$arch"; \
	  GOOS=$$os GOARCH=$$arch GOARM=7 CGO_ENABLED=0 \
	    $(GO) build -trimpath -ldflags '$(LDFLAGS)' -o $(DIST)/cos-$$os-$$arch ./cmd/cos; \
	done

checksums: cross ## SHA256SUMS for the release binaries
	cd $(DIST) && sha256sum cos-linux-* > SHA256SUMS && sha256sum -c SHA256SUMS

demo: build ## run the whole staff with simulated tethers (needs tesseract, poppler, curl)
	@scripts/demo.sh

tools: build ## print the Needle tool schema generated from deploy/cos.json
	@$(DIST)/cos tools -config deploy/cos.json

bom: ## total the parts list against the 650 dollar budget
	@awk -F, 'NR>1{t=$$3*$$4; p[$$1]+=t; tot+=t} \
	  END{for(i=1;i<=3;i++) printf "phase %d: $$%.2f\n", i, p[i]; \
	      printf "parts: $$%.2f  +8%% tax/shipping: $$%.2f  unallocated of $$650: $$%.2f\n", tot, tot*1.08, 650-tot*1.08}' bom.csv

install: ## install on this node as root; BIN=path/to/prebuilt/cos (default dist/cos)
	@[[ -x $(BIN) ]] || { echo "no binary at $(BIN): run 'make build' or pass BIN=..."; exit 1; }
	install -m 0755 $(BIN) /usr/local/bin/cos
	install -d -m 0750 /etc/cos
	[[ -e /etc/cos/cos.json ]] || install -m 0640 deploy/cos.json /etc/cos/cos.json
	install -m 0644 deploy/cos.service deploy/cos-ears.service 'deploy/cos-runbook@.service' 'deploy/cos-runbook@.timer' /etc/systemd/system/
	install -m 0644 deploy/90-ukvm-tethers.rules /etc/udev/rules.d/
	install -D -m 0644 README.md /usr/local/share/doc/cos/README.md
	systemctl daemon-reload
	@echo "next: create /etc/cos/env (COS_TOKEN=...), edit /etc/cos/cos.json and the udev rules, then: scripts/preflight.sh chief"

clean: ## remove build output
	rm -rf $(DIST)
