variable "environments" {
  description = "Map of environment configs keyed by env name"
  type = map(object({
    resource_group_name  = string
    location             = string
    workspace_name       = string
    storage_account_name = string
    filesystem_name      = string
    sql_admin_login      = string
    sql_admin_password   = string
    spark_pool_name      = optional(string, "")
    spark_pool_size      = optional(string, "Small")
    spark_min_nodes      = optional(number, 3)
    spark_max_nodes      = optional(number, 10)
    spark_version        = optional(string, "3.4")
    sql_pool_name        = optional(string, "")
    sql_pool_sku         = optional(string, "DW100c")
    firewall_rules = optional(list(object({
      name     = string
      start_ip = string
      end_ip   = string
    })), [])
    managed_vnet_enabled          = optional(bool, true)
    public_network_access_enabled = optional(bool, false)
    git_repo_name                 = optional(string, "")
    github_account_name           = optional(string, "")
    collaboration_branch          = optional(string, "main")
    root_folder                   = optional(string, "/")
  }))
}

variable "common_tags" {
  description = "Tags applied to every resource"
  type        = map(string)
  default = {
    project   = "synapse-cicd"
    terraform = "true"
  }
}

variable "github_org" {
  description = "GitHub organization name"
  type        = string
}

variable "tenant_id" {
  description = "Azure AD tenant ID"
  type        = string
}

variable "subscription_id" {
  description = "Azure subscription ID"
  type        = string
}

variable "spn_configs" {
  description = "Service principal config per environment"
  type = map(object({
    client_id     = string
    client_secret = string
  }))
  sensitive = true
}

variable "keyvault_configs" {
  description = "Optional Key Vault config per environment"
  type = map(object({
    name                = string
    sku                 = optional(string, "standard")
    soft_delete_enabled = optional(bool, true)
    purge_protection    = optional(bool, true)
  }))
  default = {}
}
