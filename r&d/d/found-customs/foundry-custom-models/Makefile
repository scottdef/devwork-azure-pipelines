# foundry-custom-models: one interface. `make help` lists targets.
SHELL       := bash
.SHELLFLAGS := -euo pipefail -c
.DEFAULT_GOAL := help

EX       ?= gemma4
FTM      := shared/bin/ftm
PY       ?= python3
ZIP      := foundry-custom-models.zip

.PHONY: help plan deploy smoke promote rollback connect info destroy data test lint ruff shellcheck \
        actionlint check bootstrap preflight checksums verify zip clean

help: ## list targets (EX=gemma4|qwen38|muse|wham)
	@awk -F':.*## ' '/^[a-z-]+:.*## /{printf "  %-12s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

plan: ## render and print the plan for EX (offline)
	$(FTM) plan $(EX)

deploy: ## full pipeline for EX: infra fetch train deploy smoke promote connect
	$(FTM) all $(EX)

smoke promote rollback connect info destroy: ## single ftm step for EX
	$(FTM) $@ $(EX)

data: ## rebuild the seed training sets from their generators
	$(PY) examples/01-gemma4-agent/data/build.py
	$(PY) examples/02-qwen38-harness/data/build.py

test: ## unit and integration tests (no Azure, no GPU)
	$(PY) -m pytest -q

ruff: ## python lint
	ruff check .

shellcheck: ## shell lint
	shellcheck shared/bin/ftm shared/containers/*/*.sh scripts/*.sh examples/03-muse-glimmer/harness/*.sh

actionlint: ## workflow lint (runs shellcheck on every run: block)
	actionlint -shellcheck "$$(command -v shellcheck)" .github/workflows/*.yml

lint: ruff shellcheck actionlint ## all linters

check: lint test ## what CI runs

bootstrap: ## print the Azure + GitHub setup plan (scripts/bootstrap-azure.sh --apply to do it)
	scripts/bootstrap-azure.sh

preflight: ## check tools, logins, workspace, clusters, Foundry project
	scripts/preflight.sh

checksums: ## write SHA256SUMS over every tracked file
	@files=$$(git ls-files 2>/dev/null || true); \
	if [[ -z "$$files" ]]; then files=$$(find . -type f -not -path './.git/*' -not -path './.ftm/*' -not -name '$(ZIP)' | sed 's|^\./||'); fi; \
	printf '%s\n' "$$files" | grep -v -e '^SHA256SUMS$$' -e '__pycache__' -e '\.pyc$$' -e '^\.pytest_cache/' -e '^\.ruff_cache/' \
	  | LC_ALL=C sort | xargs sha256sum > SHA256SUMS
	@wc -l < SHA256SUMS | xargs echo "SHA256SUMS: files"

verify: ## verify SHA256SUMS
	sha256sum -c --quiet SHA256SUMS

zip: checksums ## package the repository
	rm -f $(ZIP)
	cd .. && zip -qr $(CURDIR)/$(ZIP) $(notdir $(CURDIR)) -x '*/.ftm/*' '*/__pycache__/*' '*/.pytest_cache/*' '*/.ruff_cache/*' '*/$(ZIP)'

clean: ## remove rendered plans and caches
	rm -rf .ftm .pytest_cache .ruff_cache $(ZIP)
	find . -name __pycache__ -type d -prune -exec rm -rf {} +
