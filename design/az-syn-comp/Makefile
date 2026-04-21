# =============================================================================
# Makefile — Synapse CI/CD local developer workflow
#
# Assumes:
#   - az CLI installed and logged in  (az login)
#   - gh CLI installed and authenticated  (gh auth login)
#   - GITHUB_ORG and relevant AZURE_* vars exported in shell
#   - Python 3 available
#
# Usage:
#   make help
#   make setup-repos
#   make validate-dev
#   make deploy-dev
#   make detect-drift-dev
#   make rollback-dev VERSION=dev-v1.0.3
# =============================================================================

SHELL := /usr/bin/env bash
.DEFAULT_GOAL := help

# ── Config ────────────────────────────────────────────────────────────────────
ORG           ?= $(GITHUB_ORG)
WORKSPACE_DEV  = synapse-workspace-dev
WORKSPACE_TEST = synapse-workspace-test
WORKSPACE_PROD = synapse-workspace-prod
RG_DEV         = rg-synapse-dev
RG_TEST        = rg-synapse-test
RG_PROD        = rg-synapse-prod

# Colour helpers
BOLD  = \033[1m
RESET = \033[0m
GREEN = \033[32m
CYAN  = \033[36m
RED   = \033[31m

.PHONY: help
help: ## Show this help
	@printf "\n$(BOLD)Synapse CI/CD Makefile$(RESET)\n\n"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) \
	  | awk 'BEGIN {FS = ":.*?## "}; {printf "  $(CYAN)%-30s$(RESET) %s\n", $$1, $$2}'
	@printf "\n$(BOLD)Variables:$(RESET)\n"
	@printf "  $(CYAN)%-30s$(RESET) %s\n" "ORG" "$(ORG)"
	@printf "  $(CYAN)%-30s$(RESET) %s\n" "WORKSPACE_DEV" "$(WORKSPACE_DEV)"
	@printf "\n"

# ─────────────────────────────────────────────────────────────────────────────
# Bootstrap
# ─────────────────────────────────────────────────────────────────────────────
.PHONY: setup-repos
setup-repos: ## Bootstrap all GitHub repos, branches, rulesets and teams
	@[[ -n "$(ORG)" ]] || { echo "$(RED)ERROR: GITHUB_ORG not set$(RESET)"; exit 1; }
	chmod +x scripts/setup-repos.sh
	GITHUB_ORG=$(ORG) ./scripts/setup-repos.sh

# ─────────────────────────────────────────────────────────────────────────────
# Validation (local — no deploy)
# ─────────────────────────────────────────────────────────────────────────────
.PHONY: validate-dev
validate-dev: ## Validate artifacts in synapse-repo-dev against dev workspace
	@echo "$(BOLD)Validating synapse-repo-dev...$(RESET)"
	cd synapse-repo-dev && \
	  az synapse workspace show --name "$(WORKSPACE_DEV)" --query name -o tsv && \
	  echo "Workspace reachable ✓"

.PHONY: validate-test
validate-test: ## Validate artifacts in synapse-repo-test against test workspace
	@echo "$(BOLD)Validating synapse-repo-test...$(RESET)"
	cd synapse-repo-test && \
	  az synapse workspace show --name "$(WORKSPACE_TEST)" --query name -o tsv && \
	  echo "Workspace reachable ✓"

.PHONY: validate-prod
validate-prod: ## Validate artifacts in synapse-repo-prod against prod workspace
	@echo "$(BOLD)Validating synapse-repo-prod...$(RESET)"
	cd synapse-repo-prod && \
	  az synapse workspace show --name "$(WORKSPACE_PROD)" --query name -o tsv && \
	  echo "Workspace reachable ✓"

# ─────────────────────────────────────────────────────────────────────────────
# Trigger management
# ─────────────────────────────────────────────────────────────────────────────
.PHONY: stop-triggers-dev
stop-triggers-dev: ## Stop all running triggers in dev workspace
	chmod +x scripts/stop-triggers.sh
	./scripts/stop-triggers.sh "$(WORKSPACE_DEV)"

.PHONY: start-triggers-dev
start-triggers-dev: ## Restart triggers in dev workspace (from state file)
	chmod +x scripts/start-triggers.sh
	./scripts/start-triggers.sh "$(WORKSPACE_DEV)"

.PHONY: stop-triggers-test
stop-triggers-test: ## Stop all running triggers in test workspace
	chmod +x scripts/stop-triggers.sh
	./scripts/stop-triggers.sh "$(WORKSPACE_TEST)"

.PHONY: stop-triggers-prod
stop-triggers-prod: ## Stop all running triggers in prod workspace
	chmod +x scripts/stop-triggers.sh
	./scripts/stop-triggers.sh "$(WORKSPACE_PROD)"

# ─────────────────────────────────────────────────────────────────────────────
# Drift detection
# ─────────────────────────────────────────────────────────────────────────────
.PHONY: detect-drift-dev
detect-drift-dev: ## Compare live dev workspace against synapse-repo-dev/dev branch
	chmod +x scripts/detect-drift.sh
	./scripts/detect-drift.sh "$(WORKSPACE_DEV)" ./synapse-repo-dev

.PHONY: detect-drift-test
detect-drift-test: ## Compare live test workspace against synapse-repo-test/test branch
	chmod +x scripts/detect-drift.sh
	./scripts/detect-drift.sh "$(WORKSPACE_TEST)" ./synapse-repo-test

.PHONY: detect-drift-prod
detect-drift-prod: ## Compare live prod workspace against synapse-repo-prod/prod branch
	chmod +x scripts/detect-drift.sh
	./scripts/detect-drift.sh "$(WORKSPACE_PROD)" ./synapse-repo-prod

.PHONY: detect-drift-all
detect-drift-all: detect-drift-dev detect-drift-test detect-drift-prod ## Check drift in all workspaces

# ─────────────────────────────────────────────────────────────────────────────
# Release management
# ─────────────────────────────────────────────────────────────────────────────
.PHONY: list-releases-dev
list-releases-dev: ## List all dev releases
	gh release list --repo $(ORG)/synapse-repo-dev --limit 20

.PHONY: list-releases-test
list-releases-test: ## List all test releases
	gh release list --repo $(ORG)/synapse-repo-test --limit 20

.PHONY: list-releases-prod
list-releases-prod: ## List all prod releases
	gh release list --repo $(ORG)/synapse-repo-prod --limit 20

# ─────────────────────────────────────────────────────────────────────────────
# Rollback (triggers GitHub workflow via workflow_dispatch)
# ─────────────────────────────────────────────────────────────────────────────
# Usage:  make rollback-dev VERSION=dev-v1.0.3
.PHONY: rollback-dev
rollback-dev: ## Trigger rollback workflow in synapse-repo-dev  (VERSION=dev-vX.Y.Z required)
	@[[ -n "$(VERSION)" ]] || { echo "$(RED)ERROR: VERSION not set (e.g. make rollback-dev VERSION=dev-v1.0.3)$(RESET)"; exit 1; }
	gh workflow run rollback.yml \
	  --repo $(ORG)/synapse-repo-dev \
	  -f target_version=$(VERSION) \
	  -f confirm=ROLLBACK
	@echo "$(GREEN)Rollback workflow dispatched for $(VERSION)$(RESET)"

.PHONY: rollback-prod
rollback-prod: ## Trigger rollback workflow in synapse-repo-prod  (VERSION=prod-vX.Y.Z, INCIDENT=INC-xxx)
	@[[ -n "$(VERSION)"  ]] || { echo "$(RED)ERROR: VERSION not set$(RESET)";  exit 1; }
	@[[ -n "$(INCIDENT)" ]] || { echo "$(RED)ERROR: INCIDENT not set$(RESET)"; exit 1; }
	gh workflow run rollback.yml \
	  --repo $(ORG)/synapse-repo-prod \
	  -f target_version=$(VERSION) \
	  -f incident_reference=$(INCIDENT) \
	  -f confirm=ROLLBACK
	@echo "$(GREEN)Prod rollback dispatched: $(VERSION) / $(INCIDENT)$(RESET)"

# ─────────────────────────────────────────────────────────────────────────────
# Workspace info
# ─────────────────────────────────────────────────────────────────────────────
.PHONY: workspace-status
workspace-status: ## Show pipeline/trigger counts for all three workspaces
	@for ws in $(WORKSPACE_DEV) $(WORKSPACE_TEST) $(WORKSPACE_PROD); do \
	  printf "$(BOLD)%-35s$(RESET)" "$$ws"; \
	  PIPES=$$(az synapse pipeline list --workspace-name "$$ws" --query "length(@)" -o tsv 2>/dev/null || echo "?"); \
	  TRIGS=$$(az synapse trigger list  --workspace-name "$$ws" --query "length(@)" -o tsv 2>/dev/null || echo "?"); \
	  ACTIVE=$$(az synapse trigger list --workspace-name "$$ws" \
	    --query "[?properties.runtimeState=='Started'] | length(@)" -o tsv 2>/dev/null || echo "?"); \
	  printf "pipelines=%-4s triggers=%-4s active=%-4s\n" "$$PIPES" "$$TRIGS" "$$ACTIVE"; \
	done

.PHONY: export-dev
export-dev: ## Export all dev workspace artifacts to ./export/dev/
	chmod +x scripts/export-workspace.sh 2>/dev/null || true
	mkdir -p export/dev
	@for artifact in pipeline dataset linkedService trigger notebook; do \
	  mkdir -p export/dev/$$artifact; \
	  for name in $$(az synapse $$artifact list \
	    --workspace-name $(WORKSPACE_DEV) --query "[].name" -o tsv); do \
	    az synapse $$artifact show \
	      --workspace-name $(WORKSPACE_DEV) --name "$$name" -o json \
	      > export/dev/$$artifact/$$name.json; \
	    echo "  exported $$artifact/$$name"; \
	  done; \
	done
	@echo "$(GREEN)Export complete: ./export/dev/$(RESET)"

# ─────────────────────────────────────────────────────────────────────────────
# Git helpers
# ─────────────────────────────────────────────────────────────────────────────
.PHONY: new-feature
new-feature: ## Create a new feature branch from prod  (NAME=your-feature required)
	@[[ -n "$(NAME)" ]] || { echo "$(RED)ERROR: NAME not set (e.g. make new-feature NAME=fix-customer-pipeline)$(RESET)"; exit 1; }
	cd synapse-repo-dev && \
	  git fetch origin && \
	  git checkout prod && \
	  git pull origin prod && \
	  git checkout -b feature/$(NAME) && \
	  echo "$(GREEN)Created branch feature/$(NAME) from prod$(RESET)"

.PHONY: new-hotfix
new-hotfix: ## Create a hotfix branch from prod  (NAME=your-fix required)
	@[[ -n "$(NAME)" ]] || { echo "$(RED)ERROR: NAME not set$(RESET)"; exit 1; }
	cd synapse-repo-dev && \
	  git fetch origin && \
	  git checkout prod && \
	  git pull origin prod && \
	  git checkout -b hotfix/$(NAME) && \
	  echo "$(RED)HOTFIX branch hotfix/$(NAME) created from prod$(RESET)"
	@echo "$(RED)Remember to trigger make hotfix-deploy once ready$(RESET)"

.PHONY: hotfix-deploy
hotfix-deploy: ## Trigger hotfix workflow  (BRANCH=hotfix/xxx INCIDENT=INC-xxx required)
	@[[ -n "$(BRANCH)"   ]] || { echo "$(RED)ERROR: BRANCH not set$(RESET)";   exit 1; }
	@[[ -n "$(INCIDENT)" ]] || { echo "$(RED)ERROR: INCIDENT not set$(RESET)"; exit 1; }
	gh workflow run hotfix.yml \
	  --repo $(ORG)/synapse-repo-dev \
	  -f hotfix_branch=$(BRANCH) \
	  -f incident_reference=$(INCIDENT) \
	  -f confirm=HOTFIX
	@echo "$(RED)Hotfix workflow dispatched: $(BRANCH) / $(INCIDENT)$(RESET)"

# ─────────────────────────────────────────────────────────────────────────────
# Linting / local checks
# ─────────────────────────────────────────────────────────────────────────────
.PHONY: lint-yaml
lint-yaml: ## Lint all workflow YAML files with yamllint (pip install yamllint)
	@command -v yamllint >/dev/null 2>&1 || { echo "$(RED)yamllint not installed: pip install yamllint$(RESET)"; exit 1; }
	find . -path '*/.github/workflows/*.yml' -exec yamllint -d relaxed {} \;
	@echo "$(GREEN)YAML lint passed$(RESET)"

.PHONY: lint-sh
lint-sh: ## Lint all shell scripts with shellcheck (apt install shellcheck)
	@command -v shellcheck >/dev/null 2>&1 || { echo "$(RED)shellcheck not installed: apt install shellcheck$(RESET)"; exit 1; }
	find . -name '*.sh' -exec shellcheck {} \;
	@echo "$(GREEN)Shell lint passed$(RESET)"

.PHONY: lint
lint: lint-yaml lint-sh ## Run all linters

.PHONY: validate-json
validate-json: ## Validate all Synapse artifact JSON files are well-formed
	@echo "Validating JSON files..."
	@find synapse-repo-dev -name '*.json' -not -path '*/.git/*' | while read f; do \
	  python3 -c "import sys,json; json.load(open('$$f'))" 2>&1 \
	    && echo "  ✓ $$f" \
	    || echo "  $(RED)✗ $$f$(RESET)"; \
	done

.PHONY: check-all
check-all: lint validate-json detect-drift-all ## Run all checks
