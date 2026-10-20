# Go templates case-study collection — unified interface.
# Every target shells out to the per-repo Makefiles; this file only orchestrates.

CASES   := case-studies/01-tmplwatch case-studies/02-issueops-library case-studies/03-tmplforge
MODULES := cookbook docs/verify $(CASES)
GOFLAGS ?= -mod=mod
export GOFLAGS
export GOPROXY ?= off

.PHONY: all build test vet preflight cookbook docs checksums package clean verify

all: preflight

build:
	@for d in $(CASES); do echo "== $$d"; $(MAKE) -C $$d build; done

test:
	@for d in $(CASES); do echo "== $$d"; $(MAKE) -C $$d test; done
	@cd cookbook && go test ./...

vet:
	@for d in $(CASES); do echo "== $$d"; $(MAKE) -C $$d vet; done
	@cd cookbook && go vet ./...

preflight:
	@./scripts/preflight.sh

cookbook:
	@cd cookbook && go run ./cmd/cookbook gen

docs: cookbook
	@cd docs/verify && go run . > expected-output.txt

checksums:
	@./scripts/checksums.sh

package: checksums
	@./scripts/package.sh

verify:
	@sha256sum -c SHA256SUMS

clean:
	@for d in $(CASES); do $(MAKE) -C $$d clean; done
	@rm -f go-templates-casestudies.zip SHA256SUMS
