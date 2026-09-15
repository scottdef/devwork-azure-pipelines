# Makefile — the one front door. Every target is a thin wrapper over scripts/.
SHELL := /usr/bin/env bash
.SHELLFLAGS := -eu -o pipefail -c
.DEFAULT_GOAL := help

REGISTRY ?= models/registry.yml
ACCOUNT  ?= dev-swedencentral
MODEL    ?= qwen3-32b
DRY_RUN  ?= true

export REGISTRY DRY_RUN

_acct = $(shell yq -r '.accounts["$(ACCOUNT)"].$(1)' $(REGISTRY))
RG   := $(call _acct,resource_group)
AIS  := $(call _acct,account)
LOC  := $(call _acct,location)

.PHONY: help lint models accounts catalog render deploy validate decommission key user-config bootstrap-labels clean

help: ## list targets
	@grep -E '^[a-z-]+:.*## ' $(MAKEFILE_LIST) | awk -F':.*## ' '{printf "  \033[36m%-16s\033[0m %s\n",$$1,$$2}'

lint: ## shellcheck + actionlint + yaml
	shellcheck -x -S warning scripts/*.sh
	actionlint
	@for f in .github/ISSUE_TEMPLATE/*.yml .github/workflows/*.yml $(REGISTRY); do yq -e . "$$f" >/dev/null || exit 1; done
	@echo lint ok

models: ## list registry models
	@yq -r '.models | to_entries[] | "\(.key)\t\(.value.kind)\t\(.value.default_sku)\t\(.value.copilot_byok)\t\(.value.display)"' $(REGISTRY) | awk -F'\t' '{printf "  %-18s %-11s %-20s %-6s %s\n",$$1,$$2,$$3,$$4,$$5}'

accounts: ## list registry accounts
	@yq -r '.accounts | to_entries[] | "\(.key)\t\(.value.resource_group)/\(.value.account)\t\(.value.location)\t\(.value.environment)"' $(REGISTRY) | awk -F'\t' '{printf "  %-18s %-11s %-20s %-6s %s\n",$$1,$$2,$$3,$$4,$$5}'

catalog: ## snapshot live catalog + drift report (needs az login)
	scripts/refresh-catalog.sh

render: ## re-render issue-form dropdowns from the registry
	scripts/render-issue-template.sh

deploy: ## deploy MODEL to ACCOUNT (DRY_RUN=true by default). e.g. make deploy MODEL=qwen3-32b ACCOUNT=prod-eastus2 DRY_RUN=false
	@while IFS== read -r k v; do export "$${k^^}=$$v"; done < <(MODEL_KEY=$(MODEL) TARGET_ACCOUNT=$(ACCOUNT) scripts/resolve-model.sh); \
	  case "$$KIND" in managed) scripts/deploy-managed.sh ;; *) scripts/deploy-serverless.sh ;; esac

validate: ## validate DEPLOYMENT on ACCOUNT (keyless). e.g. make validate DEPLOYMENT=qwen3-32b
	ENDPOINT="$$(az cognitiveservices account show -g $(RG) -n $(AIS) --query properties.endpoint -o tsv)" \
	DEPLOYMENT=$(DEPLOYMENT) scripts/validate-endpoint.sh

decommission: ## delete DEPLOYMENT on ACCOUNT
	RESOURCE_GROUP=$(RG) ACCOUNT=$(AIS) DEPLOYMENT=$(DEPLOYMENT) scripts/decommission.sh

key: ## print key1 for ACCOUNT (for the one-time Copilot AI-controls registration). Do not paste into issues.
	@az cognitiveservices account keys list -g $(RG) -n $(AIS) --query key1 -o tsv

user-config: ## print Copilot CLI / VS Code snippet for DEPLOYMENT on ACCOUNT
	@EP="$$(az cognitiveservices account show -g $(RG) -n $(AIS) --query properties.endpoint -o tsv)"; \
	printf '# Copilot CLI\nexport COPILOT_PROVIDER_TYPE=azure\nexport COPILOT_PROVIDER_BASE_URL="%sopenai/deployments/%s/v1"\nexport COPILOT_PROVIDER_API_KEY="$$FOUNDRY_KEY"\nexport COPILOT_MODEL="%s"\n\n# VS Code: Manage Models → Azure → endpoint=%s deployment=%s\n' "$$EP" "$(DEPLOYMENT)" "$(DEPLOYMENT)" "$$EP" "$(DEPLOYMENT)"

bootstrap-labels: ## create the IssueOps labels in this repo
	@for l in "foundry-model-request:1D76DB" "action:deploy:0E8A16" "action:decommission:B60205" \
	          "pending-validation:FBCA04" "pending-approval:FBCA04" "deploying:5319E7" "deployed:0E8A16" \
	          "failed:B60205" "validation-failed:B60205" "cancelled:CCCCCC" "catalog-drift:D93F0B"; do \
	  name="$${l%:*}"; color="$${l##*:}"; gh label create "$$name" --color "$$color" --force >/dev/null && echo "label $$name"; done

clean:
	rm -f request.json resolve.err drift.txt
