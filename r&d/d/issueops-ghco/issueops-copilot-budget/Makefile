# The one interface. `make` alone lists targets.
SHELL := bash
.SHELLFLAGS := -euo pipefail -c
REPO ?= CoolGitOrg/issueops-copilot-budget
SCRIPTS := $(wildcard scripts/*.sh) tests/run.sh tests/gh

.PHONY: help check lint test labels get plan checksums verify
help: ## list targets
	@awk -F':.*## ' '/^[a-z-]+:.*## /{printf "  %-10s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

check: ## required tools present
	@for c in gh jq openssl date awk; do command -v $$c >/dev/null || { echo "missing: $$c"; exit 1; }; done; echo ok

lint: ## shellcheck + actionlint (each skipped if absent)
	@if command -v shellcheck >/dev/null; then shellcheck -x $(SCRIPTS); else echo "skip: shellcheck"; fi
	@if command -v actionlint >/dev/null; then actionlint; else echo "skip: actionlint"; fi

test: ## offline tests against a fake gh
	@tests/run.sh

labels: ## create the labels in $(REPO) (idempotent)
	@while read -r n c d; do gh label create "$$n" -R $(REPO) -c "$$c" -d "$$d" --force; done <<< $$'copilot-budget-request 1d76db request\nbudget:pending fbca04 awaiting-approval\nbudget:approved 0e8a16 applied\nbudget:denied b60205 denied\nbudget:rejected 6a737d failed-validation\nbudget:failed d93f0b fault'

get: ## effective budget: make get LOGIN=mona  (needs ENT_TOKEN)
	@ORG_TOKEN=unused scripts/budget.sh get $(LOGIN) | jq .

plan: ## dry run: make plan LOGIN=mona INC=50  (needs ENT_TOKEN; writes nothing)
	@ORG_TOKEN=unused DRY_RUN=1 scripts/budget.sh apply $(LOGIN) $(INC) | jq .

checksums: ## write SHA256SUMS
	@find . -type f ! -path './.git/*' ! -name SHA256SUMS | LC_ALL=C sort | xargs sha256sum > SHA256SUMS; wc -l < SHA256SUMS

verify: ## verify SHA256SUMS
	@sha256sum -c --quiet SHA256SUMS && echo ok
