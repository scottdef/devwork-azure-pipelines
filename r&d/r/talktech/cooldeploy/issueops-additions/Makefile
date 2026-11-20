.PHONY: check lint

# check runs the static checks and then every request script against a local
# cool-api. It needs coolctl on the PATH, and Go and ../demo for the scripts.
check: lint
	test/check.sh

lint:
	shellcheck -x scripts/*.sh scripts/cool/*.sh test/check.sh
	actionlint .github/workflows/*.yml
