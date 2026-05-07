###############################################################################
# variables.tf — Input variables for Synapse infrastructure
###############################################################################

# ── Azure authentication ─────────────────────────────────────────────────────
variable "subscription_id" {
  description = "Azure subscription ID"
  type        = string
}

variable "tenant_id" {
  description = "Azure AD tenant ID"
  type        = string
}

variable "client_id" {
  description = "Service principal application (client) ID"
  type        = string
  sensitive   = true
}

variable "client_secret" {
  description = "Service principal client secret"
  type        = string
  sensitive   = true
}

# ── Environment ──────────────────────────────────────────────────────────────
variable "environment" {
  description = "Deployment environment: dev, test, or prod"
  type        = string
  validation {
    condition     = contains(["dev", "test", "prod"], var.environment)
    error_message = "environment must be one of: dev, test, prod"
  }
}

variable "resource_group_name" {
  description = "Resource group containing the Synapse workspace"
  type        = string
}

variable "location" {
  description = "Azure region"
  type        = string
  default     = "eastus2"
}

# ── Synapse workspace ────────────────────────────────────────────────────────
variable "synapse_workspace_name" {
  description = "Name of the Synapse workspace (e.g. synapse-workspace-dev)"
  type        = string
}

variable "storage_account_name" {
  description = "Primary ADLS Gen2 storage account name"
  type        = string
}

variable "storage_filesystem_name" {
  description = "Primary ADLS Gen2 filesystem (container) name"
  type        = string
  default     = "synapse"
}

# ── Spark pools ──────────────────────────────────────────────────────────────
variable "spark_pools" {
  description = "Map of Spark pool configurations"
  type = map(object({
    node_size        = string
    node_count       = number
    min_node_count   = optional(number, 3)
    max_node_count   = optional(number, 10)
    spark_version    = optional(string, "3.4")
    auto_pause_delay = optional(number, 15)
  }))
  default = {}
}

# ── Linked services (Key Vault URLs per environment) ─────────────────────────
variable "key_vault_url" {
  description = "Key Vault base URL for the target environment"
  type        = string
  default     = ""
}

variable "storage_account_url" {
  description = "Default data lake storage URL (dfs endpoint)"
  type        = string
  default     = ""
}

# ── Tags ─────────────────────────────────────────────────────────────────────
variable "tags" {
  description = "Common tags applied to all resources"
  type        = map(string)
  default     = {}
}
