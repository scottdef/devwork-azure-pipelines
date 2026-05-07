# ============================================================================
# Azure Synapse Workspaces
# ============================================================================

resource "azurerm_synapse_workspace" "this" {
  for_each = var.environments

  name                                 = each.value.workspace_name
  resource_group_name                  = azurerm_resource_group.synapse[each.key].name
  location                             = azurerm_resource_group.synapse[each.key].location
  storage_data_lake_gen2_filesystem_id = azurerm_storage_data_lake_gen2_filesystem.synapse_fs[each.key].id
  sql_administrator_login              = each.value.sql_admin_login
  sql_administrator_login_password     = each.value.sql_admin_password
  managed_virtual_network_enabled      = each.value.managed_vnet_enabled
  public_network_access_enabled        = each.value.public_network_access_enabled

  dynamic "github_repo" {
    for_each = each.key == "dev" && each.value.git_repo_name != "" ? [1] : []
    content {
      account_name    = each.value.github_account_name
      branch_name     = each.value.collaboration_branch
      repository_name = each.value.git_repo_name
      root_folder     = each.value.root_folder
      git_url         = "https://github.com"
    }
  }

  identity {
    type = "SystemAssigned"
  }

  tags = merge(var.common_tags, { environment = each.key })
}

# ── Firewall Rules ──────────────────────────────────────────────────────────

locals {
  firewall_rules = flatten([
    for env_key, env in var.environments : [
      for rule in env.firewall_rules : {
        key     = "${env_key}-${rule.name}"
        env_key = env_key
        name    = rule.name
        start   = rule.start_ip
        end     = rule.end_ip
      }
    ]
  ])
}

resource "azurerm_synapse_firewall_rule" "this" {
  for_each = { for r in local.firewall_rules : r.key => r }

  name                 = each.value.name
  synapse_workspace_id = azurerm_synapse_workspace.this[each.value.env_key].id
  start_ip_address     = each.value.start
  end_ip_address       = each.value.end
}

resource "azurerm_synapse_firewall_rule" "allow_azure_services" {
  for_each = var.environments

  name                 = "AllowAllWindowsAzureIps"
  synapse_workspace_id = azurerm_synapse_workspace.this[each.key].id
  start_ip_address     = "0.0.0.0"
  end_ip_address       = "0.0.0.0"
}

# ── Spark Pools ─────────────────────────────────────────────────────────────

resource "azurerm_synapse_spark_pool" "this" {
  for_each = { for k, v in var.environments : k => v if v.spark_pool_name != "" }

  name                 = each.value.spark_pool_name
  synapse_workspace_id = azurerm_synapse_workspace.this[each.key].id
  node_size_family     = "MemoryOptimized"
  node_size            = each.value.spark_pool_size
  spark_version        = each.value.spark_version
  cache_size           = 100

  auto_scale {
    max_node_count = each.value.spark_max_nodes
    min_node_count = each.value.spark_min_nodes
  }

  auto_pause {
    delay_in_minutes = 15
  }

  tags = merge(var.common_tags, { environment = each.key })
}

# ── Dedicated SQL Pools ─────────────────────────────────────────────────────

resource "azurerm_synapse_sql_pool" "this" {
  for_each = { for k, v in var.environments : k => v if v.sql_pool_name != "" }

  name                 = each.value.sql_pool_name
  synapse_workspace_id = azurerm_synapse_workspace.this[each.key].id
  sku_name             = each.value.sql_pool_sku
  create_mode          = "Default"
  collation            = "SQL_LATIN1_GENERAL_CP1_CI_AS"

  tags = merge(var.common_tags, { environment = each.key })
}
