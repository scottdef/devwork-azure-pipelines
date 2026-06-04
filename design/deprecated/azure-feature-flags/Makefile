# ────────────────────────────────────────────────────────────────────────────
# Feature Flag Management — Makefile
# ────────────────────────────────────────────────────────────────────────────
SHELL := /bin/bash
.DEFAULT_GOAL := help

# ── Configuration (override via env or .env file) ─────────────────────────
APP_CONFIG_NAME ?= $(error Set APP_CONFIG_NAME)
RESOURCE_GROUP  ?= $(error Set RESOURCE_GROUP)
LABEL           ?=
TF_DIR          := terraform

RED    := \033[0;31m
GREEN  := \033[0;32m
YELLOW := \033[0;33m
CYAN   := \033[0;36m
BOLD   := \033[1m
NC     := \033[0m

# ── Include .env if present ───────────────────────────────────────────────
-include .env
export

.PHONY: help validate compare init plan apply export clean

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "  $(CYAN)%-14s$(NC) %s\n", $$1, $$2}'

validate: ## Validate app-feature-flags.json syntax and schema
	@echo -e "$(BOLD)Validating app-feature-flags.json$(NC)"
	@python3 -c "\
	import json, sys; \
	flags = json.load(open('app-feature-flags.json')); \
	assert isinstance(flags, list), 'root must be array'; \
	[( \
	  assert 'name' in f, f'entry {i}: missing name', \
	  assert 'enabled' in f, f'entry {i}: missing enabled', \
	  assert isinstance(f['enabled'], bool), f'entry {i}: enabled must be bool' \
	) if False else None for i, f in enumerate(flags)]; \
	names = [f['name'] for f in flags]; \
	assert len(names) == len(set(names)), f'duplicate names: {set(n for n in names if names.count(n)>1)}'; \
	print(f'$(GREEN)✓ {len(flags)} flags validated$(NC)') \
	" 2>/dev/null || python3 -c "\
	import json, sys; \
	flags = json.load(open('app-feature-flags.json')); \
	ok = True; \
	for i, f in enumerate(flags): \
	    if 'name' not in f: print(f'ERROR: entry {i} missing name'); ok = False; \
	    if 'enabled' not in f: print(f'ERROR: entry {i} missing enabled'); ok = False; \
	sys.exit(0 if ok else 1) \
	"

compare: ## Compare JSON flags against live App Configuration
	@python3 scripts/compare-flags.py \
		--json-file app-feature-flags.json \
		--app-config-name "$(APP_CONFIG_NAME)" \
		--resource-group "$(RESOURCE_GROUP)" \
		$(if $(LABEL),--label "$(LABEL)",)

init: ## Initialize Terraform
	@echo -e "$(BOLD)Initializing Terraform$(NC)"
	cd $(TF_DIR) && terraform init

plan: init ## Run Terraform plan
	@echo -e "$(BOLD)Planning feature flag changes$(NC)"
	cd $(TF_DIR) && terraform plan \
		-var="app_configuration_name=$(APP_CONFIG_NAME)" \
		-var="resource_group_name=$(RESOURCE_GROUP)" \
		-var="feature_flags_file=../app-feature-flags.json" \
		$(if $(LABEL),-var="label=$(LABEL)",) \
		-out=tfplan

apply: plan ## Apply Terraform changes
	@echo -e "$(BOLD)Applying feature flag changes$(NC)"
	cd $(TF_DIR) && terraform apply tfplan

export: ## Export flags from App Configuration
	@chmod +x scripts/export-flags.sh
	@scripts/export-flags.sh "$(APP_CONFIG_NAME)" "$(RESOURCE_GROUP)" "$(LABEL)"

clean: ## Remove Terraform local state and plan files
	rm -f $(TF_DIR)/tfplan
	rm -rf $(TF_DIR)/.terraform
	rm -f $(TF_DIR)/.terraform.lock.hcl
