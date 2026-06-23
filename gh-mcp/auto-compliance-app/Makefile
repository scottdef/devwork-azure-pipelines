# Compliance Evidence Center — Makefile

.PHONY: help build serve clean run-controls status

help: ## Show this help message
	@echo "Compliance Evidence Center - Makefile Commands"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-16s\033[0m %s\n", $$1, $$2}'

build: ## Generate the static site into _site/
	@python3 scripts/build_site.py --out _site

serve: build ## Build, then serve the site locally at http://localhost:8000
	@echo "🌐 Serving at http://localhost:8000  (Ctrl-C to stop)"
	@cd _site && python3 -m http.server 8000

clean: ## Remove the generated _site/ directory
	@rm -rf _site && echo "✅ Removed _site/"

run-controls: ## Trigger the automated compliance-check workflows (requires gh CLI)
	@gh workflow run control-org-settings.yml
	@gh workflow run control-repo-visibility.yml
	@gh workflow run control-repo-rulesets.yml
	@gh workflow run control-org-custom-role.yml
	@echo "✅ Control workflows triggered."

status: ## Show recent workflow runs (requires gh CLI)
	@gh run list --limit 10
