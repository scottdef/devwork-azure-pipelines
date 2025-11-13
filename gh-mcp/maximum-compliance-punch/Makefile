# Kickbutt Compliance Makefile
# Simple commands for common operations

.PHONY: help setup deploy test clean run-controls update validate local

help: ## Show this help message
	@echo "Kickbutt Compliance - Makefile Commands"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

setup: ## Initial setup - enable workflows and permissions
	@echo "🔧 Setting up Kickbutt Compliance..."
	@echo ""
	@echo "Manual steps required:"
	@echo "1. Go to Settings → Actions → General"
	@echo "2. Set workflow permissions to 'Read and write'"
	@echo "3. Enable 'Allow GitHub Actions to create and approve pull requests'"
	@echo ""
	@echo "4. Go to Settings → Pages"
	@echo "5. Set source to 'Deploy from branch'"
	@echo "6. Select 'gh-pages' branch"
	@echo ""
	@echo "Then run: make deploy"

deploy: ## Manually trigger trust center deployment
	@echo "🚀 Deploying trust center..."
	gh workflow run update-trust-center.yml
	@echo "✅ Workflow triggered. Check status: make status"

run-controls: ## Run all control workflows
	@echo "🔍 Running all control workflows..."
	gh workflow run control-org-settings.yml
	gh workflow run control-repo-visibility.yml
	gh workflow run control-repo-rulesets.yml
	gh workflow run control-org-custom-role.yml
	@echo "✅ All controls triggered. Wait 2-3 minutes, then run: make update"

update: ## Update trust center with latest statuses
	@echo "🔄 Updating trust center..."
	gh workflow run update-trust-center.yml
	@echo "✅ Update triggered. Check status: make status"

status: ## Show recent workflow runs
	@echo "📊 Recent workflow runs:"
	@echo ""
	gh run list --limit 10

test: ## Test status generator script locally
	@echo "🧪 Testing status generator..."
	@chmod +x scripts/generate-trust-center.sh
	@./scripts/generate-trust-center.sh || echo "Set GITHUB_REPOSITORY env var first"

local: ## Serve trust center locally for testing
	@echo "🌐 Starting local server..."
	@echo "Visit: http://localhost:8000"
	@cd trust-center && python3 -m http.server 8000

clean: ## Remove generated files
	@echo "🧹 Cleaning up..."
	@rm -f trust-center/status.json
	@echo "✅ Clean complete"
