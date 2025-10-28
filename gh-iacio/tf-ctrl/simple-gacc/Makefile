# ============================================================================
# Makefile for GitHub Organization Settings Management
# ============================================================================
# Rob Pike and Ken Thompson approve of this build system
#
# Usage:
#   make init     - Initialize terraform
#   make plan     - Show what changes will be made
#   make apply    - Apply changes to GitHub organization
#   make destroy  - Remove organization settings from terraform management
#   make validate - Validate terraform configuration
#   make fmt      - Format all terraform files
#   make clean    - Clean up terraform artifacts
#

.PHONY: help init plan apply destroy validate fmt clean show state-list

# Default target
.DEFAULT_GOAL := help

# Colors for output
RED    := \033[0;31m
GREEN  := \033[0;32m
YELLOW := \033[0;33m
BLUE   := \033[0;34m
RESET  := \033[0m

# Environment detection
UNAME_S := $(shell uname -s)
ifeq ($(UNAME_S),Linux)
    OPEN_CMD := xdg-open
endif
ifeq ($(UNAME_S),Darwin)
    OPEN_CMD := open
endif

# Terraform variables
TF_VARS_FILE := terraform.tfvars
TF_PLAN_FILE := github-org.tfplan

help: ## Show this help message
	@echo "$(BLUE)GitHub Organization Settings - Terraform Management$(RESET)"
	@echo ""
	@echo "Available targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  $(GREEN)%-15s$(RESET) %s\n", $$1, $$2}'
	@echo ""
	@echo "Prerequisites:"
	@echo "  - terraform >= 1.5"
	@echo "  - GITHUB_TOKEN environment variable set"
	@echo "  - GITHUB_OWNER environment variable set"
	@echo ""

check-env: ## Check required environment variables
	@echo "$(BLUE)Checking environment variables...$(RESET)"
	@test -n "$$GITHUB_TOKEN" || (echo "$(RED)ERROR: GITHUB_TOKEN not set$(RESET)" && exit 1)
	@test -n "$$GITHUB_OWNER" || (echo "$(RED)ERROR: GITHUB_OWNER not set$(RESET)" && exit 1)
	@echo "$(GREEN)✓ GITHUB_TOKEN is set$(RESET)"
	@echo "$(GREEN)✓ GITHUB_OWNER is set: $$GITHUB_OWNER$(RESET)"

check-tfvars: ## Check if terraform.tfvars exists
	@test -f $(TF_VARS_FILE) || \
		(echo "$(RED)ERROR: $(TF_VARS_FILE) not found$(RESET)" && \
		 echo "$(YELLOW)Copy terraform.tfvars.example to terraform.tfvars and configure$(RESET)" && \
		 exit 1)

init: check-env ## Initialize Terraform
	@echo "$(BLUE)Initializing Terraform...$(RESET)"
	terraform init
	@echo "$(GREEN)✓ Terraform initialized$(RESET)"

validate: ## Validate Terraform configuration
	@echo "$(BLUE)Validating Terraform configuration...$(RESET)"
	terraform validate
	@echo "$(GREEN)✓ Configuration is valid$(RESET)"

fmt: ## Format Terraform files
	@echo "$(BLUE)Formatting Terraform files...$(RESET)"
	terraform fmt -recursive
	@echo "$(GREEN)✓ Files formatted$(RESET)"

plan: check-env check-tfvars ## Generate and show execution plan
	@echo "$(BLUE)Generating Terraform plan...$(RESET)"
	terraform plan -var-file=$(TF_VARS_FILE) -out=$(TF_PLAN_FILE)
	@echo "$(GREEN)✓ Plan saved to $(TF_PLAN_FILE)$(RESET)"
	@echo ""
	@echo "$(YELLOW)Review the plan above, then run 'make apply' to execute$(RESET)"

apply: check-env check-tfvars ## Apply the Terraform configuration
	@echo "$(BLUE)Applying Terraform configuration...$(RESET)"
	@if [ -f $(TF_PLAN_FILE) ]; then \
		terraform apply $(TF_PLAN_FILE); \
		rm -f $(TF_PLAN_FILE); \
	else \
		terraform apply -var-file=$(TF_VARS_FILE); \
	fi
	@echo "$(GREEN)✓ Configuration applied successfully$(RESET)"

destroy: check-env check-tfvars ## Destroy the managed infrastructure
	@echo "$(RED)WARNING: This will remove organization settings from Terraform management!$(RESET)"
	@echo "$(YELLOW)This does NOT delete your organization, just removes it from terraform state$(RESET)"
	@read -p "Are you sure? [y/N] " -n 1 -r; \
	echo; \
	if [[ $$REPLY =~ ^[Yy]$$ ]]; then \
		terraform destroy -var-file=$(TF_VARS_FILE); \
	else \
		echo "$(BLUE)Destroy cancelled$(RESET)"; \
	fi

show: ## Show current Terraform state
	@echo "$(BLUE)Current Terraform state:$(RESET)"
	terraform show

state-list: ## List resources in Terraform state
	@echo "$(BLUE)Resources in Terraform state:$(RESET)"
	terraform state list

refresh: check-env check-tfvars ## Refresh Terraform state
	@echo "$(BLUE)Refreshing Terraform state...$(RESET)"
	terraform refresh -var-file=$(TF_VARS_FILE)
	@echo "$(GREEN)✓ State refreshed$(RESET)"

import: check-env ## Import existing organization settings
	@echo "$(BLUE)Importing existing organization settings...$(RESET)"
	@echo "$(YELLOW)Organization: $$GITHUB_OWNER$(RESET)"
	terraform import github_organization_settings.main $$GITHUB_OWNER
	@echo "$(GREEN)✓ Import complete$(RESET)"
	@echo ""
	@echo "$(YELLOW)Run 'make plan' to see current vs desired state$(RESET)"

clean: ## Clean up Terraform artifacts
	@echo "$(BLUE)Cleaning up...$(RESET)"
	rm -f $(TF_PLAN_FILE)
	rm -f terraform.tfstate.backup
	@echo "$(GREEN)✓ Cleanup complete$(RESET)"

deep-clean: clean ## Deep clean including .terraform directory
	@echo "$(RED)Deep cleaning (removing .terraform directory)...$(RESET)"
	rm -rf .terraform .terraform.lock.hcl
	@echo "$(GREEN)✓ Deep clean complete$(RESET)"
	@echo "$(YELLOW)Run 'make init' to reinitialize$(RESET)"

# ============================================================================
# Pipeline Targets (for CI/CD)
# ============================================================================

ci-validate: init validate fmt ## Run validation checks (CI)
	@echo "$(GREEN)✓ All validation checks passed$(RESET)"

ci-plan: init plan ## Generate plan for CI/CD review
	@echo "$(GREEN)✓ Plan generated for review$(RESET)"

ci-apply: init apply ## Apply changes in CI/CD
	@echo "$(GREEN)✓ Changes applied$(RESET)"

# ============================================================================
# Development Helpers
# ============================================================================

vars: check-tfvars ## Show variable values (excluding sensitive data)
	@echo "$(BLUE)Current variable values:$(RESET)"
	@terraform console <<< "var" || true

graph: ## Generate and display dependency graph
	@echo "$(BLUE)Generating dependency graph...$(RESET)"
	terraform graph | dot -Tsvg > graph.svg
	@echo "$(GREEN)✓ Graph saved to graph.svg$(RESET)"
	@$(OPEN_CMD) graph.svg 2>/dev/null || echo "Open graph.svg to view"

console: check-env check-tfvars ## Start Terraform console
	terraform console

# ============================================================================
# Documentation
# ============================================================================

docs: ## Generate documentation
	@echo "$(BLUE)Configuration Documentation$(RESET)"
	@echo ""
	@echo "$(GREEN)Files:$(RESET)"
	@echo "  github_org_settings.tf    - Main resource and variable definitions"
	@echo "  terraform.tfvars.example  - Example variable values"
	@echo "  terraform.tfvars          - Your actual variable values (gitignored)"
	@echo ""
	@echo "$(GREEN)Environment Variables:$(RESET)"
	@echo "  GITHUB_TOKEN  - Personal access token or GitHub App token"
	@echo "  GITHUB_OWNER  - Organization name"
	@echo ""
	@echo "$(GREEN)Workflow:$(RESET)"
	@echo "  1. Copy terraform.tfvars.example to terraform.tfvars"
	@echo "  2. Edit terraform.tfvars with your values"
	@echo "  3. Export GITHUB_TOKEN and GITHUB_OWNER"
	@echo "  4. Run: make init"
	@echo "  5. Run: make plan"
	@echo "  6. Run: make apply"
	@echo ""
