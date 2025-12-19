# terraform/main.tf
# Terraform configuration for EasyTrade namespace and resources

terraform {
  required_version = ">= 1.6.0"
  
  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = "~> 3.80"
    }
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = "~> 2.23"
    }
    helm = {
      source  = "hashicorp/helm"
      version = "~> 2.12"
    }
  }
  
  # Backend configuration for state storage
  # backend "azurerm" {
  #   resource_group_name  = "terraform-state-rg"
  #   storage_account_name = "tfstateaccount"
  #   container_name       = "tfstate"
  #   key                  = "easytrade.tfstate"
  # }
}

provider "azurerm" {
  features {}
  subscription_id = var.subscription_id
}

# Data source for existing AKS cluster
data "azurerm_kubernetes_cluster" "aks" {
  name                = var.cluster_name
  resource_group_name = var.resource_group_name
}

# Data source for existing ACR
data "azurerm_container_registry" "acr" {
  name                = var.acr_name
  resource_group_name = var.resource_group_name
}

provider "kubernetes" {
  host                   = data.azurerm_kubernetes_cluster.aks.kube_config.0.host
  client_certificate     = base64decode(data.azurerm_kubernetes_cluster.aks.kube_config.0.client_certificate)
  client_key             = base64decode(data.azurerm_kubernetes_cluster.aks.kube_config.0.client_key)
  cluster_ca_certificate = base64decode(data.azurerm_kubernetes_cluster.aks.kube_config.0.cluster_ca_certificate)
}

provider "helm" {
  kubernetes {
    host                   = data.azurerm_kubernetes_cluster.aks.kube_config.0.host
    client_certificate     = base64decode(data.azurerm_kubernetes_cluster.aks.kube_config.0.client_certificate)
    client_key             = base64decode(data.azurerm_kubernetes_cluster.aks.kube_config.0.client_key)
    cluster_ca_certificate = base64decode(data.azurerm_kubernetes_cluster.aks.kube_config.0.cluster_ca_certificate)
  }
}

# Create namespace
resource "kubernetes_namespace" "easytrade" {
  metadata {
    name = var.namespace
    
    labels = {
      "app.kubernetes.io/name"       = "easytrade"
      "app.kubernetes.io/managed-by" = "terraform"
      "istio-injection"              = "enabled"
    }
  }
}

# Create resource quota
resource "kubernetes_resource_quota" "easytrade" {
  metadata {
    name      = "easytrade-quota"
    namespace = kubernetes_namespace.easytrade.metadata[0].name
  }
  
  spec {
    hard = {
      "requests.cpu"              = var.resource_quota.requests_cpu
      "requests.memory"           = var.resource_quota.requests_memory
      "limits.cpu"                = var.resource_quota.limits_cpu
      "limits.memory"             = var.resource_quota.limits_memory
      "persistentvolumeclaims"    = var.resource_quota.pvc_count
      "services.loadbalancers"    = var.resource_quota.loadbalancers
    }
  }
}

# Create limit range
resource "kubernetes_limit_range" "easytrade" {
  metadata {
    name      = "easytrade-limits"
    namespace = kubernetes_namespace.easytrade.metadata[0].name
  }
  
  spec {
    limit {
      type = "Container"
      
      default = {
        cpu    = var.limit_range.default_cpu
        memory = var.limit_range.default_memory
      }
      
      default_request = {
        cpu    = var.limit_range.default_request_cpu
        memory = var.limit_range.default_request_memory
      }
    }
  }
}

# Database secret
resource "kubernetes_secret" "database" {
  metadata {
    name      = "easytrade-db-secret"
    namespace = kubernetes_namespace.easytrade.metadata[0].name
  }
  
  data = {
    SA_PASSWORD = var.database_sa_password
    DB_USER     = "sa"
  }
  
  type = "Opaque"
}

# ConfigMap
resource "kubernetes_config_map" "easytrade" {
  metadata {
    name      = "easytrade-config"
    namespace = kubernetes_namespace.easytrade.metadata[0].name
  }
  
  data = {
    DB_HOST                        = "db"
    DB_PORT                        = "1433"
    DB_NAME                        = "TradeManagement"
    BROKER_SERVICE_URL             = "http://broker-service:8080"
    ACCOUNTSERVICE_URL             = "http://accountservice:8089"
    LOGINSERVICE_URL               = "http://loginservice:8080"
    PRICINGSERVICE_URL             = "http://pricingservice:8080"
    OFFERSERVICE_URL               = "http://offerservice:8087"
    FEATURE_FLAG_SERVICE_URL       = "http://feature-flag-service:8080"
    PROXY_HOST                     = "frontendreverseproxy"
    PROXY_PORT                     = "80"
    FEATURE_FLAG_CACHE_DURATION_S  = "30"
    HIGH_CPU_USAGE_REQUEST_DELAY_MS = "1000"
    HIGH_CPU_USAGE_CONCURRENCY     = "4"
  }
}

# Deploy via Helm (optional)
resource "helm_release" "easytrade" {
  count = var.deploy_via_helm ? 1 : 0
  
  name       = "easytrade"
  namespace  = kubernetes_namespace.easytrade.metadata[0].name
  chart      = "${path.module}/../helm/easytrade"
  
  values = [
    file("${path.module}/../helm/easytrade/values.yaml")
  ]
  
  set {
    name  = "global.imageRegistry"
    value = var.use_acr_images ? data.azurerm_container_registry.acr.login_server : "europe-docker.pkg.dev/dynatrace-demoability/docker/easytrade"
  }
  
  set {
    name  = "global.namespace"
    value = kubernetes_namespace.easytrade.metadata[0].name
  }
  
  depends_on = [
    kubernetes_secret.database,
    kubernetes_config_map.easytrade
  ]
}
