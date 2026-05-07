SHELL := /bin/bash
.ONESHELL:
.SHELLFLAGS := -euo pipefail -c
.DEFAULT_GOAL := help
ENV ?= dev

.PHONY: help
help:
	@grep -E '^[a-zA-Z/_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[32m%-22s\033[0m %s\n", $$1, $$2}'

validate: ## Validate Synapse artifacts
	python3 scripts/validate-synapse.py .

tf/init: ## Terraform init
	cd terraform && terraform init -input=false

tf/plan: tf/init ## Terraform plan (ENV=dev|test|prod)
	cd terraform && terraform plan -var-file="environments/$(ENV).tfvars" -input=false -out=tfplan

tf/apply: tf/init ## Terraform apply
	cd terraform && terraform apply -var-file="environments/$(ENV).tfvars" -input=false -auto-approve

tf/state-download: ## Download TF state artifact
	bash scripts/manage-tf-state.sh download $(ENV) terraform/terraform.tfstate

tf/state-list: ## List TF state artifacts
	bash scripts/manage-tf-state.sh list $(ENV)

triggers/stop: ## Stop Synapse triggers
	bash scripts/toggle-triggers.sh stop synapse-workspace-$(ENV)

triggers/start: ## Restart Synapse triggers
	bash scripts/toggle-triggers.sh start synapse-workspace-$(ENV)

drift: ## Check for drift
	bash scripts/detect-drift.sh synapse-workspace-$(ENV) .

dispatch/plan: ## Trigger TF plan workflow
	gh workflow run 05-terraform-plan.yml -f environment=$(ENV)

dispatch/apply: ## Trigger TF apply workflow
	gh workflow run 06-terraform-apply.yml -f environment=$(ENV) -f confirm=apply

dispatch/full: ## Full dev→test→prod pipeline
	gh workflow run 09-workflow-orchestrator.yml -f action=full-deploy-pipeline

clean: ## Remove generated files
	rm -f active-triggers.json plan.txt apply.txt drift-report.txt terraform/tfplan terraform/.terraform.lock.hcl
	rm -rf terraform/.terraform
