###############################################################################
# Makefile — Local development commands for Synapse CI/CD
#
# Usage:
#   make init ENV=dev          Initialize terraform for an environment
#   make plan ENV=dev          Run terraform plan
#   make apply ENV=dev         Run terraform apply
#   make validate              Validate all configurations
#   make fmt                   Format terraform files
#   make drift ENV=test        Run drift detection against a workspace
###############################################################################

SHELL := /bin/bash
.DEFAULT_GOAL := help

ENV ?= dev
TF_DIR := terraform
TF_VARS := -var-file="environments/$(ENV).tfvars"

# Workspace name mapping
WORKSPACE_dev  := synapse-workspace-dev
WORKSPACE_test := synapse-workspace-test
WORKSPACE_prod := synapse-workspace-prod
WORKSPACE := $(WORKSPACE_$(ENV))

# ── Terraform ────────────────────────────────────────────────────────────────

.PHONY: init
init: ## Initialize terraform (ENV=dev|test|prod)
	cd $(TF_DIR) && terraform init -input=false

.PHONY: fmt
fmt: ## Format terraform files
	cd $(TF_DIR) && terraform fmt -recursive

.PHONY: validate
validate: fmt ## Validate terraform configuration
	cd $(TF_DIR) && terraform validate

.PHONY: plan
plan: init ## Run terraform plan (ENV=dev|test|prod)
	cd $(TF_DIR) && terraform plan $(TF_VARS) -no-color

.PHONY: apply
apply: init ## Run terraform apply (ENV=dev|test|prod)
	cd $(TF_DIR) && terraform apply $(TF_VARS)

.PHONY: destroy
destroy: ## Run terraform destroy (ENV=dev|test|prod) — USE WITH CAUTION
	cd $(TF_DIR) && terraform destroy $(TF_VARS)

.PHONY: output
output: ## Show terraform outputs
	cd $(TF_DIR) && terraform output

# ── Synapse triggers ─────────────────────────────────────────────────────────

.PHONY: triggers-stop
triggers-stop: ## Stop triggers (ENV=dev|test|prod)
	./scripts/toggle-triggers.sh stop "$(WORKSPACE)"

.PHONY: triggers-start
triggers-start: ## Start triggers (ENV=dev|test|prod)
	./scripts/toggle-triggers.sh start "$(WORKSPACE)"

# ── Drift detection ──────────────────────────────────────────────────────────

.PHONY: drift
drift: ## Run drift detection (ENV=test|prod)
	./scripts/detect-drift.sh "$(WORKSPACE)" \
		"./synapse-workspace-dev/TemplateForWorkspace.json"

# ── Setup ────────────────────────────────────────────────────────────────────

.PHONY: setup-secrets
setup-secrets: ## Set GitHub secrets (requires gh CLI)
	@echo "Setting repository secrets..."
	@read -p "Azure Subscription ID: " SUB_ID && \
		gh secret set AZURE_SUBSCRIPTION_ID --body "$$SUB_ID"
	@read -p "Azure Tenant ID: " TENANT_ID && \
		gh secret set AZURE_TENANT_ID --body "$$TENANT_ID"
	@read -p "Dev SPN Client ID: " DEV_ID && \
		gh secret set SYNAPSE_DEV_SPN_ID --body "$$DEV_ID"
	@read -sp "Dev SPN Client Secret: " DEV_SECRET && echo && \
		gh secret set SYNAPSE_DEV_SPN_SECRET --body "$$DEV_SECRET"
	@read -p "Test SPN Client ID: " TEST_ID && \
		gh secret set SYNAPSE_TEST_SPN_ID --body "$$TEST_ID"
	@read -sp "Test SPN Client Secret: " TEST_SECRET && echo && \
		gh secret set SYNAPSE_TEST_SPN_SECRET --body "$$TEST_SECRET"
	@read -p "Prod SPN Client ID: " PROD_ID && \
		gh secret set SYNAPSE_PROD_SPN_ID --body "$$PROD_ID"
	@read -sp "Prod SPN Client Secret: " PROD_SECRET && echo && \
		gh secret set SYNAPSE_PROD_SPN_SECRET --body "$$PROD_SECRET"
	@echo "All secrets configured."

.PHONY: setup-environments
setup-environments: ## Create GitHub environments (requires gh CLI)
	@echo "Creating GitHub environments..."
	gh api repos/{owner}/{repo}/environments/dev --method PUT
	gh api repos/{owner}/{repo}/environments/test --method PUT
	gh api repos/{owner}/{repo}/environments/production --method PUT
	@echo "Environments created. Configure approval gates in GitHub Settings."

# ── Help ─────────────────────────────────────────────────────────────────────

.PHONY: help
help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'
