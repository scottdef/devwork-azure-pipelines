.PHONY: check lint plan

check: lint
	test/check.sh

lint:
	@if command -v shellcheck >/dev/null; then shellcheck -x scripts/*.sh test/az test/check.sh; else echo "skip  shellcheck not found"; fi

# plan prints what a sync of ENV would apply, without asking Azure anything
# but the subscription.
plan:
	scripts/apim-sync.sh --env $(ENV) --dry-run
