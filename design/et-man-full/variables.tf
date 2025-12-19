# terraform/variables.tf
# Variables for EasyTrade Terraform deployment

variable "subscription_id" {
  description = "Azure subscription ID"
  type        = string
  default     = ""
}

variable "resource_group_name" {
  description = "Resource group containing AKS cluster"
  type        = string
  default     = "prod-cus-platform-base-rg-001"
}

variable "cluster_name" {
  description = "AKS cluster name"
  type        = string
  default     = "prod-cus-aks-sre-lab-003"
}

variable "acr_name" {
  description = "Azure Container Registry name"
  type        = string
  default     = "prod-central-image-repo"
}

variable "namespace" {
  description = "Kubernetes namespace for EasyTrade"
  type        = string
  default     = "easytrade"
}

variable "database_sa_password" {
  description = "SQL Server SA password"
  type        = string
  sensitive   = true
  default     = "yourStrong(!)Password"
}

variable "use_acr_images" {
  description = "Use ACR images instead of upstream registry"
  type        = bool
  default     = true
}

variable "deploy_via_helm" {
  description = "Deploy EasyTrade using Helm (vs raw manifests)"
  type        = bool
  default     = false
}

variable "resource_quota" {
  description = "Resource quota for the namespace"
  type = object({
    requests_cpu    = string
    requests_memory = string
    limits_cpu      = string
    limits_memory   = string
    pvc_count       = string
    loadbalancers   = string
  })
  default = {
    requests_cpu    = "16"
    requests_memory = "32Gi"
    limits_cpu      = "32"
    limits_memory   = "64Gi"
    pvc_count       = "10"
    loadbalancers   = "2"
  }
}

variable "limit_range" {
  description = "Default resource limits for containers"
  type = object({
    default_cpu            = string
    default_memory         = string
    default_request_cpu    = string
    default_request_memory = string
  })
  default = {
    default_cpu            = "1"
    default_memory         = "1Gi"
    default_request_cpu    = "100m"
    default_request_memory = "128Mi"
  }
}

variable "enable_istio" {
  description = "Enable Istio sidecar injection"
  type        = bool
  default     = true
}

variable "enable_monitoring" {
  description = "Enable monitoring (ServiceMonitor for Prometheus)"
  type        = bool
  default     = false
}

variable "enable_autoscaling" {
  description = "Enable Horizontal Pod Autoscaling"
  type        = bool
  default     = false
}

variable "enable_network_policies" {
  description = "Enable Network Policies"
  type        = bool
  default     = false
}

variable "tags" {
  description = "Tags to apply to Azure resources"
  type        = map(string)
  default = {
    Environment = "production"
    ManagedBy   = "terraform"
    Application = "easytrade"
  }
}
