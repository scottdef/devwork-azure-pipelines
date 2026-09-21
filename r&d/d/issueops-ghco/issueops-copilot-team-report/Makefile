# The one interface. `make` alone lists targets.
SHELL := bash
.SHELLFLAGS := -euo pipefail -c
REPO ?= CoolGitOrg/issueops-copilot-team-report
AW   := $(shell command -v gh-aw 2>/dev/null || echo "gh aw")

.PHONY: help check lint test compile sample labels checksums verify
help: ## list targets
	@awk -F':.*## ' '/^[a-z-]+:.*## /{printf "  %-10s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

check: ## required tools present
	@for c in gh jq curl python3 date awk; do command -v $$c >/dev/null || { echo "missing: $$c"; exit 1; }; done; echo ok

lint: ## shellcheck, actionlint on the hand-written workflow, py_compile
	@if command -v shellcheck >/dev/null; then shellcheck -x scripts/*.sh tests/run.sh tests/gh; else echo "skip: shellcheck"; fi
	@if command -v actionlint >/dev/null; then actionlint .github/workflows/report-request.yml; else echo "skip: actionlint"; fi
	@python3 -m py_compile scripts/*.py tests/*.py && echo "python ok"

test: ## offline tests: fake gh, synthetic data, end to end
	@tests/run.sh

compile: ## regenerate the .lock.yml (strict). Never edit the lock by hand.
	$(AW) compile copilot-team-report --strict

sample: ## build sample/copilot-report-sample.html from synthetic data
	@t=$$(mktemp -d); python3 tests/make_fixtures.py $$t/raw && python3 scripts/aggregate.py $$t/raw $$t/out >/dev/null && \
	python3 scripts/render.py $$t/out/report-data.json sample/copilot-report-sample.html tests/sample-insights.json; rm -rf $$t

labels: ## create the labels in $(REPO) (idempotent)
	@while read -r n c d; do gh label create "$$n" -R $(REPO) -c "$$c" -d "$$d" --force; done <<< $$'copilot-team-report 1d76db request\nreport:running fbca04 collecting\nreport:collected c5def5 baseline-posted\nreport:done 0e8a16 delivered\nreport:refused 6a737d not-admitted\nreport:failed d93f0b fault'

checksums: ## write SHA256SUMS
	@find . -type f ! -path './.git/*' ! -name SHA256SUMS ! -name '*.pyc' | LC_ALL=C sort | xargs sha256sum > SHA256SUMS; wc -l < SHA256SUMS

verify: ## verify SHA256SUMS
	@sha256sum -c --quiet SHA256SUMS && echo ok
