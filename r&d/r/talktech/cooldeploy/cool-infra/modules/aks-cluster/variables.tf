variable "name" {
  description = "Cluster name, <env>-<region>-<workload>-aks-<nnn>."
  type        = string
  validation {
    condition     = can(regex("^(sbx|dev|tst|prd)-[a-z0-9]+-[a-z0-9]+-aks-[0-9]{3}$", var.name))
    error_message = "A cluster is named <env>-<region>-<workload>-aks-<nnn>."
  }
}

variable "environment" {
  description = "sbx, dev, tst or prd."
  type        = string
}

variable "resource_group_name" {
  type = string
}

variable "location" {
  type = string
}

variable "tenant_id" {
  type = string
}

variable "kubernetes_version" {
  description = "Minor version, such as 1.30. AKS picks the patch."
  type        = string
}

variable "subnet_id" {
  description = "Subnet for the nodes."
  type        = string
}

variable "node_size" {
  type = string
}

variable "node_min" {
  type = number
}

variable "node_max" {
  type = number
}

variable "mesh" {
  description = "Enable the Istio add-on with its internal and external ingress gateways."
  type        = bool
}

variable "istio_revision" {
  description = "Revision of the Istio add-on, such as asm-1-30. List the ones on offer with: az aks mesh get-revisions --location <location>."
  type        = string
}

variable "registry_id" {
  description = "Resource ID of the platform registry."
  type        = string
}

variable "admins_group" {
  description = "Object ID of the Entra ID group that administers the cluster."
  type        = string
}

variable "tags" {
  type    = map(string)
  default = {}
}
