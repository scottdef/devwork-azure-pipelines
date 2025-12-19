# Makefile for EasyTrade Deployment on AKS
# Platform: Ubuntu 24.04 with bash, suitable for local VM or GitHub Actions

.PHONY: help
.DEFAULT_GOAL := help

# Environment variables (override with .env file)
-include .env
export

SHELL := /bin/bash
EASYTRADE_NAMESPACE ?= easytrade
CLUSTER_NAME ?= prod-cus-aks-sre-lab-003
RESOURCE_GROUP ?= prod-cus-platform-base-rg-001
ACR_NAME ?= prod-central-image-repo
ACR_LOGIN_SERVER ?= prodcentralimagerepo.azurecr.io
SUBSCRIPTION ?= tango-CICD-platform-github-gitflow
USE_ACR_IMAGES ?= true
HELM_RELEASE_NAME ?= easytrade

# Colors for output
RED := \033[0;31m
GREEN := \033[0;32m
YELLOW := \033[1;33m
NC := \033[0m # No Color

##@ General

help: ## Display this help message
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-25s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

check-env: ## Verify required environment variables
	@echo "$(GREEN)Checking environment configuration...$(NC)"
	@test -n "$(CLUSTER_NAME)" || (echo "$(RED)ERROR: CLUSTER_NAME not set$(NC)" && exit 1)
	@test -n "$(RESOURCE_GROUP)" || (echo "$(RED)ERROR: RESOURCE_GROUP not set$(NC)" && exit 1)
	@test -n "$(ACR_NAME)" || (echo "$(RED)ERROR: ACR_NAME not set$(NC)" && exit 1)
	@echo "$(GREEN)✓ Environment configuration valid$(NC)"

##@ Installation & Setup

ubuntu-install-tools: ## Install all required tools on Ubuntu 24.04
	@echo "$(GREEN)Installing Azure CLI...$(NC)"
	@curl -sL https://aka.ms/InstallAzureCLIDeb | sudo bash
	@echo "$(GREEN)Installing kubectl and kubelogin...$(NC)"
	@sudo az aks install-cli
	@echo "$(GREEN)Installing Helm...$(NC)"
	@curl https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash
	@echo "$(GREEN)Installing Docker...$(NC)"
	@bash ./scripts/ubuntu/01-install-tools.sh
	@echo "$(GREEN)✓ All tools installed$(NC)"

verify-tools: ## Verify all tools are installed
	@echo "$(GREEN)Verifying tool installations...$(NC)"
	@command -v az >/dev/null 2>&1 || (echo "$(RED)✗ Azure CLI not found$(NC)" && exit 1)
	@command -v kubectl >/dev/null 2>&1 || (echo "$(RED)✗ kubectl not found$(NC)" && exit 1)
	@command -v kubelogin >/dev/null 2>&1 || (echo "$(RED)✗ kubelogin not found$(NC)" && exit 1)
	@command -v helm >/dev/null 2>&1 || (echo "$(RED)✗ helm not found$(NC)" && exit 1)
	@command -v docker >/dev/null 2>&1 || (echo "$(RED)✗ docker not found$(NC)" && exit 1)
	@command -v git >/dev/null 2>&1 || (echo "$(RED)✗ git not found$(NC)" && exit 1)
	@echo "$(GREEN)✓ All tools verified$(NC)"

##@ Azure & AKS Configuration

azure-login: ## Login to Azure
	@echo "$(GREEN)Logging into Azure...$(NC)"
	@az login
	@az account set --subscription "$(SUBSCRIPTION)"
	@echo "$(GREEN)✓ Logged in to subscription: $(SUBSCRIPTION)$(NC)"

configure-aks: check-env azure-login ## Configure AKS cluster access
	@echo "$(GREEN)Configuring AKS access...$(NC)"
	@az aks get-credentials \
		--resource-group "$(RESOURCE_GROUP)" \
		--name "$(CLUSTER_NAME)" \
		--overwrite-existing
	@kubelogin convert-kubeconfig -l azurecli
	@echo "$(GREEN)Testing cluster connectivity...$(NC)"
	@kubectl get nodes || (echo "$(RED)Failed to connect to cluster$(NC)" && exit 1)
	@echo "$(GREEN)✓ AKS access configured$(NC)"

attach-acr: check-env ## Attach ACR to AKS cluster
	@echo "$(GREEN)Attaching ACR to AKS...$(NC)"
	@az aks update \
		--name "$(CLUSTER_NAME)" \
		--resource-group "$(RESOURCE_GROUP)" \
		--attach-acr "$(ACR_NAME)"
	@echo "$(GREEN)✓ ACR attached$(NC)"

verify-acr: check-env ## Verify ACR connectivity
	@echo "$(GREEN)Verifying ACR connectivity...$(NC)"
	@az aks check-acr \
		--name "$(CLUSTER_NAME)" \
		--resource-group "$(RESOURCE_GROUP)" \
		--acr "$(ACR_LOGIN_SERVER)"

##@ Image Management

mirror-images: ## Mirror EasyTrade images to private ACR
	@echo "$(GREEN)Mirroring images to ACR...$(NC)"
	@bash ./scripts/mirror-images.sh
	@echo "$(GREEN)✓ Images mirrored$(NC)"

acr-login: ## Login to Azure Container Registry
	@echo "$(GREEN)Logging into ACR...$(NC)"
	@az acr login --name "$(ACR_NAME)"
	@echo "$(GREEN)✓ Logged into ACR$(NC)"

list-acr-images: ## List images in ACR
	@echo "$(GREEN)EasyTrade images in ACR:$(NC)"
	@az acr repository list --name "$(ACR_NAME)" --output table | grep easytrade || echo "No images found"

##@ Deployment

create-namespace: check-env ## Create EasyTrade namespace
	@echo "$(GREEN)Creating namespace: $(EASYTRADE_NAMESPACE)$(NC)"
	@kubectl create namespace $(EASYTRADE_NAMESPACE) --dry-run=client -o yaml | kubectl apply -f -
	@kubectl label namespace $(EASYTRADE_NAMESPACE) istio-injection=enabled --overwrite
	@echo "$(GREEN)✓ Namespace created$(NC)"

deploy-manifests: create-namespace ## Deploy using raw Kubernetes manifests
	@echo "$(GREEN)Deploying EasyTrade with manifests...$(NC)"
ifeq ($(USE_ACR_IMAGES),true)
	@echo "$(YELLOW)Using ACR images: $(ACR_LOGIN_SERVER)$(NC)"
	@bash ./scripts/ubuntu/03-deploy-manifests.sh --use-acr
else
	@echo "$(YELLOW)Using upstream images$(NC)"
	@bash ./scripts/ubuntu/03-deploy-manifests.sh
endif
	@echo "$(GREEN)Waiting for rollout...$(NC)"
	@kubectl -n $(EASYTRADE_NAMESPACE) wait --for=condition=available --timeout=300s deployment --all
	@echo "$(GREEN)✓ Deployment complete$(NC)"
	@make verify-deployment

deploy-helm: create-namespace ## Deploy using Helm chart
	@echo "$(GREEN)Deploying EasyTrade with Helm...$(NC)"
ifeq ($(USE_ACR_IMAGES),true)
	@helm upgrade --install $(HELM_RELEASE_NAME) ./helm/easytrade \
		--namespace $(EASYTRADE_NAMESPACE) \
		--set global.imageRegistry=$(ACR_LOGIN_SERVER) \
		--wait --timeout 10m
else
	@helm upgrade --install $(HELM_RELEASE_NAME) ./helm/easytrade \
		--namespace $(EASYTRADE_NAMESPACE) \
		--wait --timeout 10m
endif
	@echo "$(GREEN)✓ Helm deployment complete$(NC)"
	@make verify-deployment

deploy-helm-production: create-namespace ## Deploy using Helm with production values
	@echo "$(GREEN)Deploying EasyTrade (PRODUCTION)...$(NC)"
	@helm upgrade --install $(HELM_RELEASE_NAME) ./helm/easytrade \
		--namespace $(EASYTRADE_NAMESPACE) \
		--values ./helm/easytrade/values-production.yaml \
		--set global.imageRegistry=$(ACR_LOGIN_SERVER) \
		--wait --timeout 10m
	@echo "$(GREEN)✓ Production deployment complete$(NC)"

verify-deployment: ## Verify deployment health
	@echo "$(GREEN)Verifying deployment...$(NC)"
	@kubectl -n $(EASYTRADE_NAMESPACE) get pods
	@kubectl -n $(EASYTRADE_NAMESPACE) get svc
	@bash ./scripts/health-check.sh

##@ Access Setup

setup-portforward: ## Setup kubectl port-forward for local access
	@echo "$(GREEN)Setting up port-forward to EasyTrade...$(NC)"
	@echo "$(YELLOW)Access EasyTrade at: http://localhost:8080$(NC)"
	@echo "$(YELLOW)Press Ctrl+C to stop$(NC)"
	@kubectl -n $(EASYTRADE_NAMESPACE) port-forward svc/frontendreverseproxy 8080:80

setup-portforward-background: ## Setup port-forward in background
	@echo "$(GREEN)Starting port-forward in background...$(NC)"
	@kubectl -n $(EASYTRADE_NAMESPACE) port-forward svc/frontendreverseproxy 8080:80 > /tmp/easytrade-portforward.log 2>&1 &
	@echo $$! > /tmp/easytrade-portforward.pid
	@echo "$(GREEN)✓ Port-forward running (PID: $$(cat /tmp/easytrade-portforward.pid))$(NC)"
	@echo "$(YELLOW)Access at: http://localhost:8080$(NC)"
	@echo "$(YELLOW)Stop with: make stop-portforward$(NC)"

stop-portforward: ## Stop background port-forward
	@if [ -f /tmp/easytrade-portforward.pid ]; then \
		kill $$(cat /tmp/easytrade-portforward.pid) 2>/dev/null || true; \
		rm /tmp/easytrade-portforward.pid; \
		echo "$(GREEN)✓ Port-forward stopped$(NC)"; \
	else \
		echo "$(YELLOW)No port-forward process found$(NC)"; \
	fi

setup-loadbalancer: ## Deploy internal LoadBalancer
	@echo "$(GREEN)Deploying internal LoadBalancer...$(NC)"
	@kubectl apply -f ./kubernetes/ingress/loadbalancer.yaml
	@echo "$(YELLOW)Waiting for LoadBalancer IP assignment...$(NC)"
	@timeout 120 bash -c 'until kubectl -n $(EASYTRADE_NAMESPACE) get svc easytrade-lb -o jsonpath="{.status.loadBalancer.ingress[0].ip}" 2>/dev/null | grep -q "[0-9]"; do sleep 2; done' || true
	@echo "$(GREEN)LoadBalancer IP:$(NC) $$(kubectl -n $(EASYTRADE_NAMESPACE) get svc easytrade-lb -o jsonpath='{.status.loadBalancer.ingress[0].ip}')"

setup-istio: ## Deploy Istio Gateway and VirtualService
	@echo "$(GREEN)Deploying Istio configuration...$(NC)"
	@kubectl apply -f ./kubernetes/ingress/istio-gateway.yaml
	@kubectl apply -f ./kubernetes/ingress/istio-virtualservice.yaml
	@echo "$(GREEN)✓ Istio configuration applied$(NC)"

get-access-info: ## Display all access methods
	@echo "$(GREEN)=== EasyTrade Access Information ===$(NC)"
	@echo ""
	@echo "$(YELLOW)1. Port-Forward (kubectl):$(NC)"
	@echo "   Run: make setup-portforward"
	@echo "   URL: http://localhost:8080"
	@echo ""
	@echo "$(YELLOW)2. Internal LoadBalancer:$(NC)"
	@if kubectl -n $(EASYTRADE_NAMESPACE) get svc easytrade-lb >/dev/null 2>&1; then \
		echo "   IP: $$(kubectl -n $(EASYTRADE_NAMESPACE) get svc easytrade-lb -o jsonpath='{.status.loadBalancer.ingress[0].ip}')"; \
		echo "   URL: http://$$(kubectl -n $(EASYTRADE_NAMESPACE) get svc easytrade-lb -o jsonpath='{.status.loadBalancer.ingress[0].ip}')"; \
	else \
		echo "   Run: make setup-loadbalancer"; \
	fi
	@echo ""
	@echo "$(YELLOW)3. Direct Pod IP:$(NC)"
	@echo "   $$(kubectl -n $(EASYTRADE_NAMESPACE) get pod -l app=frontendreverseproxy -o jsonpath='{.items[0].status.podIP}')"
	@echo ""
	@echo "$(YELLOW)Default Credentials:$(NC)"
	@echo "   demouser / demopass"
	@echo "   specialuser / specialpass"

##@ Feature Flags & Problem Patterns

feature-flags: ## List all feature flags
	@echo "$(GREEN)Fetching feature flags...$(NC)"
	@bash ./scripts/feature-flags.sh list

enable-problem-pattern: ## Enable a problem pattern (usage: make enable-problem-pattern PATTERN=db_not_responding)
	@bash ./scripts/feature-flags.sh enable $(PATTERN)

disable-problem-pattern: ## Disable a problem pattern (usage: make disable-problem-pattern PATTERN=db_not_responding)
	@bash ./scripts/feature-flags.sh disable $(PATTERN)

##@ Cleanup

delete-deployment: ## Delete EasyTrade deployment
	@echo "$(RED)Deleting EasyTrade deployment...$(NC)"
	@kubectl delete namespace $(EASYTRADE_NAMESPACE) --ignore-not-found=true
	@echo "$(GREEN)✓ Deployment deleted$(NC)"

helm-uninstall: ## Uninstall Helm release
	@echo "$(RED)Uninstalling Helm release...$(NC)"
	@helm uninstall $(HELM_RELEASE_NAME) --namespace $(EASYTRADE_NAMESPACE) || true
	@echo "$(GREEN)✓ Helm release uninstalled$(NC)"

clean-all: stop-portforward delete-deployment ## Complete cleanup
	@echo "$(GREEN)✓ All resources cleaned$(NC)"

##@ Monitoring & Debugging

logs: ## Tail logs from all EasyTrade pods
	@kubectl -n $(EASYTRADE_NAMESPACE) logs -l app.kubernetes.io/name=easytrade --tail=100 -f

pods: ## List all pods
	@kubectl -n $(EASYTRADE_NAMESPACE) get pods -o wide

describe-pod: ## Describe a pod (usage: make describe-pod POD=broker-service-xxx)
	@kubectl -n $(EASYTRADE_NAMESPACE) describe pod $(POD)

shell: ## Open shell in a pod (usage: make shell POD=broker-service-xxx)
	@kubectl -n $(EASYTRADE_NAMESPACE) exec -it $(POD) -- /bin/sh

debug-network: ## Deploy debug pod for network troubleshooting
	@kubectl -n $(EASYTRADE_NAMESPACE) run debug-$$(date +%s) --rm -it --image=nicolaka/netshoot --restart=Never -- /bin/bash

events: ## Show recent events
	@kubectl -n $(EASYTRADE_NAMESPACE) get events --sort-by='.lastTimestamp' | tail -30

top: ## Show resource usage
	@kubectl -n $(EASYTRADE_NAMESPACE) top pods

##@ Terraform (Infrastructure)

tf-init: ## Initialize Terraform
	@cd terraform && terraform init

tf-plan: ## Terraform plan
	@cd terraform && terraform plan -out=tfplan

tf-apply: ## Terraform apply
	@cd terraform && terraform apply tfplan

tf-destroy: ## Terraform destroy
	@cd terraform && terraform destroy

##@ Development

watch-pods: ## Watch pod status
	@watch -n 2 'kubectl -n $(EASYTRADE_NAMESPACE) get pods'

test-endpoints: ## Test all service endpoints
	@bash ./scripts/health-check.sh --verbose

generate-load: ## Generate test load (requires loadgen deployed)
	@kubectl -n $(EASYTRADE_NAMESPACE) scale deployment headless-loadgen --replicas=1

stop-load: ## Stop test load
	@kubectl -n $(EASYTRADE_NAMESPACE) scale deployment headless-loadgen --replicas=0

##@ Backup & Recovery

backup-manifests: ## Backup current deployment manifests
	@mkdir -p backups/$$(date +%Y%m%d-%H%M%S)
	@kubectl -n $(EASYTRADE_NAMESPACE) get all -o yaml > backups/$$(date +%Y%m%d-%H%M%S)/deployment.yaml
	@echo "$(GREEN)✓ Backup saved to backups/$$(date +%Y%m%d-%H%M%S)$(NC)"

helm-backup: ## Export Helm values
	@mkdir -p backups
	@helm get values $(HELM_RELEASE_NAME) -n $(EASYTRADE_NAMESPACE) > backups/helm-values-$$(date +%Y%m%d-%H%M%S).yaml
	@echo "$(GREEN)✓ Helm values backed up$(NC)"
