# Makefile for Kubernetes Dashboard Deployment
# File: Makefile

.PHONY: help install-ubuntu install-windows connect deploy-manifests deploy-helm port-forward verify clean

# Configuration
NAMESPACE := kubernetes-dashboard
CLUSTER_NAME := prod-cus-aks-sre-lab-003
RESOURCE_GROUP := prod-cus-platform-base-rg-001
SUBSCRIPTION := tango-CICD-platform-github-gitflow
ACR_NAME := prod-central-image-repo

# Colors for output
RED := \033[0;31m
GREEN := \033[0;32m
YELLOW := \033[0;33m
BLUE := \033[0;34m
NC := \033[0m # No Color

help: ## Show this help message
	@echo "$(BLUE)Kubernetes Dashboard Deployment - Make Commands$(NC)"
	@echo ""
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*?## "}; {printf "$(GREEN)%-25s$(NC) %s\n", $$1, $$2}'
	@echo ""

## Installation Commands

install-ubuntu: ## Install prerequisites on Ubuntu 24.04
	@echo "$(YELLOW)Installing prerequisites on Ubuntu...$(NC)"
	bash scripts/ubuntu/01-prerequisites.sh

install-windows: ## Install prerequisites on Windows (run in PowerShell)
	@echo "$(YELLOW)Installing prerequisites on Windows...$(NC)"
	@echo "Run: powershell.exe -ExecutionPolicy Bypass -File scripts/windows/01-prerequisites.ps1"

## Connection Commands

connect: ## Connect to AKS cluster
	@echo "$(YELLOW)Connecting to AKS cluster...$(NC)"
	az account set --subscription "$(SUBSCRIPTION)"
	az aks get-credentials \
		--resource-group "$(RESOURCE_GROUP)" \
		--name "$(CLUSTER_NAME)" \
		--overwrite-existing
	kubelogin convert-kubeconfig -l azurecli
	@echo "$(GREEN)✅ Connected to cluster$(NC)"
	kubectl get nodes

connect-windows: ## Connect to AKS cluster (PowerShell)
	@echo "$(YELLOW)Connecting to AKS cluster (Windows)...$(NC)"
	@echo "Run: powershell.exe -ExecutionPolicy Bypass -File scripts/windows/02-aks-connect.ps1"

## Deployment Commands

deploy-manifests: ## Deploy using pure Kubernetes manifests
	@echo "$(YELLOW)Deploying with manifests...$(NC)"
	bash scripts/ubuntu/03-deploy-manifests.sh

deploy-helm: ## Deploy using Helm chart
	@echo "$(YELLOW)Deploying with Helm...$(NC)"
	bash scripts/ubuntu/04-deploy-helm.sh

deploy-helm-prod: ## Deploy using Helm chart with production values
	@echo "$(YELLOW)Deploying with Helm (production)...$(NC)"
	bash scripts/ubuntu/04-deploy-helm.sh ./helm/values-prod.yaml

deploy-helm-acr: ## Deploy using Helm chart with ACR images
	@echo "$(YELLOW)Deploying with Helm (ACR images)...$(NC)"
	@$(MAKE) acr-secret
	helm upgrade --install kubernetes-dashboard kubernetes-dashboard/kubernetes-dashboard \
		--namespace $(NAMESPACE) \
		--create-namespace \
		--version 7.14.0 \
		--values helm/values-prod.yaml \
		--values helm/values-acr.yaml \
		--wait \
		--timeout 10m \
		--atomic

deploy-terraform: ## Deploy using Terraform
	@echo "$(YELLOW)Deploying with Terraform...$(NC)"
	cd terraform && \
		terraform init && \
		terraform plan && \
		terraform apply -auto-approve

## Access Commands

port-forward: ## Start port-forward to dashboard
	@echo "$(YELLOW)Starting port-forward...$(NC)"
	@echo "Access URL: $(GREEN)https://localhost:8443$(NC)"
	@echo "Press Ctrl+C to stop"
	kubectl port-forward -n $(NAMESPACE) svc/kubernetes-dashboard-kong-proxy 8443:443

port-forward-bg: ## Start port-forward in background
	@echo "$(YELLOW)Starting port-forward in background...$(NC)"
	kubectl port-forward -n $(NAMESPACE) svc/kubernetes-dashboard-kong-proxy 8443:443 &
	@echo "$(GREEN)✅ Port-forward running in background$(NC)"
	@echo "Access URL: https://localhost:8443"
	@echo "To stop: make stop-port-forward"

stop-port-forward: ## Stop background port-forward
	@echo "$(YELLOW)Stopping port-forward...$(NC)"
	pkill -f "kubectl port-forward.*8443" || true
	@echo "$(GREEN)✅ Port-forward stopped$(NC)"

nodeport: ## Create NodePort service
	@echo "$(YELLOW)Creating NodePort service...$(NC)"
	kubectl apply -f - <<EOF
	apiVersion: v1
	kind: Service
	metadata:
	  name: dashboard-nodeport
	  namespace: $(NAMESPACE)
	spec:
	  type: NodePort
	  selector:
	    k8s-app: kubernetes-dashboard
	  ports:
	    - port: 443
	      targetPort: 8443
	      nodePort: 30443
	      protocol: TCP
	EOF
	@NODE_IP=$$(kubectl get nodes -o jsonpath='{.items[0].status.addresses[?(@.type=="InternalIP")].address}'); \
	echo "$(GREEN)✅ NodePort created$(NC)"; \
	echo "Access URL: https://$$NODE_IP:30443"

loadbalancer: ## Create internal LoadBalancer service
	@echo "$(YELLOW)Creating LoadBalancer service...$(NC)"
	kubectl apply -f - <<EOF
	apiVersion: v1
	kind: Service
	metadata:
	  name: dashboard-loadbalancer
	  namespace: $(NAMESPACE)
	  annotations:
	    service.beta.kubernetes.io/azure-load-balancer-internal: "true"
	spec:
	  type: LoadBalancer
	  selector:
	    k8s-app: kubernetes-dashboard
	  ports:
	    - port: 443
	      targetPort: 8443
	      protocol: TCP
	EOF
	@echo "$(YELLOW)Waiting for LoadBalancer IP...$(NC)"
	@sleep 30
	@LB_IP=$$(kubectl get svc dashboard-loadbalancer -n $(NAMESPACE) -o jsonpath='{.status.loadBalancer.ingress[0].ip}'); \
	echo "$(GREEN)✅ LoadBalancer created$(NC)"; \
	echo "Access URL: https://$$LB_IP"

## Token Commands

token: ## Generate admin token (24 hours)
	@echo "$(YELLOW)Generating admin token...$(NC)"
	@TOKEN=$$(kubectl create token dashboard-admin -n $(NAMESPACE) --duration=24h 2>/dev/null || \
		kubectl create token dashboard-admin -n $(NAMESPACE) --duration=86400s); \
	echo "$(GREEN)Admin Token:$(NC)"; \
	echo "$$TOKEN"

token-readonly: ## Generate readonly token (24 hours)
	@echo "$(YELLOW)Generating readonly token...$(NC)"
	@TOKEN=$$(kubectl create token dashboard-readonly -n $(NAMESPACE) --duration=24h 2>/dev/null || \
		kubectl create token dashboard-readonly -n $(NAMESPACE) --duration=86400s); \
	echo "$(GREEN)Readonly Token:$(NC)"; \
	echo "$$TOKEN"

## ACR Commands

acr-login: ## Login to Azure Container Registry
	@echo "$(YELLOW)Logging into ACR...$(NC)"
	az acr login --name $(ACR_NAME)

acr-secret: ## Create ACR pull secret
	@echo "$(YELLOW)Creating ACR pull secret...$(NC)"
	kubectl create namespace $(NAMESPACE) --dry-run=client -o yaml | kubectl apply -f -
	@ACR_USER=$$(az acr credential show -n $(ACR_NAME) --query username -o tsv); \
	ACR_PASS=$$(az acr credential show -n $(ACR_NAME) --query passwords[0].value -o tsv); \
	kubectl create secret docker-registry acr-secret \
		--namespace $(NAMESPACE) \
		--docker-server=$(ACR_NAME).azurecr.io \
		--docker-username=$$ACR_USER \
		--docker-password=$$ACR_PASS \
		--dry-run=client -o yaml | kubectl apply -f -
	@echo "$(GREEN)✅ ACR secret created$(NC)"

acr-import: ## Import official images to ACR
	@echo "$(YELLOW)Importing official images to ACR...$(NC)"
	az acr import --name $(ACR_NAME) \
		--source docker.io/kubernetesui/dashboard-api:1.14.0 \
		--image kubernetes-dashboard/dashboard-api:1.14.0
	az acr import --name $(ACR_NAME) \
		--source docker.io/kubernetesui/dashboard-web:1.7.0 \
		--image kubernetes-dashboard/dashboard-web:1.7.0
	az acr import --name $(ACR_NAME) \
		--source docker.io/kubernetesui/dashboard-auth:1.4.0 \
		--image kubernetes-dashboard/dashboard-auth:1.4.0
	az acr import --name $(ACR_NAME) \
		--source docker.io/kubernetesui/dashboard-metrics-scraper:1.2.2 \
		--image kubernetes-dashboard/dashboard-metrics-scraper:1.2.2
	@echo "$(GREEN)✅ Images imported$(NC)"

## Verification Commands

verify: ## Verify deployment
	@echo "$(YELLOW)Verifying deployment...$(NC)"
	bash scripts/ubuntu/verify-deployment.sh

status: ## Show deployment status
	@echo "$(BLUE)=== Deployment Status ===$(NC)"
	kubectl get all -n $(NAMESPACE)
	@echo ""
	@echo "$(BLUE)=== Pod Status ===$(NC)"
	kubectl get pods -n $(NAMESPACE) -o wide
	@echo ""
	@echo "$(BLUE)=== Service Status ===$(NC)"
	kubectl get svc -n $(NAMESPACE)

logs: ## Show dashboard logs
	@echo "$(YELLOW)Showing logs...$(NC)"
	kubectl logs -n $(NAMESPACE) -l k8s-app=kubernetes-dashboard --tail=50 -f

logs-all: ## Show all component logs
	@echo "$(YELLOW)Showing all logs...$(NC)"
	kubectl logs -n $(NAMESPACE) --all-containers=true --tail=50 -f

events: ## Show recent events
	@echo "$(YELLOW)Showing recent events...$(NC)"
	kubectl get events -n $(NAMESPACE) --sort-by='.lastTimestamp' | tail -20

## Cleanup Commands

clean: ## Remove dashboard deployment
	@echo "$(RED)Removing dashboard deployment...$(NC)"
	@read -p "Are you sure you want to delete the dashboard? [y/N] " -n 1 -r; \
	echo; \
	if [[ $$REPLY =~ ^[Yy]$$ ]]; then \
		helm uninstall kubernetes-dashboard -n $(NAMESPACE) || true; \
		kubectl delete namespace $(NAMESPACE) || true; \
		echo "$(GREEN)✅ Dashboard removed$(NC)"; \
	else \
		echo "$(YELLOW)Cancelled$(NC)"; \
	fi

clean-terraform: ## Destroy Terraform-managed resources
	@echo "$(RED)Destroying Terraform resources...$(NC)"
	cd terraform && terraform destroy -auto-approve

## Development Commands

lint: ## Lint Kubernetes manifests
	@echo "$(YELLOW)Linting manifests...$(NC)"
	@for file in manifests/*.yaml; do \
		echo "Linting $$file"; \
		kubectl apply --dry-run=client -f "$$file" || exit 1; \
	done
	@echo "$(GREEN)✅ All manifests valid$(NC)"

helm-lint: ## Lint Helm values
	@echo "$(YELLOW)Linting Helm values...$(NC)"
	helm lint helm/

template: ## Generate Kubernetes manifests from Helm
	@echo "$(YELLOW)Generating manifests from Helm...$(NC)"
	helm template kubernetes-dashboard kubernetes-dashboard/kubernetes-dashboard \
		--values helm/values-prod.yaml \
		--namespace $(NAMESPACE) > generated-manifests.yaml
	@echo "$(GREEN)✅ Manifests generated: generated-manifests.yaml$(NC)"

## Docker Commands

docker-build: ## Build custom dashboard image
	@echo "$(YELLOW)Building Docker image...$(NC)"
	docker build -t $(ACR_NAME).azurecr.io/kubernetes-dashboard/dashboard:custom ./docker

docker-push: acr-login ## Push custom image to ACR
	@echo "$(YELLOW)Pushing Docker image...$(NC)"
	docker push $(ACR_NAME).azurecr.io/kubernetes-dashboard/dashboard:custom

## Info Commands

info: ## Show cluster and dashboard info
	@echo "$(BLUE)=== Cluster Information ===$(NC)"
	@echo "Subscription: $(SUBSCRIPTION)"
	@echo "Resource Group: $(RESOURCE_GROUP)"
	@echo "Cluster: $(CLUSTER_NAME)"
	@echo "Namespace: $(NAMESPACE)"
	@echo "ACR: $(ACR_NAME)"
	@echo ""
	@echo "$(BLUE)=== Kubernetes Version ===$(NC)"
	@kubectl version --short 2>/dev/null || kubectl version
	@echo ""
	@echo "$(BLUE)=== Current Context ===$(NC)"
	@kubectl config current-context

urls: ## Show access URLs
	@echo "$(BLUE)=== Access URLs ===$(NC)"
	@echo "Port-forward: $(GREEN)https://localhost:8443$(NC)"
	@echo ""
	@echo "To start port-forward: $(YELLOW)make port-forward$(NC)"
	@echo "To generate token: $(YELLOW)make token$(NC)"
