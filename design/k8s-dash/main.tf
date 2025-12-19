# Terraform configuration for Kubernetes Dashboard infrastructure
# File: terraform/main.tf

terraform {
  required_version = ">= 1.5.0"
  
  required_providers {
    azurerm = {
      source  = "hashicorp/azurerm"
      version = "~> 3.80.0"
    }
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = "~> 2.23.0"
    }
    helm = {
      source  = "hashicorp/helm"
      version = "~> 2.11.0"
    }
  }
  
  backend "azurerm" {
    resource_group_name  = "prod-cus-platform-terraform-rg-001"
    storage_account_name = "prodcusterraformst001"
    container_name       = "tfstate"
    key                  = "k8s-dashboard.tfstate"
  }
}

provider "azurerm" {
  features {}
  subscription_id = var.azure_subscription_id
}

# Get AKS cluster data
data "azurerm_kubernetes_cluster" "aks" {
  name                = var.aks_cluster_name
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
resource "kubernetes_namespace" "dashboard" {
  metadata {
    name = var.namespace
    
    labels = {
      name        = var.namespace
      environment = var.environment
      managed-by  = "terraform"
    }
  }
}

# Create ACR pull secret
resource "kubernetes_secret" "acr_secret" {
  metadata {
    name      = "acr-secret"
    namespace = kubernetes_namespace.dashboard.metadata[0].name
  }
  
  type = "kubernetes.io/dockerconfigjson"
  
  data = {
    ".dockerconfigjson" = jsonencode({
      auths = {
        "${var.acr_server}" = {
          username = var.acr_username
          password = var.acr_password
          auth     = base64encode("${var.acr_username}:${var.acr_password}")
        }
      }
    })
  }
}

# Deploy Dashboard using Helm
resource "helm_release" "kubernetes_dashboard" {
  name       = "kubernetes-dashboard"
  repository = "https://kubernetes.github.io/dashboard/"
  chart      = "kubernetes-dashboard"
  version    = var.dashboard_chart_version
  namespace  = kubernetes_namespace.dashboard.metadata[0].name
  
  values = [
    file("${path.module}/../helm/values-${var.environment}.yaml"),
    var.use_acr ? file("${path.module}/../helm/values-acr.yaml") : ""
  ]
  
  set {
    name  = "app.mode"
    value = "dashboard"
  }
  
  set {
    name  = "api.scaling.replicas"
    value = var.replicas
  }
  
  set {
    name  = "web.scaling.replicas"
    value = var.replicas
  }
  
  depends_on = [
    kubernetes_namespace.dashboard,
    kubernetes_secret.acr_secret
  ]
  
  timeout = 600
  wait    = true
}

# Create admin service account
resource "kubernetes_service_account" "dashboard_admin" {
  metadata {
    name      = "dashboard-admin"
    namespace = kubernetes_namespace.dashboard.metadata[0].name
    
    labels = {
      "k8s-app" = "kubernetes-dashboard-admin"
    }
  }
  
  automount_service_account_token = true
}

# Create cluster role binding for admin
resource "kubernetes_cluster_role_binding" "dashboard_admin" {
  metadata {
    name = "dashboard-admin-binding"
    
    labels = {
      "k8s-app" = "kubernetes-dashboard-admin"
    }
  }
  
  role_ref {
    api_group = "rbac.authorization.k8s.io"
    kind      = "ClusterRole"
    name      = "cluster-admin"
  }
  
  subject {
    kind      = "ServiceAccount"
    name      = kubernetes_service_account.dashboard_admin.metadata[0].name
    namespace = kubernetes_namespace.dashboard.metadata[0].name
  }
}

# Create readonly service account
resource "kubernetes_service_account" "dashboard_readonly" {
  metadata {
    name      = "dashboard-readonly"
    namespace = kubernetes_namespace.dashboard.metadata[0].name
    
    labels = {
      "k8s-app" = "kubernetes-dashboard-readonly"
    }
  }
  
  automount_service_account_token = true
}

# Create cluster role binding for readonly
resource "kubernetes_cluster_role_binding" "dashboard_readonly" {
  metadata {
    name = "dashboard-readonly-binding"
    
    labels = {
      "k8s-app" = "kubernetes-dashboard-readonly"
    }
  }
  
  role_ref {
    api_group = "rbac.authorization.k8s.io"
    kind      = "ClusterRole"
    name      = "view"
  }
  
  subject {
    kind      = "ServiceAccount"
    name      = kubernetes_service_account.dashboard_readonly.metadata[0].name
    namespace = kubernetes_namespace.dashboard.metadata[0].name
  }
}

# Optional: Create internal load balancer
resource "kubernetes_service" "dashboard_loadbalancer" {
  count = var.create_loadbalancer ? 1 : 0
  
  metadata {
    name      = "dashboard-loadbalancer"
    namespace = kubernetes_namespace.dashboard.metadata[0].name
    
    annotations = {
      "service.beta.kubernetes.io/azure-load-balancer-internal"        = "true"
      "service.beta.kubernetes.io/azure-load-balancer-internal-subnet" = var.lb_subnet_name
    }
  }
  
  spec {
    type = "LoadBalancer"
    
    selector = {
      "app.kubernetes.io/name" = "kong"
      "app.kubernetes.io/instance" = "kubernetes-dashboard"
    }
    
    port {
      port        = 443
      target_port = 8443
      protocol    = "TCP"
    }
  }
  
  depends_on = [helm_release.kubernetes_dashboard]
}
